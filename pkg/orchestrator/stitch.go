// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
	"gopkg.in/yaml.v3"
)

// rateLimitResetError wraps errTaskReset with rate limit context so the
// stitch loop can distinguish rate-limit-dominated failures from genuine
// task failures (GH-1805).
type rateLimitResetError struct {
	RateLimitWaitS int
	TotalDurationS int
}

func (e *rateLimitResetError) Error() string {
	return errTaskReset.Error()
}

func (e *rateLimitResetError) Unwrap() error {
	return errTaskReset
}

// isRateLimitDominated returns true when rate limit waits consumed more
// than half of the total task duration.
func (e *rateLimitResetError) isRateLimitDominated() bool {
	return e.TotalDurationS > 0 && e.RateLimitWaitS > e.TotalDurationS/2
}

// Stitch provides the stitch workflow: picking ready tasks and executing
// them via Claude in isolated worktrees.
type Stitch struct {
	cfg          Config
	logf         func(string, ...any)
	git          gitops.GitOps
	tracker      gh.WorkTracker
	claudeRunner *ClaudeRunner
	// Orchestrator state callbacks.
	setPhase        func(string)
	clearPhase      func()
	setGeneration   func(string)
	clearGeneration func()
	getGeneration   func() string
	openLogSink     func(string) error
	closeLogSink    func()
	// Generator helpers.
	resolveBranch        func(string) (string, error)
	ensureOnBranch       func(string) error
	enterWorktree        func() (string, error)
	pickTask             func(string, string, string, string) (stitchTask, error)
	createWorktree       func(stitchTask) error
	mergeBranch          func(string, string, string) error
	cleanupWorktree      func(stitchTask) bool
	recoverStaleBranches func(string, string, string) bool
	resetOrphanedIssues  func(string, string, string) bool
	diffNameStatus       func(string, string) ([]claude.FileChange, error)
}

// NewStitch creates a Stitch with explicit dependencies.
func NewStitch(o *Orchestrator) *Stitch {
	return &Stitch{
		cfg:                  o.cfg,
		logf:                 o.logf,
		git:                  o.git,
		tracker:              o.tracker,
		claudeRunner:         o.ClaudeRunner,
		setPhase:             o.setPhase,
		clearPhase:           o.clearPhase,
		setGeneration:        o.setGeneration,
		clearGeneration:      o.clearGeneration,
		getGeneration:        o.getGeneration,
		openLogSink:          o.openLogSink,
		closeLogSink:         o.closeLogSink,
		resolveBranch:        o.Generator.resolveBranch,
		ensureOnBranch:       o.Generator.ensureOnBranch,
		enterWorktree:        o.Generator.enterGenerationWorktree,
		pickTask:             o.Generator.pickTask,
		createWorktree:       o.Generator.createWorktree,
		mergeBranch:          o.Generator.mergeBranch,
		cleanupWorktree:      o.Generator.cleanupWorktree,
		recoverStaleBranches: o.Generator.recoverStaleBranches,
		resetOrphanedIssues:  o.Generator.resetOrphanedIssues,
		diffNameStatus:       o.Generator.diffNameStatus,
	}
}

//go:embed prompts/stitch.yaml
var defaultStitchPrompt string

//go:embed constitutions/execution.yaml
var executionConstitution string

//go:embed constitutions/go-style.yaml
var goStyleConstitution string

// Stitch picks ready tasks from GitHub Issues and invokes Claude to execute them.
// Reads all options from Config.
func (s *Stitch) Stitch() error {
	// If invoked from the main repo, enter the generation worktree (GH-1608).
	if _, err := s.enterWorktree(); err != nil {
		return err
	}
	_, err := s.RunStitch()
	return err
}

// RunStitch runs the stitch workflow using Config settings.
func (s *Stitch) RunStitch() (int, error) {
	return s.RunStitchN(s.cfg.Cobbler.MaxStitchIssuesPerCycle)
}

