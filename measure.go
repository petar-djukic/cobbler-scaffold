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
	"text/template"
	"time"
)

//go:embed prompts/measure.tmpl
var defaultMeasurePromptTmpl string

// Measure assesses project state and proposes new tasks via Claude.
// Reads all options from Config.
func (o *Orchestrator) Measure() error {
	return o.RunMeasure()
}

// RunMeasure runs the measure workflow using Config settings.
func (o *Orchestrator) RunMeasure() error {
	measureStart := time.Now()
	logf("measure: starting")
	o.logConfig("measure")

	if err := o.requireBeads(); err != nil {
		logf("measure: beads not initialized: %v", err)
		return err
	}

	branch, err := o.resolveBranch(o.cfg.GenerationBranch)
	if err != nil {
		logf("measure: resolveBranch failed: %v", err)
		return err
	}
	logf("measure: resolved branch=%s", branch)

	if err := ensureOnBranch(branch); err != nil {
		logf("measure: ensureOnBranch failed: %v", err)
		return fmt.Errorf("switching to branch: %w", err)
	}

	_ = os.MkdirAll(o.cfg.CobblerDir, 0o755)
	timestamp := time.Now().Format("20060102-150405")
	outputFile := filepath.Join(o.cfg.CobblerDir, fmt.Sprintf("proposed-issues-%s.json", timestamp))

	// Clean up old proposed-issues files.
	matches, _ := filepath.Glob(o.cfg.CobblerDir + "proposed-issues-*.json")
	if len(matches) > 0 {
		logf("measure: cleaning %d old proposed-issues file(s)", len(matches))
	}
	for _, f := range matches {
		os.Remove(f)
	}

	// Get existing issues.
	logf("measure: querying existing issues via bd list")
	existingIssues := getExistingIssues()

	issueCount := countJSONArray(existingIssues)
	logf("measure: found %d existing issue(s), maxIssues=%d", issueCount, o.cfg.MaxIssues)
	logf("measure: outputFile=%s", outputFile)

	// Snapshot LOC before Claude.
	locBefore := o.captureLOC()
	logf("measure: locBefore prod=%d test=%d", locBefore.Production, locBefore.Test)

	// Build and run prompt.
	prompt := o.buildMeasurePrompt(o.cfg.UserPrompt, existingIssues, o.cfg.MaxIssues, outputFile)
	logf("measure: prompt built, length=%d bytes", len(prompt))

	logf("measure: invoking Claude")
	claudeStart := time.Now()
	tokens, err := o.runClaude(prompt, "", o.cfg.Silence())
	if err != nil {
		logf("measure: Claude failed after %s: %v", time.Since(claudeStart).Round(time.Second), err)
		return fmt.Errorf("running Claude: %w", err)
	}
	claudeDuration := time.Since(claudeStart)
	logf("measure: Claude completed in %s", claudeDuration.Round(time.Second))

	// Snapshot LOC after Claude (measure doesn't change code, but record for consistency).
	locAfter := o.captureLOC()

	// Import proposed issues.
	if _, statErr := os.Stat(outputFile); statErr != nil {
		logf("measure: output file not found at %s (Claude may not have written it)", outputFile)
		return nil
	}

	fileInfo, _ := os.Stat(outputFile)
	logf("measure: output file found, size=%d bytes", fileInfo.Size())

	logf("measure: importing issues from %s", outputFile)
	importStart := time.Now()
	createdIDs, err := o.importIssues(outputFile)
	if err != nil {
		logf("measure: import failed after %s: %v", time.Since(importStart).Round(time.Second), err)
		return fmt.Errorf("importing issues: %w", err)
	}
	logf("measure: imported %d issue(s) in %s", len(createdIDs), time.Since(importStart).Round(time.Second))

	// Record invocation metrics on each created issue.
	rec := InvocationRecord{
		Caller:    "measure",
		StartedAt: claudeStart.UTC().Format(time.RFC3339),
		DurationS: int(claudeDuration.Seconds()),
		Tokens:    claudeTokens{Input: tokens.InputTokens, Output: tokens.OutputTokens},
		LOCBefore: locBefore,
		LOCAfter:  locAfter,
	}
	for _, id := range createdIDs {
		recordInvocation(id, rec)
	}

	if len(createdIDs) == 0 {
		logf("measure: no issues imported, keeping %s for inspection", outputFile)
	} else {
		logf("measure: removing %s (import successful)", outputFile)
		os.Remove(outputFile)
	}

	logf("measure: completed in %s", time.Since(measureStart).Round(time.Second))
	return nil
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
	logf("getExistingIssues: got %d bytes", len(out))
	return string(out)
}

func countJSONArray(jsonStr string) int {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return 0
	}
	return len(arr)
}

// MeasurePromptData is the template data for the measure prompt.
type MeasurePromptData struct {
	ExistingIssues string
	Limit          int
	OutputPath     string
	UserInput      string
}

func (o *Orchestrator) buildMeasurePrompt(userInput, existingIssues string, limit int, outputPath string) string {
	tmplStr := o.cfg.MeasurePrompt
	if tmplStr == "" {
		tmplStr = defaultMeasurePromptTmpl
	}
	tmpl := template.Must(template.New("measure").Parse(tmplStr))
	data := MeasurePromptData{
		ExistingIssues: existingIssues,
		Limit:          limit,
		OutputPath:     outputPath,
		UserInput:      userInput,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("measure prompt template: %v", err))
	}
	return buf.String()
}

type proposedIssue struct {
	Index       int    `json:"index"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Dependency  int    `json:"dependency"`
}

func (o *Orchestrator) importIssues(jsonFile string) ([]string, error) {
	logf("importIssues: reading %s", jsonFile)
	data, err := os.ReadFile(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("reading JSON file: %w", err)
	}
	logf("importIssues: read %d bytes", len(data))

	var issues []proposedIssue
	if err := json.Unmarshal(data, &issues); err != nil {
		logf("importIssues: JSON parse error: %v", err)
		return nil, fmt.Errorf("parsing JSON: %w", err)
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

	return ids, nil
}
