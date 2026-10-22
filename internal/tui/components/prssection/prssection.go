package prssection

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tasks"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

const SectionType = "pr"

// closePrompt dismisses the confirmation prompt, batching whatever the answer
// kicked off with the blink command.
func (m *Model) closePrompt(cmd tea.Cmd) tea.Cmd {
	m.PromptConfirmationBox.Reset()

	return tea.Batch(cmd, m.SetIsPromptConfirmationShown(false))
}

// runConfirmedAction performs the action the prompt was confirming.
func (m *Model) runConfirmedAction(action string) tea.Cmd {
	pr := m.GetCurrRow()
	if pr == nil {
		return nil
	}
	sid := tasks.SectionIdentifier{Id: m.Id, Type: SectionType}

	switch action {
	case "close":
		return tasks.ClosePR(m.Ctx, sid, pr)
	case "reopen":
		return tasks.ReopenPR(m.Ctx, sid, pr)
	case "ready":
		return tasks.PRReady(m.Ctx, sid, pr)
	case "merge":
		return tasks.MergePR(m.Ctx, sid, pr)
	case "merge_squash", "merge_merge", "merge_rebase":
		return tasks.MergePRWithMethod(m.Ctx, sid, pr, strings.TrimPrefix(action, "merge_"))
	case "enqueue":
		if prd, ok := pr.(*prrow.Data); ok && prd.Primary != nil {
			return tasks.EnqueuePR(m.Ctx, sid, prd.Primary.Number, prd.Primary.Id)
		}
	case "dequeue":
		if prd, ok := pr.(*prrow.Data); ok && prd.Primary != nil {
			return tasks.DequeuePR(m.Ctx, sid, prd.Primary.Number, prd.Primary.Id)
		}
	case "update":
		return tasks.UpdatePR(m.Ctx, sid, pr)
	case "approveWorkflows":
		return tasks.ApproveWorkflows(m.Ctx, sid, pr)
	}

	return nil
}

// mergeMethodFromKey maps the merge-method picker's answer to a `gh pr merge`
// strategy flag. Accepts the initial letter or the full word; "" for anything
// else, which cancels (so Enter or a stray key never merges).
func mergeMethodFromKey(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "s", "squash":
		return "squash"
	case "m", "merge":
		return "merge"
	case "r", "rebase":
		return "rebase"
	}

	return ""
}

type Model struct {
	section.BaseModel
	Prs []prrow.Data

	// The cursor row the table's cells were last built for, and when. The
	// selected row's cells carry selected styling, so a cursor move restyles the
	// two rows whose selected-ness changed instead of rebuilding every row.
	rowsBuiltSel int
	rowsBuiltAt  time.Time
	// The column widths the cells were built for. Cells are formatted for their
	// widths (titles and labels truncated when built), so a width change needs a
	// rebuild, not just a re-render.
	rowsBuiltWidths []int
}

func NewModel(
	id int,
	ctx *context.ProgramContext,
	cfg config.PrsSectionConfig,
	lastUpdated time.Time,
	createdAt time.Time,
) Model {
	m := Model{}
	m.BaseModel = section.NewModel(
		ctx,
		section.NewSectionOptions{
			Id:          id,
			Config:      cfg.ToSectionConfig(),
			Type:        SectionType,
			Columns:     GetSectionColumns(cfg, ctx),
			Singular:    m.GetItemSingularForm(),
			Plural:      m.GetItemPluralForm(),
			LastUpdated: lastUpdated,
			CreatedAt:   createdAt,
		},
	)
	m.Prs = []prrow.Data{}

	return m
}