// RunStitchN processes up to n tasks and returns the count completed.
func (s *Stitch) RunStitchN(limit int) (int, error) {
	s.setPhase("stitch")
	defer s.clearPhase()
	stitchStart := time.Now()

	// Start orchestrator log capture.
	if hdir := s.claudeRunner.historyDir(); hdir != "" {
		logPath := filepath.Join(hdir,
			stitchStart.Format("2006-01-02-15-04-05")+"-stitch-orchestrator.log")
		if err := s.openLogSink(logPath); err != nil {
			s.logf("warning: could not open orchestrator log: %v", err)
		} else {
			defer s.closeLogSink()
		}
	}

	s.logf("starting (limit=%d)", limit)
	s.claudeRunner.logConfig("stitch")

	if err := s.claudeRunner.checkClaude(); err != nil {
		return 0, err
	}

	branch, err := s.resolveBranch(s.cfg.Generation.Branch)
	if err != nil {
		s.logf("resolveBranch failed: %v", err)
		return 0, err
	}
	s.logf("resolved branch=%s", branch)
	if s.getGeneration() == "" {
		s.setGeneration(branch)
		defer s.clearGeneration()
	}

	if err := s.ensureOnBranch(branch); err != nil {
		s.logf("ensureOnBranch failed: %v", err)
		return 0, fmt.Errorf("switching to branch: %w", err)
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return 0, fmt.Errorf("getting working directory: %w", err)
	}
	s.logf("repoRoot=%s", repoRoot)

	// Resolve GitHub repo and ensure cobbler labels exist.
	ghRepo, err := s.tracker.DetectGitHubRepo(repoRoot)
	if err != nil {
		s.logf("detectGitHubRepo failed: %v", err)
		return 0, fmt.Errorf("detecting GitHub repo: %w", err)
	}
	generation := branch
	s.logf("using GitHub repo %s generation %s for issues", ghRepo, generation)
	if err := s.tracker.EnsureCobblerLabels(ghRepo); err != nil {
		s.logf("ensureCobblerLabels warning: %v", err)
	}

	worktreeBase := claude.WorktreeBasePath()
	s.logf("worktreeBase=%s", worktreeBase)

	baseBranch, err := s.git.CurrentBranch(".")
	if err != nil {
		return 0, fmt.Errorf("getting current branch: %w", err)
	}
	s.logf("baseBranch=%s", baseBranch)

	s.logf("recovering stale tasks")
	if err := s.recoverStaleTasks(baseBranch, worktreeBase, ghRepo, generation); err != nil {
		s.logf("recovery failed: %v", err)
		return 0, fmt.Errorf("recovery: %w", err)
	}

	totalTasks := 0
	maxFailures := s.cfg.Cobbler.MaxTaskFailures
	// failedTaskCounts tracks how many times each task has failed in this cycle.
	failedTaskCounts := map[string]int{}
	for {
		if limit > 0 && totalTasks >= limit {
			s.logf("reached per-cycle limit (%d), pausing for measure", limit)
			break
		}

		s.logf("looking for next ready task (completed %d so far)", totalTasks)
		task, err := s.pickTask(baseBranch, worktreeBase, ghRepo, generation)
		if err != nil {
			s.logf("no more tasks: %v", err)
			break
		}

		// If this task has already failed in the current cycle, skip it.
		// With maxFailures > 1 a task can be retried, but once it reaches the
		// limit it was closed as permanently failed and should not be picked
		// again. If it is (race with label removal), stop the loop.
		if count := failedTaskCounts[task.ID]; count > 0 {
			s.logf("task %s already failed %d time(s) this cycle, stopping stitch", task.ID, count)
			break
		}

		taskStart := time.Now()
		s.logf("executing task %d: id=%s title=%q", totalTasks+1, task.ID, task.Title)
		if err := s.doOneTask(task, baseBranch, repoRoot); err != nil {
			if errors.Is(err, errTaskReset) {
				// Check whether this failure was dominated by rate limit
				// waits. If so, don't count it toward the failure limit
				// and use a longer backoff (GH-1805).
				var rlErr *rateLimitResetError
				rateLimited := errors.As(err, &rlErr) && rlErr.isRateLimitDominated()

				if rateLimited {
					s.logf("task %s was reset after %s (rate-limited %ds/%ds, not counted as failure)",
						task.ID, time.Since(taskStart).Round(time.Second),
						rlErr.RateLimitWaitS, rlErr.TotalDurationS)
				} else {
					failedTaskCounts[task.ID]++
					count := failedTaskCounts[task.ID]
					s.logf("task %s was reset after %s (failure %d/%d)", task.ID, time.Since(taskStart).Round(time.Second), count, maxFailures)

					// Skip the task once it exceeds the retry limit (GH-1699).
					// Label it cobbler-skipped so pickReadyIssue excludes it, and
					// continue to the next task instead of halting the generation.
					if maxFailures > 0 && count >= maxFailures {
						s.logf("task %s reached max failures (%d), marking as skipped", task.ID, maxFailures)
						s.skipTask(task, count)
					}
				}

				// Back off before the next iteration. Use a longer backoff
				// when rate-limited to allow the API window to reset (GH-1805).
				var backoff time.Duration
				if rateLimited {
					rlBackoff := s.cfg.Cobbler.RateLimitBackoffSeconds
					if rlBackoff <= 0 {
						rlBackoff = 60
					}
					backoff = time.Duration(rlBackoff) * time.Second
				} else {
					count := failedTaskCounts[task.ID]
					backoff = time.Duration(count) * 2 * time.Second
				}
				s.logf("task %s: backing off %s before next task", task.ID, backoff)
				stitchSleep(backoff)
				continue
			}
			s.logf("task %s failed after %s: %v", task.ID, time.Since(taskStart).Round(time.Second), err)
			return totalTasks, fmt.Errorf("executing task %s: %w", task.ID, err)
		}
		s.logf("task %s completed in %s", task.ID, time.Since(taskStart).Round(time.Second))

		totalTasks++
	}

	s.logf("completed %d task(s) in %s", totalTasks, time.Since(stitchStart).Round(time.Second))
	return totalTasks, nil
}

