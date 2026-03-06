// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	// max_requirements_per_task. This is advisory — measure continues
	// regardless so the operator can restructure PRDs later.
	if o.cfg.Cobbler.MaxRequirementsPerTask > 0 {
		warnOversizedGroups(o.cfg.Cobbler.MaxRequirementsPerTask)
	}

	// Route target-repo defects to the target repo (prd003 R11).
	// Schema errors and constitution drift are bugs in the target project's
	// files; filing them as GitHub issues prevents Claude from proposing them
	// as measure tasks, which would fail validation and block the cycle.
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
	// Between calls, import the result into GitHub Issues and refresh the issue list
	// so subsequent calls see existing issues and avoid duplicates.
	totalIssues := o.cfg.Cobbler.MaxMeasureIssues
	var allCreatedIDs []string
	var totalTokens ClaudeResult
	maxRetries := o.cfg.Cobbler.MaxMeasureRetries

	for i := 0; i < totalIssues; i++ {
		logf("--- iteration %d/%d ---", i+1, totalIssues)

		// Refresh existing issues from GitHub before each call (except the first,
		// where we already have them).
		if i > 0 {
			refreshed, refreshErr := listActiveIssuesContext(repo, generation)
			if refreshErr != nil {
				logf("measure: warning: refreshing issue list: %v", refreshErr)
			} else {
				existingIssues = refreshed
			}
		}

		// Create a placeholder issue so users can see measure is running Claude.
		// The placeholder has no cobbler labels and is invisible to stitch and to
		// the measure context prompt. It is closed after the iteration regardless
		// of outcome (GH-568). The defer below closes it on any early-return path
		// (e.g. Claude failure) so it never stays open as an orphan (GH-747).
		placeholderNum, placeholderErr := createMeasuringPlaceholder(repo, generation, i+1)
		if placeholderErr != nil {
			logf("measure: warning: createMeasuringPlaceholder: %v", placeholderErr)
		}
		placeholderResolved := false
		if placeholderNum > 0 {
			defer func(num int) {
				if !placeholderResolved {
					closeMeasuringPlaceholderWithComment(repo, num, "Measure did not complete; closed automatically.")
				}
			}(placeholderNum)
		}

		var createdIDs []string
		var lastOutputFile string
		var lastValidationErrors []string // errors from previous attempt, fed back into retry prompt
		placeholderUpgraded := false     // set when importIssues upgraded the placeholder in-place

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
			tokens, err := o.runClaude(prompt, "", o.cfg.Silence(), "--max-turns", "1")
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

		// Track whether the placeholder was upgraded in-place (GH-578).
		phStr := fmt.Sprintf("%d", placeholderNum)
		for _, id := range createdIDs {
			if id == phStr {
				placeholderUpgraded = true
				break
			}
		}

		// Close the placeholder only when it was not upgraded in-place (GH-578).
		// An upgraded placeholder became the task issue; closing it destroys the task.
		// Mark placeholderResolved so the defer registered above is a no-op (GH-747).
		placeholderResolved = true
		if placeholderNum > 0 && !placeholderUpgraded {
			closeMeasuringPlaceholder(repo, placeholderNum)
		}

		// Record invocation metrics on each created issue.

		allCreatedIDs = append(allCreatedIDs, createdIDs...)

		if len(createdIDs) == 0 && lastOutputFile != "" {
			logf("iteration %d created no issues, keeping %s for inspection", i+1, lastOutputFile)
		} else if lastOutputFile != "" {
			os.Remove(lastOutputFile) // nolint: best-effort temp file cleanup
		}
	}

	logf("completed %d iteration(s), %d issue(s) created in %s",
		totalIssues, len(allCreatedIDs), time.Since(measureStart).Round(time.Second))
	return nil
}

// truncateSHA returns the first 8 characters of a SHA, or the full
// string if shorter.
func truncateSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
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
	// Config values are overridden only when the phaseCtx file has not
	// already set the field (file-level wins over config-level).
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
	// Apply test exclusion setting (GH-616). Default true; file-level wins
	// when already set to true by the phaseCtx file.
	if o.cfg.Cobbler.effectiveMeasureExcludeTests() && !phaseCtx.ExcludeTests {
		phaseCtx.ExcludeTests = true
		logf("buildMeasurePrompt: measure_exclude_tests=true, _test.go files will be excluded")
	}
	// Wire source summarization mode (GH-617, prd003 R12). Config wins when
	// the phaseCtx file has not already set the mode (file-level wins).
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

	// Inject package_contracts when source mode is "headers" or "custom"
	// and any PRD declares a package_contract. The contracts give the
	// measure agent structured API context alongside (or instead of) source.
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

	// Enforce releases scope: the roadmap is not filtered by release, so
	// without an explicit constraint the agent may propose tasks from adjacent
	// releases after exhausting the configured ones.
	// Filter out releases already marked as implemented in road-map.yaml so a
	// stale releases config value does not direct Claude to re-implement work
	// that is already done (GH-847).
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

