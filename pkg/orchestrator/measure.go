// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
	"gopkg.in/yaml.v3"
)

// Measure provides the measure workflow: assessing project state and
// proposing new tasks via Claude.
type Measure struct {
	cfg          Config
	logf         func(string, ...any)
	git          gitops.GitOps
	tracker      gh.WorkTracker
	claudeRunner *ClaudeRunner
	analyzer     *Analyzer
	// Orchestrator state callbacks.
	setPhase        func(string)
	clearPhase      func()
	setGeneration   func(string)
	clearGeneration func()
	getGeneration   func() string
	openLogSink     func(string) error
	closeLogSink    func()
	// Generator helpers.
	resolveBranch  func(string) (string, error)
	ensureOnBranch func(string) error
	enterWorktree  func() (string, error)
	hasUnresolved  func() bool
}

// NewMeasure creates a Measure with explicit dependencies.
func NewMeasure(o *Orchestrator) *Measure {
	return &Measure{
		cfg:             o.cfg,
		logf:            o.logf,
		git:             o.git,
		tracker:         o.tracker,
		claudeRunner:    o.ClaudeRunner,
		analyzer:        o.Analyzer,
		setPhase:        o.setPhase,
		clearPhase:      o.clearPhase,
		setGeneration:   o.setGeneration,
		clearGeneration: o.clearGeneration,
		getGeneration:   o.getGeneration,
		openLogSink:     o.openLogSink,
		closeLogSink:    o.closeLogSink,
		resolveBranch:   o.Generator.resolveBranch,
		ensureOnBranch:  o.Generator.ensureOnBranch,
		enterWorktree:   o.Generator.enterGenerationWorktree,
		hasUnresolved:   o.Generator.hasUnresolvedRequirements,
	}
}

//go:embed prompts/measure.yaml
var defaultMeasurePrompt string

//go:embed constitutions/planning.yaml
var planningConstitution string

//go:embed constitutions/issue-format.yaml
var issueFormatConstitution string

// Measure assesses project state and proposes new tasks via Claude.
// Reads all options from Config.
func (m *Measure) Measure() error {
	// If invoked from the main repo, enter the generation worktree (GH-1608).
	if _, err := m.enterWorktree(); err != nil {
		return err
	}
	return m.RunMeasure()
}

// MeasurePrompt prints the measure prompt that would be sent to Claude to stdout.
// This is useful for inspecting or debugging the prompt without invoking Claude.
// Shows the prompt for a single iteration (limit=1), which is what each
// iterative call uses.
func (m *Measure) MeasurePrompt() error {
	prompt, err := m.buildMeasurePrompt("", "", 1)
	if err != nil {
		return err
	}
	fmt.Print(prompt)
	return nil
}