// recoverStaleTasks cleans up task branches and orphaned in_progress issues
// from a previous interrupted run.
func (s *Stitch) recoverStaleTasks(baseBranch, worktreeBase, repo, generation string) error {
	s.logf("recoverStaleTasks: checking for stale branches with pattern %s", taskBranchPattern(baseBranch))
	staleBranches := s.recoverStaleBranches(baseBranch, worktreeBase, repo)

	s.logf("recoverStaleTasks: checking for orphaned in_progress issues")
	orphanedIssues := s.resetOrphanedIssues(baseBranch, repo, generation)

	s.logf("recoverStaleTasks: pruning worktrees")
	if err := s.git.WorktreePrune("."); err != nil {
		s.logf("recoverStaleTasks: worktree prune warning: %v", err)
	}

	if staleBranches || orphanedIssues {
		s.logf("recoverStaleTasks: recovered stale state (branches=%v orphans=%v)", staleBranches, orphanedIssues)
	} else {
		s.logf("recoverStaleTasks: no stale state found")
	}

	return nil
}

func (s *Stitch) doOneTask(task stitchTask, baseBranch, repoRoot string) error {
	taskStart := time.Now()
	s.logf("doOneTask: starting task %s (%s)", task.ID, task.Title)

	s.logf("doOneTask: task #%d claimed via pickReadyIssue label", task.GhNumber)

	// Pre-execution dedup: skip tasks whose R-items were already completed
	// by an earlier task in the same measure batch (GH-1434).
	reqStates := generate.LoadRequirementStates(s.cfg.Cobbler.Dir)
	if generate.AllRefsAlreadyComplete(task.Description, reqStates) {
		s.logf("doOneTask: all R-items for #%d already complete, closing as duplicate", task.GhNumber)
		s.tracker.CommentCobblerIssue(task.Repo, task.GhNumber,
			"Stitch skipped: all targeted R-items are already complete (duplicate from same measure batch).")
		if err := s.tracker.CloseCobblerIssue(task.Repo, task.GhNumber, task.Generation); err != nil {
			s.logf("doOneTask: warning closing duplicate #%d: %v", task.GhNumber, err)
		}
		return nil
	}

	// Create worktree.
	s.logf("doOneTask: creating worktree for %s", task.ID)
	wtStart := time.Now()
	if err := s.createWorktree(task); err != nil {
		s.logf("doOneTask: createWorktree failed after %s: %v", time.Since(wtStart).Round(time.Second), err)
		return fmt.Errorf("creating worktree: %w", err)
	}
	s.logf("doOneTask: worktree created in %s", time.Since(wtStart).Round(time.Second))

	// Snapshot LOC before Claude.
	locBefore := s.claudeRunner.captureLOC()
	s.logf("doOneTask: locBefore prod=%d test=%d", locBefore.Production, locBefore.Test)

	// Build and run prompt.
	prompt, promptErr := s.buildStitchPrompt(task)
	if promptErr != nil {
		s.failTask(task, "prompt build failure", taskStart)
		return promptErr
	}
	s.logf("doOneTask: prompt built, length=%d bytes", len(prompt))

	// Post "started" comment so the issue reflects pickup immediately.
	s.tracker.CommentCobblerIssue(task.Repo, task.GhNumber, fmt.Sprintf(
		"Stitch started. Branch: `%s`, prompt: %d bytes.", task.BranchName, len(prompt)))

	// Save prompt BEFORE calling Claude.
	historyTS := time.Now().Format("2006-01-02-15-04-05")
	s.claudeRunner.saveHistoryPrompt(historyTS, "stitch", prompt)

	s.logf("doOneTask: invoking Claude for task %s", task.ID)
	claudeStart := time.Now()
	tokens, claudeErr := s.claudeRunner.runClaude(prompt, task.WorktreeDir, s.cfg.Silence())

	// Save Claude log immediately.
	s.claudeRunner.saveHistoryLog(historyTS, "stitch", tokens.RawOutput)

	if claudeErr != nil {
		durS := int(time.Since(taskStart).Seconds())
		rlWaitS := tokens.RateLimitWaitS
		s.logf("doOneTask: Claude failed for %s after %s (rate_limit_wait=%ds): %v",
			task.ID, time.Since(claudeStart).Round(time.Second), rlWaitS, claudeErr)
		s.claudeRunner.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
			Caller:         "stitch",
			TaskID:         task.ID,
			TaskTitle:      task.Title,
			Status:         "failed",
			Error:          fmt.Sprintf("claude failure: %v", claudeErr),
			StartedAt:      claudeStart.UTC().Format(time.RFC3339),
			Duration:       time.Since(taskStart).Round(time.Second).String(),
			DurationS:      durS,
			RateLimitWaitS: rlWaitS,
			Tokens:         claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:        tokens.CostUSD,
			LOCBefore:      locBefore,
		})
		s.failTask(task, "Claude failure", taskStart)
		if rlWaitS > 0 {
			return &rateLimitResetError{RateLimitWaitS: rlWaitS, TotalDurationS: durS}
		}
		return errTaskReset
	}
	s.logf("doOneTask: Claude completed for %s in %s", task.ID, time.Since(claudeStart).Round(time.Second))

	// Commit Claude's changes in the worktree.
	if err := commitWorktreeChanges(task); err != nil {
		s.logf("doOneTask: worktree commit failed for %s: %v", task.ID, err)
		s.claudeRunner.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
			Caller:    "stitch",
			TaskID:    task.ID,
			TaskTitle: task.Title,
			Status:    "failed",
			Error:     fmt.Sprintf("worktree commit failure: %v", err),
			StartedAt: claudeStart.UTC().Format(time.RFC3339),
			Duration:  time.Since(taskStart).Round(time.Second).String(),
			DurationS: int(time.Since(taskStart).Seconds()),
			Tokens:    claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
		})
		s.failTask(task, "worktree commit failure", taskStart)
		return errTaskReset
	}

	// Capture locAfter from the worktree before merging.
	locAfter := s.claudeRunner.captureLOCAt(task.WorktreeDir)
	s.logf("doOneTask: locAfter prod=%d test=%d", locAfter.Production, locAfter.Test)

	// Append outcome trailers to the worktree commit before merging.
	trailerRec := claude.InvocationRecord{
		Caller:    "stitch",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		DurationS: int(time.Since(claudeStart).Seconds()),
		Tokens: claude.ClaudeTokens{
			Input:         tokens.InputTokens,
			Output:        tokens.OutputTokens,
			CacheCreation: tokens.CacheCreationTokens,
			CacheRead:     tokens.CacheReadTokens,
			CostUSD:       tokens.CostUSD,
		},
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
		NumTurns:  tokens.NumTurns,
	}
	if err := claude.AppendOutcomeTrailers(task.WorktreeDir, trailerRec, s.git.CommitAmendTrailers); err != nil {
		s.logf("doOneTask: outcome trailer warning for %s: %v", task.ID, err)
	}

	// Capture pre-merge HEAD for diffstat.
	preMergeRef, err := s.git.RevParseHEAD(".")
	if err != nil {
		s.logf("doOneTask: warning getting pre-merge ref: %v", err)
	}

	// Merge branch back.
	s.logf("doOneTask: merging %s into %s", task.BranchName, baseBranch)
	mergeStart := time.Now()
	if err := s.mergeBranch(task.BranchName, baseBranch, repoRoot); err != nil {
		s.logf("doOneTask: merge failed for %s after %s: %v", task.ID, time.Since(mergeStart).Round(time.Second), err)
		s.claudeRunner.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
			Caller:    "stitch",
			TaskID:    task.ID,
			TaskTitle: task.Title,
			Status:    "failed",
			Error:     fmt.Sprintf("merge failure: %v", err),
			StartedAt: claudeStart.UTC().Format(time.RFC3339),
			Duration:  time.Since(taskStart).Round(time.Second).String(),
			DurationS: int(time.Since(taskStart).Seconds()),
			Tokens:    claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
		})
		s.failTask(task, "merge failure", taskStart)
		return errTaskReset
	}
	s.logf("doOneTask: merge completed in %s", time.Since(mergeStart).Round(time.Second))

	// Capture per-file diff stats.
	diff, diffErr := s.git.DiffShortstat(preMergeRef, ".")
	if diffErr != nil {
		s.logf("doOneTask: warning getting diff shortstat: %v", diffErr)
	}
	s.logf("doOneTask: diff files=%d ins=%d del=%d", diff.FilesChanged, diff.Insertions, diff.Deletions)
	fileChanges, fcErr := s.diffNameStatus(preMergeRef, ".")
	if fcErr != nil {
		s.logf("doOneTask: warning getting file changes: %v", fcErr)
	}
	s.logf("doOneTask: fileChanges=%d entries", len(fileChanges))

	// Run post-merge tests to determine requirement completion status (GH-1388).
	testsPassed := runPostMergeTests(".")
	if !testsPassed {
		s.logf("doOneTask: post-merge tests failed for %s, R-items will be marked complete_with_failures", task.ID)
	}

	// Cleanup worktree.
	s.logf("doOneTask: cleaning up worktree for %s", task.ID)
	s.cleanupWorktree(task)

	// Save stitch stats.
	taskDuration := time.Since(taskStart)
	s.claudeRunner.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
		Caller:         "stitch",
		TaskID:         task.ID,
		TaskTitle:      task.Title,
		Status:         "success",
		StartedAt:      claudeStart.UTC().Format(time.RFC3339),
		Duration:       taskDuration.Round(time.Second).String(),
		DurationS:      int(taskDuration.Seconds()),
		RateLimitWaitS: tokens.RateLimitWaitS,
		Tokens:         claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
		CostUSD:        tokens.CostUSD,
		NumTurns:       tokens.NumTurns,
		DurationAPIMs:  tokens.DurationAPIMs,
		SessionID:      tokens.SessionID,
		LOCBefore:      locBefore,
		LOCAfter:       locAfter,
		Diff:          claude.HistoryDiff{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
	})

	// Save stitch report with per-file diffstat.
	s.claudeRunner.saveHistoryReport(historyTS, claude.StitchReport{
		TaskID:    task.ID,
		TaskTitle: task.Title,
		Status:    "success",
		Branch:    task.BranchName,
		Diff:      claude.HistoryDiff{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
		Files:     fileChanges,
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
	})

	// Close task with metrics.
	rec := claude.InvocationRecord{
		Caller:    "stitch",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		DurationS: int(taskDuration.Seconds()),
		Tokens:    claude.ClaudeTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens, CostUSD: tokens.CostUSD},
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
		Diff:      claude.DiffRecord{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
		NumTurns:  tokens.NumTurns,
	}
	s.logf("doOneTask: closing task %s", task.ID)
	s.closeStitchTask(task, rec, testsPassed)

	// After closing, sweep remaining open tasks whose R-items are now all
	// complete. A prior task may have over-implemented requirements beyond
	// its assignment; this sweep catches those before the next Claude
	// invocation (GH-1647).
	s.sweepCompletedTasks(task.Repo, task.Generation)

	s.logf("doOneTask: task %s finished in %s", task.ID, time.Since(taskStart).Round(time.Second))
	return nil
}

