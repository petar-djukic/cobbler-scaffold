// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed prompts/measure.yaml
var defaultMeasurePrompt string

//go:embed constitutions/planning.yaml
var planningConstitution string

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
	prompt := o.buildMeasurePrompt("", "", 1, "measure-out.yaml")
	fmt.Print(prompt)
	return nil
}

// RunMeasure runs the measure workflow using Config settings.
// It uses an iterative strategy: Claude is called once per issue with limit=1,
// and the issue is recorded in beads between calls. Each subsequent call sees
// the updated issue list, enabling Claude to reason about dependencies and
// avoid duplicates. This avoids the super-linear thinking-time scaling observed
// when requesting multiple issues in a single call (see eng04-measure-scaling).
func (o *Orchestrator) RunMeasure() error {
	setPhase("measure")
	defer clearPhase()
	measureStart := time.Now()
	logf("starting (iterative, %d issue(s) requested)", o.cfg.Cobbler.MaxMeasureIssues)
	o.logConfig("measure")

	if err := o.checkClaude(); err != nil {
		return err
	}

	if err := o.requireBeads(); err != nil {
		logf("beads not initialized: %v", err)
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

	if err := ensureOnBranch(branch); err != nil {
		logf("ensureOnBranch failed: %v", err)
		return fmt.Errorf("switching to branch: %w", err)
	}

	_ = os.MkdirAll(o.cfg.Cobbler.Dir, 0o755)

	// Clean up old measure temp files.
	matches, _ := filepath.Glob(o.cfg.Cobbler.Dir + "measure-*.yaml")
	if len(matches) > 0 {
		logf("cleaning %d old measure temp file(s)", len(matches))
	}
	for _, f := range matches {
		os.Remove(f)
	}

	// Get initial state.
	existingIssues := getExistingIssues()
	issueCount := countJSONArray(existingIssues)
	commitSHA, _ := gitRevParseHEAD()

	logf("found %d existing issue(s), maxMeasureIssues=%d, commit=%s",
		issueCount, o.cfg.Cobbler.MaxMeasureIssues, commitSHA)

	// Create a beads tracking issue for this measure invocation.
	trackingID := o.createMeasureTrackingIssue(branch, commitSHA, issueCount)

	// Snapshot LOC before Claude.
	locBefore := o.captureLOC()
	logf("locBefore prod=%d test=%d", locBefore.Production, locBefore.Test)

	// Iterative measure: call Claude once per issue with limit=1.
	// Between calls, import the result into beads and refresh the issue list
	// so subsequent calls see existing issues and avoid duplicates.
	totalIssues := o.cfg.Cobbler.MaxMeasureIssues
	var allCreatedIDs []string
	var totalTokens ClaudeResult
	claudeStart := time.Now() // overall Claude start for tracking

	for i := 0; i < totalIssues; i++ {
		logf("--- iteration %d/%d ---", i+1, totalIssues)

		// Refresh existing issues from beads before each call (except the first,
		// where we already have them).
		if i > 0 {
			existingIssues = getExistingIssues()
		}

		timestamp := time.Now().Format("20060102-150405")
		outputFile := filepath.Join(o.cfg.Cobbler.Dir, fmt.Sprintf("measure-%s.yaml", timestamp))

		prompt := o.buildMeasurePrompt(o.cfg.Cobbler.UserPrompt, existingIssues, 1, outputFile)
		logf("iteration %d prompt built, length=%d bytes", i+1, len(prompt))

		iterStart := time.Now()
		tokens, err := o.runClaude(prompt, "", o.cfg.Silence())
		iterDuration := time.Since(iterStart)

		totalTokens.InputTokens += tokens.InputTokens
		totalTokens.OutputTokens += tokens.OutputTokens
		totalTokens.CacheCreationTokens += tokens.CacheCreationTokens
		totalTokens.CacheReadTokens += tokens.CacheReadTokens
		totalTokens.CostUSD += tokens.CostUSD

		if err != nil {
			logf("Claude failed on iteration %d after %s: %v",
				i+1, iterDuration.Round(time.Second), err)
			o.closeMeasureTrackingIssue(trackingID, claudeStart, measureStart,
				totalTokens, locBefore, o.captureLOC(), len(allCreatedIDs), err)
			return fmt.Errorf("running Claude (iteration %d/%d): %w", i+1, totalIssues, err)
		}
		logf("iteration %d Claude completed in %s", i+1, iterDuration.Round(time.Second))

		// Save history artifacts (prompt, log, issues, stats) before importing.
		historyTS := time.Now().Format("2006-01-02-15-04-05")
		o.saveHistory(historyTS, prompt, tokens.RawOutput, outputFile)
		o.saveHistoryStats(historyTS, "measure", HistoryStats{
			Caller:    "measure",
			StartedAt: iterStart.UTC().Format(time.RFC3339),
			Duration:  iterDuration.Round(time.Second).String(),
			DurationS: int(iterDuration.Seconds()),
			Tokens:    historyTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens},
			CostUSD:   tokens.CostUSD,
			LOCBefore: locBefore,
			LOCAfter:  o.captureLOC(),
		})

		// Import the proposed issue.
		if _, statErr := os.Stat(outputFile); statErr != nil {
			logf("iteration %d output file not found (Claude may not have written it)", i+1)
			continue
		}

		fileInfo, _ := os.Stat(outputFile)
		logf("iteration %d output file size=%d bytes", i+1, fileInfo.Size())

		createdIDs, importErr := o.importIssues(outputFile)
		if importErr != nil {
			logf("iteration %d import failed: %v", i+1, importErr)
			continue
		}
		logf("iteration %d imported %d issue(s)", i+1, len(createdIDs))

		// Record invocation metrics on each created issue.
		rec := InvocationRecord{
			Caller:    "measure",
			StartedAt: iterStart.UTC().Format(time.RFC3339),
			DurationS: int(iterDuration.Seconds()),
			Tokens:    claudeTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens, CostUSD: tokens.CostUSD},
			LOCBefore: locBefore,
			LOCAfter:  o.captureLOC(),
		}
		for _, id := range createdIDs {
			recordInvocation(id, rec)
		}

		allCreatedIDs = append(allCreatedIDs, createdIDs...)

		if len(createdIDs) == 0 {
			logf("iteration %d created no issues, keeping %s for inspection", i+1, outputFile)
		} else {
			os.Remove(outputFile)
		}
	}

	// Close tracking issue with aggregate metrics.
	locAfter := o.captureLOC()
	o.closeMeasureTrackingIssue(trackingID, claudeStart, measureStart,
		totalTokens, locBefore, locAfter, len(allCreatedIDs), nil)

	logf("completed %d iteration(s), %d issue(s) created in %s",
		totalIssues, len(allCreatedIDs), time.Since(measureStart).Round(time.Second))
	return nil
}

