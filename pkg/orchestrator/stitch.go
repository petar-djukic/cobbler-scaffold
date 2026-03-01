// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// errTaskReset is returned by doOneTask when a task fails but the stitch
// loop should continue to the next task (e.g., Claude failure, worktree
// commit failure, merge failure). The task has been reset to open.
var errTaskReset = errors.New("task reset to open")

//go:embed prompts/stitch.yaml
var defaultStitchPrompt string

//go:embed constitutions/execution.yaml
var executionConstitution string

//go:embed constitutions/go-style.yaml
var goStyleConstitution string

// Stitch picks ready tasks from GitHub Issues and invokes Claude to execute them.
// Reads all options from Config.
func (o *Orchestrator) Stitch() error {
	_, err := o.RunStitch()
	return err
}

// RunStitch runs the stitch workflow using Config settings.
// It processes up to MaxStitchIssuesPerCycle tasks and returns the count
// of tasks completed. The caller (RunCycles) uses this to track the
// total across all cycles against MaxStitchIssues.
func (o *Orchestrator) RunStitch() (int, error) {
	return o.RunStitchN(o.cfg.Cobbler.MaxStitchIssuesPerCycle)
}

// RunStitchN processes up to n tasks and returns the count completed.
func (o *Orchestrator) RunStitchN(limit int) (int, error) {
	setPhase("stitch")
	defer clearPhase()
	stitchStart := time.Now()

	// Start orchestrator log capture.
	if hdir := o.historyDir(); hdir != "" {
		logPath := filepath.Join(hdir,
			stitchStart.Format("2006-01-02-15-04-05")+"-stitch-orchestrator.log")
		if err := openLogSink(logPath); err != nil {
			logf("warning: could not open orchestrator log: %v", err)
		} else {
			defer closeLogSink()
		}
	}

	logf("starting (limit=%d)", limit)
	o.logConfig("stitch")

	if err := o.checkClaude(); err != nil {
		return 0, err
	}

	branch, err := o.resolveBranch(o.cfg.Generation.Branch)
	if err != nil {
		logf("resolveBranch failed: %v", err)
		return 0, err
	}
	logf("resolved branch=%s", branch)
	if currentGeneration == "" {
		setGeneration(branch)
		defer clearGeneration()
	}

	if err := ensureOnBranch(branch); err != nil {
		logf("ensureOnBranch failed: %v", err)
		return 0, fmt.Errorf("switching to branch: %w", err)
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return 0, fmt.Errorf("getting working directory: %w", err)
	}
	logf("repoRoot=%s", repoRoot)

	// Resolve GitHub repo and ensure cobbler labels exist.
	ghRepo, err := detectGitHubRepo(repoRoot, o.cfg)
	if err != nil {
		logf("detectGitHubRepo failed: %v", err)
		return 0, fmt.Errorf("detecting GitHub repo: %w", err)
	}
	generation := branch
	logf("using GitHub repo %s generation %s for issues", ghRepo, generation)
	if err := ensureCobblerLabels(ghRepo); err != nil {
		logf("ensureCobblerLabels warning: %v", err)
	}

	worktreeBase := worktreeBasePath()
	logf("worktreeBase=%s", worktreeBase)

	baseBranch, err := gitCurrentBranch(".")
	if err != nil {
		return 0, fmt.Errorf("getting current branch: %w", err)
	}
	logf("baseBranch=%s", baseBranch)

	logf("recovering stale tasks")
	if err := o.recoverStaleTasks(baseBranch, worktreeBase, ghRepo, generation); err != nil {
		logf("recovery failed: %v", err)
		return 0, fmt.Errorf("recovery: %w", err)
	}

	totalTasks := 0
	// failedTaskIDs tracks tasks that returned errTaskReset in this cycle.
	// A task whose in-progress label is removed is re-eligible immediately,
	// so without this set the stitch loop retries the same task indefinitely.
	failedTaskIDs := map[string]struct{}{}
	for {
		if limit > 0 && totalTasks >= limit {
			logf("reached per-cycle limit (%d), pausing for measure", limit)
			break
		}

		logf("looking for next ready task (completed %d so far)", totalTasks)
		task, err := pickTask(baseBranch, worktreeBase, ghRepo, generation)
		if err != nil {
			logf("no more tasks: %v", err)
			break
		}

		// If this task already failed in the current cycle, stop. It was
		// reset to open and will be retried in the next measure+stitch cycle.
		if _, alreadyFailed := failedTaskIDs[task.id]; alreadyFailed {
			logf("task %s already failed this cycle, stopping stitch", task.id)
			break
		}

		taskStart := time.Now()
		logf("executing task %d: id=%s title=%q", totalTasks+1, task.id, task.title)
		if err := o.doOneTask(task, baseBranch, repoRoot); err != nil {
			if errors.Is(err, errTaskReset) {
				logf("task %s was reset after %s, continuing", task.id, time.Since(taskStart).Round(time.Second))
				failedTaskIDs[task.id] = struct{}{}
				continue
			}
			logf("task %s failed after %s: %v", task.id, time.Since(taskStart).Round(time.Second), err)
			return totalTasks, fmt.Errorf("executing task %s: %w", task.id, err)
		}
		logf("task %s completed in %s", task.id, time.Since(taskStart).Round(time.Second))

		totalTasks++
	}

	logf("completed %d task(s) in %s", totalTasks, time.Since(stitchStart).Round(time.Second))
	return totalTasks, nil
}