func (s *Stitch) buildStitchPrompt(task stitchTask) (string, error) {
	tmpl, err := ictx.ParsePromptTemplate(orDefault(s.cfg.Cobbler.StitchPrompt, defaultStitchPrompt))
	if err != nil {
		return "", fmt.Errorf("stitch prompt YAML: %w", err)
	}

	executionConst := orDefault(s.cfg.Cobbler.ExecutionConstitution, executionConstitution)
	goStyleConst := orDefault(s.cfg.Cobbler.GoStyleConstitution, goStyleConstitution)

	// Load per-phase context file (srd003 R9.9).
	stitchCtxPath := filepath.Join(s.cfg.Cobbler.Dir, "stitch_context.yaml")
	phaseCtx, phaseErr := ictx.LoadPhaseContext(stitchCtxPath)
	if phaseErr != nil {
		return "", fmt.Errorf("loading stitch context: %w", phaseErr)
	}
	if phaseCtx != nil {
		s.logf("buildStitchPrompt: using phase context from %s", stitchCtxPath)
	} else {
		s.logf("buildStitchPrompt: no phase context file, using config defaults")
	}

	// Apply stitch_exclude_tests from config (GH-1440).
	if s.cfg.Cobbler.effectiveStitchExcludeTests() {
		if phaseCtx == nil {
			phaseCtx = &PhaseContext{}
		}
		if !phaseCtx.ExcludeTests {
			phaseCtx.ExcludeTests = true
			s.logf("buildStitchPrompt: stitch_exclude_tests=true, _test.go files will be excluded")
		}
	}

	// Exclude SRDs from stitch context — Claude reads them via required_reading
	// instead. This avoids double-delivery (inline + Read tool) and shrinks the
	// prompt by 10-30KB (GH-1464).
	{
		if phaseCtx == nil {
			phaseCtx = &PhaseContext{}
		}
		srdExclude := "docs/specs/software-requirements/srd*.yaml"
		if phaseCtx.Exclude == "" {
			phaseCtx.Exclude = srdExclude
		} else {
			phaseCtx.Exclude = phaseCtx.Exclude + "\n" + srdExclude
		}
	}

	// Build project context from the worktree directory.
	var projectCtx *ProjectContext
	if task.WorktreeDir != "" {
		orig, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("buildStitchPrompt: getwd: %w", err)
		}
		if err := os.Chdir(task.WorktreeDir); err != nil {
			s.logf("buildStitchPrompt: chdir to worktree error: %v", err)
		} else {
			defer os.Chdir(orig)
			scopedProject := s.cfg.Project
			if scoped := scopeSourceDirs(s.cfg.Project.GoSourceDirs, task.Description); len(scoped) > 0 {
				s.logf("buildStitchPrompt: scoped go_source_dirs %v -> %v", s.cfg.Project.GoSourceDirs, scoped)
				scopedProject.GoSourceDirs = scoped
			}
			ctx, ctxErr := buildProjectContext("", scopedProject, phaseCtx)
			if ctxErr != nil {
				s.logf("buildStitchPrompt: buildProjectContext error: %v", ctxErr)
			} else {
				projectCtx = ctx
			}
		}
	}
	s.logf("buildStitchPrompt: projectCtx=%v", projectCtx != nil)

	// Selective stitch context: filter source files to required_reading.
	if projectCtx != nil {
		requiredReading := parseRequiredReading(task.Description)
		var sourcePaths []string
		for _, entry := range requiredReading {
			clean := ictx.StripParenthetical(entry)
			if strings.HasSuffix(clean, ".go") {
				sourcePaths = append(sourcePaths, clean)
			}
		}
		if len(sourcePaths) > 0 {
			before := len(projectCtx.SourceCode)
			projectCtx.SourceCode = ictx.FilterSourceFiles(projectCtx.SourceCode, sourcePaths)
			s.logf("buildStitchPrompt: filtered source files %d -> %d (required_reading has %d source paths)",
				before, len(projectCtx.SourceCode), len(sourcePaths))
		} else {
			s.logf("buildStitchPrompt: no source paths in required_reading, keeping all %d source files",
				len(projectCtx.SourceCode))
		}

		// Context budget enforcement.
		ictx.ApplyContextBudget(projectCtx, s.cfg.Cobbler.MaxContextBytes, sourcePaths)
	}

	taskContext := fmt.Sprintf("Task ID: %s\nType: %s\nTitle: %s",
		task.ID, task.IssueType, task.Title)

	repoFiles := s.git.LsFiles(task.WorktreeDir)

	// Load OOD context.
	oodContracts, oodProtocols := ictx.LoadOODPromptContext()
	if len(oodProtocols) > 0 {
		s.logf("buildStitchPrompt: injecting %d shared_protocols", len(oodProtocols))
	}
	if len(oodContracts) > 0 {
		s.logf("buildStitchPrompt: injecting %d package_contracts", len(oodContracts))
	}

	// Load semantic model from SRD (informational context for stitch).
	semanticModel := ictx.LoadSRDSemanticModel()
	if semanticModel != nil {
		s.logf("buildStitchPrompt: injecting semantic_model from SRD")
	}

	doc := StitchPromptDoc{
		Role:                  tmpl.Role,
		RepositoryFiles:       repoFiles,
		ProjectContext:        projectCtx,
		Context:               taskContext,
		ExecutionConstitution: ictx.ParseYAMLNode(executionConst),
		GoStyleConstitution:   ictx.ParseYAMLNode(goStyleConst),
		Task:                  tmpl.Task,
		Constraints:           tmpl.Constraints,
		Description:           task.Description,
		SemanticModel:         semanticModel,
		SharedProtocols:       oodProtocols,
		PackageContracts:      oodContracts,
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("marshaling stitch prompt: %w", err)
	}

	s.logf("buildStitchPrompt: %d bytes", len(out))
	return string(out), nil
}