// measureReleasesConstraint returns a hard constraint string to append to the
// measure prompt when a release scope is configured. Returns "" when no scope
// is set. Releases (list) takes precedence over Release (single string).
func measureReleasesConstraint(releases []string, release string) string {
	if len(releases) > 0 {
		return fmt.Sprintf(
			"\n\nRelease scope: You MUST only propose tasks for use cases in releases [%s]. Do not propose tasks for any other release.",
			strings.Join(releases, ", "),
		)
	}
	if release != "" {
		return fmt.Sprintf(
			"\n\nRelease scope: You MUST only propose tasks for use cases in release %q or earlier. Do not propose tasks for later releases.",
			release,
		)
	}
	return ""
}

// filterImplementedReleases returns a copy of releases with any entry whose
// road-map status is "implemented" or "done" removed. Releases not found in
// road-map.yaml are kept (unknown status is not treated as implemented).
// Returns nil when all releases are filtered out.
func filterImplementedReleases(releases []string) []string {
	if len(releases) == 0 {
		return releases
	}
	rm := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return releases
	}
	status := make(map[string]string, len(rm.Releases))
	for _, rel := range rm.Releases {
		status[rel.Version] = rel.Status
	}
	var out []string
	for _, r := range releases {
		if ucStatusDone(status[r]) {
			logf("filterImplementedReleases: dropping implemented release %s from constraint", r)
			continue
		}
		out = append(out, r)
	}
	return out
}

// filterImplementedRelease returns the release string unchanged unless the
// road-map marks that release as implemented/done, in which case "" is
// returned so no legacy single-release constraint is emitted.
func filterImplementedRelease(release string) string {
	if release == "" {
		return ""
	}
	rm := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return release
	}
	for _, rel := range rm.Releases {
		if rel.Version == release && ucStatusDone(rel.Status) {
			logf("filterImplementedRelease: dropping implemented release %s from constraint", release)
			return ""
		}
	}
	return release
}