func (m *Model) Update(msg tea.Msg) (section.Section, tea.Cmd) {
	var cmd tea.Cmd
	var err error

	switch msg := msg.(type) {
	case tea.KeyMsg:

		if m.IsSearchFocused() {
			switch msg.String() {
			case "ctrl+c", "esc":
				m.SearchBar.SetValue(m.SearchValue)
				blinkCmd := m.SetIsSearching(false)
				return m, blinkCmd

			case "enter":
				m.SearchValue = m.SearchBar.Value()
				m.SyncSmartFilterWithSearchValue()
				m.SetIsSearching(false)
				m.ResetRows()
				return m, tea.Batch(m.FetchNextPageSectionRows()...)
			}

			break
		}

		if m.IsPromptConfirmationFocused() {
			action := m.GetPromptConfirmationAction()
			pressed := msg.String()

			// The merge-method picker answers with a strategy letter rather than
			// y/N: the letter IS the confirmation, and it resolves on that single
			// keystroke. The choice is remembered for the repo once the merge
			// succeeds.
			if action == "merge_method" {
				if method := mergeMethodFromKey(pressed); method != "" {
					return m, m.closePrompt(m.runConfirmedAction("merge_" + method))
				}
				if pressed == "esc" || pressed == "ctrl+c" {
					return m, m.closePrompt(nil)
				}
			}

			// y/N confirmations resolve on one keystroke — including a repeat of
			// the key that opened them (press `m` to merge, `m` again to confirm).
			switch m.DecideConfirmKey(pressed) {
			case section.ConfirmAccept:
				return m, m.closePrompt(m.runConfirmedAction(action))
			case section.ConfirmCancel:
				return m, m.closePrompt(nil)
			}

			switch pressed {
			case "ctrl+c", "esc":
				return m, m.closePrompt(nil)

			case "enter":
				// Typed-then-Enter still works, and an empty answer cancels
				// (the prompt reads "(y/N)").
				input := m.PromptConfirmationBox.Value()
				if action == "merge_method" {
					if method := mergeMethodFromKey(input); method != "" {
						return m, m.closePrompt(m.runConfirmedAction("merge_" + method))
					}
					return m, m.closePrompt(nil)
				}
				if input == "Y" || input == "y" {
					return m, m.closePrompt(m.runConfirmedAction(action))
				}

				return m, m.closePrompt(nil)
			}

			break
		}

		switch {
		case key.Matches(msg, keys.PRKeys.Diff):
			cmd = m.diff()

		case key.Matches(msg, keys.PRKeys.ToggleSmartFiltering):
			before := m.IsFilteredByCurrentRemote

			// If we're filtering by the current repo - we want to remove it
			// If there's no repo filter we want to add the current repo filter.
			if m.HasCurrentRepoNameInConfiguredFilter() || !m.HasRepoNameInConfiguredFilter() {
				m.IsFilteredByCurrentRemote = !before
			}
			log.Debug(
				"toggled smart filtering",
				"before",
				before,
				"after",
				m.IsFilteredByCurrentRemote,
			)
			searchValue := m.GetSearchValue()
			if m.SearchValue != searchValue {
				m.SearchValue = searchValue
				m.SearchBar.SetValue(searchValue)
				m.SetIsSearching(false)
				m.ResetRows()
				return m, tea.Batch(m.FetchNextPageSectionRows()...)
			}

		case key.Matches(msg, keys.PRKeys.Checkout):
			cmd, err = m.checkout()
			if err != nil {
				m.Ctx.Error = err
			}

		case key.Matches(msg, keys.PRKeys.WatchChecks):
			cmd = m.watchChecks()
		}

	case tasks.UpdatePRMsg:
		for i, currPr := range m.Prs {
			if currPr.Primary.Number != msg.PrNumber {
				continue
			}

			if msg.IsClosed != nil {
				if *msg.IsClosed {
					currPr.Primary.State = "CLOSED"
				} else {
					currPr.Primary.State = "OPEN"
				}
			}
			if msg.NewComment != nil {
				currPr.Enriched.Comments.Nodes = append(
					currPr.Enriched.Comments.Nodes, *msg.NewComment)
			}
			if msg.AddedAssignees != nil {
				currPr.Primary.Assignees.Nodes = addAssignees(
					currPr.Primary.Assignees.Nodes, msg.AddedAssignees.Nodes)
			}
			if msg.RemovedAssignees != nil {
				currPr.Primary.Assignees.Nodes = removeAssignees(
					currPr.Primary.Assignees.Nodes, msg.RemovedAssignees.Nodes)
			}
			if msg.Labels != nil {
				currPr.Primary.Labels.Nodes = msg.Labels.Nodes
			}
			if msg.ReadyForReview != nil && *msg.ReadyForReview {
				currPr.Primary.IsDraft = false
			}
			if msg.IsMerged != nil && *msg.IsMerged {
				currPr.Primary.State = "MERGED"
				currPr.Primary.Mergeable = ""
			}
			if msg.IsInMergeQueue != nil {
				currPr.Primary.IsInMergeQueue = *msg.IsInMergeQueue
			}
			m.Prs[i] = currPr
			m.SetIsLoading(false)
			m.rebuildRows()
			break
		}

	case SectionPullRequestsFetchedMsg:
		if m.LastFetchTaskId == msg.TaskId {
			if m.PageInfo != nil {
				m.Prs = append(m.Prs, msg.Prs...)
			} else {
				m.Prs = msg.Prs
			}
			m.TotalCount = msg.TotalCount
			m.PageInfo = &msg.PageInfo
			m.SetIsLoading(false)
			m.rebuildRows()
			m.restoreSelection()
			m.Table.UpdateLastUpdated(time.Now())
			m.UpdateTotalItemsCount(m.TotalCount)
		}
	}

	search, searchCmd := m.SearchBar.Update(msg)
	m.syncSelectedRows()
	m.SearchBar = search

	prompt, promptCmd := m.PromptConfirmationBox.Update(msg)
	m.PromptConfirmationBox = prompt

	table, tableCmd := m.Table.Update(msg)
	m.Table = table

	return m, tea.Batch(cmd, searchCmd, promptCmd, tableCmd)
}