func (s *Stitch) closeStitchTask(task stitchTask, rec claude.InvocationRecord, testsPassed bool) {
	s.logf("closeStitchTask: closing #%d %q", task.GhNumber, task.Title)
	locDeltaProd := rec.LOCAfter.Production - rec.LOCBefore.Production
	locDeltaTest := rec.LOCAfter.Test - rec.LOCBefore.Test
	comment := fmt.Sprintf(
		"Stitch completed in %dm %ds. LOC delta: %+d prod, %+d test. Cost: $%.2f. Turns: %d. Tokens: %din %dout.",
		rec.DurationS/60, rec.DurationS%60,
		locDeltaProd, locDeltaTest,
		rec.Tokens.CostUSD,
		rec.NumTurns,
		rec.Tokens.Input, rec.Tokens.Output,
	)
	if !testsPassed {
		comment += " Tests: FAILED."
	}
	s.tracker.CommentCobblerIssue(task.Repo, task.GhNumber, comment)

	// Update requirement states before closing (GH-1378).
	// Pass test result so failures are tracked as complete_with_failures (GH-1388).
	if err := generate.UpdateRequirementsFile(s.cfg.Cobbler.Dir, task.Description, task.GhNumber, testsPassed); err != nil {
		s.logf("closeStitchTask: warning updating requirements: %v", err)
	} else if s.git.HasChanges(".") {
		// Commit requirement state immediately so it survives interruptions (GH-1385).
		_ = s.git.StageAll(".")
		_ = s.git.Commit(fmt.Sprintf("Update requirement states after #%d", task.GhNumber), ".")
	}

	if err := s.tracker.CloseCobblerIssue(task.Repo, task.GhNumber, task.Generation); err != nil {
		s.logf("closeStitchTask: closeCobblerIssue warning for #%d: %v", task.GhNumber, err)
	}
	s.logf("closeStitchTask: #%d closed", task.GhNumber)
}

