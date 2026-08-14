package tui

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	log "charm.land/log/v2"
	"github.com/atotto/clipboard"
	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/go-gh/v2/pkg/repository"
	zone "github.com/lrstanley/bubblezone/v2"

	gitm "github.com/aymanbagabas/git-module"
	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/git"
	"github.com/dlvhdr/gh-dash/v4/internal/prefs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/branch"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/branchsidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/footer"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issuessection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/issueview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/notificationview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prrow"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prssection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/prview"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/reposection"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/section"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/sidebar"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/table"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tabs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/tasks"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/markdown"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

type Model struct {
	keys             *keys.KeyMap
	sidebar          sidebar.Model
	prView           prview.Model
	issueSidebar     issueview.Model
	branchSidebar    branchsidebar.Model
	notificationView notificationview.Model
	currSectionId    int
	footer           footer.Model
	repo             section.Section
	prs              []section.Section
	issues           []section.Section
	notifications    []section.Section
	tabs             tabs.Model
	ctx              *context.ProgramContext
	taskSpinner      spinner.Model
	tasks            map[string]context.Task
	positionOverride string // "" means no override, "right" or "bottom"

	// Mouse drag-selection (see mouseselect.go).
	mouseDown bool
	dragged   bool
	sel       textSelection
	// Double-click bookkeeping (see mouseclick.go).
	lastClickRow int
	lastClickAt  time.Time
	// Draggable preview divider (see separator.go, layoutstate.go).
	resizingSeparator bool
	// previewHeightOverride is the dragged divider position. 0 is a legitimate
	// value (a collapsed preview), so previewHeightOverrideSet -- not a zero
	// check -- decides whether it beats the configured default.
	previewHeightOverride    int
	previewHeightOverrideSet bool
	layoutStateKey           string
	// frameBuf holds the last frame View rendered, so a drag-release can read
	// the text under the selection. View has a value receiver and cannot write
	// to the model, hence the pointer.
	frameBuf *string
}

type Repositories struct {
	GHRepo  *repository.Repository
	GitRepo *gitm.Repository
}

func NewModel(location config.Location, repos Repositories) Model {
	taskSpinner := spinner.Model{Spinner: spinner.Dot}
	m := Model{
		keys:        keys.Keys,
		sidebar:     sidebar.NewModel(),
		taskSpinner: taskSpinner,
		tasks:       map[string]context.Task{},
		frameBuf:    new(string),
		// -1, not 0: a zero value would look like "row 0 was just clicked".
		lastClickRow: -1,
	}

	// Restore where this dashboard's divider was last left.
	m.layoutStateKey = layoutKey(location.ConfigFlag, location.RepoPath)
	m.previewHeightOverride, m.previewHeightOverrideSet = loadPreviewHeight(m.layoutStateKey)

	// Prefer the ldflags-injected Version; fall back to module build info, then
	// to "dev". A local `go build` has no module version, so the fork's build
	// recipe sets Version via -ldflags (see version.go).
	version := Version
	if version == "" || version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			version = info.Main.Version
		}
	}

	m.ctx = &context.ProgramContext{
		GHRepo:     repos.GHRepo,
		GitRepo:    repos.GitRepo,
		ConfigFlag: location.ConfigFlag,
		RepoPath:   location.RepoPath,
		Version:    version,
		// Restored before the first fetch, so a dashboard left with merged PRs
		// hidden doesn't pay for the recently-merged query on startup.
		HideMergedPRs: loadMergedHidden(m.layoutStateKey),
		StartTask: func(task context.Task) tea.Cmd {
			log.Info("Starting task", "id", task.Id)
			task.StartTime = time.Now()
			m.tasks[task.Id] = task
			return m.taskSpinner.Tick
		},
		HasDarkBackground: true,
		BackgroundSource:  "default",
		Theme:             *theme.DefaultTheme,
	}

	m.footer = footer.NewModel(m.ctx)
	m.prView = prview.NewModel(m.ctx)
	m.issueSidebar = issueview.NewModel(m.ctx)
	m.branchSidebar = branchsidebar.NewModel(m.ctx)
	m.notificationView = notificationview.NewModel(m.ctx)
	m.tabs = tabs.NewModel(m.ctx)

	return m
}