func (m *Model) EnrichPR(data data.EnrichedPullRequestData) {
	for i, currPr := range m.Prs {
		if currPr.Primary.Number != data.Number {
			continue
		}

		m.Prs[i].IsEnriched = true
		m.Prs[i].Enriched = data
	}
}

func GetSectionColumns(
	cfg config.PrsSectionConfig,
	ctx *context.ProgramContext,
) []table.Column {
	dLayout := ctx.Config.Defaults.Layout.Prs
	sLayout := cfg.Layout

	updatedAtLayout := config.MergeColumnConfigs(
		dLayout.UpdatedAt,
		sLayout.UpdatedAt,
	)
	createdAtLayout := config.MergeColumnConfigs(
		dLayout.CreatedAt,
		sLayout.CreatedAt,
	)
	repoLayout := config.MergeColumnConfigs(dLayout.Repo, sLayout.Repo)
	titleLayout := config.MergeColumnConfigs(dLayout.Title, sLayout.Title)
	authorLayout := config.MergeColumnConfigs(dLayout.Author, sLayout.Author)
	assigneesLayout := config.MergeColumnConfigs(
		dLayout.Assignees,
		sLayout.Assignees,
	)
	baseLayout := config.MergeColumnConfigs(dLayout.Base, sLayout.Base)
	numCommentsLayout := config.MergeColumnConfigs(
		dLayout.NumComments,
		sLayout.NumComments,
	)
	reviewStatusLayout := config.MergeColumnConfigs(
		dLayout.ReviewStatus,
		sLayout.ReviewStatus,
	)
	stateLayout := config.MergeColumnConfigs(dLayout.State, sLayout.State)
	ciLayout := config.MergeColumnConfigs(dLayout.Ci, sLayout.Ci)
	labelsLayout := config.MergeColumnConfigs(dLayout.Labels, sLayout.Labels)
	linesLayout := config.MergeColumnConfigs(dLayout.Lines, sLayout.Lines)

	if !ctx.Config.Theme.Ui.Table.Compact {
		return []table.Column{
			{
				Title:  "",
				Width:  utils.IntPtr(3),
				Hidden: stateLayout.Hidden,
			},
			{
				Title:  "Title",
				Grow:   utils.BoolPtr(true),
				Hidden: titleLayout.Hidden,
			},
			{
				Title:  constants.LabelsIcon,
				Width:  labelsLayout.Width,
				Hidden: labelsLayout.Hidden,
			},
			{
				Title:  "Assignees",
				Width:  assigneesLayout.Width,
				Hidden: assigneesLayout.Hidden,
			},
			{
				Title:  "Base",
				Width:  baseLayout.Width,
				Hidden: baseLayout.Hidden,
			},
			{
				Title:  constants.CommentsIcon,
				Width:  utils.IntPtr(4),
				Hidden: numCommentsLayout.Hidden,
			},
			{
				Title:  "󰯢",
				Width:  utils.IntPtr(4),
				Hidden: reviewStatusLayout.Hidden,
			},
			{
				Title:  "",
				Width:  &ctx.Styles.PrSection.CiCellWidth,
				Grow:   new(bool),
				Hidden: ciLayout.Hidden,
			},
			{
				Title:  "",
				Width:  linesLayout.Width,
				Hidden: linesLayout.Hidden,
			},
			{
				Title:  "󱦻",
				Width:  updatedAtLayout.Width,
				Hidden: updatedAtLayout.Hidden,
			},
			{
				Title:  "󱡢",
				Width:  createdAtLayout.Width,
				Hidden: createdAtLayout.Hidden,
			},
		}
	}

	return []table.Column{
		{
			Title:  "",
			Width:  utils.IntPtr(3),
			Hidden: stateLayout.Hidden,
		},
		{
			Title:  "",
			Width:  repoLayout.Width,
			Hidden: repoLayout.Hidden,
		},
		{
			Title:  "Title",
			Grow:   utils.BoolPtr(true),
			Hidden: titleLayout.Hidden,
		},
		{
			Title:  "Author",
			Width:  authorLayout.Width,
			Hidden: authorLayout.Hidden,
		},
		{
			Title:  constants.LabelsIcon,
			Width:  labelsLayout.Width,
			Hidden: labelsLayout.Hidden,
		},
		{
			Title:  "Assignees",
			Width:  assigneesLayout.Width,
			Hidden: assigneesLayout.Hidden,
		},
		{
			Title:  "Base",
			Width:  baseLayout.Width,
			Hidden: baseLayout.Hidden,
		},
		{
			Title:  constants.CommentsIcon,
			Width:  utils.IntPtr(4),
			Hidden: numCommentsLayout.Hidden,
		},
		{
			Title:  "󰯢",
			Width:  utils.IntPtr(4),
			Hidden: reviewStatusLayout.Hidden,
		},
		{
			Title:  "",
			Width:  &ctx.Styles.PrSection.CiCellWidth,
			Grow:   new(bool),
			Hidden: ciLayout.Hidden,
		},
		{
			Title:  "",
			Width:  linesLayout.Width,
			Hidden: linesLayout.Hidden,
		},
		{
			Title:  "󱦻",
			Width:  updatedAtLayout.Width,
			Hidden: updatedAtLayout.Hidden,
		},
		{
			Title:  "󱡢",
			Width:  createdAtLayout.Width,
			Hidden: createdAtLayout.Hidden,
		},
	}
}

