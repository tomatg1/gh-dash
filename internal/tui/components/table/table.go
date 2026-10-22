package table

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/common"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/listviewport"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

type Model struct {
	ctx            context.ProgramContext
	Columns        []Column
	Rows           []Row
	EmptyState     *string
	loadingMessage string
	isLoading      bool
	loadingSpinner spinner.Model
	dimensions     constants.Dimensions
	rowsViewport   listviewport.Model
	ContentHeight  int // Optional: override content height (0 = use default from config)

	// Render cache. rendered[i] is row i as last rendered, and a row is only
	// re-rendered when its cells change or its selected-ness flips. Rendering
	// every row used to happen on every cursor move and on every message (the
	// program context is re-synced into every tab's table per message), and it
	// was the single largest cost in the event loop.
	rendered    []string
	renderedSel int    // cursor row the cache was rendered with; -1 = none
	renderedKey string // what EVERY row's rendering depends on; see renderKey
	dirty       []bool // rows whose cells changed since they were rendered
	synced      bool   // the viewport has been given content at least once

	// Last header and last full view, kept because View runs every frame and
	// its output rarely changes. Behind a pointer: View has a value receiver.
	viewMemo *tableViewMemo
}

type tableViewMemo struct {
	headerKey string
	header    string
	bodyIn    string
	headerIn  string
	out       string
	ok        bool
}

type Column struct {
	Title         string
	Hidden        *bool
	Width         *int
	ComputedWidth int
	Grow          *bool
	Align         *lipgloss.Position // Optional alignment (defaults to left)
}

type Row []string

func NewModel(
	ctx context.ProgramContext,
	dimensions constants.Dimensions,
	lastUpdated time.Time,
	createdAt time.Time,
	columns []Column,
	rows []Row,
	itemTypeLabel string,
	emptyState *string,
	loadingMessage string,
	isLoading bool,
) Model {
	itemHeight := 1
	if !ctx.Config.Theme.Ui.Table.Compact {
		itemHeight += 1
	}
	if ctx.Config.Theme.Ui.Table.ShowSeparator {
		itemHeight += 1
	}

	loadingSpinner := spinner.New()
	loadingSpinner.Spinner = spinner.Dot
	loadingSpinner.Style = lipgloss.NewStyle().Foreground(ctx.Theme.SecondaryText)

	return Model{
		viewMemo:       &tableViewMemo{},
		renderedSel:    -1,
		ctx:            ctx,
		Columns:        columns,
		Rows:           rows,
		EmptyState:     emptyState,
		loadingMessage: loadingMessage,
		isLoading:      isLoading,
		loadingSpinner: loadingSpinner,
		dimensions:     dimensions,
		rowsViewport: listviewport.NewModel(
			ctx,
			dimensions,
			lastUpdated,
			createdAt,
			itemTypeLabel,
			len(rows),
			itemHeight,
		),
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.isLoading {
		m.loadingSpinner, cmd = m.loadingSpinner.Update(msg)
	}
	return m, cmd
}

func (m Model) StartLoadingSpinner() tea.Cmd {
	return m.loadingSpinner.Tick
}

func (m Model) View() string {
	header := m.cachedHeader()
	body := m.renderBody()
	if mm := m.viewMemo; mm != nil && mm.ok && mm.headerIn == header && mm.bodyIn == body {
		return mm.out
	}

	out := lipgloss.JoinVertical(lipgloss.Left, header, body)
	if mm := m.viewMemo; mm != nil {
		mm.headerIn, mm.bodyIn, mm.out, mm.ok = header, body, out, true
	}

	return out
}

// cachedHeader renders the header only when something it depends on changed:
// the width, the theme's config, or the columns.
func (m *Model) cachedHeader() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%p|", m.dimensions.Width, m.ctx.Config)
	for _, c := range m.Columns {
		fmt.Fprintf(&b, "%s|%v|%v|%v|%v;", c.Title, deref(c.Hidden), derefInt(c.Width), deref(c.Grow), c.Align != nil && *c.Align == lipgloss.Right)
	}
	key := b.String()

	mm := m.viewMemo
	if mm != nil && mm.headerKey == key && mm.header != "" {
		return mm.header
	}
	header := m.renderHeader()
	if mm != nil {
		mm.headerKey, mm.header = key, header
	}

	return header
}