func (m *Model) initScreen() tea.Msg {
	showError := func(err error) {
		styles := log.DefaultStyles()
		styles.Key = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)
		styles.Separator = lipgloss.NewStyle()

		logger := log.New(os.Stderr)
		logger.SetStyles(styles)
		logger.SetTimeFormat(time.RFC3339)
		logger.SetReportTimestamp(true)
		logger.SetPrefix("Reading config file")
		logger.SetReportCaller(true)

		logger.
			Fatal(
				"failed parsing config file",
				"location",
				m.ctx.ConfigFlag,
				"err",
				err,
			)
	}

	cfg, err := config.ParseConfig(
		config.Location{RepoPath: m.ctx.RepoPath, ConfigFlag: m.ctx.ConfigFlag},
	)
	if err != nil {
		showError(err)
		return initMsg{Config: cfg}
	}

	var url string
	if config.IsFeatureEnabled(config.FF_REPO_VIEW) && m.ctx.RepoPath != "" {
		res, err := git.GetOriginUrl(m.ctx.RepoPath)
		if err != nil {
			showError(err)
			return initMsg{Config: cfg}
		}
		url = res
	}

	err = keys.Rebind(
		cfg.Keybindings.Universal,
		cfg.Keybindings.Issues,
		cfg.Keybindings.Prs,
		cfg.Keybindings.Branches,
		cfg.Keybindings.Notifications,
		cfg.Keybindings.Cmp,
	)
	if err != nil {
		showError(err)
	}

	return initMsg{Config: cfg, RepoUrl: url}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.initScreen)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd             tea.Cmd
		tabsCmd         tea.Cmd
		sidebarCmd      tea.Cmd
		prViewCmd       tea.Cmd
		issueSidebarCmd tea.Cmd
		footerCmd       tea.Cmd
		cmds            []tea.Cmd
		currSection     = m.getCurrSection()
		currRowData     = m.getCurrRowData()
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		log.Info("Key pressed", "key", msg.String())
		m.ctx.Error = nil
		// Any keystroke can move what's under the highlight, so drop it.
		m.sel.active = false

		if currSection != nil && (currSection.IsSearchFocused() ||
			currSection.IsPromptConfirmationFocused()) {
			cmd = m.updateSection(currSection.GetId(), currSection.GetType(), msg)
			return m, cmd
		}

		if m.prView.IsTextInputBoxFocused() {
			m.prView, cmd = m.prView.Update(msg)
			m.syncSidebar()
			return m, cmd
		}

		if m.issueSidebar.IsTextInputBoxFocused() {
			m.issueSidebar, cmd, _ = m.issueSidebar.Update(msg)
			m.syncSidebar()
			return m, cmd
		}

		if m.footer.ShowConfirmQuit && (msg.String() == "y" || msg.String() == "enter") {
			return m, tea.Quit
		} else if m.footer.ShowConfirmQuit {
			m.footer.SetShowConfirmQuit(false)
			return m, nil
		}

		// Handle notification PR/Issue action confirmation
		if m.notificationView.HasPendingAction() {
			var action string
			m.notificationView, action = m.notificationView.Update(msg)
			m.footer.SetLeftSection("")
			if action != "" {
				return m, m.executeNotificationAction(action)
			}
			return m, nil
		}

		switch {
		case m.isUserDefinedKeybinding(msg):
			cmd = m.executeKeybinding(msg.String())
			return m, cmd

		case key.Matches(msg, m.keys.PrevSection):
			prevSection := m.getSectionAt(m.getPrevSectionId())
			if prevSection != nil {
				m.setCurrSectionId(prevSection.GetId())
				cmd = m.onViewedRowChanged()
			}

		case key.Matches(msg, m.keys.NextSection):
			nextSectionId := m.getNextSectionId()
			nextSection := m.getSectionAt(nextSectionId)
			if nextSection != nil {
				m.setCurrSectionId(nextSection.GetId())
				cmd = m.onViewedRowChanged()
			}

		case key.Matches(msg, m.keys.Down):
			if currSection != nil {
				prevRow := currSection.CurrRow()
				nextRow := currSection.NextRow()
				if prevRow != nextRow && nextRow == currSection.NumRows()-1 &&
					m.ctx.View != config.RepoView {
					cmds = append(cmds, currSection.FetchNextPageSectionRows()...)
				}
				cmd = m.onViewedRowChanged()
			}

		case key.Matches(msg, m.keys.Up):
			if currSection != nil {
				currSection.PrevRow()
				cmd = m.onViewedRowChanged()
			}

		case key.Matches(msg, m.keys.FirstLine):
			if currSection != nil {
				currSection.FirstItem()
				cmd = m.onViewedRowChanged()
			}

		case key.Matches(msg, m.keys.LastLine):
			if currSection != nil {
				if currSection.CurrRow()+1 < currSection.NumRows() {
					cmds = append(cmds, currSection.FetchNextPageSectionRows()...)
				}
				currSection.LastItem()
				cmd = m.onViewedRowChanged()
			}

		case key.Matches(msg, m.keys.TogglePreview):
			m.sidebar.IsOpen = !m.sidebar.IsOpen
			m.syncMainContentDimensions()

		case key.Matches(msg, m.keys.TogglePreviewPosition):
			if m.sidebar.IsOpen {
				if m.ctx.PreviewPosition == "right" {
					m.positionOverride = "bottom"
				} else {
					m.positionOverride = "right"
				}
				m.syncMainContentDimensions()
				m.syncProgramContext()
				cmd := m.syncSidebar()
				cmds = append(cmds, cmd)
			}

		case key.Matches(msg, m.keys.Refresh):
			if currSection != nil {
				data.ClearEnrichmentCache()
				currSection.ResetFilters()
				currSection.ResetRows()
				m.syncSidebar()
				currSection.SetIsLoading(true)
				cmds = append(cmds, currSection.FetchNextPageSectionRows()...)
			}

		case key.Matches(msg, m.keys.RefreshAll):
			data.ClearEnrichmentCache()
			newSections, fetchSectionsCmds := m.fetchAllViewSections()
			m.setCurrentViewSections(newSections)
			cmds = append(cmds, fetchSectionsCmds)

		case key.Matches(msg, m.keys.Redraw):
			// with bubbletea v2's declarative approach, if we just clear the screen then tea will redraw for us
			return m, tea.ClearScreen

		case key.Matches(msg, m.keys.Search):
			if currSection != nil {
				cmd = currSection.SetIsSearching(true)
				return m, cmd
			}

		case key.Matches(msg, m.keys.Help):
			m.footer.ShowAll = !m.footer.ShowAll
			m.syncMainContentDimensions()

		case key.Matches(msg, m.keys.CopyNumber):
			var cmd tea.Cmd
			if currRowData == nil || reflect.ValueOf(currRowData).IsNil() {
				cmd = m.notifyErr("Current selection isn't associated with a PR/Issue")
				return m, cmd
			}
			number := fmt.Sprint(currRowData.GetNumber())
			err := clipboard.WriteAll(number)
			if err != nil {
				cmd = m.notifyErr(fmt.Sprintf("Failed copying to clipboard %v", err))
			} else {
				cmd = m.notify(fmt.Sprintf("Copied %s to clipboard", number))
			}
			return m, cmd

		case key.Matches(msg, m.keys.CopyUrl):
			var cmd tea.Cmd
			if currRowData == nil || reflect.ValueOf(currRowData).IsNil() {
				cmd = m.notifyErr("Current selection isn't associated with a PR/Issue")
				return m, cmd
			}
			url := currRowData.GetUrl()
			err := clipboard.WriteAll(url)
			if err != nil {
				cmd = m.notifyErr(fmt.Sprintf("Failed copying to clipboard %v", err))
			} else {
				cmd = m.notify(fmt.Sprintf("Copied %s to clipboard", url))
			}
			return m, cmd

		case key.Matches(msg, m.keys.Quit):
			if !m.ctx.Config.ConfirmQuit {
				return m, tea.Quit
			}

			m.footer.SetShowConfirmQuit(true)

		case m.ctx.View == config.RepoView:
			switch {
			case key.Matches(msg, m.keys.OpenGithub):
				cmds = append(cmds, m.repo.(*reposection.Model).OpenGithub())

			case key.Matches(msg, keys.BranchKeys.Delete):
				if currSection != nil {
					currSection.SetPromptConfirmationAction("delete")
					cmd = currSection.SetIsPromptConfirmationShown(true)
				}
				return m, cmd

			case key.Matches(msg, keys.BranchKeys.New):
				if currSection != nil {
					currSection.SetPromptConfirmationAction("new")
					cmd = currSection.SetIsPromptConfirmationShown(true)
				}
				return m, cmd

			case key.Matches(msg, keys.BranchKeys.CreatePr):
				if currSection != nil {
					currSection.SetPromptConfirmationAction("create_pr")
					cmd = currSection.SetIsPromptConfirmationShown(true)
				}
				return m, cmd

			case key.Matches(msg, keys.BranchKeys.ViewPRs):
				cmds = append(cmds, m.switchSelectedView())
			}
		case m.ctx.View == config.PRsView:
			switch {
			case key.Matches(msg, keys.PRKeys.PrevSidebarTab),
				key.Matches(msg, keys.PRKeys.NextSidebarTab):
				var scmds []tea.Cmd
				var scmd tea.Cmd
				m.prView, scmd = m.prView.Update(msg)
				scmds = append(scmds, scmd)
				m.syncSidebar()
				return m, tea.Batch(scmds...)

			case key.Matches(msg, m.keys.OpenGithub):
				cmds = append(cmds, m.openBrowser())

			case key.Matches(msg, keys.PRKeys.Approve):
				return m, m.openSidebarForPRInput(m.prView.SetIsApproving)

			case key.Matches(msg, keys.PRKeys.Assign):
				return m, m.openSidebarForPRInput(m.prView.SetIsAssigning)

			case key.Matches(msg, keys.PRKeys.Unassign):
				return m, m.openSidebarForPRInput(m.prView.SetIsUnassigning)

			case key.Matches(msg, keys.PRKeys.Label):
				return m, m.openSidebarForPRInput(m.prView.SetIsLabeling)

			case key.Matches(msg, keys.PRKeys.Comment):
				return m, m.openSidebarForPRInput(m.prView.SetIsCommenting)

			case key.Matches(msg, keys.PRKeys.Close):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "close", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.PRKeys.Ready):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "ready", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.PRKeys.Reopen):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "reopen", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.PRKeys.Merge):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, m.mergeAction(currRowData), msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.PRKeys.Update):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "update", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.PRKeys.ApproveWorkflows):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "approveWorkflows", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.PRKeys.ToggleMerged):
				return m, m.toggleMergedVisibility()

			case key.Matches(msg, keys.PRKeys.ViewIssues):
				cmds = append(cmds, m.switchSelectedView())

			case key.Matches(msg, keys.PRKeys.SummaryViewMore):
				m.prView.SetSummaryViewMore()
				m.syncSidebar()
				return m, nil
			}
		case m.ctx.View == config.IssuesView:
			switch {
			case key.Matches(msg, m.keys.OpenGithub):
				cmds = append(cmds, m.openBrowser())

			case key.Matches(msg, keys.IssueKeys.Label):
				return m, m.openSidebarForInput(m.issueSidebar.SetIsLabeling)

			case key.Matches(msg, keys.IssueKeys.Assign):
				return m, m.openSidebarForInput(m.issueSidebar.SetIsAssigning)

			case key.Matches(msg, keys.IssueKeys.Unassign):
				return m, m.openSidebarForInput(m.issueSidebar.SetIsUnassigning)

			case key.Matches(msg, keys.IssueKeys.Comment):
				return m, m.openSidebarForInput(m.issueSidebar.SetIsCommenting)

			case key.Matches(msg, keys.IssueKeys.Checkout):
				cmd, err := m.issueSidebar.Checkout()
				if err != nil {
					m.ctx.Error = err
				}
				return m, cmd

			case key.Matches(msg, keys.IssueKeys.Close):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "close", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.IssueKeys.Reopen):
				if currRowData != nil {
					cmd = m.promptConfirmation(currSection, "reopen", msg.String())
				}
				return m, cmd

			case key.Matches(msg, keys.IssueKeys.ViewPRs):
				cmds = append(cmds, m.switchSelectedView())
			}
		case m.ctx.View == config.NotificationsView:
			switch {
			case key.Matches(msg, m.keys.OpenGithub):
				cmds = append(cmds, m.openBrowser())
				return m, tea.Batch(cmds...)

			// Handle Enter to (re)load notification content - check before subject handlers
			// so Enter always works, even after viewing a notification
			case key.Matches(msg, keys.NotificationKeys.View):
				cmds = append(cmds, m.loadNotificationContent())

			// Return from PR/Issue detail back to the default notification prompt
			case key.Matches(msg, keys.NotificationKeys.BackToNotification):
				return m, m.backToNotification()

			// PR keybindings when viewing a PR notification
			case m.notificationView.GetSubjectPR() != nil:
				// Check for PR actions first (before updating prView)
				if !m.prView.IsTextInputBoxFocused() {
					action := prview.MsgToAction(msg)
					if action != nil {
						switch action.Type {
						case prview.PRActionApprove:
							return m, m.openSidebarForPRInput(m.prView.SetIsApproving)

						case prview.PRActionAssign:
							return m, m.openSidebarForPRInput(m.prView.SetIsAssigning)

						case prview.PRActionUnassign:
							return m, m.openSidebarForPRInput(m.prView.SetIsUnassigning)

						case prview.PRActionLabel:
							return m, m.openSidebarForPRInput(m.prView.SetIsLabeling)

						case prview.PRActionComment:
							return m, m.openSidebarForPRInput(m.prView.SetIsCommenting)

						case prview.PRActionDiff:
							if pr := m.notificationView.GetSubjectPR(); pr != nil {
								cmd = common.DiffPR(pr.GetNumber(), pr.GetRepoNameWithOwner(),
									m.ctx.Config.GetFullScreenDiffPagerEnv())
							}
							return m, cmd

						case prview.PRActionCheckout:
							if pr := m.notificationView.GetSubjectPR(); pr != nil {
								cmd, _ = notificationssection.CheckoutPR(
									m.ctx, pr.GetNumber(), pr.GetRepoNameWithOwner())
							}
							return m, cmd

						case prview.PRActionClose:
							cmd = m.promptConfirmationForNotificationPR("close", msg.String())
							return m, cmd

						case prview.PRActionReady:
							cmd = m.promptConfirmationForNotificationPR("ready", msg.String())
							return m, cmd

						case prview.PRActionReopen:
							cmd = m.promptConfirmationForNotificationPR("reopen", msg.String())
							return m, cmd

						case prview.PRActionMerge:
							cmd = m.promptConfirmationForNotificationPR(
								m.notificationMergeAction(), msg.String())
							return m, cmd

						case prview.PRActionUpdate:
							cmd = m.promptConfirmationForNotificationPR("update", msg.String())
							return m, cmd

						case prview.PRActionApproveWorkflows:
							cmd = m.promptConfirmationForNotificationPR("approveWorkflows", msg.String())
							return m, cmd

						case prview.PRActionSummaryViewMore:
							m.prView.SetSummaryViewMore()
							m.syncSidebar()
							return m, nil
						}
					}
				}

				// Handle 's' key to switch views
				if key.Matches(msg, keys.PRKeys.ViewIssues) {
					cmds = append(cmds, m.switchSelectedView())
				}

				// No action matched - update prView for navigation (tab switching, scrolling)
				var prCmd tea.Cmd
				m.prView, prCmd = m.prView.Update(msg)
				m.syncSidebar()
				cmds = append(cmds, prCmd)

			// Issue keybindings when viewing an Issue notification
			case m.notificationView.GetSubjectIssue() != nil:
				var issueCmd tea.Cmd
				var action *issueview.IssueAction
				m.issueSidebar, issueCmd, action = m.issueSidebar.Update(msg)

				if action != nil {
					switch action.Type {
					case issueview.IssueActionLabel:
						return m, m.openSidebarForInput(m.issueSidebar.SetIsLabeling)

					case issueview.IssueActionAssign:
						return m, m.openSidebarForInput(m.issueSidebar.SetIsAssigning)

					case issueview.IssueActionUnassign:
						return m, m.openSidebarForInput(m.issueSidebar.SetIsUnassigning)

					case issueview.IssueActionComment:
						return m, m.openSidebarForInput(m.issueSidebar.SetIsCommenting)

					case issueview.IssueActionCheckout:
						cmd, err := m.issueSidebar.Checkout()
						if err != nil {
							m.ctx.Error = err
						}
						return m, cmd

					case issueview.IssueActionClose:
						cmd = m.promptConfirmationForNotificationIssue("close", msg.String())
						return m, cmd

					case issueview.IssueActionReopen:
						cmd = m.promptConfirmationForNotificationIssue("reopen", msg.String())
						return m, cmd
					}
				}

				// Handle 's' key to switch views
				if key.Matches(msg, keys.IssueKeys.ViewPRs) {
					cmds = append(cmds, m.switchSelectedView())
				}

				// Sync sidebar and return issueCmd for navigation
				m.syncSidebar()
				cmds = append(cmds, issueCmd)

			case key.Matches(msg, keys.NotificationKeys.MarkAsDone):
				cmds = append(
					cmds,
					m.updateSection(currSection.GetId(), currSection.GetType(), msg),
				)

			case key.Matches(msg, keys.NotificationKeys.MarkAllAsDone):
				cmd = m.promptConfirmation(currSection, "done_all", msg.String())
				return m, cmd

			case key.Matches(msg, keys.NotificationKeys.Open):
				cmd = m.updateSection(currSection.GetId(), currSection.GetType(), msg)
				return m, cmd

			case key.Matches(msg, keys.NotificationKeys.SortByRepo):
				cmd = m.updateSection(currSection.GetId(), currSection.GetType(), msg)
				return m, cmd

			case key.Matches(msg, keys.PRKeys.ViewIssues):
				cmds = append(cmds, m.switchSelectedView())
			}
		}

	case initMsg:
		m.ctx.Config = &msg.Config
		m.ctx.RepoUrl = msg.RepoUrl
		m.ctx.Theme = theme.ParseTheme(m.ctx.Config)
		m.ctx.Styles = context.InitStyles(m.ctx.Theme)
		m.taskSpinner.Style = lipgloss.NewStyle().
			Background(m.ctx.Theme.SelectedBackground)

		m.ctx.View = m.ctx.Config.Defaults.View
		m.currSectionId = m.getCurrentViewDefaultSection()
		m.sidebar.IsOpen = msg.Config.Defaults.Preview.Open
		m.syncMainContentDimensions()

		newSections, fetchSectionsCmds := m.fetchAllViewSections()
		// Restore each section's remembered selection so it re-selects the same
		// item once its first fetch lands (see restoreSelection in each section).
		for _, s := range newSections {
			if s == nil {
				continue
			}
			if url := loadSelection(m.layoutStateKey, s.GetConfig().Title); url != "" {
				s.SetPendingSelection(url)
			}
		}
		m.setCurrentViewSections(newSections)
		m.tabs.SetCurrSectionId(1)

		if m.ctx.BackgroundSource != "bubbletea" {
			log.Debugf("Setting markdownStyle in initMsg")
			m.ctx.HasDarkBackground = compat.HasDarkBackground
			m.ctx.BackgroundSource = "compat"
			log.Debugf(
				"HasDarkBackground: %t, BackgroundSource: %s",
				m.ctx.HasDarkBackground,
				m.ctx.BackgroundSource,
			)
			markdown.InitializeMarkdownStyle(m.ctx)
		}

		cmds = append(cmds, fetchSectionsCmds, m.tabs.Init(), fetchUser,
			m.doRefreshAtInterval(), m.doUpdateFooterAtInterval(),
			m.prewarmBrowserCmd())

	case intervalRefresh:
		newSections, fetchSectionsCmds := m.fetchAllViewSections()
		m.setCurrentViewSections(newSections)
		cmds = append(cmds, fetchSectionsCmds, m.doRefreshAtInterval())

	case userFetchedMsg:
		m.ctx.User = msg.user

	case constants.TaskFinishedMsg:
		task, ok := m.tasks[msg.TaskId]
		if ok {
			log.Info("Task finished", "id", task.Id)
			if msg.Err != nil {
				log.Error("Task finished with error", "id", task.Id, "err", msg.Err)
				task.State = context.TaskError
				task.Error = msg.Err
			} else {
				task.State = context.TaskFinished
			}
			now := time.Now()
			task.FinishedTime = &now
			m.tasks[msg.TaskId] = task
			clear := tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return constants.ClearTaskMsg{TaskId: msg.TaskId}
			})
			cmds = append(cmds, clear)

			scmd := m.updateSection(msg.SectionId, msg.SectionType, msg.Msg)
			cmds = append(cmds, scmd)

			syncCmd := m.syncSidebar()
			cmds = append(cmds, syncCmd)
		}

	case prview.EnrichedPrMsg:
		if msg.Err == nil {
			m.prView.SetEnrichedPR(msg.Data)
			m.prs[msg.Id].(*prssection.Model).EnrichPR(msg.Data)
			syncCmd := m.syncSidebar()
			cmds = append(cmds, syncCmd)
		} else {
			log.Error("failed enriching pr", "err", msg.Err)
		}

	case notificationPRFetchedMsg:
		if msg.Err == nil {
			// Convert enriched PR to prrow.Data for display
			prData := msg.PR.ToPullRequestData()
			m.notificationView.SetSubjectPR(&prrow.Data{
				Primary:    &prData,
				Enriched:   msg.PR,
				IsEnriched: true,
			}, msg.NotificationId)
			keys.SetNotificationSubject(keys.NotificationSubjectPR)
			// Update sidebar with PR view
			width := m.sidebar.GetSidebarContentWidth()
			m.prView.SetSectionId(0)
			m.prView.SetRow(m.notificationView.GetSubjectPR())
			m.prView.SetWidth(width)
			m.prView.SetEnrichedPR(msg.PR)
			// Switch to Activity tab and scroll to bottom if there's a latest comment
			// (indicates there's new activity to show)
			if msg.LatestCommentUrl != "" {
				m.prView.GoToActivityTab()
				m.sidebar.SetContent(m.prView.View())
				m.sidebar.ScrollToBottom()
			} else {
				// For notifications without comments (new PRs, state changes, etc.)
				// show the Overview tab without scrolling
				m.prView.GoToFirstTab()
				m.sidebar.SetContent(m.prView.View())
			}
			m.markNotificationAsRead(msg.NotificationId)
		} else {
			log.Error("failed fetching notification PR", "err", msg.Err)
		}

	case notificationIssueFetchedMsg:
		if msg.Err == nil {
			m.notificationView.SetSubjectIssue(&msg.Issue, msg.NotificationId)
			keys.SetNotificationSubject(keys.NotificationSubjectIssue)
			// Update sidebar with Issue view
			width := m.sidebar.GetSidebarContentWidth()
			m.issueSidebar.SetSectionId(0)
			m.issueSidebar.SetRow(m.notificationView.GetSubjectIssue())
			m.issueSidebar.SetWidth(width)
			m.sidebar.SetContent(m.issueSidebar.View())
			// Scroll to bottom if there's a latest comment (indicates new activity)
			if msg.LatestCommentUrl != "" {
				m.sidebar.ScrollToBottom()
			}
			m.markNotificationAsRead(msg.NotificationId)
		} else {
			log.Error("failed fetching notification Issue", "err", msg.Err)
		}

	case notificationssection.UpdateNotificationReadStateMsg:
		m.updateNotificationSections(msg)

	case notificationssection.UpdateNotificationCommentsMsg:
		cmds = append(cmds, m.updateNotificationSections(msg))

	case spinner.TickMsg:
		if len(m.tasks) > 0 {
			taskSpinner, internalTickCmd := m.taskSpinner.Update(msg)
			m.taskSpinner = taskSpinner
			rTask := m.renderRunningTask()
			m.footer.SetRightSection(rTask)
			cmd = internalTickCmd
		}

	case constants.ClearTaskMsg:
		m.footer.SetRightSection("")
		delete(m.tasks, msg.TaskId)

	case section.SectionMsg:
		cmd = m.updateRelevantSection(msg)

		if msg.Id == m.currSectionId {
			cmds = append(cmds, m.onViewedRowChanged())
		}

	case execProcessFinishedMsg, tea.FocusMsg:
		if currSection != nil {
			cmds = append(cmds, currSection.FetchNextPageSectionRows()...)
		}

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		// Pressing on the divider starts a resize, not a text selection.
		if m.onSeparator(msg.Y) {
			m.resizingSeparator = true
			return m, nil
		}
		// A press only *starts* something. Whether it is a click or the first
		// cell of a drag-selection is unknowable until the button comes back
		// up, so nothing is actioned here.
		m.mouseDown = true
		m.dragged = false
		m.sel = textSelection{
			anchorX: msg.X, anchorY: msg.Y,
			cursorX: msg.X, cursorY: msg.Y,
		}
		return m, nil

	case tea.MouseMotionMsg:
		if m.resizingSeparator {
			if msg.Button == tea.MouseLeft {
				m.setPreviewHeightFromSeparatorY(msg.Y)
			}
			return m, nil
		}
		// Motion only arrives while a button is held (MouseModeCellMotion).
		if !m.mouseDown || msg.Button != tea.MouseLeft {
			return m, nil
		}
		// Sub-threshold jitter is a sloppy click, not a drag: ignore it so the
		// release still selects/opens the row instead of leaving a lingering
		// one-character selection highlight. Once a drag, always a drag.
		if !m.dragged && !pastDragThreshold(m.sel.anchorX, m.sel.anchorY, msg.X, msg.Y) {
			return m, nil
		}
		m.dragged = true
		m.sel.active = true
		m.sel.cursorX, m.sel.cursorY = msg.X, msg.Y
		return m, nil

	case tea.MouseReleaseMsg:
		// Checked before the mouseDown guard: a separator press never arms one.
		if m.resizingSeparator {
			m.resizingSeparator = false
			if err := savePreviewHeight(m.layoutStateKey, m.previewHeightOverride); err != nil {
				log.Debug("could not persist the preview height", "err", err)
			}
			return m, nil
		}

		if !m.mouseDown {
			return m, nil
		}
		m.mouseDown = false

		// A drag is a text selection, never an action -- dragging back onto the
		// press cell must not open a browser.
		if m.dragged {
			if m.sel.isEmpty() || m.frameBuf == nil {
				m.sel.active = false
				return m, nil
			}
			text := selectedText(*m.frameBuf, m.sel)
			if text == "" {
				m.sel.active = false
				return m, nil
			}
			if err := copyToClipboard(text); err != nil {
				cmds = append(cmds, m.notifyErr(fmt.Sprintf("Failed copying to clipboard %v", err)))
			} else {
				cmds = append(cmds, m.notify(fmt.Sprintf("Copied %d characters", len(text))))
			}
			return m, tea.Batch(cmds...)
		}

		// Press and release on the same cell: a plain click.
		m.sel.active = false

		if zone.Get("donate").InBounds(msg) {
			log.Info("Donate clicked", "msg", msg)
			openCmd := func() tea.Msg {
				// Discard the launcher's stdout/stderr so any noise (e.g.
				// GTK / GVFS warnings from xdg-open / gnome-open) does not
				// leak into the TUI's terminal and corrupt the display.
				// See #829, #584, #679.
				b := browser.New("", io.Discard, io.Discard)
				err := b.Browse("https://github.com/sponsors/dlvhdr")
				if err != nil {
					return constants.ErrMsg{Err: err}
				}
				return nil
			}
			cmds = append(cmds, openCmd)
			return m, tea.Batch(cmds...)
		}

		// Clicking a tab switches to that section.
		for i := range m.getCurrentViewSections() {
			if !zone.Get(tabs.TabZoneID(i)).InBounds(msg) {
				continue
			}
			if s := m.getSectionAt(i); s != nil {
				m.setCurrSectionId(s.GetId())
				cmds = append(cmds, m.onViewedRowChanged())
			}
			return m, tea.Batch(cmds...)
		}

		// Footer: the help toggle and the PRs/Issues/Notifications switcher.
		if zone.Get(footer.ZoneHelp).InBounds(msg) {
			m.footer.ShowAll = !m.footer.ShowAll
			// Reflow now. Expanding/collapsing help changes the content height,
			// and a mouse branch returns early -- skipping the syncProgramContext
			// at the end of Update that the keyboard path falls through to. Both
			// syncs together are what resize the section viewports; without them
			// the frame grew, the "? help" target moved, and the panes weren't
			// restored on close.
			m.syncMainContentDimensions()
			m.syncProgramContext()
			return m, tea.Batch(cmds...)
		}
		for _, view := range []config.ViewType{
			config.PRsView, config.IssuesView, config.NotificationsView,
		} {
			if zone.Get(footer.ViewZoneID(view)).InBounds(msg) {
				cmds = append(cmds, m.setSelectedView(view))
				m.syncProgramContext()
				return m, tea.Batch(cmds...)
			}
		}

		// Preview detail tabs (Overview / Activity / Commits / Checks / Files).
		if m.sidebar.IsOpen {
			for i := range prview.NumTabs() {
				if !zone.Get(prview.TabZoneID(i)).InBounds(msg) {
					continue
				}
				m.prView.SetTabIdx(i)
				cmds = append(cmds, m.syncSidebar())
				return m, tea.Batch(cmds...)
			}
		}

		// Row clicks. The sub-regions of a row (its number, its icons) mean
		// something more specific than "this row", so they are tested first.
		// A bare click anywhere else only selects; opening needs a double-click.
		if currSection != nil {
			for i := range currSection.NumRows() {
				hit := rowHit{
					number:   zone.Get(common.RowTargetZoneID(i, common.ZoneNumber)).InBounds(msg),
					ci:       zone.Get(common.RowTargetZoneID(i, common.ZoneCi)).InBounds(msg),
					review:   zone.Get(common.RowTargetZoneID(i, common.ZoneReview)).InBounds(msg),
					comments: zone.Get(common.RowTargetZoneID(i, common.ZoneComments)).InBounds(msg),
				}
				if !hit.targeted() && !zone.Get(table.RowZoneID(i)).InBounds(msg) {
					continue
				}

				currSection.SetCurrRow(i)
				cmds = append(cmds, m.onViewedRowChanged())

				// Rebuild the section's rows so the PREVIOUSLY selected row loses
				// its highlight. BuildRows() bakes the selected styling into each
				// cell, so SetCurrRow's SyncViewPortContent alone re-renders with
				// stale selection. The section rebuilds at the tail of its Update
				// on any message; the keyboard path reaches that via
				// updateCurrentSection, but this early-returning click path skips
				// it -- so trigger the same rebuild here. table.Update ignores
				// mouse events, so forwarding the release msg only rebuilds.
				cmds = append(cmds, m.updateCurrentSection(msg))

				// Only a bare click can pair into a double-click; clicking an
				// icon must not arm one.
				isDouble := false
				if !hit.targeted() {
					isDouble = m.registerRowClick(i)
				}
				if url := rowClickAction(m.currRowURL(), hit, isDouble); url != "" {
					cmds = append(cmds, m.openURL(url))
				}

				return m, tea.Batch(cmds...)
			}
		}

	case tea.MouseWheelMsg:
		// Rows move under the pointer; a kept highlight would mark the wrong text.
		m.sel.active = false
		if currSection == nil {
			return m, nil
		}
		// Default matches macOS natural scrolling: a two-finger-down gesture (the
		// terminal reports it as a wheel-up event there) moves the selection DOWN
		// the list. mouseWheelReverse flips it for a classic mouse.
		down := msg.Button == tea.MouseWheelUp
		if m.ctx.Config.Defaults.MouseWheelReverse {
			down = msg.Button == tea.MouseWheelDown
		}
		if msg.Button == tea.MouseWheelUp || msg.Button == tea.MouseWheelDown {
			if down {
				currSection.NextRow()
			} else {
				currSection.PrevRow()
			}
			cmds = append(cmds, m.onViewedRowChanged())
		}

	case tea.WindowSizeMsg:
		m.onWindowSizeChanged(msg)

	case tea.BackgroundColorMsg:
		log.Debugf("Setting markdownStyle in BackgroundColorMsg")
		m.ctx.HasDarkBackground = msg.IsDark()
		m.ctx.BackgroundSource = "bubbletea"
		log.Debugf(
			"HasDarkBackground: %t, BackgroundSource: %s",
			m.ctx.HasDarkBackground,
			m.ctx.BackgroundSource,
		)
		markdown.InitializeMarkdownStyle(m.ctx)

	case updateFooterMsg:
		cmds = append(cmds, cmd, m.doUpdateFooterAtInterval())

	case constants.ErrMsg:
		m.ctx.Error = msg.Err
	}

	m.syncProgramContext()

	var bsCmd tea.Cmd
	m.branchSidebar, bsCmd = m.branchSidebar.Update(msg)
	cmds = append(cmds, bsCmd)

	m.sidebar, sidebarCmd = m.sidebar.Update(msg)

	if m.prView.IsTextInputBoxFocused() {
		m.prView, prViewCmd = m.prView.Update(msg)
		m.syncSidebar()
	}

	if m.issueSidebar.IsTextInputBoxFocused() {
		m.issueSidebar, issueSidebarCmd, _ = m.issueSidebar.Update(msg)
		m.syncSidebar()
	}

	if currSection != nil {
		if currSection.IsPromptConfirmationFocused() {
			m.footer.SetLeftSection(currSection.GetPromptConfirmation())
		}

		if !currSection.IsPromptConfirmationFocused() {
			m.footer.SetLeftSection(currSection.GetPagerContent())
		}
	}

	tm, tabsCmd := m.tabs.Update(msg)
	m.tabs = tm

	sectionCmd := m.updateCurrentSection(msg)
	cmds = append(
		cmds,
		cmd,
		tabsCmd,
		sidebarCmd,
		footerCmd,
		sectionCmd,
		prViewCmd,
		issueSidebarCmd,
	)

	return m, tea.Batch(cmds...)
}

