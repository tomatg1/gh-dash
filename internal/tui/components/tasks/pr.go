package tasks

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/data"
	"github.com/dlvhdr/gh-dash/v4/internal/prefs"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/utils"
)

type SectionIdentifier struct {
	Id   int
	Type string
}

type UpdatePRMsg struct {
	PrNumber         int
	IsClosed         *bool
	NewComment       *data.Comment
	ReadyForReview   *bool
	IsMerged         *bool
	IsInMergeQueue   *bool
	AddedAssignees   *data.Assignees
	RemovedAssignees *data.Assignees
	Labels           *data.PRLabels
}

type UpdateBranchMsg struct {
	Name      string
	IsCreated *bool
	NewPr     *data.PullRequestData
}

func buildTaskId(prefix string, prNumber int) string {
	return fmt.Sprintf("%s_%d", prefix, prNumber)
}

type GitHubTask struct {
	Id           string
	Args         []string
	Section      SectionIdentifier
	StartText    string
	FinishedText string
	Msg          func(c *exec.Cmd, err error) tea.Msg
}

func fireTask(ctx *context.ProgramContext, task GitHubTask) tea.Cmd {
	start := context.Task{
		Id:           task.Id,
		StartText:    task.StartText,
		FinishedText: task.FinishedText,
		State:        context.TaskStart,
		Error:        nil,
	}

	startCmd := ctx.StartTask(start)
	return tea.Batch(startCmd, func() tea.Msg {
		log.Info("Running task", "cmd", "gh "+strings.Join(task.Args, " "))
		c := exec.Command("gh", task.Args...)

		// Capture stderr so a failure surfaces gh's actual message (e.g. a
		// GraphQL error) in the footer instead of a bare "exit status 1".
		var stderr bytes.Buffer
		c.Stderr = &stderr

		err := c.Run()
		if err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				err = fmt.Errorf("%s", msg)
			}
		}
		return constants.TaskFinishedMsg{
			TaskId:      task.Id,
			SectionId:   task.Section.Id,
			SectionType: task.Section.Type,
			Err:         err,
			Msg:         task.Msg(c, err),
		}
	})
}

func OpenBranchPR(ctx *context.ProgramContext, section SectionIdentifier, branch string) tea.Cmd {
	return fireTask(ctx, GitHubTask{
		Id: fmt.Sprintf("branch_open_%s", branch),
		Args: []string{
			"pr",
			"view",
			"--web",
			branch,
			"-R",
			ctx.RepoUrl,
		},
		Section:      section,
		StartText:    fmt.Sprintf("Opening PR for branch %s", branch),
		FinishedText: fmt.Sprintf("PR for branch %s has been opened", branch),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{}
		},
	})
}

func ReopenPR(ctx *context.ProgramContext, section SectionIdentifier, pr data.RowData) tea.Cmd {
	prNumber := pr.GetNumber()
	return fireTask(ctx, GitHubTask{
		Id: buildTaskId("pr_reopen", prNumber),
		Args: []string{
			"pr",
			"reopen",
			fmt.Sprint(prNumber),
			"-R",
			pr.GetRepoNameWithOwner(),
		},
		Section:      section,
		StartText:    fmt.Sprintf("Reopening PR #%d", prNumber),
		FinishedText: fmt.Sprintf("PR #%d has been reopened", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{
				PrNumber: prNumber,
				IsClosed: utils.BoolPtr(false),
			}
		},
	})
}

func ClosePR(ctx *context.ProgramContext, section SectionIdentifier, pr data.RowData) tea.Cmd {
	prNumber := pr.GetNumber()
	return fireTask(ctx, GitHubTask{
		Id: buildTaskId("pr_close", prNumber),
		Args: []string{
			"pr",
			"close",
			fmt.Sprint(prNumber),
			"-R",
			pr.GetRepoNameWithOwner(),
		},
		Section:      section,
		StartText:    fmt.Sprintf("Closing PR #%d", prNumber),
		FinishedText: fmt.Sprintf("PR #%d has been closed", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{
				PrNumber: prNumber,
				IsClosed: utils.BoolPtr(true),
			}
		},
	})
}

func PRReady(ctx *context.ProgramContext, section SectionIdentifier, pr data.RowData) tea.Cmd {
	prNumber := pr.GetNumber()
	return fireTask(ctx, GitHubTask{
		Id: buildTaskId("pr_ready", prNumber),
		Args: []string{
			"pr",
			"ready",
			fmt.Sprint(prNumber),
			"-R",
			pr.GetRepoNameWithOwner(),
		},
		Section:      section,
		StartText:    fmt.Sprintf("Marking PR #%d as ready for review", prNumber),
		FinishedText: fmt.Sprintf("PR #%d has been marked as ready for review", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{
				PrNumber:       prNumber,
				ReadyForReview: utils.BoolPtr(true),
			}
		},
	})
}