func deref(b *bool) bool {
	return b != nil && *b
}

func derefInt(n *int) int {
	if n == nil {
		return -1
	}

	return *n
}

func (m *Model) SetIsLoading(isLoading bool) {
	m.isLoading = isLoading
}

func (m *Model) SetDimensions(dimensions constants.Dimensions) {
	m.dimensions = dimensions
	m.rowsViewport.SetDimensions(constants.Dimensions{
		Width:  m.dimensions.Width,
		Height: m.dimensions.Height,
	})
}

func (m *Model) ResetCurrItem() {
	m.rowsViewport.ResetCurrItem()
}

func (m *Model) GetCurrItem() int {
	return m.rowsViewport.GetCurrItem()
}

func (m *Model) PrevItem() int {
	currItem := m.rowsViewport.PrevItem()
	m.SyncViewPortContent()

	return currItem
}

func (m *Model) NextItem() int {
	currItem := m.rowsViewport.NextItem()
	m.SyncViewPortContent()

	return currItem
}

func (m *Model) FirstItem() int {
	currItem := m.rowsViewport.FirstItem()
	m.SyncViewPortContent()

	return currItem
}

func (m *Model) LastItem() int {
	currItem := m.rowsViewport.LastItem()
	m.SyncViewPortContent()

	return currItem
}

// RowZoneID is the bubblezone id for a table row. It lives here so the renderer
// that marks the zone and the click handler that reads it cannot drift apart.
func RowZoneID(rowID int) string {
	return fmt.Sprintf("row-%d", rowID)
}

// ScrollState is where the rows are scrolled to; see listviewport.ScrollState.
func (m *Model) ScrollState() (top, curr int) {
	return m.rowsViewport.ScrollState()
}

// RestoreScroll restores a ScrollState taken from the table this one replaces,
// so the selected row stays on the same screen line across a rebuild.
func (m *Model) RestoreScroll(top, curr int) {
	m.rowsViewport.RestoreScroll(top, curr)
	m.SyncViewPortContent()
}

// SetCurrItem selects an absolute row index. Used by mouse clicks.
func (m *Model) SetCurrItem(id int) int {
	currItem := m.rowsViewport.SetCurrItem(id)
	m.SyncViewPortContent()

	return currItem
}

func (m *Model) cacheColumnWidths(columns []string) {
	for i, col := range columns {
		if m.Columns[i].Hidden != nil && *m.Columns[i].Hidden {
			continue
		}
		m.Columns[i].ComputedWidth = lipgloss.Width(col)
	}
}

// renderKey captures everything every row's rendering depends on: the header
// columns (titles, computed widths, hidden columns -- so width and layout), the
// column alignment, the table's height mode and separator, and the config the
// theme came from. When it changes, every row is re-rendered.
func (m *Model) renderKey(headerColumns []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%t|%t|%p|", m.dimensions.Width, m.ContentHeight,
		m.ctx.Config.Theme.Ui.Table.Compact, m.ctx.Config.Theme.Ui.Table.ShowSeparator, m.ctx.Config)
	for i, c := range m.Columns {
		if c.Align != nil {
			fmt.Fprintf(&b, "a%d=%v|", i, *c.Align)
		}
	}
	b.WriteString(strings.Join(headerColumns, "\x00"))

	return b.String()
}

// SyncViewPortContent brings the viewport's content up to date, re-rendering
// only the rows that need it: those whose cells changed, the previous and the
// new cursor row, or all of them when something every row depends on changed.
// When nothing changed it does no rendering at all.
func (m *Model) SyncViewPortContent() {
	headerColumns := m.renderHeaderColumns()
	m.cacheColumnWidths(headerColumns)

	all := false
	if key := m.renderKey(headerColumns); key != m.renderedKey {
		m.renderedKey = key
		all = true
	}
	if len(m.rendered) != len(m.Rows) {
		m.rendered = resizeStrings(m.rendered, len(m.Rows))
		m.dirty = resizeDirty(m.dirty, len(m.Rows))
		all = true
	}

	curr := m.rowsViewport.GetCurrItem()
	changed := false
	for i := range m.Rows {
		if all || m.dirty[i] || (curr != m.renderedSel && (i == curr || i == m.renderedSel)) {
			m.rendered[i] = m.renderRow(i, headerColumns)
			m.dirty[i] = false
			changed = true
		}
	}
	m.renderedSel = curr

	if changed || !m.synced {
		m.synced = true
		m.rowsViewport.SyncViewPort(lipgloss.JoinVertical(lipgloss.Left, m.rendered...))
	}
}