// teaMouseMode maps the configured mouse mode onto bubbletea's. Mouse capture
// powers clickable UI, but it intercepts click-drag, so `none` is what you set
// when you want your terminal's native text selection back.
func teaMouseMode(mm config.MouseMode) tea.MouseMode {
	switch mm {
	case config.MouseModeNone:
		return tea.MouseModeNone
	case config.MouseModeAllMotion:
		return tea.MouseModeAllMotion
	default:
		return tea.MouseModeCellMotion
	}
}

func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.ReportFocus = true
	v.MouseMode = tea.MouseModeCellMotion

	if m.ctx.Config == nil {
		v.Content = lipgloss.Place(
			m.ctx.ScreenWidth,
			m.ctx.ScreenHeight,
			lipgloss.Center,
			lipgloss.Center,
			"Reading config...",
		)
		return v
	}

	v.MouseMode = teaMouseMode(m.ctx.Config.Defaults.MouseMode)

	s := strings.Builder{}
	if m.ctx.View != config.RepoView {
		s.WriteString(m.tabs.View())
	}
	s.WriteString("\n")
	content := "No sections defined"
	currSection := m.getCurrSection()
	if currSection != nil {
		if m.ctx.PreviewPosition == "bottom" && m.sidebar.IsOpen {
			content = lipgloss.JoinVertical(
				lipgloss.Left,
				m.getCurrSection().View(),
				m.sidebar.View(),
			)
		} else {
			content = lipgloss.JoinHorizontal(
				lipgloss.Top,
				m.getCurrSection().View(),
				m.sidebar.View(),
			)
		}
	}
	s.WriteString(content)
	s.WriteString("\n")
	if m.ctx.Error != nil {
		s.WriteString(
			m.ctx.Styles.Common.ErrorStyle.
				Width(m.ctx.ScreenWidth).
				Render(fmt.Sprintf("%s %s",
					m.ctx.Styles.Common.FailureGlyph,
					lipgloss.NewStyle().
						Foreground(m.ctx.Theme.ErrorText).
						Render(m.ctx.Error.Error()),
				)),
		)
	} else {
		s.WriteString(m.footer.View())
	}

	// Keep the un-highlighted frame: a drag-release reads the selected text
	// from it, and highlighting is purely a presentation pass on top.
	frame := zone.Scan(s.String())
	if m.frameBuf != nil {
		*m.frameBuf = frame
	}

	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(highlightFrame(frame, m.sel)),
	}

	if currSection != nil {
		searchCmp := currSection.ViewCompletions()
		if searchCmp != "" {
			y := common.HeaderHeight + common.SearchHeight + 1
			layers = append(layers, lipgloss.NewLayer(searchCmp).X(1).Y(y))
		}
	}

	prCmp := m.prView.ViewCompletions()
	previewPos := m.ctx.PreviewCursorPosition()
	if prCmp != "" {
		y := m.ctx.ScreenHeight - common.FooterHeight - m.prView.InputBoxLineFromBottom() - common.InputBoxHeight - 6
		layers = append(layers, lipgloss.NewLayer(prCmp).X(previewPos.X+3).Y(y))
	}

	issueCmp := m.issueSidebar.ViewCompletions()
	if issueCmp != "" {
		y := m.ctx.ScreenHeight - common.FooterHeight - m.issueSidebar.InputBoxLineFromButton() - common.InputBoxHeight - 6
		layers = append(layers, lipgloss.NewLayer(issueCmp).X(previewPos.X+3).Y(y))
	}

	comp := lipgloss.NewCompositor(layers...)
	v.SetContent(comp.Render())

	return v
}