// taskBranchName returns the git branch name for a stitch task.
// Uses "task/<base>-<id>" instead of "<base>/task/<id>" to avoid
// ref conflicts when the base branch is "main".
func taskBranchName(baseBranch, issueID string) string {
	return "task/" + baseBranch + "-" + issueID
}

// taskBranchPattern returns the glob pattern for listing task branches.
func taskBranchPattern(baseBranch string) string {
	return "task/" + baseBranch + "-*"
}

type stitchTask struct {
	id          string // cobbler_index as string — used for branch/worktree naming
	title       string
	description string
	issueType   string
	branchName  string
	worktreeDir string
	ghNumber    int    // GitHub issue number — used for closing/labelling
	generation  string // generation label value
	repo        string // GitHub owner/repo
}

// recoverStaleTasks cleans up task branches and orphaned in_progress issues
// from a previous interrupted run.
func (o *Orchestrator) recoverStaleTasks(baseBranch, worktreeBase, repo, generation string) error {
	logf("recoverStaleTasks: checking for stale branches with pattern %s", taskBranchPattern(baseBranch))
	staleBranches := recoverStaleBranches(baseBranch, worktreeBase, repo)

	logf("recoverStaleTasks: checking for orphaned in_progress issues")
	orphanedIssues := resetOrphanedIssues(baseBranch, repo, generation)

	logf("recoverStaleTasks: pruning worktrees")
	if err := gitWorktreePrune("."); err != nil {
		logf("recoverStaleTasks: worktree prune warning: %v", err)
	}

	if staleBranches || orphanedIssues {
		logf("recoverStaleTasks: recovered stale state (branches=%v orphans=%v)", staleBranches, orphanedIssues)
	} else {
		logf("recoverStaleTasks: no stale state found")
	}

	return nil
}

// recoverStaleBranches removes leftover task branches and worktrees,
// removing the in-progress label from their issues. Returns true if any were recovered.
func recoverStaleBranches(baseBranch, worktreeBase, repo string) bool {
	branches := gitListBranches(taskBranchPattern(baseBranch), ".")
	if len(branches) == 0 {
		logf("recoverStaleBranches: no stale branches found")
		return false
	}

	logf("recoverStaleBranches: found %d stale branch(es): %v", len(branches), branches)
	for _, branch := range branches {
		logf("recoverStaleBranches: recovering %s", branch)

		issueID := strings.TrimPrefix(branch, "task/"+baseBranch+"-")
		worktreeDir := filepath.Join(worktreeBase, issueID)

		if _, err := os.Stat(worktreeDir); err == nil {
			logf("recoverStaleBranches: removing worktree %s", worktreeDir)
			if err := gitWorktreeRemove(worktreeDir, "."); err != nil {
				logf("recoverStaleBranches: worktree remove warning: %v", err)
			}
		} else {
			logf("recoverStaleBranches: no worktree at %s", worktreeDir)
		}

		logf("recoverStaleBranches: deleting branch %s", branch)
		if err := gitForceDeleteBranch(branch, "."); err != nil {
			logf("recoverStaleBranches: branch delete warning: %v", err)
		}

		if issueID != "" {
			var num int
			if _, err := fmt.Sscanf(issueID, "%d", &num); err == nil && num > 0 {
				logf("recoverStaleBranches: removing in-progress label from issue #%d", num)
				if err := removeInProgressLabel(repo, num); err != nil {
					logf("recoverStaleBranches: label removal warning: %v", err)
				}
			}
		}
	}
	return true
}