// SetRows replaces the rows, marking only those whose cells actually differ for
// re-rendering -- callers rebuild all rows freely, and most of them are unchanged.
func (m *Model) SetRows(rows []Row) {
	m.dirty = resizeDirty(m.dirty, len(rows))
	for i := range rows {
		if i >= len(m.Rows) || !slices.Equal(m.Rows[i], rows[i]) {
			m.dirty[i] = true
		}
	}
	m.Rows = rows
	m.rowsViewport.SetNumItems(len(m.Rows))
	m.SyncViewPortContent()
}

// SetRow replaces one row's cells, re-rendering only that row.
func (m *Model) SetRow(i int, row Row) {
	if i < 0 || i >= len(m.Rows) {
		return
	}
	m.Rows[i] = row
	m.dirty = resizeDirty(m.dirty, len(m.Rows))
	m.dirty[i] = true
	m.SyncViewPortContent()
}

func resizeStrings(s []string, n int) []string {
	if n <= cap(s) {
		return s[:n]
	}

	return append(s, make([]string, n-len(s))...)
}

func resizeDirty(d []bool, n int) []bool {
	if n <= len(d) {
		return d[:n]
	}

	return append(d, make([]bool, n-len(d))...)
}

// SetContentHeight sets a custom content height for rows (use 0 to use default config-based height)
func (m *Model) SetContentHeight(height int) {
	m.ContentHeight = height
	// Recalculate item height for viewport
	itemHeight := height
	if m.ctx.Config.Theme.Ui.Table.ShowSeparator {
		itemHeight += 1
	}
	m.rowsViewport.SetItemHeight(itemHeight)
}

func (m *Model) OnLineDown() {
	m.rowsViewport.NextItem()
}

func (m *Model) OnLineUp() {
	m.rowsViewport.PrevItem()
}

func (m *Model) getShownColumns() []Column {
	shownColumns := make([]Column, 0, len(m.Columns))
	for _, col := range m.Columns {
		if col.Hidden != nil && *col.Hidden {
			continue
		}

		shownColumns = append(shownColumns, col)
	}
	return shownColumns
}

func (m *Model) renderHeaderColumns() []string {
	shownColumns := m.getShownColumns()
	renderedColumns := make([]string, len(shownColumns))
	takenWidth := 0
	numGrowingColumns := 0
	for i, column := range shownColumns {
		if column.Grow != nil && *column.Grow {
			numGrowingColumns += 1
			continue
		}

		if column.Width != nil {
			title := column.Title
			style := m.ctx.Styles.Table.TitleCellStyle
			// Align headers for right-aligned columns
			// Use padding-free style to avoid truncation from PlaceHorizontal + padding
			if column.Align != nil && *column.Align == lipgloss.Right {
				// Center short headers (icons), right-align longer headers (text)
				headerAlign := lipgloss.Right
				if lipgloss.Width(title) <= 2 {
					headerAlign = lipgloss.Center
				}
				title = lipgloss.PlaceHorizontal(*column.Width, headerAlign, title)
				style = lipgloss.NewStyle().Bold(true).Foreground(m.ctx.Theme.PrimaryText)
			}
			renderedColumns[i] = style.
				Width(*column.Width).
				MaxWidth(*column.Width).
				Render(title)
			takenWidth += *column.Width
			continue
		}

		cell := m.ctx.Styles.Table.TitleCellStyle.Render(column.Title)
		renderedColumns[i] = cell
		takenWidth += lipgloss.Width(cell)
	}

	if numGrowingColumns == 0 {
		return renderedColumns
	}

	leftoverWidth := m.dimensions.Width - takenWidth
	growCellWidth := leftoverWidth / numGrowingColumns
	for i, column := range shownColumns {
		if column.Grow == nil || !*column.Grow {
			continue
		}

		renderedColumns[i] = m.ctx.Styles.Table.TitleCellStyle.
			Width(growCellWidth).
			MaxWidth(growCellWidth).
			Render(column.Title)
	}

	return renderedColumns
}

