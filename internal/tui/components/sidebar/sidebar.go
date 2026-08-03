package sidebar

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/keys"
)

type Model struct {
	IsOpen     bool
	data       string
	viewport   viewport.Model
	ctx        *context.ProgramContext
	emptyState string
}

func NewModel() Model {
	vp := viewport.New(
		viewport.WithWidth(0),
		viewport.WithHeight(0),
	)

	return Model{
		IsOpen:     false,
		data:       "",
		viewport:   vp,
		ctx:        nil,
		emptyState: "Nothing selected...",
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Keys.PageDown):
			m.viewport.HalfPageDown()

		case key.Matches(msg, keys.Keys.PageUp):
			m.viewport.HalfPageUp()
		}
	}

	return m, nil
}

func (m Model) View() string {
	if !m.IsOpen {
		return ""
	}

	if m.ctx.PreviewPosition == "bottom" {
		height := max(0, m.ctx.DynamicPreviewHeight)
		width := m.ctx.DynamicPreviewWidth
		style := m.ctx.Styles.Sidebar.BottomRoot.
			Height(height).
			Width(width)

		// Fully collapsed: the pane is nothing but the divider line, so dragging
		// the divider to the bottom hides the preview's title and pager and gives
		// every other row to the list. The border is drawn directly rather than via
		// the style, because rendering empty content through it still emits a blank
		// content row — one row more than budgeted, which is exactly what pushes
		// the status bar off the screen.
		if height == 0 {
			return lipgloss.NewStyle().
				Foreground(m.ctx.Theme.PrimaryBorder).
				Render(strings.Repeat(lipgloss.ThickBorder().Top, max(0, width)))
		}

		if m.data == "" {
			return style.Align(lipgloss.Center).Render(
				lipgloss.PlaceVertical(height, lipgloss.Center, m.emptyState),
			)
		}

		// The scroll pager costs a row, so it only appears once there's one to
		// spare; at a single row the content itself wins.
		body := m.viewport.View()
		if height > m.ctx.Styles.Sidebar.PagerHeight {
			body = lipgloss.JoinVertical(
				lipgloss.Top,
				body,
				m.ctx.Styles.Sidebar.PagerStyle.
					Render(fmt.Sprintf("%d%%", int(m.viewport.ScrollPercent()*100))),
			)
		}

		return style.Render(body)
	}

	// Right mode
	height := m.ctx.MainContentHeight
	style := m.ctx.Styles.Sidebar.Root.
		Height(height).
		Width(m.ctx.DynamicPreviewWidth)

	if m.data == "" {
		return style.Align(lipgloss.Center).Render(
			lipgloss.PlaceVertical(height, lipgloss.Center, m.emptyState),
		)
	}

	return style.Render(lipgloss.JoinVertical(
		lipgloss.Top,
		m.viewport.View(),
		m.ctx.Styles.Sidebar.PagerStyle.
			Render(fmt.Sprintf("%d%%", int(m.viewport.ScrollPercent()*100))),
	))
}

func (m *Model) SetContent(data string) {
	m.data = data
	m.viewport.SetContent(data)
}

func (m *Model) GetSidebarContentWidth() int {
	if m.ctx == nil || m.ctx.Config == nil {
		return 0
	}
	if m.ctx.PreviewPosition == "bottom" {
		return max(0, m.ctx.DynamicPreviewWidth)
	}
	return max(0, m.ctx.DynamicPreviewWidth-m.ctx.Styles.Sidebar.BorderWidth)
}

func (m *Model) ScrollToTop() {
	m.viewport.GotoTop()
}

func (m *Model) ScrollToBottom() {
	m.viewport.GotoBottom()
}

func (m *Model) YOffset() int {
	return m.viewport.YOffset()
}

func (m *Model) ScrollToPercent(percent float64) {
	totalLines := m.viewport.TotalLineCount()
	targetLine := int(float64(totalLines) * percent)
	m.viewport.SetYOffset(targetLine)
}

func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	if ctx == nil {
		return
	}
	m.ctx = ctx
	if m.ctx.PreviewPosition == "bottom" {
		// Mirrors View: the pager only takes its row when the pane is tall enough
		// to show it, so a one-row pane is all content.
		h := m.ctx.DynamicPreviewHeight
		if h > m.ctx.Styles.Sidebar.PagerHeight {
			h -= m.ctx.Styles.Sidebar.PagerHeight
		}
		m.viewport.SetHeight(max(0, h))
	} else {
		m.viewport.SetHeight(m.ctx.MainContentHeight - m.ctx.Styles.Sidebar.PagerHeight)
	}
	m.viewport.SetWidth(m.GetSidebarContentWidth())
}
