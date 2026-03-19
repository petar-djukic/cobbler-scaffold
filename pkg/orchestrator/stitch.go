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
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
	"gopkg.in/yaml.v3"
)

//go:embed prompts/stitch.yaml
var defaultStitchPrompt string

//go:embed constitutions/execution.yaml
var executionConstitution string

//go:embed constitutions/go-style.yaml
var goStyleConstitution string

// Stitch picks ready tasks from GitHub Issues and invokes Claude to execute them.
// Reads all options from Config.
func (o *Orchestrator) Stitch() error {
	// If invoked from the main repo, enter the generation worktree (GH-1608).
	if _, err := enterGenerationWorktree(); err != nil {
		return err
	}
	_, err := o.RunStitch()
	return err
}

// RunStitch runs the stitch workflow using Config settings.
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

	worktreeBase := claude.WorktreeBasePath()
	logf("worktreeBase=%s", worktreeBase)

	baseBranch, err := defaultGitOps.CurrentBranch(".")
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
	maxFailures := o.cfg.Cobbler.MaxTaskFailures
	// failedTaskCounts tracks how many times each task has failed in this cycle.
	failedTaskCounts := map[string]int{}
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

		// If this task has already failed in the current cycle, skip it.
		// With maxFailures > 1 a task can be retried, but once it reaches the
		// limit it was closed as permanently failed and should not be picked
		// again. If it is (race with label removal), stop the loop.
		if count := failedTaskCounts[task.ID]; count > 0 {
			logf("task %s already failed %d time(s) this cycle, stopping stitch", task.ID, count)
			break
		}

		taskStart := time.Now()
		logf("executing task %d: id=%s title=%q", totalTasks+1, task.ID, task.Title)
		if err := o.doOneTask(task, baseBranch, repoRoot); err != nil {
			if errors.Is(err, errTaskReset) {
				failedTaskCounts[task.ID]++
				count := failedTaskCounts[task.ID]
				logf("task %s was reset after %s (failure %d/%d)", task.ID, time.Since(taskStart).Round(time.Second), count, maxFailures)

				// Close the task as permanently failed once the limit is reached.
				if maxFailures > 0 && count >= maxFailures {
					logf("task %s reached max failures (%d), closing as permanently failed", task.ID, maxFailures)
					o.closeTaskAsFailed(task, count)
				}

				// Back off before the next iteration to reduce API pressure.
				backoff := time.Duration(count) * 2 * time.Second
				logf("task %s: backing off %s before next task", task.ID, backoff)
				stitchSleep(backoff)
				continue
			}
			logf("task %s failed after %s: %v", task.ID, time.Since(taskStart).Round(time.Second), err)
			return totalTasks, fmt.Errorf("executing task %s: %w", task.ID, err)
		}
		logf("task %s completed in %s", task.ID, time.Since(taskStart).Round(time.Second))

		totalTasks++
	}

	logf("completed %d task(s) in %s", totalTasks, time.Since(stitchStart).Round(time.Second))
	return totalTasks, nil
}

// recoverStaleTasks cleans up task branches and orphaned in_progress issues
// from a previous interrupted run.
func (o *Orchestrator) recoverStaleTasks(baseBranch, worktreeBase, repo, generation string) error {
	logf("recoverStaleTasks: checking for stale branches with pattern %s", taskBranchPattern(baseBranch))
	staleBranches := recoverStaleBranches(baseBranch, worktreeBase, repo)

	logf("recoverStaleTasks: checking for orphaned in_progress issues")
	orphanedIssues := resetOrphanedIssues(baseBranch, repo, generation)

	logf("recoverStaleTasks: pruning worktrees")
	if err := defaultGitOps.WorktreePrune("."); err != nil {
		logf("recoverStaleTasks: worktree prune warning: %v", err)
	}

	if staleBranches || orphanedIssues {
		logf("recoverStaleTasks: recovered stale state (branches=%v orphans=%v)", staleBranches, orphanedIssues)
	} else {
		logf("recoverStaleTasks: no stale state found")
	}

	return nil
}