// RunMeasure runs the measure workflow using Config settings.
// repo is the GitHub owner/repo where issues are created.
// It uses an iterative strategy: Claude is called once per issue with limit=1,
// and the issue is recorded on GitHub between calls. Each subsequent call sees
// the updated issue list, enabling Claude to reason about dependencies and
// avoid duplicates. This avoids the super-linear thinking-time scaling observed
// when requesting multiple issues in a single call (see eng04-measure-scaling).
func (m *Measure) RunMeasure() error {
	m.setPhase("measure")
	defer m.clearPhase()
	measureStart := time.Now()

	// Start orchestrator log capture.
	if hdir := m.claudeRunner.historyDir(); hdir != "" {
		logPath := filepath.Join(hdir,
			measureStart.Format("2006-01-02-15-04-05")+"-measure-orchestrator.log")
		if err := m.openLogSink(logPath); err != nil {
			m.logf("warning: could not open orchestrator log: %v", err)
		} else {
			defer m.closeLogSink()
		}
	}

	m.logf("starting (iterative, %d issue(s) requested)", m.cfg.Cobbler.MaxMeasureIssues)
	m.claudeRunner.logConfig("measure")

	if err := m.claudeRunner.checkClaude(); err != nil {
		return err
	}

	branch, err := m.resolveBranch(m.cfg.Generation.Branch)
	if err != nil {
		m.logf("resolveBranch failed: %v", err)
		return err
	}
	m.logf("resolved branch=%s", branch)
	if m.getGeneration() == "" {
		m.setGeneration(branch)
		defer m.clearGeneration()
	}
	generation := branch

	if err := m.ensureOnBranch(branch); err != nil {
		m.logf("ensureOnBranch failed: %v", err)
		return fmt.Errorf("switching to branch: %w", err)
	}

	_ = os.MkdirAll(m.cfg.Cobbler.Dir, 0o755) // best-effort; dir may already exist

	// Resolve the GitHub repo for issue management.
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	repo, err := m.tracker.DetectGitHubRepo(repoRoot)
	if err != nil {
		m.logf("detectGitHubRepo failed: %v", err)
		return fmt.Errorf("detecting GitHub repo: %w", err)
	}
	m.logf("using GitHub repo %s for issues", repo)

	// Ensure the cobbler labels and generation label exist on the repo.
	if err := m.tracker.EnsureCobblerLabels(repo); err != nil {
		m.logf("ensureCobblerLabels warning: %v", err)
	}
	m.tracker.EnsureCobblerGenLabel(repo, generation) // nolint: best-effort

	// Run pre-cycle analysis so the measure prompt sees current project state.
	m.analyzer.RunPreCycleAnalysis()

	// Warn about SRD requirement groups whose sub-item count exceeds
	// max_requirements_per_task.
	if m.cfg.Cobbler.MaxRequirementsPerTask > 0 {
		warnOversizedGroups(m.cfg.Cobbler.MaxRequirementsPerTask)
	}

	// Route target-repo defects to the target repo (srd003 R11).
	if analysis := an.LoadAnalysisDoc(m.cfg.Cobbler.Dir); analysis != nil && len(analysis.Defects) > 0 {
		if targetRepo := m.tracker.ResolveTargetRepo(); targetRepo != "" {
			m.logf("measure: filing %d defect(s) as bug issues in %s", len(analysis.Defects), targetRepo)
			m.tracker.FileTargetRepoDefects(targetRepo, analysis.Defects)
		} else {
			m.logf("measure: no target repo configured; skipping %d defect(s)", len(analysis.Defects))
		}
	}

	// Clean up old measure temp files.
	matches, _ := filepath.Glob(m.cfg.Cobbler.Dir + "measure-*.yaml") // empty list on error is acceptable
	if len(matches) > 0 {
		m.logf("cleaning %d old measure temp file(s)", len(matches))
	}
	for _, f := range matches {
		os.Remove(f) // nolint: best-effort temp file cleanup
	}

	// Get initial state: open GitHub issues for this generation.
	existingIssues, _ := m.tracker.ListActiveIssuesContext(repo, generation)
	commitSHA, _ := m.git.RevParseHEAD(".") // empty string on error is acceptable for logging

	m.logf("existing issues context len=%d, maxMeasureIssues=%d, commit=%s",
		len(existingIssues), m.cfg.Cobbler.MaxMeasureIssues, commitSHA)

	// Snapshot LOC before Claude.
	locBefore := m.claudeRunner.captureLOC()
	m.logf("locBefore prod=%d test=%d", locBefore.Production, locBefore.Test)

	// Measure loop: call Claude with limit=tasksPerCall, up to maxIssues total.
	maxIssues := m.cfg.Cobbler.MaxMeasureIssues
	tasksPerCall := m.cfg.Cobbler.MeasureTasksPerCall
	if tasksPerCall <= 0 {
		tasksPerCall = maxIssues
	}
	totalCalls := (maxIssues + tasksPerCall - 1) / tasksPerCall // ceiling division
	m.logf("measure: maxIssues=%d tasksPerCall=%d totalCalls=%d", maxIssues, tasksPerCall, totalCalls)
	var allCreatedIDs []string
	var totalTokens claude.ClaudeResult
	maxRetries := m.cfg.Cobbler.MaxMeasureRetries

	// Create a single placeholder issue for the entire measure pass (GH-1467).
	// Previously one placeholder was created per iteration, flooding the tracker.
	placeholderNum, placeholderErr := m.tracker.CreateMeasuringPlaceholder(repo, generation, 0)
	if placeholderErr != nil {
		m.logf("measure: warning: createMeasuringPlaceholder: %v", placeholderErr)
	}
	placeholderResolved := false
	if placeholderNum > 0 {
		defer func() {
			if !placeholderResolved {
				m.tracker.CloseMeasuringPlaceholderWithComment(repo, placeholderNum, "Measure did not complete; closed automatically.")
			}
		}()
	}
	taskID := ""
	if placeholderNum > 0 {
		taskID = fmt.Sprintf("%d", placeholderNum)
	}

	for i := 0; i < totalCalls && len(allCreatedIDs) < maxIssues; i++ {
		// Clamp the per-call limit so we don't exceed maxIssues.
		callLimit := tasksPerCall
		if remaining := maxIssues - len(allCreatedIDs); callLimit > remaining {
			callLimit = remaining
		}
		m.logf("--- iteration %d/%d (limit=%d, created so far=%d) ---", i+1, totalCalls, callLimit, len(allCreatedIDs))

		// Refresh existing issues from GitHub before each call (except the first).
		if i > 0 {
			refreshed, refreshErr := m.tracker.ListActiveIssuesContext(repo, generation)
			if refreshErr != nil {
				m.logf("measure: warning: refreshing issue list: %v", refreshErr)
			} else {
				existingIssues = refreshed
			}
		}

		var createdIDs []string
		var lastOutputFile string
		var lastValidationErrors []string // errors from previous attempt, fed back into retry prompt

		// Attempt loop: try Claude + import, retrying on validation failure.
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				m.logf("iteration %d retry %d/%d (validation rejected previous output)",
					i+1, attempt, maxRetries)
			}

			timestamp := time.Now().Format("20060102-150405")
			outputFile := filepath.Join(m.cfg.Cobbler.Dir, fmt.Sprintf("measure-%s.yaml", timestamp))
			lastOutputFile = outputFile

			prompt, promptErr := m.buildMeasurePrompt(m.cfg.Cobbler.UserPrompt, existingIssues, callLimit, lastValidationErrors...)
			if promptErr != nil {
				return promptErr
			}
			m.logf("iteration %d prompt built, length=%d bytes", i+1, len(prompt))

			// Save prompt BEFORE calling Claude so it's on disk even if Claude times out.
			historyTS := time.Now().Format("2006-01-02-15-04-05")
			m.claudeRunner.saveHistoryPrompt(historyTS, "measure", prompt)

			iterStart := time.Now()
			tokens, err := m.claudeRunner.runMeasureClaude(prompt, "", m.cfg.Silence(), "--max-turns", "1")
			iterDuration := time.Since(iterStart)

			totalTokens.InputTokens += tokens.InputTokens
			totalTokens.OutputTokens += tokens.OutputTokens
			totalTokens.CacheCreationTokens += tokens.CacheCreationTokens
			totalTokens.CacheReadTokens += tokens.CacheReadTokens
			totalTokens.CostUSD += tokens.CostUSD

			if err != nil {
				m.logf("Claude failed on iteration %d after %s: %v",
					i+1, iterDuration.Round(time.Second), err)
				// Save log and stats even on failure.
				m.claudeRunner.saveHistoryLog(historyTS, "measure", tokens.RawOutput)
				m.claudeRunner.saveHistoryStats(historyTS, "measure", claude.HistoryStats{
					Caller:        "measure",
					TaskID:        taskID,
					Status:        "failed",
					Error:         fmt.Sprintf("claude failure (iteration %d/%d): %v", i+1, totalCalls, err),
					StartedAt:     iterStart.UTC().Format(time.RFC3339),
					Duration:      iterDuration.Round(time.Second).String(),
					DurationS:     int(iterDuration.Seconds()),
					Tokens:        claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
					CostUSD:       tokens.CostUSD,
					NumTurns:      tokens.NumTurns,
					DurationAPIMs: tokens.DurationAPIMs,
					SessionID:     tokens.SessionID,
					LOCBefore:     locBefore,
					LOCAfter:      m.claudeRunner.captureLOC(),
				})
				return fmt.Errorf("running Claude (iteration %d/%d): %w", i+1, totalCalls, err)
			}
			m.logf("iteration %d Claude completed in %s", i+1, iterDuration.Round(time.Second))

			// Save remaining history artifacts (log, issues, stats) after Claude.
			m.saveHistory(historyTS, tokens.RawOutput, outputFile)
			m.claudeRunner.saveHistoryStats(historyTS, "measure", claude.HistoryStats{
				Caller:        "measure",
				TaskID:        taskID,
				Status:        "success",
				StartedAt:     iterStart.UTC().Format(time.RFC3339),
				Duration:      iterDuration.Round(time.Second).String(),
				DurationS:     int(iterDuration.Seconds()),
				Tokens:        claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
				CostUSD:       tokens.CostUSD,
				NumTurns:      tokens.NumTurns,
				DurationAPIMs: tokens.DurationAPIMs,
				SessionID:     tokens.SessionID,
				LOCBefore:     locBefore,
				LOCAfter:      m.claudeRunner.captureLOC(),
			})

			// Extract YAML from Claude's text output and write to file.
			textOutput := claude.ExtractTextFromStreamJSON(tokens.RawOutput)
			yamlContent, extractErr := claude.ExtractYAMLBlock(textOutput)
			if extractErr != nil {
				m.logf("iteration %d YAML extraction failed: %v", i+1, extractErr)
				if attempt < maxRetries {
					continue // retry
				}
				m.logf("iteration %d retries exhausted, no YAML extracted", i+1)
				break
			}
			if err := os.WriteFile(outputFile, yamlContent, 0o644); err != nil {
				m.logf("iteration %d failed to write output file: %v", i+1, err)
				break
			}
			m.logf("iteration %d extracted YAML, size=%d bytes", i+1, len(yamlContent))

			var importErr error
			var validationErrs []string
			createdIDs, validationErrs, importErr = m.importIssues(outputFile, repo, generation, placeholderNum)
			if importErr != nil {
				m.logf("iteration %d import failed: %v", i+1, importErr)
				if attempt < maxRetries {
					lastValidationErrors = validationErrs // feed errors back into next prompt
					_ = os.Remove(outputFile)             // best-effort cleanup before retry
					continue                              // retry
				}
				// Retries exhausted: accept with warning (R5).
				m.logf("iteration %d retries exhausted, accepting last result with warnings", i+1)
				var forceErr error
				createdIDs, forceErr = m.importIssuesForce(outputFile, repo, generation, placeholderNum)
				if forceErr != nil {
					m.logf("iteration %d force import failed: %v", i+1, forceErr)
				}
			}
			break // success or retries exhausted
		}

		m.logf("iteration %d imported %d issue(s)", i+1, len(createdIDs))
		allCreatedIDs = append(allCreatedIDs, createdIDs...)

		if len(createdIDs) == 0 && lastOutputFile != "" {
			m.logf("iteration %d created no issues, keeping %s for inspection", i+1, lastOutputFile)
		} else if lastOutputFile != "" {
			os.Remove(lastOutputFile) // nolint: best-effort temp file cleanup
		}
	}

	// Retry once if measure returned empty but unresolved requirements remain.
	// Claude non-deterministically returns [] on large prompts; a single retry
	// recovers ~95% of these cases (GH-1513).
	if len(allCreatedIDs) == 0 && m.hasUnresolved() {
		m.logf("measure: 0 issues created but unresolved requirements remain — retrying once")

		// Refresh existing issues for the retry.
		refreshed, refreshErr := m.tracker.ListActiveIssuesContext(repo, generation)
		if refreshErr == nil {
			existingIssues = refreshed
		}

		timestamp := time.Now().Format("20060102-150405")
		outputFile := filepath.Join(m.cfg.Cobbler.Dir, fmt.Sprintf("measure-%s.yaml", timestamp))

		retryLimit := tasksPerCall
		if retryLimit > maxIssues {
			retryLimit = maxIssues
		}
		prompt, promptErr := m.buildMeasurePrompt(m.cfg.Cobbler.UserPrompt, existingIssues, retryLimit)
		if promptErr == nil {
			historyTS := time.Now().Format("2006-01-02-15-04-05")
			m.claudeRunner.saveHistoryPrompt(historyTS, "measure", prompt)

			retryStart := time.Now()
			tokens, err := m.claudeRunner.runMeasureClaude(prompt, "", m.cfg.Silence(), "--max-turns", "1")
			retryDuration := time.Since(retryStart)

			totalTokens.InputTokens += tokens.InputTokens
			totalTokens.OutputTokens += tokens.OutputTokens
			totalTokens.CostUSD += tokens.CostUSD

			if err == nil {
				m.saveHistory(historyTS, tokens.RawOutput, outputFile)
				m.claudeRunner.saveHistoryStats(historyTS, "measure", claude.HistoryStats{
					Caller:    "measure",
					TaskID:    fmt.Sprintf("%d", placeholderNum),
					Status:    "success",
					StartedAt: retryStart.UTC().Format(time.RFC3339),
					Duration:  retryDuration.Round(time.Second).String(),
					DurationS: int(retryDuration.Seconds()),
					Tokens:    claude.HistoryTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
					CostUSD:   tokens.CostUSD,
					NumTurns:  tokens.NumTurns,
					LOCBefore: locBefore,
					LOCAfter:  m.claudeRunner.captureLOC(),
				})

				textOutput := claude.ExtractTextFromStreamJSON(tokens.RawOutput)
				yamlContent, extractErr := claude.ExtractYAMLBlock(textOutput)
				if extractErr == nil {
					if writeErr := os.WriteFile(outputFile, yamlContent, 0o644); writeErr == nil {
						retryIDs, _, importErr := m.importIssues(outputFile, repo, generation, placeholderNum)
						if importErr == nil {
							allCreatedIDs = append(allCreatedIDs, retryIDs...)
							m.logf("measure: retry created %d issue(s)", len(retryIDs))
						}
					}
				}
			}
		}
	}

	// Finalize the single placeholder with all created issues (GH-1467).
	placeholderResolved = true
	if placeholderNum > 0 {
		var childNums []int
		for _, id := range allCreatedIDs {
			if n, err := fmt.Sscanf(id, "%d", new(int)); n == 1 && err == nil {
				var v int
				fmt.Sscanf(id, "%d", &v)
				childNums = append(childNums, v)
			}
		}
		comment := fmt.Sprintf("Measure completed. limit=%d/call, %d issue(s) created.", tasksPerCall, len(allCreatedIDs))
		if totalTokens.CostUSD > 0 {
			comment += fmt.Sprintf("\nCost: $%.2f, Tokens: %din %dout",
				totalTokens.CostUSD, totalTokens.InputTokens, totalTokens.OutputTokens)
		}
		m.tracker.FinalizeMeasurePlaceholder(repo, placeholderNum, generation, comment, childNums)
	}

	m.logf("completed %d iteration(s), %d issue(s) created in %s",
		totalCalls, len(allCreatedIDs), time.Since(measureStart).Round(time.Second))
	return nil
}