func MergePR(ctx *context.ProgramContext, section SectionIdentifier, pr data.RowData) tea.Cmd {
	prNumber := pr.GetNumber()
	c := exec.Command(
		"gh",
		"pr",
		"merge",
		fmt.Sprint(prNumber),
		"-R",
		pr.GetRepoNameWithOwner(),
	)

	taskId := fmt.Sprintf("merge_%d", prNumber)
	task := context.Task{
		Id:           taskId,
		StartText:    fmt.Sprintf("Merging PR #%d", prNumber),
		FinishedText: fmt.Sprintf("PR #%d has been merged", prNumber),
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := ctx.StartTask(task)

	return tea.Batch(startCmd, tea.ExecProcess(c, func(err error) tea.Msg {
		isMerged := err == nil && c.ProcessState.ExitCode() == 0

		return constants.TaskFinishedMsg{
			SectionId:   section.Id,
			SectionType: section.Type,
			TaskId:      taskId,
			Err:         err,
			Msg: UpdatePRMsg{
				PrNumber: prNumber,
				IsMerged: &isMerged,
			},
		}
	}))
}

// MergePRWithMethod merges a PR with an explicit strategy ("squash" | "merge" |
// "rebase"). Passing the method flag is what makes `gh pr merge` non-interactive
// -- without it gh prompts for the strategy and then for a final submit, per PR
// -- so unlike MergePR this runs as a normal background task (no TUI suspend,
// and a failure surfaces gh's real error in the footer).
//
// The method is remembered for the repo only on success, so a strategy the repo
// doesn't allow is never learned.
func MergePRWithMethod(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	pr data.RowData,
	method string,
) tea.Cmd {
	prNumber := pr.GetNumber()
	repo := pr.GetRepoNameWithOwner()

	return fireTask(ctx, GitHubTask{
		Id:           buildTaskId("pr_merge", prNumber),
		Args:         []string{"pr", "merge", fmt.Sprint(prNumber), "-R", repo, "--" + method},
		Section:      section,
		StartText:    fmt.Sprintf("Merging PR #%d (%s)", prNumber, method),
		FinishedText: fmt.Sprintf("PR #%d has been merged (%s)", prNumber, method),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			if err != nil {
				return UpdatePRMsg{}
			}
			// Learn the choice only once it's proven to work on this repo.
			_ = prefs.SaveMergeMethod(repo, method)

			return UpdatePRMsg{PrNumber: prNumber, IsMerged: utils.BoolPtr(true)}
		},
	})
}

// enqueueMutation adds a PR to its base branch's merge queue.
const enqueueMutation = `mutation($id:ID!){ enqueuePullRequest(input:{pullRequestId:$id}){ mergeQueueEntry{ position } } }`

// dequeueMutation removes a PR from the merge queue. Note the input field is
// `id` (the pull request's node id), NOT `pullRequestId` as enqueue uses -- the
// two mutations are asymmetric.
const dequeueMutation = `mutation($id:ID!){ dequeuePullRequest(input:{id:$id}){ mergeQueueEntry{ position } } }`

// EnqueuePR adds a PR to its base branch's merge queue via the native
// enqueuePullRequest mutation (the same path as the web "Merge when ready"
// button). Unlike `gh pr merge`, this does NOT require the repo's "Allow
// auto-merge" setting, so it works on merge-queue repos that keep it disabled.
func EnqueuePR(ctx *context.ProgramContext, section SectionIdentifier, prNumber int, prId string) tea.Cmd {
	return fireTask(ctx, GitHubTask{
		Id:           buildTaskId("pr_enqueue", prNumber),
		Args:         []string{"api", "graphql", "-f", "query=" + enqueueMutation, "-f", "id=" + prId},
		Section:      section,
		StartText:    fmt.Sprintf("Adding PR #%d to the merge queue", prNumber),
		FinishedText: fmt.Sprintf("PR #%d added to the merge queue", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			if err != nil {
				return UpdatePRMsg{}
			}
			return UpdatePRMsg{PrNumber: prNumber, IsInMergeQueue: utils.BoolPtr(true)}
		},
	})
}

// DequeuePR removes a PR from its base branch's merge queue via the native
// dequeuePullRequest mutation.
func DequeuePR(ctx *context.ProgramContext, section SectionIdentifier, prNumber int, prId string) tea.Cmd {
	return fireTask(ctx, GitHubTask{
		Id:           buildTaskId("pr_dequeue", prNumber),
		Args:         []string{"api", "graphql", "-f", "query=" + dequeueMutation, "-f", "id=" + prId},
		Section:      section,
		StartText:    fmt.Sprintf("Removing PR #%d from the merge queue", prNumber),
		FinishedText: fmt.Sprintf("PR #%d removed from the merge queue", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			if err != nil {
				return UpdatePRMsg{}
			}
			return UpdatePRMsg{PrNumber: prNumber, IsInMergeQueue: utils.BoolPtr(false)}
		},
	})
}