func (m Model) BuildRows() []table.Row {
	rows := make([]table.Row, 0, len(m.Prs))
	currItem := m.Table.GetCurrItem()
	for i := range m.Prs {
		rows = append(rows, m.buildRow(i, currItem == i))
	}

	return rows
}

func (m Model) buildRow(i int, selected bool) table.Row {
	pr := m.Prs[i]
	prModel := prrow.PullRequest{
		Ctx:     m.Ctx,
		Data:    &pr,
		Columns: m.Table.Columns, ShowAuthorIcon: m.ShowAuthorIcon,
	}

	return prModel.ToTableRow(i, selected)
}

// rebuildRows rebuilds every row from the data. Needed when the data changed;
// a cursor move only needs syncSelectedRows.
func (m *Model) rebuildRows() {
	// Record the widths the cells are built WITH, not the ones after SetRows: a
	// fresh table only computes its column widths on its first sync, so its first
	// build sees zero widths (untruncated titles). If the sync changed the widths,
	// build once more; widths come from the layout, not the rows, so that settles.
	for range 2 {
		widths := m.columnWidths()
		m.Table.SetRows(m.BuildRows())
		m.rowsBuiltWidths = widths
		if slices.Equal(widths, m.columnWidths()) {
			break
		}
	}
	m.rowsBuiltSel = m.Table.GetCurrItem()
	m.rowsBuiltAt = time.Now()
}