// sweepCompletedTasks closes open tasks whose R-items are now all complete
// in requirements.yaml. This handles the case where a prior task over-
// implemented requirements beyond its assignment (GH-1647).
func (s *Stitch) sweepCompletedTasks(repo, generation string) {
	reqStates := generate.LoadRequirementStates(s.cfg.Cobbler.Dir)
	if len(reqStates) == 0 {
		return
	}

	issues, err := s.tracker.ListOpenCobblerIssues(repo, generation)
	if err != nil {
		s.logf("sweepCompletedTasks: list issues: %v", err)
		return
	}

	swept := 0
	for _, iss := range issues {
		if !generate.AllRefsAlreadyComplete(iss.Description, reqStates) {
			continue
		}
		s.logf("sweepCompletedTasks: all R-items for #%d already complete, closing", iss.Number)
		s.tracker.CommentCobblerIssue(repo, iss.Number,
			"Stitch skipped: all targeted R-items are already complete (swept after prior task over-implemented).")
		if err := s.tracker.CloseCobblerIssue(repo, iss.Number, generation); err != nil {
			s.logf("sweepCompletedTasks: close #%d warning: %v", iss.Number, err)
		}
		swept++
	}
	if swept > 0 {
		s.logf("sweepCompletedTasks: closed %d already-complete task(s)", swept)
	}
}