func (m *Measure) buildMeasurePrompt(userInput, existingIssues string, limit int, validationErrors ...string) (string, error) {
	tmpl, err := ictx.ParsePromptTemplate(orDefault(m.cfg.Cobbler.MeasurePrompt, defaultMeasurePrompt))
	if err != nil {
		return "", fmt.Errorf("measure prompt YAML: %w", err)
	}

	planningConst := orDefault(m.cfg.Cobbler.PlanningConstitution, planningConstitution)

	// Load per-phase context file (srd003 R9.8).
	measureCtxPath := filepath.Join(m.cfg.Cobbler.Dir, "measure_context.yaml")
	phaseCtx, phaseErr := ictx.LoadPhaseContext(measureCtxPath)
	if phaseErr != nil {
		return "", fmt.Errorf("loading measure context: %w", phaseErr)
	}
	if phaseCtx != nil {
		m.logf("buildMeasurePrompt: using phase context from %s", measureCtxPath)
	} else {
		m.logf("buildMeasurePrompt: no phase context file, using config defaults")
	}

	// Apply CobblerConfig measure source settings to phaseCtx (GH-565).
	if phaseCtx == nil {
		phaseCtx = &PhaseContext{}
	}
	if m.cfg.Cobbler.MeasureExcludeSource && !phaseCtx.ExcludeSource {
		phaseCtx.ExcludeSource = true
		m.logf("buildMeasurePrompt: measure_exclude_source=true from config")
	}
	if m.cfg.Cobbler.MeasureSourcePatterns != "" && phaseCtx.SourcePatterns == "" {
		phaseCtx.SourcePatterns = m.cfg.Cobbler.MeasureSourcePatterns
		m.logf("buildMeasurePrompt: measure_source_patterns set from config")
	}
	if m.cfg.Cobbler.effectiveMeasureExcludeTests() && !phaseCtx.ExcludeTests {
		phaseCtx.ExcludeTests = true
		m.logf("buildMeasurePrompt: measure_exclude_tests=true, _test.go files will be excluded")
	}
	if m.cfg.Cobbler.MeasureSourceMode != "" && phaseCtx.SourceMode == "" {
		phaseCtx.SourceMode = m.cfg.Cobbler.MeasureSourceMode
		m.logf("buildMeasurePrompt: measure_source_mode=%q from config", phaseCtx.SourceMode)
	}
	if m.cfg.Cobbler.MeasureSummarizeCommand != "" && phaseCtx.SummarizeCommand == "" {
		phaseCtx.SummarizeCommand = m.cfg.Cobbler.MeasureSummarizeCommand
		m.logf("buildMeasurePrompt: measure_summarize_command set from config")
	}

	// Auto-derive SourcePatterns from the road-map when MeasureRoadmapSource
	// is enabled and no manual patterns are already set (GH-534).
	if m.cfg.Cobbler.MeasureRoadmapSource && !phaseCtx.ExcludeSource && phaseCtx.SourcePatterns == "" {
		uc, err := selectNextPendingUseCase(m.cfg.Project)
		if err != nil {
			m.logf("buildMeasurePrompt: road-map source selection error: %v", err)
		} else if uc != nil {
			pkgPaths := ictx.ParseTouchpointPackages(uc.Touchpoints)
			if len(pkgPaths) > 0 {
				var patterns []string
				for _, p := range pkgPaths {
					patterns = append(patterns, p+"/**/*.go")
				}
				phaseCtx.SourcePatterns = strings.Join(patterns, "\n")
				m.logf("buildMeasurePrompt: road-map source: UC=%s packages=%v", uc.ID, pkgPaths)
			} else {
				m.logf("buildMeasurePrompt: road-map source: UC=%s has no package touchpoints, loading all source", uc.ID)
			}
		} else {
			m.logf("buildMeasurePrompt: road-map source: all use cases done, loading all source")
		}
	}

	projectCtx, ctxErr := buildProjectContext(existingIssues, m.cfg.Project, phaseCtx)
	if ctxErr != nil {
		m.logf("buildMeasurePrompt: buildProjectContext error: %v", ctxErr)
		projectCtx = &ProjectContext{}
	}

	placeholders := map[string]string{
		"limit":            fmt.Sprintf("%d", limit),
		"lines_min":        fmt.Sprintf("%d", m.cfg.Cobbler.EstimatedLinesMin),
		"lines_max":        fmt.Sprintf("%d", m.cfg.Cobbler.EstimatedLinesMax),
		"max_requirements": fmt.Sprintf("%d", m.cfg.Cobbler.MaxRequirementsPerTask),
		"max_weight":       fmt.Sprintf("%d", m.cfg.Cobbler.MaxWeightPerTask),
	}

	// Inject package_contracts when source mode is "headers" or "custom".
	var measureContracts []OODPackageContractRef
	sourceMode := phaseCtx.SourceMode
	if sourceMode == "headers" || sourceMode == "custom" {
		contracts, _ := ictx.LoadOODPromptContext()
		if len(contracts) > 0 {
			measureContracts = contracts
			m.logf("buildMeasurePrompt: injecting %d package_contracts (source_mode=%s)", len(contracts), sourceMode)
		}
	}

	doc := MeasurePromptDoc{
		Role:                    tmpl.Role,
		ProjectContext:          projectCtx,
		PlanningConstitution:    ictx.ParseYAMLNode(planningConst),
		IssueFormatConstitution: ictx.ParseYAMLNode(issueFormatConstitution),
		Task:                    ictx.SubstitutePlaceholders(tmpl.Task, placeholders),
		Constraints:             ictx.SubstitutePlaceholders(tmpl.Constraints, placeholders),
		OutputFormat:            ictx.SubstitutePlaceholders(tmpl.OutputFormat, placeholders),
		GoldenExample:           m.cfg.Cobbler.GoldenExample,
		AdditionalContext:       userInput,
		ValidationErrors:        validationErrors,
		PackageContracts:        measureContracts,
	}

	// Enforce releases scope.
	activeReleases := filterImplementedReleases(m.cfg.Project.Releases)
	activeRelease := filterImplementedRelease(m.cfg.Project.Release)
	doc.Constraints += measureReleasesConstraint(activeReleases, activeRelease)

	// When max_weight_per_task is set, inject a chain-of-thought weight-
	// reasoning step between "Reason about priorities" (step 3) and
	// "Propose tasks" (step 4). This makes the agent do the arithmetic
	// explicitly before writing YAML, which is more reliable than a prose
	// constraint (GH-2077, replaces GH-1832 constraint).
	if maxW := m.cfg.Cobbler.MaxWeightPerTask; maxW > 0 {
		cotStep := fmt.Sprintf("\n\n  3b. **Check weight budgets** — Before proposing each task, "+
			"list the requirements you plan to include and their weights from requirements.yaml. "+
			"Sum the total. If total exceeds %d, move the heaviest requirement(s) to a separate task. "+
			"Show your arithmetic:\n\n"+
			"      Task: cmd/foo R2.5 (w=4), R2.6 (w=1), R3.1 (w=1)\n"+
			"      Total weight: 4+1+1 = 6 → exceeds %d\n"+
			"      Fix: move R2.5 (w=4) to its own task\n\n"+
			"      Every requirement in requirements.yaml has a weight field (default 1). "+
			"Each task's total weight must not exceed %d.", maxW, maxW, maxW)
		doc.Task += cotStep
		doc.Constraints += fmt.Sprintf("\n- Each task's total requirement weight must not exceed %d. "+
			"Follow step 3b to verify weight budgets before proposing tasks.", maxW)
		m.logf("buildMeasurePrompt: max_weight_per_task=%d CoT step injected", maxW)
	}

	// results for projects with high documentation-to-code ratios (GH-1882).
	if minIssues := m.cfg.Cobbler.MinMeasureIssues; minIssues > 0 && m.hasUnresolved() {
		doc.Constraints += fmt.Sprintf("\n- MANDATORY: You MUST propose at least %d task(s). "+
			"The requirements.yaml file shows unresolved R-items with status \"ready\". "+
			"Returning an empty list [] is NOT acceptable when ready requirements exist. "+
			"Analyze the ready R-items and propose implementation tasks for them.", minIssues)
		m.logf("buildMeasurePrompt: min_measure_issues=%d constraint injected", minIssues)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("marshaling measure prompt: %w", err)
	}

	m.logf("buildMeasurePrompt: %d bytes limit=%d userInput=%v",
		len(out), limit, userInput != "")
	return string(out), nil
}