func (m *Model) columnWidths() []int {
	widths := make([]int, len(m.Table.Columns))
	for i, c := range m.Table.Columns {
		widths[i] = c.ComputedWidth
	}

	return widths
}

// UpdateProgramContext re-syncs the section with the program context -- which
// the program does for every tab after any layout change -- and rebuilds the
// cells if that changed the column widths they were formatted for.
func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.BaseModel.UpdateProgramContext(ctx)
	if !slices.Equal(m.columnWidths(), m.rowsBuiltWidths) {
		m.rebuildRows()
	}
}

// rowsRefreshEvery bounds how stale the relative times in the rows can get
// between full rebuilds. Ages are formatted to the second ("now", "12s"), so
// this keeps recent ones honest; a full rebuild is a few milliseconds, so every
// 10s is nothing next to the per-message rebuild it replaces.
const rowsRefreshEvery = 10 * time.Second

// syncSelectedRows restyles the rows whose selected-ness changed since the rows
// were built -- the one the cursor left and the one it entered. This used to be
// a rebuild of every row on every message, the section's main per-message cost.
func (m *Model) syncSelectedRows() {
	if time.Since(m.rowsBuiltAt) > rowsRefreshEvery {
		m.rebuildRows()
		return
	}
	curr := m.Table.GetCurrItem()
	if curr == m.rowsBuiltSel {
		return
	}
	for _, i := range []int{m.rowsBuiltSel, curr} {
		if i >= 0 && i < len(m.Prs) {
			m.Table.SetRow(i, m.buildRow(i, i == curr))
		}
	}
	m.rowsBuiltSel = curr
}

func (m *Model) NumRows() int {
	return len(m.Prs)
}

type SectionPullRequestsFetchedMsg struct {
	Prs        []prrow.Data
	TotalCount int
	PageInfo   data.PageInfo
	TaskId     string
}

// DropMergedRows removes merged PRs from the section's data and rebuilds its
// rows, keeping whatever is still selected selected.
//
// The filtering has to happen here rather than in BuildRows because the table
// cursor indexes Prs directly (see GetCurrRow) — hiding rows at render time
// would leave the cursor pointing at a different PR than the highlighted one.
func (m *Model) DropMergedRows() {
	var selected string
	if r := m.GetCurrRow(); r != nil {
		selected = r.GetUrl()
	}

	kept := make([]prrow.Data, 0, len(m.Prs))
	for _, pr := range m.Prs {
		if pr.Primary != nil && pr.Primary.State == "MERGED" {
			continue
		}
		kept = append(kept, pr)
	}
	if len(kept) == len(m.Prs) {
		return
	}

	m.Prs = kept
	if selected != "" {
		m.SetPendingSelection(selected)
	}
	m.rebuildRows()
	m.restoreSelection()
}

func (m *Model) GetCurrRow() data.RowData {
	idx := m.Table.GetCurrItem()
	if idx < 0 || idx >= len(m.Prs) {
		return nil
	}
	pr := m.Prs[idx]
	return &pr
}

// restoreSelection re-selects, by URL, whatever was selected before a refresh
// rebuilt this section.
func (m *Model) restoreSelection() {
	urls := make([]string, len(m.Prs))
	for i := range m.Prs {
		if m.Prs[i].Primary != nil {
			urls[i] = m.Prs[i].Primary.GetUrl()
		}
	}
	m.RestoreSelection(urls)
}