// createMeasureTrackingIssue creates a beads issue to track the measure
// invocation. The description includes the branch, commit SHA, and number
// of existing issues being analyzed. Returns the issue ID or "" on failure.
func (o *Orchestrator) createMeasureTrackingIssue(branch, commitSHA string, existingIssueCount int) string {
	title := fmt.Sprintf("measure: plan on %s at %s", branch, truncateSHA(commitSHA))
	description := fmt.Sprintf(
		"Measure invocation.\n\nBranch: %s\nCommit: %s\nExisting issues: %d\nMax new issues: %d",
		branch, commitSHA, existingIssueCount, o.cfg.Cobbler.MaxMeasureIssues,
	)

	out, err := bdCreateTask(title, description)
	if err != nil {
		logf("createMeasureTrackingIssue: bd create failed: %v", err)
		return ""
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &created); err != nil || created.ID == "" {
		logf("createMeasureTrackingIssue: parse failed: %v", err)
		return ""
	}

	logf("tracking issue created: %s", created.ID)

	// Claim the issue immediately.
	if err := bdUpdateStatus(created.ID, "in_progress"); err != nil {
		logf("createMeasureTrackingIssue: status update warning: %v", err)
	}
	o.beadsCommit(fmt.Sprintf("Open measure tracking issue %s", created.ID))

	return created.ID
}