type initMsg struct {
	Config  config.Config
	RepoUrl string
}

// Message types for notification subject fetching
type notificationPRFetchedMsg struct {
	NotificationId   string
	PR               data.EnrichedPullRequestData
	LatestCommentUrl string
	Err              error
}

type notificationIssueFetchedMsg struct {
	NotificationId   string
	Issue            data.IssueData
	LatestCommentUrl string
	Err              error
}

func (m *Model) setCurrSectionId(newSectionId int) {
	m.currSectionId = newSectionId
	m.tabs.SetCurrSectionId(newSectionId)
}

func (m *Model) updateNotificationSections(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.notifications {
		if m.notifications[i] != nil {
			var cmd tea.Cmd
			m.notifications[i], cmd = m.notifications[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) markNotificationAsRead(notificationId string) {
	readStateMsg := notificationssection.UpdateNotificationReadStateMsg{
		Id:     notificationId,
		Unread: false,
	}
	m.updateNotificationSections(readStateMsg)
}

func (m *Model) onViewedRowChanged() tea.Cmd {
	m.prView.SetSummaryViewLess()
	m.prView.GoToFirstTab()
	sidebarCmd := m.syncSidebar()
	enrichCmd := m.prView.EnrichCurrRow()
	m.sidebar.ScrollToTop()
	m.notificationView.ResetSubject()
	keys.SetNotificationSubject(keys.NotificationSubjectNone)
	m.persistSelection()
	return tea.Batch(sidebarCmd, enrichCmd)
}

// persistSelection remembers the current section's selected item so it can be
// restored on the next launch of this instance. Best-effort; errors are ignored.
func (m *Model) persistSelection() {
	s := m.getCurrSection()
	if s == nil {
		return
	}
	url := ""
	if r := s.GetCurrRow(); r != nil {
		url = r.GetUrl()
	}
	_ = saveSelection(m.layoutStateKey, s.GetConfig().Title, url)
}

func (m *Model) onWindowSizeChanged(msg tea.WindowSizeMsg) {
	log.Info("window size changed", "width", msg.Width, "height", msg.Height)
	m.footer.SetWidth(msg.Width)
	m.ctx.ScreenWidth = msg.Width
	m.ctx.ScreenHeight = msg.Height
	if m.ctx.Config != nil {
		if m.ctx.Config.Defaults.Preview.Position == "auto" ||
			m.ctx.Config.Defaults.Preview.Position == "" {
			m.positionOverride = ""
		}
		m.syncMainContentDimensions()
		m.syncSidebar()
	}
}

func (m *Model) syncProgramContext() {
	for _, section := range m.getCurrentViewSections() {
		section.UpdateProgramContext(m.ctx)
	}
	m.tabs.UpdateProgramContext(m.ctx)
	m.footer.UpdateProgramContext(m.ctx)
	m.sidebar.UpdateProgramContext(m.ctx)
	m.prView.UpdateProgramContext(m.ctx)
	m.issueSidebar.UpdateProgramContext(m.ctx)
	m.branchSidebar.UpdateProgramContext(m.ctx)
	m.notificationView.UpdateProgramContext(m.ctx)
}

func (m *Model) updateSection(id int, sType string, msg tea.Msg) (cmd tea.Cmd) {
	var updatedSection section.Section
	switch sType {
	case reposection.SectionType:
		m.repo, cmd = m.repo.Update(msg)

	case notificationssection.SectionType:
		if id < len(m.notifications) && m.notifications[id] != nil {
			m.notifications[id], cmd = m.notifications[id].Update(msg)
		}

	case prssection.SectionType:
		updatedSection, cmd = m.prs[id].Update(msg)
		m.prs[id] = updatedSection
	case issuessection.SectionType:
		updatedSection, cmd = m.issues[id].Update(msg)
		m.issues[id] = updatedSection
	}

	currSection := m.getCurrSection()
	if currSection != nil && id == currSection.GetId() {
		if _, ok := msg.(prssection.SectionPullRequestsFetchedMsg); ok {
			cmd = m.onViewedRowChanged()
		}
	}

	return cmd
}

func (m *Model) updateRelevantSection(msg section.SectionMsg) (cmd tea.Cmd) {
	return m.updateSection(msg.Id, msg.Type, msg)
}

func (m *Model) updateCurrentSection(msg tea.Msg) (cmd tea.Cmd) {
	section := m.getCurrSection()
	if section == nil {
		return nil
	}
	return m.updateSection(section.GetId(), section.GetType(), msg)
}

const minTableWidthForRightPreview = 80

func (m *Model) resolvePreviewPosition() string {
	pos := m.ctx.Config.Defaults.Preview.Position
	if pos == "" {
		pos = "auto"
	}

	if m.positionOverride != "" {
		return m.positionOverride
	}

	if pos == "right" || pos == "bottom" {
		return pos
	}

	// auto: check if right mode would leave enough room for the main content
	w := m.ctx.Config.Defaults.Preview.Width
	if w > 0 && w < 1 {
		w *= float64(m.ctx.ScreenWidth)
	}
	previewWidth := min(int(w), m.ctx.ScreenWidth)
	tableWidth := m.ctx.ScreenWidth - previewWidth
	if tableWidth < minTableWidthForRightPreview {
		return "bottom"
	}
	return "right"
}

// mergeAction picks what the `m` key does for the given row: on a repo that uses
// the merge queue (defaults.mergeQueueRepos) it toggles the queue -- dequeue if
// the PR is already queued, else enqueue -- otherwise it's a plain merge. Any
// non-PR row falls back to "merge".
func (m *Model) mergeAction(row data.RowData) string {
	prd, ok := row.(*prrow.Data)
	if !ok || prd.Primary == nil {
		return "merge"
	}
	repo := prd.GetRepoNameWithOwner()
	if m.ctx.Config.Defaults.UsesMergeQueue(repo) {
		if prd.Primary.IsInMergeQueue {
			return "dequeue"
		}

		return "enqueue"
	}

	// No merge queue: merge with an explicit strategy so gh doesn't prompt for
	// the method and then again to submit. Ask only when we have no answer yet.
	if method := resolveMergeMethod(m.ctx.Config.Defaults, repo); method != "" {
		return "merge_" + method
	}

	return "merge_method"
}

// notificationMergeAction resolves what `m` does for the PR previewed from a
// notification, exactly as mergeAction resolves it in the PRs view -- whether a
// repo uses a merge queue, and which strategy it merges with, are facts about the
// repo, not about the pane the key was pressed in.
//
// The one difference: this view has no merge-method picker (its confirmation
// accepts only y/N), so "ask which strategy" degrades to gh's own interactive
// merge rather than a prompt nothing can answer.
func (m *Model) notificationMergeAction() string {
	pr := m.notificationView.GetSubjectPR()
	if pr == nil {
		return "merge"
	}

	if action := m.mergeAction(pr); action != "merge_method" {
		return action
	}

	return "merge"
}

// toggleMergedVisibility flips the `M` toggle and persists it for this instance.
//
// Hiding drops merged PRs from the loaded rows immediately (no refetch — the data
// is already here). Showing needs a fetch, because while hidden the sections skip
// the recently-merged query entirely.
func (m *Model) toggleMergedVisibility() tea.Cmd {
	m.ctx.HideMergedPRs = !m.ctx.HideMergedPRs
	if err := saveMergedHidden(m.layoutStateKey, m.ctx.HideMergedPRs); err != nil {
		log.Error("failed persisting merged visibility", "err", err)
	}
	m.syncProgramContext()

	if !m.ctx.HideMergedPRs {
		newSections, fetchCmd := m.fetchAllViewSections()
		m.setCurrentViewSections(newSections)

		return fetchCmd
	}

	for _, s := range m.prs {
		if prSection, ok := s.(*prssection.Model); ok {
			prSection.DropMergedRows()
		}
	}

	return nil
}

// resolveMergeMethod picks the merge strategy for a repo: what the config says
// (per-repo override, else the global default), else the choice remembered from
// the last successful merge of that repo. "" means "ask".
//
// GitHub exposes no per-repo default merge method -- only which methods are
// allowed -- so there is nothing to auto-detect unless a repo permits exactly
// one strategy. Configure it, or answer once and it's remembered machine-wide.
func resolveMergeMethod(d config.Defaults, repoNameWithOwner string) string {
	if method := d.ResolveMergeMethod(repoNameWithOwner); method != "" {
		return method
	}

	return prefs.LoadMergeMethod(repoNameWithOwner)
}

func (m *Model) getBaseContentHeight() int {
	if m.footer.ShowAll {
		// Measure actual footer height — the ExpandedHelpHeight constant
		// doesn't account for custom keybindings or view-specific bindings.
		footerHeight := lipgloss.Height(m.footer.View())
		return m.ctx.ScreenHeight - common.TabsHeight - footerHeight
	}
	return m.ctx.ScreenHeight - common.TabsHeight - common.FooterHeight
}

func (m *Model) syncMainContentDimensions() {
	m.ctx.PreviewPosition = m.resolvePreviewPosition()

	if !m.sidebar.IsOpen {
		m.ctx.MainContentWidth = m.ctx.ScreenWidth
		m.ctx.MainContentHeight = m.getBaseContentHeight()
		m.ctx.DynamicPreviewWidth = 0
		m.ctx.DynamicPreviewHeight = 0
		m.ctx.SidebarOpen = false
		return
	}

	m.ctx.SidebarOpen = true

	if m.ctx.PreviewPosition == "bottom" {
		m.ctx.MainContentWidth = m.ctx.ScreenWidth

		// Subtract border height: lipgloss Height() sets content height,
		// and BorderTop adds an extra row outside of that.
		availableHeight := m.getBaseContentHeight() - m.ctx.Styles.Sidebar.BorderWidth

		// Keep the preview between both floors: tall enough to render itself
		// (short-changing it pushes the status bar off the bottom) and short
		// enough to leave the list its minimum. Applies to a dragged override
		// and the configured default alike — a height remembered from a taller
		// terminal, or a percentage that lands too small here, must not break
		// either end.
		previewHeight := 0
		if m.previewHeightOverrideSet {
			previewHeight = m.previewHeightOverride
		} else {
			h := m.ctx.Config.Defaults.Preview.Height
			if h > 0 && h < 1 {
				h *= float64(availableHeight)
			}
			previewHeight = int(h)
		}

		m.ctx.DynamicPreviewHeight = clampPreviewHeight(previewHeight, availableHeight)
		m.ctx.MainContentHeight = availableHeight - m.ctx.DynamicPreviewHeight
		m.ctx.DynamicPreviewWidth = m.ctx.ScreenWidth
	} else {
		m.ctx.MainContentHeight = m.getBaseContentHeight()

		w := m.ctx.Config.Defaults.Preview.Width
		if w > 0 && w < 1 {
			w *= float64(m.ctx.ScreenWidth)
		}
		m.ctx.DynamicPreviewWidth = min(int(w), m.ctx.ScreenWidth)
		m.ctx.MainContentWidth = m.ctx.ScreenWidth - m.ctx.DynamicPreviewWidth
		m.ctx.DynamicPreviewHeight = 0
	}
}

func (m *Model) openSidebarForPRInput(setFunc func(bool) tea.Cmd) tea.Cmd {
	m.prView.GoToFirstTab()
	return m.openSidebarForInput(setFunc)
}

func (m *Model) openSidebarForInput(setFunc func(bool) tea.Cmd) tea.Cmd {
	m.sidebar.IsOpen = true
	cmd := setFunc(true)
	m.syncMainContentDimensions()
	m.syncSidebar()
	m.sidebar.ScrollToBottom()
	return cmd
}

func (m *Model) backToNotification() tea.Cmd {
	if m.notificationView.GetSubjectPR() == nil && m.notificationView.GetSubjectIssue() == nil {
		return nil
	}

	m.notificationView.ClearSubject()
	keys.SetNotificationSubject(keys.NotificationSubjectNone)
	m.sidebar.ScrollToTop()
	return m.syncSidebar()
}

// promptConfirmation opens a confirmation prompt for action. openKey is the key
// that triggered it, recorded so pressing that same key again confirms (press `m`
// to merge, `m` again to go through with it); pass "" when there is no such key.
func (m *Model) promptConfirmation(
	currSection section.Section,
	action string,
	openKey string,
) tea.Cmd {
	if currSection != nil {
		currSection.SetPromptConfirmationAction(action)
		currSection.SetPromptConfirmationKey(openKey)
		return currSection.SetIsPromptConfirmationShown(true)
	}
	return nil
}

func (m *Model) syncSidebar() tea.Cmd {
	if !m.sidebar.IsOpen {
		return nil
	}

	currRowData := m.getCurrRowData()
	width := m.sidebar.GetSidebarContentWidth()
	var cmd tea.Cmd

	if currRowData == nil {
		m.sidebar.SetContent("")
		return nil
	}

	switch row := currRowData.(type) {
	case branch.BranchData:
		cmd = m.branchSidebar.SetRow(&row)
		m.sidebar.SetContent(m.branchSidebar.View())
	case *prrow.Data:
		m.prView.SetSectionId(m.currSectionId)
		m.prView.SetRow(row)
		m.prView.SetWidth(width)
		m.sidebar.SetContent(m.prView.View())
		// Scroll to bottom if in input mode to keep inputbox visible
		if m.prView.IsTextInputBoxFocused() {
			m.sidebar.ScrollToBottom()
		}
	case *data.IssueData:
		m.issueSidebar.SetSectionId(m.currSectionId)
		m.issueSidebar.SetRow(row)
		m.issueSidebar.SetWidth(width)
		m.sidebar.SetContent(m.issueSidebar.View())
		// Scroll to bottom if in input mode to keep inputbox visible
		if m.issueSidebar.IsTextInputBoxFocused() {
			m.sidebar.ScrollToBottom()
		}
	case *notificationrow.Data:
		notifId := row.GetId()

		// Check if we already have cached data for this notification (user already viewed it)
		if m.notificationView.GetSubjectId() == notifId {
			// Use cached data
			if m.notificationView.GetSubjectPR() != nil {
				m.prView.SetSectionId(0)
				m.prView.SetRow(m.notificationView.GetSubjectPR())
				m.prView.SetWidth(width)
				m.sidebar.SetContent(m.prView.View())
				// Scroll to bottom if in input mode to keep inputbox visible
				if m.prView.IsTextInputBoxFocused() {
					m.sidebar.ScrollToBottom()
				}
			} else if m.notificationView.GetSubjectIssue() != nil {
				m.issueSidebar.SetSectionId(0)
				m.issueSidebar.SetRow(m.notificationView.GetSubjectIssue())
				m.issueSidebar.SetWidth(width)
				m.sidebar.SetContent(m.issueSidebar.View())
				// Scroll to bottom if in input mode to keep inputbox visible
				if m.issueSidebar.IsTextInputBoxFocused() {
					m.sidebar.ScrollToBottom()
				}
			}
			return nil
		}

		// Clear cached subject when navigating to a different notification
		// so key dispatch doesn't route keys to the wrong subject's handler.
		m.notificationView.ClearSubject()
		keys.SetNotificationSubject(keys.NotificationSubjectNone)
		// Show prompt to view notification (don't auto-fetch)
		// User must press Enter to view content and mark as read
		m.sidebar.SetContent(m.renderNotificationPrompt(row))
	}

	return cmd
}

func (m *Model) renderNotificationPrompt(row *notificationrow.Data) string {
	var content strings.Builder

	subjectType := row.GetSubjectType()
	leftMargin := "      " // Left margin for content

	// Styles
	normalText := lipgloss.NewStyle().Foreground(m.ctx.Theme.PrimaryText)
	faintText := lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText)
	// Highlighted key style for main prompt (with background)
	highlightKeyStyle := lipgloss.NewStyle().
		Foreground(m.ctx.Theme.PrimaryText).
		Background(m.ctx.Theme.FaintBorder).
		Padding(0, 1)
	// Simple key style for table (no background)
	keyStyle := lipgloss.NewStyle().
		Foreground(m.ctx.Theme.PrimaryText)
	actionStyle := lipgloss.NewStyle().Foreground(m.ctx.Theme.SuccessText)
	headerStyle := lipgloss.NewStyle().
		Foreground(m.ctx.Theme.PrimaryText).
		Bold(true)

	// Determine subject type display name and primary action
	typeName := "PR"
	enterAction := "view"
	if subjectType == "Issue" {
		typeName = "Issue"
	} else if subjectType != "PullRequest" {
		typeName = subjectType
		enterAction = "open in browser"
	}

	// Main prompt: "Press Enter to view the PR" or "Press Enter to open in browser"
	content.WriteString("\n")
	content.WriteString(leftMargin)
	content.WriteString(normalText.Render("Press "))
	content.WriteString(highlightKeyStyle.Render("Enter"))
	if enterAction == "view" {
		content.WriteString(normalText.Render(fmt.Sprintf(" to %s the %s", enterAction, typeName)))
	} else {
		content.WriteString(normalText.Render(fmt.Sprintf(" to %s", enterAction)))
	}
	content.WriteString("\n")

	// Note about marking as read
	content.WriteString(leftMargin)
	content.WriteString(faintText.Render("(Note: this will mark it as read)"))
	content.WriteString("\n")

	content.WriteString("\n")

	// Other Actions header
	content.WriteString(leftMargin)
	content.WriteString(headerStyle.Render("Other Actions"))
	content.WriteString("\n\n")

	// Key-action pairs (simple list without borders)
	actions := []struct {
		key    string
		action string
	}{
		{"D", "mark as done"},
		{"m", "mark as read"},
		{"u", "unsubscribe"},
		{"b", "toggle bookmark"},
		{"t", "toggle filtering"},
		{"S", "sort by repo"},
		{"o", "open in browser"},
	}

	keyWidth := 7 // Width for key column
	for _, a := range actions {
		content.WriteString(leftMargin)
		// Right-align the key in its column
		padding := strings.Repeat(" ", keyWidth-len(a.key))
		content.WriteString(padding)
		content.WriteString(keyStyle.Render(a.key))
		content.WriteString("  ")
		content.WriteString(actionStyle.Render(a.action))
		content.WriteString("\n")
	}

	// Add Enter and Esc at the end
	content.WriteString(leftMargin)
	padding := strings.Repeat(" ", keyWidth-len("Enter"))
	content.WriteString(padding)
	content.WriteString(keyStyle.Render("Enter"))
	content.WriteString("  ")
	content.WriteString(actionStyle.Render(enterAction))
	content.WriteString("\n")
	content.WriteString(leftMargin)
	escPadding := strings.Repeat(" ", keyWidth-len("Esc"))
	content.WriteString(escPadding)
	content.WriteString(keyStyle.Render("Esc"))
	content.WriteString("  ")
	content.WriteString(actionStyle.Render("go back"))

	return content.String()
}

// loadNotificationContent fetches and displays notification content, marking it as read
func (m *Model) loadNotificationContent() tea.Cmd {
	currRowData := m.getCurrRowData()
	row, ok := currRowData.(*notificationrow.Data)
	if !ok || row == nil {
		return nil
	}

	notifId := row.GetId()
	subjectType := row.GetSubjectType()
	subjectUrl := row.GetUrl()
	latestCommentUrl := row.GetLatestCommentUrl()

	// Show loading indicator
	width := m.sidebar.GetSidebarContentWidth()
	m.notificationView.SetRow(row)
	m.notificationView.SetWidth(width)
	m.sidebar.SetContent(m.notificationView.View())

	switch subjectType {
	case "PullRequest":
		return tea.Batch(
			func() tea.Msg {
				_ = data.MarkNotificationRead(notifId)
				return notificationssection.UpdateNotificationReadStateMsg{
					Id:     notifId,
					Unread: false,
				}
			},
			func() tea.Msg {
				pr, err := data.FetchPullRequest(subjectUrl)
				return notificationPRFetchedMsg{
					NotificationId:   notifId,
					PR:               pr,
					LatestCommentUrl: latestCommentUrl,
					Err:              err,
				}
			},
		)
	case "Issue":
		return tea.Batch(
			func() tea.Msg {
				_ = data.MarkNotificationRead(notifId)
				return notificationssection.UpdateNotificationReadStateMsg{
					Id:     notifId,
					Unread: false,
				}
			},
			func() tea.Msg {
				issue, err := data.FetchIssue(subjectUrl)
				return notificationIssueFetchedMsg{
					NotificationId:   notifId,
					Issue:            issue,
					LatestCommentUrl: latestCommentUrl,
					Err:              err,
				}
			},
		)
	default:
		// For discussions, releases, etc. - mark as read and open in browser
		// since we can't show rich content for these types
		return tea.Batch(
			func() tea.Msg {
				_ = data.MarkNotificationRead(notifId)
				return notificationssection.UpdateNotificationReadStateMsg{
					Id:     notifId,
					Unread: false,
				}
			},
			m.openBrowser(),
		)
	}
}

func (m *Model) fetchAllViewSections() ([]section.Section, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)
	cmds = append(cmds, m.tabs.SetAllLoading()...)

	switch m.ctx.View {
	case config.RepoView:
		var cmd tea.Cmd
		s, cmd := reposection.FetchAllBranches(m.ctx)
		cmds = append(cmds, cmd)
		m.repo = &s
		return nil, tea.Batch(cmds...)
	case config.NotificationsView:
		s, notifCmd := notificationssection.FetchAllSections(m.ctx, m.notifications)
		cmds = append(cmds, notifCmd)
		m.notifications = s
		return s, tea.Batch(cmds...)
	case config.PRsView:
		s, prcmds := prssection.FetchAllSections(m.ctx, m.prs)
		cmds = append(cmds, prcmds)
		return s, tea.Batch(cmds...)
	default:
		s, issuecmds := issuessection.FetchAllSections(m.ctx, m.issues)
		cmds = append(cmds, issuecmds)
		return s, tea.Batch(cmds...)
	}
}

func (m *Model) getCurrentViewSections() []section.Section {
	switch m.ctx.View {
	case config.RepoView:
		if m.repo == nil {
			return []section.Section{}
		}
		return []section.Section{m.repo}
	case config.NotificationsView:
		if len(m.notifications) == 0 {
			return []section.Section{}
		}
		return m.notifications
	case config.PRsView:
		return m.prs
	default:
		return m.issues
	}
}

func (m *Model) getCurrentViewDefaultSection() int {
	switch m.ctx.View {
	case config.RepoView:
		return 0
	case config.NotificationsView:
		return 1 // First notification section after search section
	case config.PRsView:
		return 1
	default:
		return 1
	}
}

func (m *Model) setCurrentViewSections(newSections []section.Section) {
	if newSections == nil {
		return
	}

	// Handle notifications view with search section like PRs/Issues
	if m.ctx.View == config.NotificationsView {
		missingSearchSection := len(newSections) == 0 ||
			(len(newSections) > 0 && newSections[0].GetId() != 0)
		s := make([]section.Section, 0)
		if missingSearchSection {
			// Check if we have an existing search section to preserve
			if len(m.notifications) > 0 && m.notifications[0] != nil &&
				m.notifications[0].GetId() == 0 {
				// Preserve existing search section with its filter state
				s = append(s, m.notifications[0])
			} else {
				// Create new search section only if none exists
				search := notificationssection.NewModel(
					0,
					m.ctx,
					config.NotificationsSectionConfig{
						Title:   "",
						Filters: "archived:false",
					},
					time.Now(),
				)
				s = append(s, &search)
			}
		}
		m.notifications = append(s, newSections...)
		m.tabs.SetSections(m.notifications)
		return
	}

	missingSearchSection := len(newSections) == 0 ||
		(len(newSections) > 0 && newSections[0].GetId() != 0)
	s := make([]section.Section, 0)
	if m.ctx.View == config.PRsView {
		if missingSearchSection {
			search := prssection.NewModel(
				0,
				m.ctx,
				config.PrsSectionConfig{
					Title:   "",
					Filters: "archived:false",
				},
				time.Now(),
				time.Now(),
			)
			s = append(s, &search)
		}
		m.prs = append(s, newSections...)
		newSections = m.prs
	} else {
		if missingSearchSection {
			search := issuessection.NewModel(
				0,
				m.ctx,
				config.IssuesSectionConfig{
					Title:   "",
					Filters: "",
				},
				time.Now(),
				time.Now(),
			)
			s = append(s, &search)
		}
		m.issues = append(s, newSections...)
		newSections = m.issues
	}

	m.tabs.SetSections(newSections)
}

func (m *Model) switchSelectedView() tea.Cmd {
	repoFF := config.IsFeatureEnabled(config.FF_REPO_VIEW)

	// Reset notification subject when leaving notifications view
	if m.ctx.View == config.NotificationsView {
		keys.SetNotificationSubject(keys.NotificationSubjectNone)
		m.notificationView.ClearSubject()
	}

	// View cycle: Notifications → PRs → Issues (→ Repo if enabled) → Notifications
	if repoFF {
		switch m.ctx.View {
		case config.NotificationsView:
			m.ctx.View = config.PRsView
		case config.PRsView:
			m.ctx.View = config.IssuesView
		case config.IssuesView:
			m.ctx.View = config.RepoView
		case config.RepoView:
			m.ctx.View = config.NotificationsView
		}
	} else {
		switch m.ctx.View {
		case config.NotificationsView:
			m.ctx.View = config.PRsView
		case config.PRsView:
			m.ctx.View = config.IssuesView
		default:
			m.ctx.View = config.NotificationsView
		}
	}

	return m.applyViewChange()
}

// setSelectedView jumps straight to a view (from a footer click) rather than
// cycling. A click on the already-active view is a no-op.
func (m *Model) setSelectedView(target config.ViewType) tea.Cmd {
	if m.ctx.View == target {
		return nil
	}
	if m.ctx.View == config.NotificationsView {
		keys.SetNotificationSubject(keys.NotificationSubjectNone)
		m.notificationView.ClearSubject()
	}
	m.ctx.View = target

	return m.applyViewChange()
}

// applyViewChange resyncs sections and layout after m.ctx.View has been set,
// and is shared by the cycle (keyboard) and the direct jump (footer click).
func (m *Model) applyViewChange() tea.Cmd {
	m.syncMainContentDimensions()
	m.setCurrSectionId(m.getCurrentViewDefaultSection())

	var cmds []tea.Cmd
	currSections := m.getCurrentViewSections()
	if len(currSections) == 0 {
		newSections, fetchSectionsCmds := m.fetchAllViewSections()
		currSections = newSections
		cmds = append(cmds, m.tabs.SetAllLoading()...)
		cmds = append(cmds, fetchSectionsCmds)
	}
	m.setCurrentViewSections(currSections)
	cmds = append(cmds, m.onViewedRowChanged())

	return tea.Batch(cmds...)
}

func (m *Model) isUserDefinedKeybinding(msg tea.KeyMsg) bool {
	if m.ctx == nil || m.ctx.Config == nil {
		return false
	}
	for _, keybinding := range m.ctx.Config.Keybindings.Universal {
		if keybinding.Builtin == "" && keybinding.Key == msg.String() {
			return true
		}
	}

	if m.ctx.View == config.IssuesView {
		for _, keybinding := range m.ctx.Config.Keybindings.Issues {
			if keybinding.Builtin == "" && keybinding.Key == msg.String() {
				return true
			}
		}
	}

	if m.ctx.View == config.PRsView {
		for _, keybinding := range m.ctx.Config.Keybindings.Prs {
			if keybinding.Builtin == "" && keybinding.Key == msg.String() {
				return true
			}
		}
	}

	if m.ctx.View == config.RepoView {
		for _, keybinding := range m.ctx.Config.Keybindings.Branches {
			if keybinding.Builtin == "" && keybinding.Key == msg.String() {
				return true
			}
		}
	}

	if m.ctx.View == config.NotificationsView {
		for _, keybinding := range m.ctx.Config.Keybindings.Notifications {
			if keybinding.Builtin == "" && keybinding.Key == msg.String() {
				return true
			}
		}

		currRowData := m.getCurrRowData()
		if nData, ok := currRowData.(*notificationrow.Data); ok {
			switch nData.Notification.Subject.Type {
			case "PullRequest":
				for _, keybinding := range m.ctx.Config.Keybindings.Prs {
					if keybinding.Builtin == "" && keybinding.Key == msg.String() {
						return true
					}
				}
			case "Issue":
				for _, keybinding := range m.ctx.Config.Keybindings.Issues {
					if keybinding.Builtin == "" && keybinding.Key == msg.String() {
						return true
					}
				}
			}
		}
	}

	return false
}

func (m *Model) renderRunningTask() string {
	tasks := make([]context.Task, 0, len(m.tasks))
	for _, value := range m.tasks {
		tasks = append(tasks, value)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].FinishedTime != nil && tasks[j].FinishedTime == nil {
			return false
		}
		if tasks[j].FinishedTime != nil && tasks[i].FinishedTime == nil {
			return true
		}
		if tasks[j].FinishedTime != nil && tasks[i].FinishedTime != nil {
			return tasks[i].FinishedTime.After(*tasks[j].FinishedTime)
		}

		return tasks[i].StartTime.After(tasks[j].StartTime)
	})
	task := tasks[0]

	var currTaskStatus string
	switch task.State {
	case context.TaskStart:
		currTaskStatus = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.taskSpinner.View(),
			lipgloss.NewStyle().
				Background(m.ctx.Theme.SelectedBackground).Render(task.StartText),
		)
	case context.TaskError:
		currTaskStatus = lipgloss.NewStyle().
			Foreground(m.ctx.Theme.ErrorText).
			Background(m.ctx.Theme.SelectedBackground).
			Render(fmt.Sprintf("%s %s", constants.FailureIcon, task.Error.Error()))
	case context.TaskFinished:
		currTaskStatus = lipgloss.NewStyle().
			Foreground(m.ctx.Theme.SuccessText).
			Background(m.ctx.Theme.SelectedBackground).
			Render(fmt.Sprintf("%s %s", constants.SuccessIcon, task.FinishedText))
	}

	var numProcessing int
	for _, task := range m.tasks {
		if task.State == context.TaskStart {
			numProcessing += 1
		}
	}

	stats := ""
	if numProcessing > 1 {
		stats = lipgloss.NewStyle().
			Foreground(m.ctx.Theme.FaintText).
			Background(m.ctx.Theme.SelectedBackground).
			Render(fmt.Sprintf("[ %d] ", numProcessing))
	}

	return lipgloss.NewStyle().
		Padding(0, 1).
		Height(1).
		Background(m.ctx.Theme.SelectedBackground).
		Render(strings.TrimSpace(lipgloss.JoinHorizontal(lipgloss.Top, stats, currTaskStatus)))
}