func (o *Orchestrator) doOneTask(task stitchTask, baseBranch, repoRoot string) error {
	taskStart := time.Now()
	logf("doOneTask: starting task %s (%s)", task.ID, task.Title)

	logf("doOneTask: task #%d claimed via pickReadyIssue label", task.GhNumber)

	// Pre-execution dedup: skip tasks whose R-items were already completed
	// by an earlier task in the same measure batch (GH-1434).
	reqStates := generate.LoadRequirementStates(o.cfg.Cobbler.Dir)
	if generate.AllRefsAlreadyComplete(task.Description, reqStates) {
		logf("doOneTask: all R-items for #%d already complete, closing as duplicate", task.GhNumber)
		commentCobblerIssue(task.Repo, task.GhNumber,
			"Stitch skipped: all targeted R-items are already complete (duplicate from same measure batch).")
		if err := closeCobblerIssue(task.Repo, task.GhNumber, task.Generation); err != nil {
			logf("doOneTask: warning closing duplicate #%d: %v", task.GhNumber, err)
		}
		return nil
	}

	// Create worktree.
	logf("doOneTask: creating worktree for %s", task.ID)
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
		o.failTask(task, "prompt build failure", taskStart)
		return promptErr
	}
	logf("doOneTask: prompt built, length=%d bytes", len(prompt))

	// Post "started" comment so the issue reflects pickup immediately.
	commentCobblerIssue(task.Repo, task.GhNumber, fmt.Sprintf(
		"Stitch started. Branch: `%s`, prompt: %d bytes.", task.BranchName, len(prompt)))

	// Save prompt BEFORE calling Claude.
	historyTS := time.Now().Format("2006-01-02-15-04-05")
	o.saveHistoryPrompt(historyTS, "stitch", prompt)

	logf("doOneTask: invoking Claude for task %s", task.ID)
	claudeStart := time.Now()
	tokens, claudeErr := o.runClaude(prompt, task.WorktreeDir, o.cfg.Silence())

	// Save Claude log immediately.
	o.saveHistoryLog(historyTS, "stitch", tokens.RawOutput)

	if claudeErr != nil {
		logf("doOneTask: Claude failed for %s after %s: %v", task.ID, time.Since(claudeStart).Round(time.Second), claudeErr)
		o.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
			Caller:    "stitch",
			TaskID:    task.ID,
			TaskTitle: task.Title,
			Status:    "failed",
			Error:     fmt.Sprintf("claude failure: %v", claudeErr),
			StartedAt: claudeStart.UTC().Format(time.RFC3339),
			Duration:  time.Since(taskStart).Round(time.Second).String(),
			DurationS: int(time.Since(taskStart).Seconds()),
			Tokens:    claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
		})
		o.failTask(task, "Claude failure", taskStart)
		return errTaskReset
	}
	logf("doOneTask: Claude completed for %s in %s", task.ID, time.Since(claudeStart).Round(time.Second))

	// Commit Claude's changes in the worktree.
	if err := commitWorktreeChanges(task); err != nil {
		logf("doOneTask: worktree commit failed for %s: %v", task.ID, err)
		o.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
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
		o.failTask(task, "worktree commit failure", taskStart)
		return errTaskReset
	}

	// Capture locAfter from the worktree before merging.
	locAfter := o.captureLOCAt(task.WorktreeDir)
	logf("doOneTask: locAfter prod=%d test=%d", locAfter.Production, locAfter.Test)

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
	if err := claude.AppendOutcomeTrailers(task.WorktreeDir, trailerRec, defaultGitOps.CommitAmendTrailers); err != nil {
		logf("doOneTask: outcome trailer warning for %s: %v", task.ID, err)
	}

	// Capture pre-merge HEAD for diffstat.
	preMergeRef, err := defaultGitOps.RevParseHEAD(".")
	if err != nil {
		logf("doOneTask: warning getting pre-merge ref: %v", err)
	}

	// Merge branch back.
	logf("doOneTask: merging %s into %s", task.BranchName, baseBranch)
	mergeStart := time.Now()
	if err := mergeBranch(task.BranchName, baseBranch, repoRoot); err != nil {
		logf("doOneTask: merge failed for %s after %s: %v", task.ID, time.Since(mergeStart).Round(time.Second), err)
		o.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
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
		o.failTask(task, "merge failure", taskStart)
		return errTaskReset
	}
	logf("doOneTask: merge completed in %s", time.Since(mergeStart).Round(time.Second))

	// Capture per-file diff stats.
	diff, diffErr := defaultGitOps.DiffShortstat(preMergeRef, ".")
	if diffErr != nil {
		logf("doOneTask: warning getting diff shortstat: %v", diffErr)
	}
	logf("doOneTask: diff files=%d ins=%d del=%d", diff.FilesChanged, diff.Insertions, diff.Deletions)
	fileChanges, fcErr := diffNameStatus(preMergeRef, ".")
	if fcErr != nil {
		logf("doOneTask: warning getting file changes: %v", fcErr)
	}
	logf("doOneTask: fileChanges=%d entries", len(fileChanges))

	// Run post-merge tests to determine requirement completion status (GH-1388).
	testsPassed := runPostMergeTests(".")
	if !testsPassed {
		logf("doOneTask: post-merge tests failed for %s, R-items will be marked complete_with_failures", task.ID)
	}

	// Cleanup worktree.
	logf("doOneTask: cleaning up worktree for %s", task.ID)
	cleanupWorktree(task)

	// Save stitch stats.
	taskDuration := time.Since(taskStart)
	o.saveHistoryStats(historyTS, "stitch", claude.HistoryStats{
		Caller:        "stitch",
		TaskID:        task.ID,
		TaskTitle:     task.Title,
		Status:        "success",
		StartedAt:     claudeStart.UTC().Format(time.RFC3339),
		Duration:      taskDuration.Round(time.Second).String(),
		DurationS:     int(taskDuration.Seconds()),
		Tokens:        claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
		CostUSD:       tokens.CostUSD,
		NumTurns:      tokens.NumTurns,
		DurationAPIMs: tokens.DurationAPIMs,
		SessionID:     tokens.SessionID,
		LOCBefore:     locBefore,
		LOCAfter:      locAfter,
		Diff:          claude.HistoryDiff{Files: diff.FilesChanged, Insertions: diff.Insertions, Deletions: diff.Deletions},
	})

	// Save stitch report with per-file diffstat.
	o.saveHistoryReport(historyTS, claude.StitchReport{
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
	logf("doOneTask: closing task %s", task.ID)
	o.closeStitchTask(task, rec, testsPassed)

	// After closing, sweep remaining open tasks whose R-items are now all
	// complete. A prior task may have over-implemented requirements beyond
	// its assignment; this sweep catches those before the next Claude
	// invocation (GH-1647).
	o.sweepCompletedTasks(task.Repo, task.Generation)

	logf("doOneTask: task %s finished in %s", task.ID, time.Since(taskStart).Round(time.Second))
	return nil
}

func (o *Orchestrator) buildStitchPrompt(task stitchTask) (string, error) {
	tmpl, err := parsePromptTemplate(orDefault(o.cfg.Cobbler.StitchPrompt, defaultStitchPrompt))
	if err != nil {
		return "", fmt.Errorf("stitch prompt YAML: %w", err)
	}

	executionConst := orDefault(o.cfg.Cobbler.ExecutionConstitution, executionConstitution)
	goStyleConst := orDefault(o.cfg.Cobbler.GoStyleConstitution, goStyleConstitution)

	// Load per-phase context file (prd003 R9.9).
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

	// Apply stitch_exclude_tests from config (GH-1440).
	if o.cfg.Cobbler.effectiveStitchExcludeTests() {
		if phaseCtx == nil {
			phaseCtx = &PhaseContext{}
		}
		if !phaseCtx.ExcludeTests {
			phaseCtx.ExcludeTests = true
			logf("buildStitchPrompt: stitch_exclude_tests=true, _test.go files will be excluded")
		}
	}

	// Exclude PRDs from stitch context — Claude reads them via required_reading
	// instead. This avoids double-delivery (inline + Read tool) and shrinks the
	// prompt by 10-30KB (GH-1464).
	{
		if phaseCtx == nil {
			phaseCtx = &PhaseContext{}
		}
		prdExclude := "docs/specs/product-requirements/prd*.yaml"
		if phaseCtx.Exclude == "" {
			phaseCtx.Exclude = prdExclude
		} else {
			phaseCtx.Exclude = phaseCtx.Exclude + "\n" + prdExclude
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
			logf("buildStitchPrompt: chdir to worktree error: %v", err)
		} else {
			defer os.Chdir(orig)
			scopedProject := o.cfg.Project
			if scoped := scopeSourceDirs(o.cfg.Project.GoSourceDirs, task.Description); len(scoped) > 0 {
				logf("buildStitchPrompt: scoped go_source_dirs %v -> %v", o.cfg.Project.GoSourceDirs, scoped)
				scopedProject.GoSourceDirs = scoped
			}
			ctx, ctxErr := buildProjectContext("", scopedProject, phaseCtx)
			if ctxErr != nil {
				logf("buildStitchPrompt: buildProjectContext error: %v", ctxErr)
			} else {
				projectCtx = ctx
			}
		}
	}
	logf("buildStitchPrompt: projectCtx=%v", projectCtx != nil)

	// Selective stitch context: filter source files to required_reading.
	if projectCtx != nil {
		requiredReading := parseRequiredReading(task.Description)
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

		// Context budget enforcement.
		applyContextBudget(projectCtx, o.cfg.Cobbler.MaxContextBytes, sourcePaths)
	}

	taskContext := fmt.Sprintf("Task ID: %s\nType: %s\nTitle: %s",
		task.ID, task.IssueType, task.Title)

	repoFiles := defaultGitOps.LsFiles(task.WorktreeDir)

	// Load OOD context.
	oodContracts, oodProtocols := loadOODPromptContext()
	if len(oodProtocols) > 0 {
		logf("buildStitchPrompt: injecting %d shared_protocols", len(oodProtocols))
	}
	if len(oodContracts) > 0 {
		logf("buildStitchPrompt: injecting %d package_contracts", len(oodContracts))
	}

	// Load semantic model from PRD (informational context for stitch).
	semanticModel := loadPRDSemanticModel()
	if semanticModel != nil {
		logf("buildStitchPrompt: injecting semantic_model from PRD")
	}

	doc := StitchPromptDoc{
		Role:                  tmpl.Role,
		RepositoryFiles:       repoFiles,
		ProjectContext:        projectCtx,
		Context:               taskContext,
		ExecutionConstitution: parseYAMLNode(executionConst),
		GoStyleConstitution:   parseYAMLNode(goStyleConst),
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

	logf("buildStitchPrompt: %d bytes", len(out))
	return string(out), nil
}

func (o *Orchestrator) closeStitchTask(task stitchTask, rec claude.InvocationRecord, testsPassed bool) {
	logf("closeStitchTask: closing #%d %q", task.GhNumber, task.Title)
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
	commentCobblerIssue(task.Repo, task.GhNumber, comment)

	// Update requirement states before closing (GH-1378).
	// Pass test result so failures are tracked as complete_with_failures (GH-1388).
	if err := generate.UpdateRequirementsFile(o.cfg.Cobbler.Dir, task.Description, task.GhNumber, testsPassed); err != nil {
		logf("closeStitchTask: warning updating requirements: %v", err)
	} else if defaultGitOps.HasChanges(".") {
		// Commit requirement state immediately so it survives interruptions (GH-1385).
		_ = defaultGitOps.StageAll(".")
		_ = defaultGitOps.Commit(fmt.Sprintf("Update requirement states after #%d", task.GhNumber), ".")
	}

	if err := closeCobblerIssue(task.Repo, task.GhNumber, task.Generation); err != nil {
		logf("closeStitchTask: closeCobblerIssue warning for #%d: %v", task.GhNumber, err)
	}
	logf("closeStitchTask: #%d closed", task.GhNumber)
}

// sweepCompletedTasks closes open tasks whose R-items are now all complete
// in requirements.yaml. This handles the case where a prior task over-
// implemented requirements beyond its assignment (GH-1647).
func (o *Orchestrator) sweepCompletedTasks(repo, generation string) {
	reqStates := generate.LoadRequirementStates(o.cfg.Cobbler.Dir)
	if len(reqStates) == 0 {
		return
	}

	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		logf("sweepCompletedTasks: list issues: %v", err)
		return
	}

	swept := 0
	for _, iss := range issues {
		if !generate.AllRefsAlreadyComplete(iss.Description, reqStates) {
			continue
		}
		logf("sweepCompletedTasks: all R-items for #%d already complete, closing", iss.Number)
		commentCobblerIssue(repo, iss.Number,
			"Stitch skipped: all targeted R-items are already complete (swept after prior task over-implemented).")
		if err := closeCobblerIssue(repo, iss.Number, generation); err != nil {
			logf("sweepCompletedTasks: close #%d warning: %v", iss.Number, err)
		}
		swept++
	}
	if swept > 0 {
		logf("sweepCompletedTasks: closed %d already-complete task(s)", swept)
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
		logf("runPostMergeTests: tests failed: %v\n%s", err, out)
		if ctx.Err() == context.DeadlineExceeded {
			logf("runPostMergeTests: timed out, treating as passed")
			return true
		}
		return false
	}
	logf("runPostMergeTests: all tests passed")
	return true
}

// resetTask removes the in-progress label from a failed task, cleans up its
// worktree and branch.
func (o *Orchestrator) resetTask(task stitchTask, reason string) {
	logf("resetTask: resetting #%d to ready (%s)", task.GhNumber, reason)
	if err := removeInProgressLabel(task.Repo, task.GhNumber); err != nil {
		logf("resetTask: WARNING removeInProgressLabel failed for #%d: %v", task.GhNumber, err)
	}
	if !cleanupWorktree(task) {
		logf("resetTask: skipping force branch delete for %s (worktree not removed)", task.BranchName)
		return
	}
	if err := defaultGitOps.ForceDeleteBranch(task.BranchName, "."); err != nil {
		logf("resetTask: WARNING force branch delete failed for %s: %v", task.BranchName, err)
	}
}

// closeTaskAsFailed closes a task as permanently failed after exceeding the
// maximum failure count. Posts a comment and closes the issue so measure can
// create a replacement if needed (GH-1562).
func (o *Orchestrator) closeTaskAsFailed(task stitchTask, failureCount int) {
	comment := fmt.Sprintf(
		"Stitch abandoned after %d consecutive failures. Closing as permanently failed (GH-1562).",
		failureCount,
	)
	commentCobblerIssue(task.Repo, task.GhNumber, comment)
	if err := closeCobblerIssue(task.Repo, task.GhNumber, task.Generation); err != nil {
		logf("closeTaskAsFailed: warning closing #%d: %v", task.GhNumber, err)
	}
}

// stitchSleep is the sleep function used for backoff between failed tasks.
// It is a package-level var so tests can replace it.
var stitchSleep = time.Sleep

// failTask posts a failure comment on the task issue, then resets it.
func (o *Orchestrator) failTask(task stitchTask, reason string, startedAt time.Time) {
	durationS := int(time.Since(startedAt).Seconds())
	comment := fmt.Sprintf(
		"Stitch failed after %dm %ds. Error: %s.",
		durationS/60, durationS%60, reason,
	)
	commentCobblerIssue(task.Repo, task.GhNumber, comment)
	o.resetTask(task, reason)
}