// closeMeasureTrackingIssue records invocation metrics and closes the
// tracking issue. If trackingID is empty, this is a no-op.
func (o *Orchestrator) closeMeasureTrackingIssue(trackingID string, claudeStart, measureStart time.Time, tokens ClaudeResult, locBefore, locAfter LocSnapshot, issuesCreated int, claudeErr error) {
	if trackingID == "" {
		return
	}

	rec := InvocationRecord{
		Caller:    "measure",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		DurationS: int(time.Since(measureStart).Seconds()),
		Tokens:    claudeTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens, CacheCreation: tokens.CacheCreationTokens, CacheRead: tokens.CacheReadTokens, CostUSD: tokens.CostUSD},
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
	}
	recordInvocation(trackingID, rec)

	// Add a summary comment.
	status := "success"
	if claudeErr != nil {
		status = fmt.Sprintf("failed: %v", claudeErr)
	}
	summary := fmt.Sprintf("issues_created: %d, status: %s", issuesCreated, status)
	_ = bdCommentAdd(trackingID, summary)

	if err := bdClose(trackingID); err != nil {
		logf("closeMeasureTrackingIssue: bd close warning: %v", err)
	}
	o.beadsCommit(fmt.Sprintf("Close measure tracking issue %s", trackingID))
}

// truncateSHA returns the first 8 characters of a SHA, or the full
// string if shorter.
func truncateSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func getExistingIssues() string {
	if _, err := exec.LookPath(binBd); err != nil {
		logf("getExistingIssues: bd not on PATH: %v", err)
		return "[]"
	}
	out, err := bdListJSON()
	if err != nil {
		logf("getExistingIssues: bd list failed: %v", err)
		return "[]"
	}

	// Extract IDs from the list and fetch full content for each issue.
	var issues []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &issues); err != nil || len(issues) == 0 {
		logf("getExistingIssues: parse or empty: err=%v len=%d", err, len(issues))
		return string(out)
	}

	logf("getExistingIssues: fetching full content for %d issue(s)", len(issues))
	var fullIssues []json.RawMessage
	for _, issue := range issues {
		detail, err := bdShowJSON(issue.ID)
		if err != nil {
			logf("getExistingIssues: bd show %s failed: %v", issue.ID, err)
			continue
		}
		// bd show --json may return a single-element array [{}] instead of
		// a plain object {}. Unwrap the array to avoid nested arrays.
		trimmed := bytes.TrimSpace(detail)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			var arr []json.RawMessage
			if err := json.Unmarshal(trimmed, &arr); err == nil && len(arr) == 1 {
				fullIssues = append(fullIssues, arr[0])
				continue
			}
		}
		fullIssues = append(fullIssues, json.RawMessage(detail))
	}

	result, err := json.Marshal(fullIssues)
	if err != nil {
		logf("getExistingIssues: marshal failed: %v", err)
		return string(out)
	}
	logf("getExistingIssues: got %d full issue(s), %d bytes", len(fullIssues), len(result))
	return string(result)
}

func countJSONArray(jsonStr string) int {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return 0
	}
	return len(arr)
}

func (o *Orchestrator) buildMeasurePrompt(userInput, existingIssues string, limit int, outputPath string) string {
	promptYAML := o.cfg.Cobbler.MeasurePrompt
	if promptYAML == "" {
		promptYAML = defaultMeasurePrompt
	}

	def, err := parsePromptDef(promptYAML)
	if err != nil {
		panic(fmt.Sprintf("measure prompt YAML: %v", err))
	}

	planningConst := o.cfg.Cobbler.PlanningConstitution
	if planningConst == "" {
		planningConst = planningConstitution
	}

	projectCtx, ctxErr := buildProjectContext(existingIssues, o.cfg.Project.GoSourceDirs)
	if ctxErr != nil {
		logf("buildMeasurePrompt: buildProjectContext error: %v", ctxErr)
		projectCtx = "# Error building project context\n"
	}

	data := map[string]string{
		"project_context":       projectCtx,
		"planning_constitution": planningConst,
		"output_path":           outputPath,
		"limit":                 fmt.Sprintf("%d", limit),
		"lines_min":             fmt.Sprintf("%d", o.cfg.Cobbler.EstimatedLinesMin),
		"lines_max":             fmt.Sprintf("%d", o.cfg.Cobbler.EstimatedLinesMax),
		"user_input":            userInput,
	}

	logf("buildMeasurePrompt: projectCtx=%d bytes limit=%d userInput=%v",
		len(projectCtx), limit, userInput != "")
	return renderPrompt(def, data)
}