type userFetchedMsg struct {
	user string
}

func fetchUser() tea.Msg {
	user, err := data.CurrentLoginName()
	if err != nil {
		return constants.ErrMsg{
			Err: err,
		}
	}

	return userFetchedMsg{
		user: user,
	}
}

type intervalRefresh time.Time

func (m *Model) doRefreshAtInterval() tea.Cmd {
	secs := m.ctx.Config.Defaults.EffectiveRefetchSeconds()
	if secs <= 0 {
		return nil
	}

	return tea.Tick(
		time.Duration(secs)*time.Second,
		func(t time.Time) tea.Msg {
			return intervalRefresh(t)
		},
	)
}

type updateFooterMsg struct{}

func (m *Model) doUpdateFooterAtInterval() tea.Cmd {
	return tea.Tick(
		time.Second*10,
		func(t time.Time) tea.Msg {
			return updateFooterMsg{}
		},
	)
}

// promptConfirmationForNotificationPR shows a confirmation prompt for PR actions
// when viewing a PR from a notification. This is separate from section-based
// confirmation because the notification section doesn't know about PR actions.
func (m *Model) promptConfirmationForNotificationPR(action, openKey string) tea.Cmd {
	prompt := m.notificationView.SetPendingPRAction(action)
	m.notificationView.SetPendingKey(openKey)
	if prompt == "" {
		return nil
	}
	m.footer.SetLeftSection(m.ctx.Styles.ListViewPort.PagerStyle.Render(prompt))
	return nil
}