func (m *Model) FetchNextPageSectionRows() []tea.Cmd {
	if m == nil {
		return nil
	}

	if m.PageInfo != nil && !m.PageInfo.HasNextPage {
		return nil
	}

	var cmds []tea.Cmd

	startCursor := time.Now().String()
	if m.PageInfo != nil {
		startCursor = m.PageInfo.StartCursor
	}
	taskId := fmt.Sprintf("fetching_prs_%d_%s", m.Id, startCursor)
	isFirstFetch := m.LastFetchTaskId == ""
	m.LastFetchTaskId = taskId
	task := context.Task{
		Id:        taskId,
		StartText: fmt.Sprintf(`Fetching PRs for "%s"`, m.Config.Title),
		FinishedText: fmt.Sprintf(
			`PRs for "%s" have been fetched`,
			m.Config.Title,
		),
		State: context.TaskStart,
		Error: nil,
	}
	startCmd := m.Ctx.StartTask(task)
	cmds = append(cmds, startCmd)

	fetchCmd := func() tea.Msg {
		limit := m.Config.Limit
		if limit == nil {
			limit = &m.Ctx.Config.Defaults.PrsLimit
		}

		res, err := data.FetchPullRequests(m.GetFilters(), *limit, m.PageInfo)
		if err != nil {
			return constants.TaskFinishedMsg{
				SectionId:   m.Id,
				SectionType: m.Type,
				TaskId:      taskId,
				Err:         err,
			}
		}

		// search() doesn't return isInMergeQueue, so enrich queued state from the
		// authoritative per-repo merge queue (scoped to mergeQueueRepos).
		data.EnrichMergeQueueStatus(m.Ctx.Config.Defaults, res.Prs)

		// Keep recently-merged PRs visible for a configurable window. Only on the
		// first page (PageInfo == nil) so pagination doesn't re-append them, and
		// not at all while the `M` toggle hides them — no point paying for a search
		// whose results would be filtered straight back out.
		if m.PageInfo == nil && !m.Ctx.HideMergedPRs {
			if w := m.Ctx.Config.Defaults.ResolveMergedWindow(m.Config.ShowMergedFor); w > 0 {
				// No date qualifier in the search; the window is applied to
				// mergedAt here (see data.MergedQuery for why).
				if mres, mErr := data.FetchPullRequests(data.MergedQuery(m.GetFilters()), *limit, nil); mErr == nil {
					recent := data.MergedSince(mres.Prs, time.Now().Add(-w))
					// Newest merge first: search returns them by last update, not merge.
					data.SortByMergedAtDesc(recent)
					res.Prs = append(res.Prs, recent...)
				}
			}
		}

		prs := make([]prrow.Data, 0)
		for _, pr := range res.Prs {
			prs = append(prs, prrow.Data{Primary: &pr})
		}
		return constants.TaskFinishedMsg{
			SectionId:   m.Id,
			SectionType: m.Type,
			TaskId:      taskId,
			Msg: SectionPullRequestsFetchedMsg{
				Prs:        prs,
				TotalCount: res.TotalCount,
				PageInfo:   res.PageInfo,
				TaskId:     taskId,
			},
		}
	}
	cmds = append(cmds, fetchCmd)

	m.IsLoading = true
	if isFirstFetch {
		m.SetIsLoading(true)
		cmds = append(cmds, m.Table.StartLoadingSpinner())
	}

	return cmds
}

func (m *Model) ResetRows() {
	m.Prs = nil
	m.BaseModel.ResetRows()
}