func (m *Model) renderHeader() string {
	headerColumns := m.renderHeaderColumns()
	header := ansi.Truncate(
		lipgloss.JoinHorizontal(lipgloss.Top, headerColumns...),
		m.dimensions.Width,
		constants.Ellipsis,
	)
	return m.ctx.Styles.Table.HeaderStyle.
		Width(m.dimensions.Width).
		Height(common.TableHeaderHeight).
		Render(header)
}

func (m *Model) renderBody() string {
	bodyStyle := lipgloss.NewStyle().
		Height(m.dimensions.Height).
		Width(m.dimensions.Width)

	if m.isLoading {
		return lipgloss.Place(
			m.dimensions.Width,
			m.dimensions.Height,
			lipgloss.Center,
			lipgloss.Center,
			fmt.Sprintf("%s%s", m.loadingSpinner.View(), m.loadingMessage),
		)
	}

	if len(m.Rows) == 0 && m.EmptyState != nil {
		return bodyStyle.Render(*m.EmptyState)
	}

	return m.rowsViewport.View()
}

func (m *Model) renderRow(rowId int, headerColumns []string) string {
	var style lipgloss.Style

	if m.rowsViewport.GetCurrItem() == rowId {
		style = m.ctx.Styles.Table.SelectedCellStyle
	} else {
		style = m.ctx.Styles.Table.CellStyle
	}

	renderedColumns := make([]string, 0, len(m.Columns))
	headerColId := 0

	for i, column := range m.Columns {
		if column.Hidden != nil && *column.Hidden {
			continue
		}

		colWidth := lipgloss.Width(headerColumns[headerColId])
		colHeight := 1
		if m.ContentHeight > 0 {
			// Use custom content height if set
			colHeight = m.ContentHeight
		} else if !m.ctx.Config.Theme.Ui.Table.Compact {
			colHeight = 2
		}
		col := m.Rows[rowId][i]
		// For multi-line content, truncate long lines and pad short lines
		// so lines don't wrap and background color extends properly
		// Account for cell padding (1 left + 1 right = 2)
		contentWidth := max(colWidth-2, 1)
		var renderedCol string
		if strings.Contains(col, "\n") {
			// For multi-line content, apply cell style to each line individually
			// This ensures background color extends properly despite ANSI resets in content
			lines := strings.Split(col, "\n")
			renderedLines := make([]string, len(lines))
			for j, line := range lines {
				lineWidth := lipgloss.Width(line)
				if lineWidth > contentWidth {
					line = ansi.Truncate(line, contentWidth, constants.Ellipsis)
				}
				lineStyle := style.Width(colWidth).MaxWidth(colWidth).Height(1)
				if column.Align != nil {
					lineStyle = lineStyle.Align(*column.Align)
				}
				renderedLines[j] = lineStyle.Render(line)
			}
			renderedCol = strings.Join(renderedLines, "\n")
		} else {
			cellStyle := style.
				Width(colWidth).
				MaxWidth(colWidth).
				Height(colHeight).
				MaxHeight(colHeight)
			if column.Align != nil {
				cellStyle = cellStyle.Align(*column.Align)
			}
			renderedCol = cellStyle.Render(col)
		}

		renderedColumns = append(renderedColumns, renderedCol)
		headerColId++
	}

	// Mark outside the style, not inside: MaxWidth truncates, and a zone marker
	// sliced in half would never be scanned back out.
	return common.MarkZone(RowZoneID(rowId), m.ctx.Styles.Table.RowStyle.
		BorderBottom(m.ctx.Config.Theme.Ui.Table.ShowSeparator).
		MaxWidth(m.dimensions.Width).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, renderedColumns...)))
}

func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.ctx = *ctx
	m.rowsViewport.UpdateProgramContext(ctx)
}

func (m *Model) LastUpdated() time.Time {
	return m.rowsViewport.LastUpdated
}

func (m *Model) CreatedAt() time.Time {
	return m.rowsViewport.CreatedAt
}

func (m *Model) UpdateLastUpdated(t time.Time) {
	m.rowsViewport.LastUpdated = t
}

func (m *Model) UpdateTotalItemsCount(count int) {
	m.rowsViewport.SetTotalItems(count)
}

func (m *Model) IsLoading() bool {
	return m.isLoading
}