// resetOrphanedIssues finds in_progress GitHub issues with no corresponding
// task branch and removes their in-progress label. Returns true if any were reset.
func resetOrphanedIssues(baseBranch, repo, generation string) bool {
	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		logf("resetOrphanedIssues: list issues failed: %v", err)
		return false
	}

	recovered := false
	for _, iss := range issues {
		if !hasLabel(iss, cobblerLabelInProgress) {
			continue
		}
		id := fmt.Sprintf("%d", iss.Number)
		taskBranch := taskBranchName(baseBranch, id)
		if !gitBranchExists(taskBranch, ".") {
			recovered = true
			logf("resetOrphanedIssues: orphaned issue #%d (no branch %s), removing in-progress label", iss.Number, taskBranch)
			if err := removeInProgressLabel(repo, iss.Number); err != nil {
				logf("resetOrphanedIssues: label removal warning for #%d: %v", iss.Number, err)
			}
		} else {
			logf("resetOrphanedIssues: issue #%d has branch %s, skipping", iss.Number, taskBranch)
		}
	}
	return recovered
}

func pickTask(baseBranch, worktreeBase, repo, generation string) (stitchTask, error) {
	logf("pickTask: calling pickReadyIssue repo=%s generation=%s", repo, generation)
	iss, err := pickReadyIssue(repo, generation)
	if err != nil {
		logf("pickTask: no tasks available: %v", err)
		return stitchTask{}, fmt.Errorf("no tasks available")
	}

	id := fmt.Sprintf("%d", iss.Number)
	task := stitchTask{
		id:          id,
		title:       iss.Title,
		description: iss.Description,
		issueType:   "task",
		branchName:  taskBranchName(baseBranch, id),
		worktreeDir: filepath.Join(worktreeBase, id),
		ghNumber:    iss.Number,
		generation:  generation,
		repo:        repo,
	}

	// Validate the issue description as YAML with required fields.
	if err := validateIssueDescription(task.description); err != nil {
		logf("pickTask: description validation warning: %v", err)
	}

	logf("pickTask: picked #%d id=%s branch=%s worktree=%s", iss.Number, task.id, task.branchName, task.worktreeDir)
	logf("pickTask: title=%q", task.title)
	logf("pickTask: descriptionLen=%d", len(task.description))
	return task, nil
}

// parseRequiredReading extracts the required_reading list from a YAML task
// description. Returns nil if the field is absent or unparseable.
func parseRequiredReading(description string) []string {
	if description == "" {
		return nil
	}
	var parsed struct {
		RequiredReading []string `yaml:"required_reading"`
	}
	if err := yaml.Unmarshal([]byte(description), &parsed); err != nil {
		logf("parseRequiredReading: YAML parse error: %v", err)
		return nil
	}
	return parsed.RequiredReading
}