func CreatePR(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	branchName string,
	title string,
) tea.Cmd {
	c := exec.Command(
		"gh",
		"pr",
		"create",
		"--title",
		title,
		"-R",
		ctx.RepoUrl,
	)

	taskId := fmt.Sprintf("create_pr_%s", title)
	task := context.Task{
		Id:           taskId,
		StartText:    fmt.Sprintf(`Creating PR "%s"`, title),
		FinishedText: fmt.Sprintf(`PR "%s" has been created`, title),
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := ctx.StartTask(task)

	return tea.Batch(startCmd, tea.ExecProcess(c, func(err error) tea.Msg {
		isCreated := err == nil && c.ProcessState.ExitCode() == 0

		return constants.TaskFinishedMsg{
			SectionId:   section.Id,
			SectionType: section.Type,
			TaskId:      taskId,
			Err:         nil,
			Msg:         UpdateBranchMsg{Name: branchName, IsCreated: &isCreated},
		}
	}))
}

func updatePRTask(section SectionIdentifier, pr data.RowData) GitHubTask {
	prNumber := pr.GetNumber()
	return GitHubTask{
		Id: buildTaskId("pr_update", prNumber),
		Args: []string{
			"pr",
			"update-branch",
			fmt.Sprint(prNumber),
			"-R",
			pr.GetRepoNameWithOwner(),
		},
		Section:      section,
		StartText:    fmt.Sprintf("Updating PR #%d", prNumber),
		FinishedText: fmt.Sprintf("PR #%d has been updated", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{
				PrNumber: prNumber,
			}
		},
	}
}

func UpdatePR(ctx *context.ProgramContext, section SectionIdentifier, pr data.RowData) tea.Cmd {
	return fireTask(ctx, updatePRTask(section, pr))
}

func AssignPR(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	pr data.RowData,
	usernames []string,
) tea.Cmd {
	prNumber := pr.GetNumber()
	args := []string{
		"pr",
		"edit",
		fmt.Sprint(prNumber),
		"-R",
		pr.GetRepoNameWithOwner(),
	}
	for _, assignee := range usernames {
		args = append(args, "--add-assignee", assignee)
	}
	return fireTask(ctx, GitHubTask{
		Id:           buildTaskId("pr_assign", prNumber),
		Args:         args,
		Section:      section,
		StartText:    fmt.Sprintf("Assigning pr #%d to %s", prNumber, usernames),
		FinishedText: fmt.Sprintf("pr #%d has been assigned to %s", prNumber, usernames),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			returnedAssignees := data.Assignees{Nodes: []data.Assignee{}}
			for _, assignee := range usernames {
				returnedAssignees.Nodes = append(
					returnedAssignees.Nodes,
					data.Assignee{Login: assignee},
				)
			}
			return UpdatePRMsg{
				PrNumber:       prNumber,
				AddedAssignees: &returnedAssignees,
			}
		},
	})
}

func UnassignPR(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	pr data.RowData,
	usernames []string,
) tea.Cmd {
	prNumber := pr.GetNumber()
	args := []string{
		"pr",
		"edit",
		fmt.Sprint(prNumber),
		"-R",
		pr.GetRepoNameWithOwner(),
	}
	for _, assignee := range usernames {
		args = append(args, "--remove-assignee", assignee)
	}
	return fireTask(ctx, GitHubTask{
		Id:           buildTaskId("pr_unassign", prNumber),
		Args:         args,
		Section:      section,
		StartText:    fmt.Sprintf("Unassigning %s from pr #%d", usernames, prNumber),
		FinishedText: fmt.Sprintf("%s unassigned from pr #%d", usernames, prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			returnedAssignees := data.Assignees{Nodes: []data.Assignee{}}
			for _, assignee := range usernames {
				returnedAssignees.Nodes = append(
					returnedAssignees.Nodes,
					data.Assignee{Login: assignee},
				)
			}
			return UpdatePRMsg{
				PrNumber:         prNumber,
				RemovedAssignees: &returnedAssignees,
			}
		},
	})
}

