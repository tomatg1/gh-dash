package tui

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/urlopen"
)

// currRowURL is the selected row's URL, or "" when there is no selection. The
// reflect check matches openBrowser's: RowData is an interface that can hold a
// typed nil.
func (m *Model) currRowURL() string {
	currRow := m.getCurrRowData()
	if currRow == nil || reflect.ValueOf(currRow).IsNil() {
		return ""
	}

	return currRow.GetUrl()
}

// openURL opens an arbitrary URL, so a click on a row's checks or review icon
// can land on that PR's sub-page rather than its conversation.
func (m *Model) openURL(url string) tea.Cmd {
	if url == "" {
		return nil
	}

	taskId := fmt.Sprintf("open_browser_%d", time.Now().UnixNano())
	task := context.Task{
		Id:           taskId,
		StartText:    "Opening in browser",
		FinishedText: "Opened in browser",
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := m.ctx.StartTask(task)
	cmdTemplate := m.ctx.Config.Defaults.URLOpenCommand
	openCmd := func() tea.Msg {
		err := urlopen.Open(cmdTemplate, url)
		return constants.TaskFinishedMsg{TaskId: taskId, Err: err}
	}

	return tea.Batch(startCmd, openCmd)
}

func (m *Model) openBrowser() tea.Cmd {
	taskId := fmt.Sprintf("open_browser_%d", time.Now().Unix())
	task := context.Task{
		Id:           taskId,
		StartText:    "Opening in browser",
		FinishedText: "Opened in browser",
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := m.ctx.StartTask(task)
	cmdTemplate := m.ctx.Config.Defaults.URLOpenCommand
	openCmd := func() tea.Msg {
		currRow := m.getCurrRowData()
		if currRow == nil || reflect.ValueOf(currRow).IsNil() {
			return constants.TaskFinishedMsg{
				TaskId: taskId,
				Err:    errors.New("current selection doesn't have a URL"),
			}
		}
		err := urlopen.Open(cmdTemplate, currRow.GetUrl())
		return constants.TaskFinishedMsg{TaskId: taskId, Err: err}
	}
	return tea.Batch(startCmd, openCmd)
}