// validateIssueDescription checks that a description parses as valid YAML
// and contains the required top-level keys defined by the issue-format
// constitution. Returns an error describing what is missing; callers
// should log a warning but not block on validation failures.
func validateIssueDescription(desc string) error {
	if desc == "" {
		return fmt.Errorf("empty description")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(desc), &parsed); err != nil {
		return fmt.Errorf("not valid YAML: %w", err)
	}

	required := []string{"deliverable_type", "required_reading", "files", "requirements", "acceptance_criteria"}
	var missing []string
	for _, key := range required {
		if _, ok := parsed[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (o *Orchestrator) doOneTask(task stitchTask, baseBranch, repoRoot string) error {
	taskStart := time.Now()
	logf("doOneTask: starting task %s (%s)", task.id, task.title)

	// The cobbler-in-progress label was added by pickReadyIssue; no separate claim step is needed.
	logf("doOneTask: task #%d claimed via pickReadyIssue label", task.ghNumber)

	// Create worktree.
	logf("doOneTask: creating worktree for %s", task.id)
	wtStart := time.Now()
	if err := createWorktree(task); err != nil {
		logf("doOneTask: createWorktree failed after %s: %v", time.Since(wtStart).Round(time.Second), err)
		return fmt.Errorf("creating worktree: %w", err)
	}
	logf("doOneTask: worktree created in %s", time.Since(wtStart).Round(time.Second))

	// Snapshot LOC before Claude.
	locBefore := o.captureLOC()
	logf("doOneTask: locBefore prod=%d test=%d", locBefore.Production, locBefore.Test)

	// Build and run prompt.
	prompt, promptErr := o.buildStitchPrompt(task)
	if promptErr != nil {
		o.resetTask(task, "prompt build failure")
		return promptErr
	}
	logf("doOneTask: prompt built, length=%d bytes", len(prompt))

	// Save prompt BEFORE calling Claude so it's on disk even if Claude times out.
	historyTS := time.Now().Format("2006-01-02-15-04-05")
	o.saveHistoryPrompt(historyTS, "stitch", prompt)

	logf("doOneTask: invoking Claude for task %s", task.id)
	claudeStart := time.Now()
	tokens, claudeErr := o.runClaude(prompt, task.worktreeDir, o.cfg.Silence())

	// Save Claude log immediately — even on failure, partial output is valuable.
	o.saveHistoryLog(historyTS, "stitch", tokens.RawOutput)

	if claudeErr != nil {
		logf("doOneTask: Claude failed for %s after %s: %v", task.id, time.Since(claudeStart).Round(time.Second), claudeErr)
		o.saveHistoryStats(historyTS, "stitch", HistoryStats{
			Caller:    "stitch",
			TaskID:    task.id,
			TaskTitle: task.title,
			Status:    "failed",
			Error:     fmt.Sprintf("claude failure: %v", claudeErr),
			StartedAt: claudeStart.UTC().Format(time.RFC3339),
			Duration:  time.Since(taskStart).Round(time.Second).String(),
			DurationS: int(time.Since(taskStart).Seconds()),
			Tokens:    historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
		})
		o.resetTask(task, "Claude failure")
		return errTaskReset
	}
	logf("doOneTask: Claude completed for %s in %s", task.id, time.Since(claudeStart).Round(time.Second))

	// Commit Claude's changes in the worktree. Claude does not run git;
	// the orchestrator manages all git operations externally.
	if err := commitWorktreeChanges(task); err != nil {
		logf("doOneTask: worktree commit failed for %s: %v", task.id, err)
		o.saveHistoryStats(historyTS, "stitch", HistoryStats{
			Caller:    "stitch",
			TaskID:    task.id,
			TaskTitle: task.title,
			Status:    "failed",
			Error:     fmt.Sprintf("worktree commit failure: %v", err),
			StartedAt: claudeStart.UTC().Format(time.RFC3339),
			Duration:  time.Since(taskStart).Round(time.Second).String(),
			DurationS: int(time.Since(taskStart).Seconds()),
			Tokens:    historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
		})
		o.resetTask(task, "worktree commit failure")
		return errTaskReset
	}

	// Append outcome trailers to the worktree commit before merging.
	// Trailers must be on the pre-merge commit so they travel into the
	// generation branch history. LOCAfter and Diff are not yet available
	// at this point; the full record is saved in HistoryStats YAML files.
	trailerRec := InvocationRecord{
		Caller:    "stitch",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		DurationS: int(time.Since(claudeStart).Seconds()),
		Tokens: claudeTokens{
			Input:         tokens.InputTokens,
			Output:        tokens.OutputTokens,
			CacheCreation: tokens.CacheCreationTokens,
			CacheRead:     tokens.CacheReadTokens,
			CostUSD:       tokens.CostUSD,
		},
		LOCBefore: locBefore,
	}
	if err := appendOutcomeTrailers(task.worktreeDir, trailerRec); err != nil {
		logf("doOneTask: outcome trailer warning for %s: %v", task.id, err)
	}

	// Capture pre-merge HEAD for diffstat.
	preMergeRef, err := gitRevParseHEAD(".")
	if err != nil {
		logf("doOneTask: warning getting pre-merge ref: %v", err)
	}

	// Merge branch back.
	logf("doOneTask: merging %s into %s", task.branchName, baseBranch)
	mergeStart := time.Now()
	if err := mergeBranch(task.branchName, baseBranch, repoRoot); err != nil {
		logf("doOneTask: merge failed for %s after %s: %v", task.id, time.Since(mergeStart).Round(time.Second), err)
		o.saveHistoryStats(historyTS, "stitch", HistoryStats{
			Caller:    "stitch",
			TaskID:    task.id,
			TaskTitle: task.title,
			Status:    "failed",
			Error:     fmt.Sprintf("merge failure: %v", err),
			StartedAt: claudeStart.UTC().Format(time.RFC3339),
			Duration:  time.Since(taskStart).Round(time.Second).String(),
			DurationS: int(time.Since(taskStart).Seconds()),
			Tokens:    historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
		})
		o.resetTask(task, "merge failure")
		return errTaskReset
	}
	logf("doOneTask: merge completed in %s", time.Since(mergeStart).Round(time.Second))

	// Capture LOC diff, per-file diff, and post-merge LOC.
	diff, diffErr := gitDiffShortstat(preMergeRef, ".")
	if diffErr != nil {
		logf("doOneTask: warning getting diff shortstat: %v", diffErr)
	}
	logf("doOneTask: diff files=%d ins=%d del=%d", diff.FilesChanged, diff.Insertions, diff.Deletions)
	fileChanges, fcErr := gitDiffNameStatus(preMergeRef, ".")
	if fcErr != nil {
		logf("doOneTask: warning getting file changes: %v", fcErr)
	}
	logf("doOneTask: fileChanges=%d entries", len(fileChanges))
	locAfter := o.captureLOC()
	logf("doOneTask: locAfter prod=%d test=%d", locAfter.Production, locAfter.Test)

	// Cleanup worktree.
	logf("doOneTask: cleaning up worktree for %s", task.id)
	cleanupWorktree(task)

	// Save stitch stats (log was saved immediately after runClaude).
	taskDuration := time.Since(taskStart)
	o.saveHistoryStats(historyTS, "stitch", HistoryStats{
		Caller:    "stitch",
		TaskID:    task.id,
		TaskTitle: task.title,
		Status:    "success",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		Duration:  taskDuration.Round(time.Second).String(),
		DurationS: int(taskDuration.Seconds()),
		Tokens:    historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
		CostUSD:   tokens.CostUSD,
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
		Diff:      historyDiff{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
	})

	// Save stitch report with per-file diffstat.
	o.saveHistoryReport(historyTS, StitchReport{
		TaskID:    task.id,
		TaskTitle: task.title,
		Status:    "success",
		Branch:    task.branchName,
		Diff:      historyDiff{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
		Files:     fileChanges,
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
	})

	// Close task with metrics.
	rec := InvocationRecord{
		Caller:    "stitch",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		DurationS: int(taskDuration.Seconds()),
		Tokens:    claudeTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens, CostUSD: tokens.CostUSD},
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
		Diff:      diffRecord{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
	}
	logf("doOneTask: closing task %s", task.id)
	o.closeStitchTask(task, rec)

	logf("doOneTask: task %s finished in %s", task.id, time.Since(taskStart).Round(time.Second))
	return nil
}

func createWorktree(task stitchTask) error {
	logf("createWorktree: dir=%s branch=%s", task.worktreeDir, task.branchName)

	if err := os.MkdirAll(filepath.Dir(task.worktreeDir), 0o755); err != nil {
		return fmt.Errorf("creating worktree parent directory: %w", err)
	}

	if !gitBranchExists(task.branchName, ".") {
		logf("createWorktree: branch %s does not exist, creating", task.branchName)
		if err := gitCreateBranch(task.branchName, "."); err != nil {
			logf("createWorktree: gitCreateBranch failed: %v", err)
			return fmt.Errorf("creating branch %s: %w", task.branchName, err)
		}
		logf("createWorktree: branch %s created", task.branchName)
	} else {
		logf("createWorktree: branch %s already exists", task.branchName)
	}

	logf("createWorktree: adding worktree")
	cmd := gitWorktreeAdd(task.worktreeDir, task.branchName, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logf("createWorktree: worktree add failed: %v", err)
		return fmt.Errorf("adding worktree: %w", err)
	}

	logf("createWorktree: worktree ready at %s on branch %s", task.worktreeDir, task.branchName)
	return nil
}

func (o *Orchestrator) buildStitchPrompt(task stitchTask) (string, error) {
	tmpl, err := parsePromptTemplate(orDefault(o.cfg.Cobbler.StitchPrompt, defaultStitchPrompt))
	if err != nil {
		return "", fmt.Errorf("stitch prompt YAML: %w", err)
	}

	executionConst := orDefault(o.cfg.Cobbler.ExecutionConstitution, executionConstitution)
	goStyleConst := orDefault(o.cfg.Cobbler.GoStyleConstitution, goStyleConstitution)

	// Load per-phase context file (prd003 R9.9). Resolved from the
	// original working directory before chdir to worktree.
	stitchCtxPath := filepath.Join(o.cfg.Cobbler.Dir, "stitch_context.yaml")
	phaseCtx, phaseErr := loadPhaseContext(stitchCtxPath)
	if phaseErr != nil {
		return "", fmt.Errorf("loading stitch context: %w", phaseErr)
	}
	if phaseCtx != nil {
		logf("buildStitchPrompt: using phase context from %s", stitchCtxPath)
	} else {
		logf("buildStitchPrompt: no phase context file, using config defaults")
	}

	// Build project context from the worktree directory so source code
	// reflects the latest state after prior stitches have been merged.
	var projectCtx *ProjectContext
	if task.worktreeDir != "" {
		orig, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("buildStitchPrompt: getwd: %w", err)
		}
		if err := os.Chdir(task.worktreeDir); err != nil {
			logf("buildStitchPrompt: chdir to worktree error: %v", err)
		} else {
			defer os.Chdir(orig)
			ctx, ctxErr := buildProjectContext("", o.cfg.Project, phaseCtx)
			if ctxErr != nil {
				logf("buildStitchPrompt: buildProjectContext error: %v", ctxErr)
			} else {
				projectCtx = ctx
			}
		}
	}
	logf("buildStitchPrompt: projectCtx=%v", projectCtx != nil)

	// Selective stitch context (eng05 rec D): filter source files to only
	// those listed in the task's required_reading. Documentation files are
	// not filtered; only SourceCode is filtered.
	if projectCtx != nil {
		requiredReading := parseRequiredReading(task.description)
		var sourcePaths []string
		for _, entry := range requiredReading {
			clean := stripParenthetical(entry)
			if strings.HasSuffix(clean, ".go") {
				sourcePaths = append(sourcePaths, clean)
			}
		}
		if len(sourcePaths) > 0 {
			before := len(projectCtx.SourceCode)
			projectCtx.SourceCode = filterSourceFiles(projectCtx.SourceCode, sourcePaths)
			logf("buildStitchPrompt: filtered source files %d -> %d (required_reading has %d source paths)",
				before, len(projectCtx.SourceCode), len(sourcePaths))
		} else {
			logf("buildStitchPrompt: no source paths in required_reading, keeping all %d source files",
				len(projectCtx.SourceCode))
		}

		// Context budget enforcement: truncate non-required source files
		// when the serialized context exceeds MaxContextBytes.
		applyContextBudget(projectCtx, o.cfg.Cobbler.MaxContextBytes, sourcePaths)
	}

	taskContext := fmt.Sprintf("Task ID: %s\nType: %s\nTitle: %s",
		task.id, task.issueType, task.title)

	repoFiles := gitLsFiles(task.worktreeDir)

	doc := StitchPromptDoc{
		Role:                  tmpl.Role,
		RepositoryFiles:       repoFiles,
		ProjectContext:        projectCtx,
		Context:               taskContext,
		ExecutionConstitution: parseYAMLNode(executionConst),
		GoStyleConstitution:   parseYAMLNode(goStyleConst),
		Task:                  tmpl.Task,
		Constraints:           tmpl.Constraints,
		Description:           task.description,
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("marshaling stitch prompt: %w", err)
	}

	logf("buildStitchPrompt: %d bytes", len(out))
	return string(out), nil
}

func mergeBranch(branchName, baseBranch, repoRoot string) error {
	logf("mergeBranch: %s into %s (repoRoot=%s)", branchName, baseBranch, repoRoot)

	logf("mergeBranch: checking out %s", baseBranch)
	if err := gitCheckout(baseBranch, "."); err != nil {
		logf("mergeBranch: checkout failed: %v", err)
		return fmt.Errorf("checking out %s: %w", baseBranch, err)
	}
	logf("mergeBranch: checked out %s", baseBranch)

	logf("mergeBranch: merging %s", branchName)
	cmd := gitMergeCmd(branchName, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logf("mergeBranch: merge failed: %v", err)
		return fmt.Errorf("merging %s: %w", branchName, err)
	}

	logf("mergeBranch: merge successful")
	return nil
}

// commitWorktreeChanges stages and commits all changes Claude made in the
// worktree. Claude does not run git commands; the orchestrator handles git
// externally. Returns nil if there are no changes to commit.
func commitWorktreeChanges(task stitchTask) error {
	logf("commitWorktreeChanges: staging changes in %s", task.worktreeDir)

	addCmd := exec.Command(binGit, "add", "-A")
	addCmd.Dir = task.worktreeDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A: %w\n%s", err, out)
	}

	// Check if there are staged changes to commit.
	diffCmd := exec.Command(binGit, "diff", "--cached", "--quiet")
	diffCmd.Dir = task.worktreeDir
	if diffCmd.Run() == nil {
		logf("commitWorktreeChanges: no changes to commit for %s", task.id)
		return nil
	}

	msg := fmt.Sprintf("Task %s: %s", task.id, task.title)
	logf("commitWorktreeChanges: committing %q", msg)
	commitCmd := exec.Command(binGit, "commit", "--no-verify", "-m", msg)
	commitCmd.Dir = task.worktreeDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}

	logf("commitWorktreeChanges: committed in worktree for %s", task.id)
	return nil
}

// resetTask removes the in-progress label from a failed task, cleans up its
// worktree and branch. The reason string is included in log messages for traceability.
func (o *Orchestrator) resetTask(task stitchTask, reason string) {
	logf("resetTask: resetting #%d to ready (%s)", task.ghNumber, reason)
	if err := removeInProgressLabel(task.repo, task.ghNumber); err != nil {
		logf("resetTask: WARNING removeInProgressLabel failed for #%d: %v", task.ghNumber, err)
	}
	cleanupWorktree(task)
	if err := gitForceDeleteBranch(task.branchName, "."); err != nil {
		logf("resetTask: WARNING force branch delete failed for %s: %v", task.branchName, err)
	}
}

func cleanupWorktree(task stitchTask) {
	logf("cleanupWorktree: removing worktree %s", task.worktreeDir)
	if err := gitWorktreeRemove(task.worktreeDir, "."); err != nil {
		logf("cleanupWorktree: worktree remove failed, skipping branch delete: %v", err)
		return
	}

	logf("cleanupWorktree: deleting branch %s", task.branchName)
	if err := gitDeleteBranch(task.branchName, "."); err != nil {
		logf("cleanupWorktree: branch delete warning: %v", err)
	}

	logf("cleanupWorktree: done for task %s", task.id)
}

func (o *Orchestrator) closeStitchTask(task stitchTask, rec InvocationRecord) {
	logf("closeStitchTask: closing #%d %q", task.ghNumber, task.title)
	if err := closeCobblerIssue(task.repo, task.ghNumber, task.generation); err != nil {
		logf("closeStitchTask: closeCobblerIssue warning for #%d: %v", task.ghNumber, err)
	}
	logf("closeStitchTask: #%d closed", task.ghNumber)
}