type proposedIssue struct {
	Index       int    `yaml:"index"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Dependency  int    `yaml:"dependency"`
}

func (o *Orchestrator) importIssues(yamlFile string) ([]string, error) {
	logf("importIssues: reading %s", yamlFile)
	data, err := os.ReadFile(yamlFile)
	if err != nil {
		return nil, fmt.Errorf("reading YAML file: %w", err)
	}
	logf("importIssues: read %d bytes", len(data))

	var issues []proposedIssue
	if err := yaml.Unmarshal(data, &issues); err != nil {
		logf("importIssues: YAML parse error: %v", err)
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	logf("importIssues: parsed %d proposed issue(s)", len(issues))
	for i, issue := range issues {
		logf("importIssues: [%d] title=%q dep=%d", i, issue.Title, issue.Dependency)
	}

	// Pass 1: create all issues and collect their beads IDs.
	indexToID := make(map[int]string)
	for _, issue := range issues {
		logf("importIssues: creating task %d: %s", issue.Index, issue.Title)
		out, err := bdCreateTask(issue.Title, issue.Description)
		if err != nil {
			logf("importIssues: bd create failed for %q: %v", issue.Title, err)
			continue
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out, &created); err == nil && created.ID != "" {
			indexToID[issue.Index] = created.ID
			logf("importIssues: created task %d -> beads id=%s", issue.Index, created.ID)
		} else {
			logf("importIssues: bd create returned unparseable output for %q: %s", issue.Title, string(out))
		}
	}

	// Pass 2: wire up dependencies.
	for _, issue := range issues {
		if issue.Dependency < 0 {
			continue
		}
		childID, hasChild := indexToID[issue.Index]
		parentID, hasParent := indexToID[issue.Dependency]
		if !hasChild || !hasParent {
			logf("importIssues: skipping dependency %d->%d (child=%v parent=%v)", issue.Index, issue.Dependency, hasChild, hasParent)
			continue
		}
		logf("importIssues: linking %s (task %d) depends on %s (task %d)", childID, issue.Index, parentID, issue.Dependency)
		if err := bdAddDep(childID, parentID); err != nil {
			logf("importIssues: bd dep add failed: %s -> %s: %v", childID, parentID, err)
		}
	}

	// Collect created IDs in stable order.
	var ids []string
	for _, issue := range issues {
		if id, ok := indexToID[issue.Index]; ok {
			ids = append(ids, id)
		}
	}

	if len(ids) > 0 {
		o.beadsCommit("Add issues from measure")
	}
	logf("importIssues: %d of %d issue(s) imported", len(ids), len(issues))

	// Append new issues to the persistent measure list.
	appendMeasureLog(o.cfg.Cobbler.Dir, issues)

	return ids, nil
}

// saveHistory persists measure artifacts (prompt, issues YAML, stream-json log)
// to the configured history directory. Each iteration produces three files named
// YYYY-MM-DD-HH-MM-SS-measure-{prompt,issues,log}.{yaml,log}.
func (o *Orchestrator) saveHistory(ts, prompt string, rawOutput []byte, issuesFile string) {
	dir := o.cfg.Cobbler.HistoryDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logf("saveHistory: mkdir %s: %v", dir, err)
		return
	}

	base := ts + "-measure"
	saved := 0

	if err := os.WriteFile(filepath.Join(dir, base+"-prompt.yaml"), []byte(prompt), 0o644); err != nil {
		logf("saveHistory: write prompt: %v", err)
	} else {
		saved++
	}

	if err := os.WriteFile(filepath.Join(dir, base+"-log.log"), rawOutput, 0o644); err != nil {
		logf("saveHistory: write log: %v", err)
	} else {
		saved++
	}

	if data, err := os.ReadFile(issuesFile); err == nil {
		if err := os.WriteFile(filepath.Join(dir, base+"-issues.yaml"), data, 0o644); err != nil {
			logf("saveHistory: write issues: %v", err)
		} else {
			saved++
		}
	}

	logf("saveHistory: saved %d file(s) to %s/%s-*", saved, dir, base)
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