// runPostMergeTests runs `go test ./...` in the given directory and returns
// true if all tests pass. Uses a 5-minute timeout to avoid blocking the
// pipeline indefinitely. Returns true on any execution error (fail open) to
// avoid marking R-items as failed due to infrastructure issues.
var runPostMergeTests = func(dir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "runPostMergeTests: tests failed: %v\n%s\n", err, out)
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(os.Stderr, "runPostMergeTests: timed out, treating as passed\n")
			return true
		}
		return false
	}
	fmt.Fprintf(os.Stderr, "runPostMergeTests: all tests passed\n")
	return true
}

// resetTask removes the in-progress label from a failed task, cleans up its
// worktree and branch.
func (s *Stitch) resetTask(task stitchTask, reason string) {
	s.logf("resetTask: resetting #%d to ready (%s)", task.GhNumber, reason)
	if err := s.tracker.RemoveIssueLabel(task.Repo, task.GhNumber, gh.LabelInProgress); err != nil {
		s.logf("resetTask: WARNING removeInProgressLabel failed for #%d: %v", task.GhNumber, err)
	}
	if !s.cleanupWorktree(task) {
		s.logf("resetTask: skipping force branch delete for %s (worktree not removed)", task.BranchName)
		return
	}
	if err := s.git.ForceDeleteBranch(task.BranchName, "."); err != nil {
		s.logf("resetTask: WARNING force branch delete failed for %s: %v", task.BranchName, err)
	}
}

