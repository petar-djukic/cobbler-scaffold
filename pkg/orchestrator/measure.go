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

	"gopkg.in/yaml.v3"
)

//go:embed prompts/measure.yaml
var defaultMeasurePrompt string

//go:embed constitutions/planning.yaml
var planningConstitution string

//go:embed constitutions/issue-format.yaml
var issueFormatConstitution string

// Measure assesses project state and proposes new tasks via Claude.
// Reads all options from Config.
func (o *Orchestrator) Measure() error {
	return o.RunMeasure()
}

// MeasurePrompt prints the measure prompt that would be sent to Claude to stdout.
// This is useful for inspecting or debugging the prompt without invoking Claude.
// Shows the prompt for a single iteration (limit=1), which is what each
// iterative call uses.
func (o *Orchestrator) MeasurePrompt() error {
	prompt, err := o.buildMeasurePrompt("", "", 1)
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
func (o *Orchestrator) RunMeasure() error {
	setPhase("measure")
	defer clearPhase()
	measureStart := time.Now()

	// Start orchestrator log capture.
	if hdir := o.historyDir(); hdir != "" {
		logPath := filepath.Join(hdir,
			measureStart.Format("2006-01-02-15-04-05")+"-measure-orchestrator.log")
		if err := openLogSink(logPath); err != nil {
			logf("warning: could not open orchestrator log: %v", err)
		} else {
			defer closeLogSink()
		}
	}

	logf("starting (iterative, %d issue(s) requested)", o.cfg.Cobbler.MaxMeasureIssues)
	o.logConfig("measure")

	if err := o.checkClaude(); err != nil {
		return err
	}

	branch, err := o.resolveBranch(o.cfg.Generation.Branch)
	if err != nil {
		logf("resolveBranch failed: %v", err)
		return err
	}
	logf("resolved branch=%s", branch)
	if currentGeneration == "" {
		setGeneration(branch)
		defer clearGeneration()
	}
	generation := branch

	if err := ensureOnBranch(branch); err != nil {
		logf("ensureOnBranch failed: %v", err)
		return fmt.Errorf("switching to branch: %w", err)
	}

	_ = os.MkdirAll(o.cfg.Cobbler.Dir, 0o755) // best-effort; dir may already exist

	// Resolve the GitHub repo for issue management.
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	repo, err := detectGitHubRepo(repoRoot, o.cfg)
	if err != nil {
		logf("detectGitHubRepo failed: %v", err)
		return fmt.Errorf("detecting GitHub repo: %w", err)
	}
	logf("using GitHub repo %s for issues", repo)

	// Ensure the cobbler labels and generation label exist on the repo.
	if err := ensureCobblerLabels(repo); err != nil {
		logf("ensureCobblerLabels warning: %v", err)
	}
	ensureCobblerGenLabel(repo, generation) // nolint: best-effort

	// Run pre-cycle analysis so the measure prompt sees current project state.
	o.RunPreCycleAnalysis()

	// Warn about PRD requirement groups whose sub-item count exceeds
	// max_requirements_per_task.
	if o.cfg.Cobbler.MaxRequirementsPerTask > 0 {
		warnOversizedGroups(o.cfg.Cobbler.MaxRequirementsPerTask)
	}

	// Route target-repo defects to the target repo (prd003 R11).
	if analysis := loadAnalysisDoc(o.cfg.Cobbler.Dir); analysis != nil && len(analysis.Defects) > 0 {
		if targetRepo := resolveTargetRepo(o.cfg); targetRepo != "" {
			logf("measure: filing %d defect(s) as bug issues in %s", len(analysis.Defects), targetRepo)
			fileTargetRepoDefects(targetRepo, analysis.Defects)
		} else {
			logf("measure: no target repo configured; skipping %d defect(s)", len(analysis.Defects))
		}
	}

	// Clean up old measure temp files.
	matches, _ := filepath.Glob(o.cfg.Cobbler.Dir + "measure-*.yaml") // empty list on error is acceptable
	if len(matches) > 0 {
		logf("cleaning %d old measure temp file(s)", len(matches))
	}
	for _, f := range matches {
		os.Remove(f) // nolint: best-effort temp file cleanup
	}

	// Get initial state: open GitHub issues for this generation.
	existingIssues, _ := listActiveIssuesContext(repo, generation)
	commitSHA, _ := gitRevParseHEAD(".") // empty string on error is acceptable for logging

	logf("existing issues context len=%d, maxMeasureIssues=%d, commit=%s",
		len(existingIssues), o.cfg.Cobbler.MaxMeasureIssues, commitSHA)

	// Snapshot LOC before Claude.
	locBefore := o.captureLOC()
	logf("locBefore prod=%d test=%d", locBefore.Production, locBefore.Test)

	// Iterative measure: call Claude once per issue with limit=1.
	totalIssues := o.cfg.Cobbler.MaxMeasureIssues
	var allCreatedIDs []string
	var totalTokens ClaudeResult
	maxRetries := o.cfg.Cobbler.MaxMeasureRetries

	// Create a single placeholder issue for the entire measure pass (GH-1467).
	// Previously one placeholder was created per iteration, flooding the tracker.
	placeholderNum, placeholderErr := createMeasuringPlaceholder(repo, generation, 0)
	if placeholderErr != nil {
		logf("measure: warning: createMeasuringPlaceholder: %v", placeholderErr)
	}
	placeholderResolved := false
	if placeholderNum > 0 {
		defer func() {
			if !placeholderResolved {
				closeMeasuringPlaceholderWithComment(repo, placeholderNum, "Measure did not complete; closed automatically.")
			}
		}()
	}
	taskID := ""
	if placeholderNum > 0 {
		taskID = fmt.Sprintf("%d", placeholderNum)
	}

	for i := 0; i < totalIssues; i++ {
		logf("--- iteration %d/%d ---", i+1, totalIssues)

		// Refresh existing issues from GitHub before each call (except the first).
		if i > 0 {
			refreshed, refreshErr := listActiveIssuesContext(repo, generation)
			if refreshErr != nil {
				logf("measure: warning: refreshing issue list: %v", refreshErr)
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
				logf("iteration %d retry %d/%d (validation rejected previous output)",
					i+1, attempt, maxRetries)
			}

			timestamp := time.Now().Format("20060102-150405")
			outputFile := filepath.Join(o.cfg.Cobbler.Dir, fmt.Sprintf("measure-%s.yaml", timestamp))
			lastOutputFile = outputFile

			prompt, promptErr := o.buildMeasurePrompt(o.cfg.Cobbler.UserPrompt, existingIssues, 1, lastValidationErrors...)
			if promptErr != nil {
				return promptErr
			}
			logf("iteration %d prompt built, length=%d bytes", i+1, len(prompt))

			// Save prompt BEFORE calling Claude so it's on disk even if Claude times out.
			historyTS := time.Now().Format("2006-01-02-15-04-05")
			o.saveHistoryPrompt(historyTS, "measure", prompt)

			iterStart := time.Now()
			tokens, err := o.runMeasureClaude(prompt, "", o.cfg.Silence(), "--max-turns", "1")
			iterDuration := time.Since(iterStart)

			totalTokens.InputTokens += tokens.InputTokens
			totalTokens.OutputTokens += tokens.OutputTokens
			totalTokens.CacheCreationTokens += tokens.CacheCreationTokens
			totalTokens.CacheReadTokens += tokens.CacheReadTokens
			totalTokens.CostUSD += tokens.CostUSD

			if err != nil {
				logf("Claude failed on iteration %d after %s: %v",
					i+1, iterDuration.Round(time.Second), err)
				// Save log and stats even on failure.
				o.saveHistoryLog(historyTS, "measure", tokens.RawOutput)
				o.saveHistoryStats(historyTS, "measure", HistoryStats{
					Caller:        "measure",
					TaskID:        taskID,
					Status:        "failed",
					Error:         fmt.Sprintf("claude failure (iteration %d/%d): %v", i+1, totalIssues, err),
					StartedAt:     iterStart.UTC().Format(time.RFC3339),
					Duration:      iterDuration.Round(time.Second).String(),
					DurationS:     int(iterDuration.Seconds()),
					Tokens:        historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
					CostUSD:       tokens.CostUSD,
					NumTurns:      tokens.NumTurns,
					DurationAPIMs: tokens.DurationAPIMs,
					SessionID:     tokens.SessionID,
					LOCBefore:     locBefore,
					LOCAfter:      o.captureLOC(),
				})
				return fmt.Errorf("running Claude (iteration %d/%d): %w", i+1, totalIssues, err)
			}
			logf("iteration %d Claude completed in %s", i+1, iterDuration.Round(time.Second))

			// Save remaining history artifacts (log, issues, stats) after Claude.
			o.saveHistory(historyTS, tokens.RawOutput, outputFile)
			o.saveHistoryStats(historyTS, "measure", HistoryStats{
				Caller:        "measure",
				TaskID:        taskID,
				Status:        "success",
				StartedAt:     iterStart.UTC().Format(time.RFC3339),
				Duration:      iterDuration.Round(time.Second).String(),
				DurationS:     int(iterDuration.Seconds()),
				Tokens:        historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
				CostUSD:       tokens.CostUSD,
				NumTurns:      tokens.NumTurns,
				DurationAPIMs: tokens.DurationAPIMs,
				SessionID:     tokens.SessionID,
				LOCBefore:     locBefore,
				LOCAfter:      o.captureLOC(),
			})

			// Extract YAML from Claude's text output and write to file.
			textOutput := extractTextFromStreamJSON(tokens.RawOutput)
			yamlContent, extractErr := extractYAMLBlock(textOutput)
			if extractErr != nil {
				logf("iteration %d YAML extraction failed: %v", i+1, extractErr)
				if attempt < maxRetries {
					continue // retry
				}
				logf("iteration %d retries exhausted, no YAML extracted", i+1)
				break
			}
			if err := os.WriteFile(outputFile, yamlContent, 0o644); err != nil {
				logf("iteration %d failed to write output file: %v", i+1, err)
				break
			}
			logf("iteration %d extracted YAML, size=%d bytes", i+1, len(yamlContent))

			var importErr error
			var validationErrs []string
			createdIDs, validationErrs, importErr = o.importIssues(outputFile, repo, generation, placeholderNum)
			if importErr != nil {
				logf("iteration %d import failed: %v", i+1, importErr)
				if attempt < maxRetries {
					lastValidationErrors = validationErrs // feed errors back into next prompt
					_ = os.Remove(outputFile)             // best-effort cleanup before retry
					continue                              // retry
				}
				// Retries exhausted: accept with warning (R5).
				logf("iteration %d retries exhausted, accepting last result with warnings", i+1)
				var forceErr error
				createdIDs, forceErr = o.importIssuesForce(outputFile, repo, generation, placeholderNum)
				if forceErr != nil {
					logf("iteration %d force import failed: %v", i+1, forceErr)
				}
			}
			break // success or retries exhausted
		}

		logf("iteration %d imported %d issue(s)", i+1, len(createdIDs))
		allCreatedIDs = append(allCreatedIDs, createdIDs...)

		if len(createdIDs) == 0 && lastOutputFile != "" {
			logf("iteration %d created no issues, keeping %s for inspection", i+1, lastOutputFile)
		} else if lastOutputFile != "" {
			os.Remove(lastOutputFile) // nolint: best-effort temp file cleanup
		}
	}

	// Retry once if measure returned empty but unresolved requirements remain.
	// Claude non-deterministically returns [] on large prompts; a single retry
	// recovers ~95% of these cases (GH-1513).
	if len(allCreatedIDs) == 0 && o.hasUnresolvedRequirements() {
		logf("measure: 0 issues created but unresolved requirements remain — retrying once")

		// Refresh existing issues for the retry.
		refreshed, refreshErr := listActiveIssuesContext(repo, generation)
		if refreshErr == nil {
			existingIssues = refreshed
		}

		timestamp := time.Now().Format("20060102-150405")
		outputFile := filepath.Join(o.cfg.Cobbler.Dir, fmt.Sprintf("measure-%s.yaml", timestamp))

		prompt, promptErr := o.buildMeasurePrompt(o.cfg.Cobbler.UserPrompt, existingIssues, 1)
		if promptErr == nil {
			historyTS := time.Now().Format("2006-01-02-15-04-05")
			o.saveHistoryPrompt(historyTS, "measure", prompt)

			retryStart := time.Now()
			tokens, err := o.runMeasureClaude(prompt, "", o.cfg.Silence(), "--max-turns", "1")
			retryDuration := time.Since(retryStart)

			totalTokens.InputTokens += tokens.InputTokens
			totalTokens.OutputTokens += tokens.OutputTokens
			totalTokens.CostUSD += tokens.CostUSD

			if err == nil {
				o.saveHistory(historyTS, tokens.RawOutput, outputFile)
				o.saveHistoryStats(historyTS, "measure", HistoryStats{
					Caller:    "measure",
					TaskID:    fmt.Sprintf("%d", placeholderNum),
					Status:    "success",
					StartedAt: retryStart.UTC().Format(time.RFC3339),
					Duration:  retryDuration.Round(time.Second).String(),
					DurationS: int(retryDuration.Seconds()),
					Tokens:    historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
					CostUSD:   tokens.CostUSD,
					NumTurns:  tokens.NumTurns,
					LOCBefore: locBefore,
					LOCAfter:  o.captureLOC(),
				})

				textOutput := extractTextFromStreamJSON(tokens.RawOutput)
				yamlContent, extractErr := extractYAMLBlock(textOutput)
				if extractErr == nil {
					if writeErr := os.WriteFile(outputFile, yamlContent, 0o644); writeErr == nil {
						retryIDs, _, importErr := o.importIssues(outputFile, repo, generation, placeholderNum)
						if importErr == nil {
							allCreatedIDs = append(allCreatedIDs, retryIDs...)
							logf("measure: retry created %d issue(s)", len(retryIDs))
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
		comment := fmt.Sprintf("Measure completed. %d iteration(s), %d issue(s) created.", totalIssues, len(allCreatedIDs))
		if totalTokens.CostUSD > 0 {
			comment += fmt.Sprintf("\nCost: $%.2f, Tokens: %din %dout",
				totalTokens.CostUSD, totalTokens.InputTokens, totalTokens.OutputTokens)
		}
		finalizeMeasurePlaceholder(repo, placeholderNum, generation, comment, childNums)
	}

	logf("completed %d iteration(s), %d issue(s) created in %s",
		totalIssues, len(allCreatedIDs), time.Since(measureStart).Round(time.Second))
	return nil
}

func (o *Orchestrator) buildMeasurePrompt(userInput, existingIssues string, limit int, validationErrors ...string) (string, error) {
	tmpl, err := parsePromptTemplate(orDefault(o.cfg.Cobbler.MeasurePrompt, defaultMeasurePrompt))
	if err != nil {
		return "", fmt.Errorf("measure prompt YAML: %w", err)
	}

	planningConst := orDefault(o.cfg.Cobbler.PlanningConstitution, planningConstitution)

	// Load per-phase context file (prd003 R9.8).
	measureCtxPath := filepath.Join(o.cfg.Cobbler.Dir, "measure_context.yaml")
	phaseCtx, phaseErr := loadPhaseContext(measureCtxPath)
	if phaseErr != nil {
		return "", fmt.Errorf("loading measure context: %w", phaseErr)
	}
	if phaseCtx != nil {
		logf("buildMeasurePrompt: using phase context from %s", measureCtxPath)
	} else {
		logf("buildMeasurePrompt: no phase context file, using config defaults")
	}

	// Apply CobblerConfig measure source settings to phaseCtx (GH-565).
	if phaseCtx == nil {
		phaseCtx = &PhaseContext{}
	}
	if o.cfg.Cobbler.MeasureExcludeSource && !phaseCtx.ExcludeSource {
		phaseCtx.ExcludeSource = true
		logf("buildMeasurePrompt: measure_exclude_source=true from config")
	}
	if o.cfg.Cobbler.MeasureSourcePatterns != "" && phaseCtx.SourcePatterns == "" {
		phaseCtx.SourcePatterns = o.cfg.Cobbler.MeasureSourcePatterns
		logf("buildMeasurePrompt: measure_source_patterns set from config")
	}
	if o.cfg.Cobbler.effectiveMeasureExcludeTests() && !phaseCtx.ExcludeTests {
		phaseCtx.ExcludeTests = true
		logf("buildMeasurePrompt: measure_exclude_tests=true, _test.go files will be excluded")
	}
	if o.cfg.Cobbler.MeasureSourceMode != "" && phaseCtx.SourceMode == "" {
		phaseCtx.SourceMode = o.cfg.Cobbler.MeasureSourceMode
		logf("buildMeasurePrompt: measure_source_mode=%q from config", phaseCtx.SourceMode)
	}
	if o.cfg.Cobbler.MeasureSummarizeCommand != "" && phaseCtx.SummarizeCommand == "" {
		phaseCtx.SummarizeCommand = o.cfg.Cobbler.MeasureSummarizeCommand
		logf("buildMeasurePrompt: measure_summarize_command set from config")
	}

	// Auto-derive SourcePatterns from the road-map when MeasureRoadmapSource
	// is enabled and no manual patterns are already set (GH-534).
	if o.cfg.Cobbler.MeasureRoadmapSource && !phaseCtx.ExcludeSource && phaseCtx.SourcePatterns == "" {
		uc, err := selectNextPendingUseCase(o.cfg.Project)
		if err != nil {
			logf("buildMeasurePrompt: road-map source selection error: %v", err)
		} else if uc != nil {
			pkgPaths := parseTouchpointPackages(uc.Touchpoints)
			if len(pkgPaths) > 0 {
				var patterns []string
				for _, p := range pkgPaths {
					patterns = append(patterns, p+"/**/*.go")
				}
				phaseCtx.SourcePatterns = strings.Join(patterns, "\n")
				logf("buildMeasurePrompt: road-map source: UC=%s packages=%v", uc.ID, pkgPaths)
			} else {
				logf("buildMeasurePrompt: road-map source: UC=%s has no package touchpoints, loading all source", uc.ID)
			}
		} else {
			logf("buildMeasurePrompt: road-map source: all use cases done, loading all source")
		}
	}

	projectCtx, ctxErr := buildProjectContext(existingIssues, o.cfg.Project, phaseCtx)
	if ctxErr != nil {
		logf("buildMeasurePrompt: buildProjectContext error: %v", ctxErr)
		projectCtx = &ProjectContext{}
	}

	placeholders := map[string]string{
		"limit":            fmt.Sprintf("%d", limit),
		"lines_min":        fmt.Sprintf("%d", o.cfg.Cobbler.EstimatedLinesMin),
		"lines_max":        fmt.Sprintf("%d", o.cfg.Cobbler.EstimatedLinesMax),
		"max_requirements": fmt.Sprintf("%d", o.cfg.Cobbler.MaxRequirementsPerTask),
	}

	// Inject package_contracts when source mode is "headers" or "custom".
	var measureContracts []OODPackageContractRef
	sourceMode := phaseCtx.SourceMode
	if sourceMode == "headers" || sourceMode == "custom" {
		contracts, _ := loadOODPromptContext()
		if len(contracts) > 0 {
			measureContracts = contracts
			logf("buildMeasurePrompt: injecting %d package_contracts (source_mode=%s)", len(contracts), sourceMode)
		}
	}

	doc := MeasurePromptDoc{
		Role:                    tmpl.Role,
		ProjectContext:          projectCtx,
		PlanningConstitution:    parseYAMLNode(planningConst),
		IssueFormatConstitution: parseYAMLNode(issueFormatConstitution),
		Task:                    substitutePlaceholders(tmpl.Task, placeholders),
		Constraints:             substitutePlaceholders(tmpl.Constraints, placeholders),
		OutputFormat:            substitutePlaceholders(tmpl.OutputFormat, placeholders),
		GoldenExample:           o.cfg.Cobbler.GoldenExample,
		AdditionalContext:       userInput,
		ValidationErrors:        validationErrors,
		PackageContracts:        measureContracts,
	}

	// Enforce releases scope.
	activeReleases := filterImplementedReleases(o.cfg.Project.Releases)
	activeRelease := filterImplementedRelease(o.cfg.Project.Release)
	doc.Constraints += measureReleasesConstraint(activeReleases, activeRelease)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("marshaling measure prompt: %w", err)
	}

	logf("buildMeasurePrompt: %d bytes limit=%d userInput=%v",
		len(out), limit, userInput != "")
	return string(out), nil
}

// proposedIssue is aliased from internal/github in issues_gh.go.

// importIssues imports proposed issues from a YAML file into GitHub.
func (o *Orchestrator) importIssues(yamlFile, repo, generation string, ph int) ([]string, []string, error) {
	return o.importIssuesImpl(yamlFile, repo, generation, false, ph)
}

// importIssuesForce imports issues bypassing enforcing validation.
func (o *Orchestrator) importIssuesForce(yamlFile, repo, generation string, ph int) ([]string, error) {
	ids, _, err := o.importIssuesImpl(yamlFile, repo, generation, true, ph)
	return ids, err
}

func (o *Orchestrator) importIssuesImpl(yamlFile, repo, generation string, skipEnforcement bool, ph int) ([]string, []string, error) {
	logf("importIssues: reading %s", yamlFile)
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, nil, fmt.Errorf("reading YAML file: %w", err)
	}
	logf("importIssues: read %d bytes", len(data))

	var issues []proposedIssue
	if err := yaml.Unmarshal(data, &issues); err != nil {
		logf("importIssues: YAML parse error: %v", err)
		return nil, nil, fmt.Errorf("parsing YAML: %w", err)
	}

	logf("importIssues: parsed %d proposed issue(s)", len(issues))
	for i, issue := range issues {
		logf("importIssues: [%d] title=%q dep=%d", i, issue.Title, issue.Dependency)
	}

	// Validate proposed issues against P9/P7 rules and completed R-items (GH-1386).
	subItemCounts := loadPRDSubItemCounts()
	reqStates := loadRequirementStates(o.cfg.Cobbler.Dir)
	vr := validateMeasureOutput(issues, o.cfg.Cobbler.MaxRequirementsPerTask, subItemCounts, reqStates)
	if len(vr.Warnings) > 0 {
		logf("importIssues: %d warning(s)", len(vr.Warnings))
	}
	if vr.HasErrors() && o.cfg.Cobbler.EnforceMeasureValidation && !skipEnforcement {
		return nil, vr.Errors, fmt.Errorf("measure validation failed (%d error(s)): %s",
			len(vr.Errors), strings.Join(vr.Errors, "; "))
	}

	// Deduplicate: fetch existing issues for this generation and skip any
	// proposed issue whose normalized title or output files match an existing
	// one (GH-1026, GH-1352, GH-1373).
	existingTitles := make(map[string]int) // normalized title → issue number
	existingFiles := make(map[string]int)  // file path → issue number
	if existing, err := listAllCobblerIssues(repo, generation); err == nil {
		hasOpen := false
		for _, ex := range existing {
			if ex.State == "open" {
				hasOpen = true
				break
			}
		}
		if hasOpen {
			for _, ex := range existing {
				existingTitles[normalizeIssueTitle(ex.Title)] = ex.Number
				for _, fp := range extractDescriptionFiles(ex.Description) {
					existingFiles[fp] = ex.Number
				}
			}
		}
	}
	var filtered []proposedIssue
	for _, issue := range issues {
		norm := normalizeIssueTitle(issue.Title)
		if dup, ok := existingTitles[norm]; ok {
			logf("importIssues: skipping duplicate %q — title matches #%d", issue.Title, dup)
			continue
		}
		// Check if any proposed output file overlaps with an existing issue (GH-1373).
		if dup := fileOverlap(extractDescriptionFiles(issue.Description), existingFiles); dup > 0 {
			logf("importIssues: skipping duplicate %q — output files overlap with #%d", issue.Title, dup)
			continue
		}
		filtered = append(filtered, issue)
	}
	issues = filtered

	// Create all issues on GitHub as separate stitch tasks (GH-1367).
	// The measure placeholder remains a distinct [measure] issue.
	var ids []string
	for _, issue := range issues {
		logf("importIssues: creating task %d: %s (dep=%d)", issue.Index, issue.Title, issue.Dependency)
		ghNum, err := createCobblerIssue(repo, generation, issue)
		if err != nil {
			logf("importIssues: createCobblerIssue failed for %q: %v", issue.Title, err)
			continue
		}
		ids = append(ids, fmt.Sprintf("%d", ghNum))
	}

	if len(ids) > 0 {
		waitForIssuesVisible(repo, generation, len(ids))
		if err := promoteReadyIssues(repo, generation); err != nil {
			logf("importIssues: promoteReadyIssues warning: %v", err)
		}
	}
	logf("importIssues: %d of %d issue(s) imported", len(ids), len(issues))

	// Append new issues to the persistent measure list.
	appendMeasureLog(o.cfg.Cobbler.Dir, issues)

	return ids, nil, nil
}

// fileOverlap returns the issue number of the first existing issue whose files
// overlap with the proposed file list. Returns 0 if no overlap is found.
func fileOverlap(proposedFiles []string, existingFiles map[string]int) int {
	for _, fp := range proposedFiles {
		if dup, ok := existingFiles[fp]; ok {
			return dup
		}
	}
	return 0
}

// saveHistory persists measure artifacts (log, issues YAML) to the configured
// history directory.
func (o *Orchestrator) saveHistory(ts string, rawOutput []byte, issuesFile string) {
	o.saveHistoryLog(ts, "measure", rawOutput)

	dir := o.historyDir()
	if dir == "" {
		return
	}
	base := ts + "-measure"
	if data, err := os.ReadFile(issuesFile); err == nil {
		if err := os.WriteFile(filepath.Join(dir, base+"-issues.yaml"), data, 0o644); err != nil {
			logf("saveHistory: write issues: %v", err)
		}
	}
}