func FetchAllSections(
	ctx *context.ProgramContext,
	prs []section.Section,
) (sections []section.Section, fetchAllCmd tea.Cmd) {
	fetchPRsCmds := make([]tea.Cmd, 0, len(ctx.Config.PRSections))
	sections = make([]section.Section, 0, len(ctx.Config.PRSections))
	for i, sectionConfig := range ctx.Config.PRSections {
		sectionModel := NewModel(
			i+1, // 0 is the search section
			ctx,
			sectionConfig,
			time.Now(),
			time.Now(),
		)
		// Before any rows are built: they are no longer rebuilt on every message,
		// so a setting applied after the carry-over would not reach them.
		if sectionConfig.Layout.AuthorIcon.Hidden != nil {
			sectionModel.ShowAuthorIcon = !*sectionConfig.Layout.AuthorIcon.Hidden
		}
		// prs[0] is the search section, so this section's predecessor is at i+1 --
		// which needs len(prs) > i+1, not >= i+1 (that admits an out-of-range read
		// when the slice ends exactly at i).
		if len(prs) > i+1 && prs[i+1] != nil {
			oldSection := prs[i+1].(*Model)
			sectionModel.Prs = oldSection.Prs
			sectionModel.LastFetchTaskId = oldSection.LastFetchTaskId
			// Keep the cursor on the same PR across the refresh. Render the
			// carried-over rows with the cursor already in place NOW, so the list
			// doesn't flash to the top row while the fetch is in flight (a keypress
			// in that window would otherwise act on the wrong PR). SetPendingSelection
			// stays armed so the cursor is re-pinned by URL once the fresh, possibly
			// reordered data lands.
			if r := oldSection.GetCurrRow(); r != nil {
				sectionModel.SetPendingSelection(r.GetUrl())
			}
			sectionModel.rebuildRows()
			sectionModel.Table.RestoreScroll(oldSection.Table.ScrollState())
			sectionModel.syncSelectedRows() // cells for the restored cursor, not row 0
			// Carry the search UI too: a filter mid-edit would otherwise be
			// destroyed keystroke by keystroke, and an applied one would revert to
			// the configured filter on the next tick.
			if cmd := sectionModel.RestoreSearchState(oldSection.GetSearchState()); cmd != nil {
				fetchPRsCmds = append(fetchPRsCmds, cmd)
			}
		}
		sections = append(sections, &sectionModel)
		fetchPRsCmds = append(
			fetchPRsCmds,
			sectionModel.FetchNextPageSectionRows()...)
	}
	return sections, tea.Batch(fetchPRsCmds...)
}

func addAssignees(assignees, addedAssignees []data.Assignee) []data.Assignee {
	newAssignees := assignees
	for _, assignee := range addedAssignees {
		if !assigneesContains(newAssignees, assignee) {
			newAssignees = append(newAssignees, assignee)
		}
	}

	return newAssignees
}

func removeAssignees(
	assignees, removedAssignees []data.Assignee,
) []data.Assignee {
	newAssignees := []data.Assignee{}
	for _, assignee := range assignees {
		if !assigneesContains(removedAssignees, assignee) {
			newAssignees = append(newAssignees, assignee)
		}
	}

	return newAssignees
}

func assigneesContains(assignees []data.Assignee, assignee data.Assignee) bool {
	return slices.Contains(assignees, assignee)
}

func (m Model) GetItemSingularForm() string {
	return "PR"
}

func (m Model) GetItemPluralForm() string {
	return "PRs"
}

func (m Model) GetTotalCount() int {
	return m.TotalCount
}

func (m *Model) SetIsLoading(val bool) {
	m.IsLoading = val
	m.Table.SetIsLoading(val)
}

func (m Model) GetPagerContent() string {
	pagerContent := ""
	timeElapsed := utils.TimeElapsed(m.LastUpdated())
	if timeElapsed == "now" {
		timeElapsed = "just now"
	} else {
		timeElapsed = fmt.Sprintf("~%v ago", timeElapsed)
	}
	if m.TotalCount > 0 {
		pagerContent = fmt.Sprintf(
			"%v Updated %v • %v %v/%v (fetched %v)",
			constants.WaitingIcon,
			timeElapsed,
			m.SingularForm,
			m.Table.GetCurrItem()+1,
			m.TotalCount,
			len(m.Table.Rows),
		)
	}
	pager := m.Ctx.Styles.ListViewPort.PagerStyle.Render(pagerContent)
	return pager
}