// closeTaskAsFailed closes a task as permanently failed after exceeding the
// maximum failure count. Posts a comment and closes the issue so measure can
// create a replacement if needed (GH-1562).
// skipTask labels a task as cobbler-skipped after it exceeds the retry limit
// (GH-1699). The task remains open but is excluded from future pickReadyIssue
// calls. The generation continues with the next available task.
func (s *Stitch) skipTask(task stitchTask, failureCount int) {
	comment := fmt.Sprintf(
		"Stitch skipped after %d consecutive failures. Task labeled cobbler-skipped and excluded from future picks (GH-1699).",
		failureCount,
	)
	s.tracker.CommentCobblerIssue(task.Repo, task.GhNumber, comment)

	// Remove in-progress and ready labels, add skipped label.
	if err := s.tracker.RemoveIssueLabel(task.Repo, task.GhNumber, gh.LabelInProgress); err != nil {
		s.logf("skipTask: remove in-progress label from #%d: %v", task.GhNumber, err)
	}
	if err := s.tracker.RemoveIssueLabel(task.Repo, task.GhNumber, gh.LabelReady); err != nil {
		s.logf("skipTask: remove ready label from #%d: %v", task.GhNumber, err)
	}
	if err := s.tracker.AddIssueLabel(task.Repo, task.GhNumber, gh.LabelSkipped); err != nil {
		s.logf("skipTask: add skipped label to #%d: %v", task.GhNumber, err)
	}
}

// stitchSleep is the sleep function used for backoff between failed tasks.
// It is a package-level var so tests can replace it.
var stitchSleep = time.Sleep

// failTask posts a failure comment on the task issue, then resets it.
func (s *Stitch) failTask(task stitchTask, reason string, startedAt time.Time) {
	durationS := int(time.Since(startedAt).Seconds())
	comment := fmt.Sprintf(
		"Stitch failed after %dm %ds. Error: %s.",
		durationS/60, durationS%60, reason,
	)
	s.tracker.CommentCobblerIssue(task.Repo, task.GhNumber, comment)
	s.resetTask(task, reason)
}