func CommentOnPR(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	pr data.RowData,
	body string,
) tea.Cmd {
	prNumber := pr.GetNumber()
	return fireTask(ctx, GitHubTask{
		Id: buildTaskId("pr_comment", prNumber),
		Args: []string{
			"pr",
			"comment",
			fmt.Sprint(prNumber),
			"-R",
			pr.GetRepoNameWithOwner(),
			"-b",
			body,
		},
		Section:      section,
		StartText:    fmt.Sprintf("Commenting on PR #%d", prNumber),
		FinishedText: fmt.Sprintf("Commented on PR #%d", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{
				PrNumber: prNumber,
				NewComment: &data.Comment{
					Author:    struct{ Login string }{Login: ctx.User},
					Body:      body,
					UpdatedAt: time.Now(),
				},
			}
		},
	})
}

func ApprovePR(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	pr data.RowData,
	comment string,
) tea.Cmd {
	prNumber := pr.GetNumber()
	args := []string{
		"pr",
		"review",
		"-R",
		pr.GetRepoNameWithOwner(),
		fmt.Sprint(prNumber),
		"--approve",
	}
	if comment != "" {
		args = append(args, "--body", comment)
	}
	return fireTask(ctx, GitHubTask{
		Id:           buildTaskId("pr_approve", prNumber),
		Args:         args,
		Section:      section,
		StartText:    fmt.Sprintf("Approving pr #%d", prNumber),
		FinishedText: fmt.Sprintf("pr #%d has been approved", prNumber),
		Msg: func(c *exec.Cmd, err error) tea.Msg {
			return UpdatePRMsg{
				PrNumber: prNumber,
			}
		},
	})
}

func ApproveWorkflows(
	ctx *context.ProgramContext,
	section SectionIdentifier,
	pr data.RowData,
) tea.Cmd {
	prNumber := pr.GetNumber()
	repo := pr.GetRepoNameWithOwner()
	taskId := buildTaskId("pr_approve_workflows", prNumber)

	task := context.Task{
		Id:           taskId,
		StartText:    fmt.Sprintf("Approving workflows for PR #%d", prNumber),
		FinishedText: fmt.Sprintf("Workflows for PR #%d have been approved", prNumber),
		State:        context.TaskStart,
		Error:        nil,
	}
	startCmd := ctx.StartTask(task)

	return tea.Batch(startCmd, func() tea.Msg {
		// Step 1: Get head SHA
		shaCmd := exec.Command("gh", "pr", "view", fmt.Sprint(prNumber),
			"-R", repo, "--json", "headRefOid", "--jq", ".headRefOid")
		shaOut, err := shaCmd.Output()
		if err != nil {
			return constants.TaskFinishedMsg{
				TaskId:      taskId,
				SectionId:   section.Id,
				SectionType: section.Type,
				Err:         fmt.Errorf("failed to get head SHA: %w", err),
				Msg:         UpdatePRMsg{PrNumber: prNumber},
			}
		}
		sha := strings.TrimSpace(string(shaOut))

		// Step 2: Get workflow run IDs awaiting approval
		runsCmd := exec.Command("gh", "api",
			fmt.Sprintf("repos/%s/actions/runs?status=action_required&head_sha=%s", repo, sha),
			"--jq", ".workflow_runs[].id")
		runsOut, err := runsCmd.Output()
		if err != nil {
			return constants.TaskFinishedMsg{
				TaskId:      taskId,
				SectionId:   section.Id,
				SectionType: section.Type,
				Err:         fmt.Errorf("failed to get workflow runs: %w", err),
				Msg:         UpdatePRMsg{PrNumber: prNumber},
			}
		}

		runIds := strings.Fields(strings.TrimSpace(string(runsOut)))
		if len(runIds) == 0 {
			return constants.TaskFinishedMsg{
				TaskId:      taskId,
				SectionId:   section.Id,
				SectionType: section.Type,
				Err:         fmt.Errorf("no workflows awaiting approval"),
				Msg:         UpdatePRMsg{PrNumber: prNumber},
			}
		}

		// Step 3: Approve each run (best-effort)
		var lastErr error
		approved := 0
		for _, runId := range runIds {
			log.Info("Approving workflow run", "runId", runId, "pr", prNumber)
			approveCmd := exec.Command("gh", "api", "-X", "POST",
				fmt.Sprintf("repos/%s/actions/runs/%s/approve", repo, runId))
			output, err := approveCmd.CombinedOutput()
			if err != nil {
				outStr := string(output)
				if strings.Contains(outStr, "not from a fork pull request") {
					lastErr = fmt.Errorf(
						"workflow not approvable via API (only fork PR workflows can be approved)",
					)
				} else {
					lastErr = fmt.Errorf("failed to approve run %s: %w", runId, err)
				}
			} else {
				approved++
			}
		}

		return constants.TaskFinishedMsg{
			TaskId:      taskId,
			SectionId:   section.Id,
			SectionType: section.Type,
			Err:         lastErr,
			Msg:         UpdatePRMsg{PrNumber: prNumber},
		}
	})
}