// proposedIssue is aliased from internal/github in issues_gh.go.

// importIssues imports proposed issues from a YAML file into GitHub.
func (m *Measure) importIssues(yamlFile, repo, generation string, ph int) ([]string, []string, error) {
	return m.importIssuesImpl(yamlFile, repo, generation, false, ph)
}

// importIssuesForce imports issues bypassing enforcing validation.
func (m *Measure) importIssuesForce(yamlFile, repo, generation string, ph int) ([]string, error) {
	ids, _, err := m.importIssuesImpl(yamlFile, repo, generation, true, ph)
	return ids, err
}

func (m *Measure) importIssuesImpl(yamlFile, repo, generation string, skipEnforcement bool, ph int) ([]string, []string, error) {
	m.logf("importIssues: reading %s", yamlFile)
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading YAML file: %w", err)
	}
	m.logf("importIssues: read %d bytes", len(data))

	var issues []proposedIssue
	if err := yaml.Unmarshal(data, &issues); err != nil {
		m.logf("importIssues: YAML parse error: %v", err)
		return nil, nil, fmt.Errorf("parsing YAML: %w", err)
	}

	m.logf("importIssues: parsed %d proposed issue(s)", len(issues))
	for i, issue := range issues {
		m.logf("importIssues: [%d] title=%q dep=%d", i, issue.Title, issue.Dependency)
	}

	// Validate proposed issues against P9/P7 rules and completed R-items (GH-1386).
	subItemCounts := loadSRDSubItemCounts()
	reqStates := loadRequirementStates(m.cfg.Cobbler.Dir)
	vr := validateMeasureOutput(issues, m.cfg.Cobbler.MaxRequirementsPerTask, m.cfg.Cobbler.MaxWeightPerTask, subItemCounts, reqStates)
	if len(vr.Warnings) > 0 {
		m.logf("importIssues: %d warning(s)", len(vr.Warnings))
	}
	// Collect enforced errors based on per-category flags (GH-2070).
	if !skipEnforcement {
		var enforced []string
		if m.cfg.Cobbler.EnforceWeightValidation && len(vr.WeightErrors) > 0 {
			enforced = append(enforced, vr.WeightErrors...)
		}
		if m.cfg.Cobbler.EnforceGranularityValidation && len(vr.GranularityErrors) > 0 {
			enforced = append(enforced, vr.GranularityErrors...)
		}
		if m.cfg.Cobbler.EnforceFileNamingValidation && len(vr.FileNamingErrors) > 0 {
			enforced = append(enforced, vr.FileNamingErrors...)
		}
		// Uncategorized errors (e.g., completed R-items) are always enforced
		// when any enforcement flag is active.
		if (m.cfg.Cobbler.EnforceWeightValidation || m.cfg.Cobbler.EnforceGranularityValidation || m.cfg.Cobbler.EnforceFileNamingValidation) && len(vr.Errors) > 0 {
			enforced = append(enforced, vr.Errors...)
		}
		if len(enforced) > 0 {
			return nil, enforced, fmt.Errorf("measure validation failed (%d error(s)): %s",
				len(enforced), strings.Join(enforced, "; "))
		}
	}

	// Deduplicate: fetch existing issues for this generation and skip any
	// proposed issue whose normalized title or output files match an existing
	// one (GH-1026, GH-1352, GH-1373).
	existingTitles := make(map[string]int) // normalized title → issue number
	existingFiles := make(map[string]int)  // file path → issue number
	if existing, err := m.tracker.ListAllCobblerIssues(repo, generation); err == nil {
		hasOpen := false
		for _, ex := range existing {
			if ex.State == "open" {
				hasOpen = true
				break
			}
		}
		if hasOpen {
			for _, ex := range existing {
				existingTitles[gh.NormalizeIssueTitle(ex.Title)] = ex.Number
				for _, fp := range gh.ExtractDescriptionFiles(ex.Description) {
					existingFiles[fp] = ex.Number
				}
			}
		}
	}
	var filtered []proposedIssue
	for _, issue := range issues {
		norm := gh.NormalizeIssueTitle(issue.Title)
		if dup, ok := existingTitles[norm]; ok {
			m.logf("importIssues: skipping duplicate %q — title matches #%d", issue.Title, dup)
			continue
		}
		// Check if any proposed output file overlaps with an existing issue (GH-1373).
		if dup, overlap := fileOverlap(gh.ExtractDescriptionFiles(issue.Description), existingFiles); overlap {
			m.logf("importIssues: skipping duplicate %q — output files overlap with #%d", issue.Title, dup)
			continue
		}
		filtered = append(filtered, issue)
		// Track accepted title for intra-batch dedup (GH-1605).
		// File overlap is only checked against existing GitHub issues, not
		// within the same batch — tasks in the same package naturally share
		// files and are not duplicates (GH-1646).
		existingTitles[norm] = issue.Index
	}
	issues = filtered

	// Hard-filter proposals for out-of-scope releases (GH-1703).
	// The prompt constraint instructs Claude to stay in scope, but this
	// filter rejects any proposals that slip through anyway.
	activeReleases := filterImplementedReleases(m.cfg.Project.Releases)
	if len(activeReleases) > 0 {
		var scoped []proposedIssue
		for _, issue := range issues {
			if generate.IsOutOfScopeRelease(issue.Title, issue.Description, activeReleases) {
				rel := generate.ExtractReleaseFromText(issue.Title + " " + issue.Description)
				m.logf("importIssues: rejecting out-of-scope task %q (release %s not in %v)", issue.Title, rel, activeReleases)
				continue
			}
			scoped = append(scoped, issue)
		}
		issues = scoped
	}

	// Create all issues on GitHub as separate stitch tasks (GH-1367).
	// The measure placeholder remains a distinct [measure] issue.
	var ids []string
	for _, issue := range issues {
		m.logf("importIssues: creating task %d: %s (dep=%d)", issue.Index, issue.Title, issue.Dependency)
		ghNum, err := m.tracker.CreateCobblerIssue(repo, generation, issue)
		if err != nil {
			m.logf("importIssues: createCobblerIssue failed for %q: %v", issue.Title, err)
			continue
		}
		ids = append(ids, fmt.Sprintf("%d", ghNum))
	}

	if len(ids) > 0 {
		m.tracker.WaitForIssuesVisible(repo, generation, len(ids))
		if err := m.tracker.PromoteReadyIssues(repo, generation); err != nil {
			m.logf("importIssues: promoteReadyIssues warning: %v", err)
		}
	}
	m.logf("importIssues: %d of %d issue(s) imported", len(ids), len(issues))

	// Append new issues to the persistent measure list.
	appendMeasureLog(m.cfg.Cobbler.Dir, issues)

	return ids, nil, nil
}

// fileOverlap returns the issue number of the first existing issue whose files
// overlap with the proposed file list, and true if an overlap was found.
func fileOverlap(proposedFiles []string, existingFiles map[string]int) (int, bool) {
	for _, fp := range proposedFiles {
		if dup, ok := existingFiles[fp]; ok {
			return dup, true
		}
	}
	return 0, false
}

// saveHistory persists measure artifacts (log, issues YAML) to the configured
// history directory.
func (m *Measure) saveHistory(ts string, rawOutput []byte, issuesFile string) {
	m.claudeRunner.saveHistoryLog(ts, "measure", rawOutput)

	dir := m.claudeRunner.historyDir()
	if dir == "" {
		return
	}
	base := ts + "-measure"
	if data, err := os.ReadFile(issuesFile); err == nil {
		if err := os.WriteFile(filepath.Join(dir, base+"-issues.yaml"), data, 0o644); err != nil {
			m.logf("saveHistory: write issues: %v", err)
		}
	}
}