// promptConfirmationForNotificationIssue shows a confirmation prompt for Issue actions
// when viewing an Issue from a notification.
func (m *Model) promptConfirmationForNotificationIssue(action, openKey string) tea.Cmd {
	prompt := m.notificationView.SetPendingIssueAction(action)
	m.notificationView.SetPendingKey(openKey)
	if prompt == "" {
		return nil
	}
	m.footer.SetLeftSection(m.ctx.Styles.ListViewPort.PagerStyle.Render(prompt))
	return nil
}

// executeNotificationAction executes a PR/Issue action after user confirmation
func (m *Model) executeNotificationAction(action string) tea.Cmd {
	if action == "" {
		return nil
	}

	sid := tasks.SectionIdentifier{Id: m.currSectionId, Type: notificationssection.SectionType}
	pr := m.notificationView.GetSubjectPR()
	issue := m.notificationView.GetSubjectIssue()

	switch action {
	case "pr_close":
		if pr != nil {
			return tasks.ClosePR(m.ctx, sid, pr)
		}
	case "pr_reopen":
		if pr != nil {
			return tasks.ReopenPR(m.ctx, sid, pr)
		}
	case "pr_ready":
		if pr != nil {
			return tasks.PRReady(m.ctx, sid, pr)
		}
	case "pr_merge":
		if pr != nil {
			return tasks.MergePR(m.ctx, sid, pr)
		}
	case "pr_merge_squash", "pr_merge_merge", "pr_merge_rebase":
		if pr != nil {
			return tasks.MergePRWithMethod(
				m.ctx, sid, pr, strings.TrimPrefix(action, "pr_merge_"))
		}
	case "pr_enqueue":
		if pr != nil && pr.Primary != nil {
			return tasks.EnqueuePR(m.ctx, sid, pr.Primary.Number, pr.Primary.Id)
		}
	case "pr_dequeue":
		if pr != nil && pr.Primary != nil {
			return tasks.DequeuePR(m.ctx, sid, pr.Primary.Number, pr.Primary.Id)
		}
	case "pr_update":
		if pr != nil {
			return tasks.UpdatePR(m.ctx, sid, pr)
		}
	case "pr_approveWorkflows":
		if pr != nil {
			return tasks.ApproveWorkflows(m.ctx, sid, pr)
		}
	case "issue_close":
		if issue != nil {
			return tasks.CloseIssue(m.ctx, sid, issue)
		}
	case "issue_reopen":
		if issue != nil {
			return tasks.ReopenIssue(m.ctx, sid, issue)
		}
	}

	return nil
}