type proposedIssue struct {
	Index       int    `yaml:"index"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Dependency  int    `yaml:"dependency"`
}

// importIssues imports proposed issues from a YAML file into GitHub. It returns
// the created issue IDs, any validation error strings (for retry feedback), and
// a non-nil error when validation fails in enforcing mode. ph is the measuring
// placeholder issue number; when ph > 0 and exactly one issue is proposed, the
// placeholder is upgraded in-place instead of creating a new issue (GH-578).
func (o *Orchestrator) importIssues(yamlFile, repo, generation string, ph int) ([]string, []string, error) {
	return o.importIssuesImpl(yamlFile, repo, generation, false, ph)
}

// importIssuesForce imports issues bypassing enforcing validation. Used when
// retries are exhausted to accept the last result with warnings (R5). ph is
// the placeholder number passed through to importIssuesImpl (GH-578).
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

	// Validate proposed issues against P9/P7 rules. Load PRD sub-item
	// counts so the validator can expand group references (GH-122).
	subItemCounts := loadPRDSubItemCounts()
	vr := validateMeasureOutput(issues, o.cfg.Cobbler.MaxRequirementsPerTask, subItemCounts)
	if len(vr.Warnings) > 0 {
		logf("importIssues: %d warning(s)", len(vr.Warnings))
	}
	if vr.HasErrors() && o.cfg.Cobbler.EnforceMeasureValidation && !skipEnforcement {
		return nil, vr.Errors, fmt.Errorf("measure validation failed (%d error(s)): %s",
			len(vr.Errors), strings.Join(vr.Errors, "; "))
	}

	// Create all issues on GitHub. When a placeholder number is given and exactly
	// one issue is proposed, upgrade the placeholder in-place instead of creating
	// a new issue, eliminating the two-issue dance (GH-578).
	var ids []string
	upgraded := false
	if ph > 0 && len(issues) == 1 {
		if err := upgradeMeasuringPlaceholder(repo, ph, generation, issues[0]); err != nil {
			logf("importIssues: upgradeMeasuringPlaceholder #%d failed, falling back to createCobblerIssue: %v", ph, err)
		} else {
			ids = append(ids, fmt.Sprintf("%d", ph))
			upgraded = true
		}
	}
	if !upgraded {
		for _, issue := range issues {
			logf("importIssues: creating task %d: %s (dep=%d)", issue.Index, issue.Title, issue.Dependency)
			ghNum, err := createCobblerIssue(repo, generation, issue)
			if err != nil {
				logf("importIssues: createCobblerIssue failed for %q: %v", issue.Title, err)
				continue
			}
			ids = append(ids, fmt.Sprintf("%d", ghNum))
		}
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

// issueDescription is the subset of fields parsed from an issue description
// YAML for advisory validation.
type issueDescription struct {
	DeliverableType    string              `yaml:"deliverable_type"`
	Files              []issueDescFile     `yaml:"files"`
	Requirements       []issueDescItem     `yaml:"requirements"`
	AcceptanceCriteria []issueDescItem     `yaml:"acceptance_criteria"`
	DesignDecisions    []issueDescItem     `yaml:"design_decisions"`
}

type issueDescFile struct {
	Path string `yaml:"path"`
}

type issueDescItem struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// validationResult holds the outcome of measure output validation.
type validationResult struct {
	Warnings []string // advisory issues (logged but do not block import)
	Errors   []string // blocking issues (cause rejection in enforcing mode)
}

// HasErrors returns true if the validation found blocking issues.
func (v validationResult) HasErrors() bool {
	return len(v.Errors) > 0
}

// validateMeasureOutput checks proposed issues against P9 granularity ranges
// and P7 file naming conventions. Returns structured warnings and errors.
// All issues are logged regardless of enforcing mode. maxReqs is the
// operator-configured requirement cap (0 = unlimited). subItemCounts maps
// PRD stems to group IDs to sub-item counts; when a task requirement
// references a PRD group, the expanded sub-item count is used instead of 1.
// Expanded-count violations are logged as warnings (best-effort), not errors.
func validateMeasureOutput(issues []proposedIssue, maxReqs int, subItemCounts map[string]map[string]int) validationResult {
	var result validationResult
	for _, issue := range issues {
		var desc issueDescription
		if err := yaml.Unmarshal([]byte(issue.Description), &desc); err != nil {
			msg := fmt.Sprintf("[%d] %q: could not parse description: %v", issue.Index, issue.Title, err)
			logf("validateMeasureOutput: %s", msg)
			result.Warnings = append(result.Warnings, msg)
			continue
		}

		rCount := len(desc.Requirements)
		acCount := len(desc.AcceptanceCriteria)
		dCount := len(desc.DesignDecisions)

		// Compute expanded requirement count by resolving PRD group
		// references to their sub-item counts (GH-122).
		expandedCount := expandedRequirementCount(desc.Requirements, subItemCounts)

		// Enforce max_requirements_per_task on the expanded sub-item count,
		// not the top-level group count (GH-535). A requirement referencing
		// "prd003 R2" where R2 has 10 sub-items counts as 10, not 1.
		if maxReqs > 0 && expandedCount > maxReqs {
			msg := fmt.Sprintf("[%d] %q: expanded sub-item count is %d, max is %d", issue.Index, issue.Title, expandedCount, maxReqs)
			logf("validateMeasureOutput: %s", msg)
			result.Errors = append(result.Errors, msg)
		}

		if desc.DeliverableType == "code" {
			if rCount < 5 || rCount > 8 {
				msg := fmt.Sprintf("[%d] %q: requirement count %d outside P9 range 5-8", issue.Index, issue.Title, rCount)
				logf("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
			if acCount < 5 || acCount > 8 {
				msg := fmt.Sprintf("[%d] %q: acceptance criteria count %d outside P9 range 5-8", issue.Index, issue.Title, acCount)
				logf("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
			if dCount < 3 || dCount > 5 {
				msg := fmt.Sprintf("[%d] %q: design decision count %d outside P9 range 3-5", issue.Index, issue.Title, dCount)
				logf("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
		} else if desc.DeliverableType == "documentation" {
			if rCount < 2 || rCount > 4 {
				msg := fmt.Sprintf("[%d] %q: requirement count %d outside P9 doc range 2-4", issue.Index, issue.Title, rCount)
				logf("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
			if acCount < 3 || acCount > 5 {
				msg := fmt.Sprintf("[%d] %q: acceptance criteria count %d outside P9 doc range 3-5", issue.Index, issue.Title, acCount)
				logf("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
		}

		// Check for P7 violation: file named after its package.
		for _, f := range desc.Files {
			parts := strings.Split(f.Path, "/")
			if len(parts) >= 2 {
				dir := parts[len(parts)-2]
				file := parts[len(parts)-1]
				if file == dir+".go" || file == dir+"_test.go" {
					msg := fmt.Sprintf("[%d] %q: file %s matches package name (P7 violation)", issue.Index, issue.Title, f.Path)
					logf("validateMeasureOutput: %s", msg)
					result.Errors = append(result.Errors, msg)
				}
			}
		}
	}
	return result
}

// prdRefPattern matches PRD requirement references in task requirement text.
// Examples: "prd003 R2", "prd004-ts R1.3", "prd001-orchestrator-core R5".
// Group 1 = PRD stem (e.g., "prd003" or "prd004-ts").
// Group 2 = requirement group number (e.g., "2" from "R2").
// Group 3 = optional sub-item number (e.g., "3" from "R1.3"); empty for groups.
var prdRefPattern = regexp.MustCompile(`(prd\d+[-\w]*)\s+R(\d+)(?:\.(\d+))?`)

// loadPRDSubItemCounts loads all PRDs from the standard path and returns a
// map of PRD stem -> group key -> sub-item count. A group with no sub-items
// maps to 1. The stem is the filename without path and extension (e.g.,
// "prd003-cobbler-workflows"); an additional entry keyed by the short prefix
// (e.g., "prd003") is added for fuzzy matching.
func loadPRDSubItemCounts() map[string]map[string]int {
	paths, _ := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	counts := make(map[string]map[string]int, len(paths)*2)
	for _, path := range paths {
		prd := loadYAML[PRDDoc](path)
		if prd == nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		groupCounts := make(map[string]int, len(prd.Requirements))
		for key, group := range prd.Requirements {
			if len(group.Items) > 0 {
				groupCounts[key] = len(group.Items)
			} else {
				groupCounts[key] = 1
			}
		}
		counts[stem] = groupCounts
		// Add short prefix entry (e.g., "prd003") for fuzzy matching.
		if idx := strings.IndexByte(stem, '-'); idx > 0 {
			short := stem[:idx]
			if _, exists := counts[short]; !exists {
				counts[short] = groupCounts
			}
		}
	}
	return counts
}

// expandedRequirementCount computes the effective requirement count by
// parsing PRD group references from each requirement's text and expanding
// groups to their sub-item counts. A requirement referencing "prd003 R2"
// where R2 has 4 sub-items counts as 4, not 1. Requirements without a
// recognized PRD reference or referencing a specific sub-item (R1.3)
// count as 1.
func expandedRequirementCount(reqs []issueDescItem, subItemCounts map[string]map[string]int) int {
	if len(subItemCounts) == 0 {
		return len(reqs)
	}
	total := 0
	for _, req := range reqs {
		matches := prdRefPattern.FindStringSubmatch(req.Text)
		if matches == nil {
			total++
			continue
		}
		prdStem := matches[1]
		groupNum := matches[2]
		subItem := matches[3]

		// Specific sub-item reference (e.g., R1.3) counts as 1.
		if subItem != "" {
			total++
			continue
		}

		// Group reference (e.g., R2). Look up sub-item count.
		groupKey := "R" + groupNum
		if groups, ok := subItemCounts[prdStem]; ok {
			if count, found := groups[groupKey]; found {
				total += count
				continue
			}
		}
		// PRD or group not found — count as 1.
		total++
	}
	return total
}

// warnOversizedGroups loads PRDs and logs a warning for each requirement
// group whose sub-item count exceeds maxReqs. This is advisory and runs
// before the measure prompt is built so operators can restructure PRDs.
func warnOversizedGroups(maxReqs int) {
	paths, _ := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	for _, path := range paths {
		prd := loadYAML[PRDDoc](path)
		if prd == nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		keys := make([]string, 0, len(prd.Requirements))
		for k := range prd.Requirements {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			group := prd.Requirements[key]
			if len(group.Items) > maxReqs {
				logf("warning: %s %s has %d sub-items (max_requirements_per_task=%d); consider splitting this requirement group",
					stem, key, len(group.Items), maxReqs)
			}
		}
	}
}

// saveHistory persists measure artifacts (log, issues YAML) to the configured
// history directory. The prompt is saved separately before runClaude.
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

// appendMeasureLog merges newIssues into the persistent measure.yaml list.
// measure.yaml is a single growing YAML list of all issues proposed across runs.
func appendMeasureLog(cobblerDir string, newIssues []proposedIssue) {
	logPath := filepath.Join(cobblerDir, "measure.yaml")

	var existing []proposedIssue
	if data, err := os.ReadFile(logPath); err == nil {
		if err := yaml.Unmarshal(data, &existing); err != nil {
			logf("appendMeasureLog: could not parse existing list, starting fresh: %v", err)
			existing = nil
		}
	}

	combined := append(existing, newIssues...)
	out, err := yaml.Marshal(combined)
	if err != nil {
		logf("appendMeasureLog: marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(logPath, out, 0o644); err != nil {
		logf("appendMeasureLog: write failed: %v", err)
		return
	}
	logf("appendMeasureLog: %d total issues in %s", len(combined), logPath)
}
