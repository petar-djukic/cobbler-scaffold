// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ClaudeResult holds token usage from a Claude invocation.
// InputTokens is the total input (non-cached + cache creation + cache read).
// CacheCreationTokens and CacheReadTokens break down how the input was served.
// RawOutput contains the full stream-json output from Claude for history.
type ClaudeResult struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	CostUSD             float64
	RawOutput           []byte
}

// LocSnapshot holds a point-in-time LOC count.
type LocSnapshot struct {
	Production int `json:"production"`
	Test       int `json:"test"`
}

// captureLOC returns the current Go LOC counts. Errors are swallowed
// because stats collection is best-effort.
func (o *Orchestrator) captureLOC() LocSnapshot {
	rec, err := o.CollectStats()
	if err != nil {
		logf("captureLOC: collectStats error: %v", err)
		return LocSnapshot{}
	}
	return LocSnapshot{Production: rec.GoProdLOC, Test: rec.GoTestLOC}
}

// InvocationRecord is the JSON blob recorded as a beads comment after
// every Claude invocation.
type InvocationRecord struct {
	Caller    string       `json:"caller"`
	StartedAt string      `json:"started_at"`
	DurationS int         `json:"duration_s"`
	Tokens    claudeTokens `json:"tokens"`
	LOCBefore LocSnapshot  `json:"loc_before"`
	LOCAfter  LocSnapshot  `json:"loc_after"`
	Diff      diffRecord   `json:"diff"`
}

type claudeTokens struct {
	Input         int     `json:"input"`
	Output        int     `json:"output"`
	CacheCreation int     `json:"cache_creation"`
	CacheRead     int     `json:"cache_read"`
	CostUSD       float64 `json:"cost_usd"`
}

type diffRecord struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

// HistoryStats is the YAML-serializable stats file saved alongside prompt
// and log artifacts in the history directory.
type HistoryStats struct {
	Caller    string       `yaml:"caller"`
	TaskID    string       `yaml:"task_id,omitempty"`
	TaskTitle string       `yaml:"task_title,omitempty"`
	Status    string       `yaml:"status,omitempty"`
	Error     string       `yaml:"error,omitempty"`
	StartedAt string       `yaml:"started_at"`
	Duration  string       `yaml:"duration"`
	DurationS int          `yaml:"duration_s"`
	Tokens    historyTokens `yaml:"tokens"`
	CostUSD   float64      `yaml:"cost_usd"`
	LOCBefore LocSnapshot  `yaml:"loc_before"`
	LOCAfter  LocSnapshot  `yaml:"loc_after"`
	Diff      historyDiff  `yaml:"diff"`
}

type historyTokens struct {
	Input         int `yaml:"input"`
	Output        int `yaml:"output"`
	CacheCreation int `yaml:"cache_creation"`
	CacheRead     int `yaml:"cache_read"`
}

type historyDiff struct {
	Files      int `yaml:"files"`
	Insertions int `yaml:"insertions"`
	Deletions  int `yaml:"deletions"`
}

// saveHistoryStats writes a stats YAML file to the history directory.
// The file is named {ts}-{phase}-stats.yaml.
func (o *Orchestrator) saveHistoryStats(ts, phase string, stats HistoryStats) {
	dir := o.cfg.Cobbler.HistoryDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logf("saveHistoryStats: mkdir %s: %v", dir, err)
		return
	}

	data, err := yaml.Marshal(&stats)
	if err != nil {
		logf("saveHistoryStats: marshal: %v", err)
		return
	}

	path := filepath.Join(dir, ts+"-"+phase+"-stats.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logf("saveHistoryStats: write %s: %v", path, err)
		return
	}
	logf("saveHistoryStats: saved %s", path)
}

// saveHistoryPrompt writes the prompt to the history directory.
// Called BEFORE runClaude so the prompt is on disk even if Claude times out.
func (o *Orchestrator) saveHistoryPrompt(ts, phase, prompt string) {
	dir := o.cfg.Cobbler.HistoryDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logf("saveHistoryPrompt: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, ts+"-"+phase+"-prompt.yaml")
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		logf("saveHistoryPrompt: write: %v", err)
	} else {
		logf("saveHistoryPrompt: saved %s", path)
	}
}

// saveHistoryLog writes the raw Claude output to the history directory.
// Called AFTER runClaude completes.
func (o *Orchestrator) saveHistoryLog(ts, phase string, rawOutput []byte) {
	dir := o.cfg.Cobbler.HistoryDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logf("saveHistoryLog: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, ts+"-"+phase+"-log.log")
	if err := os.WriteFile(path, rawOutput, 0o644); err != nil {
		logf("saveHistoryLog: write: %v", err)
	} else {
		logf("saveHistoryLog: saved %s", path)
	}
}

// recordInvocation serializes an InvocationRecord to JSON and adds it
// as a beads comment on the given issue.
func recordInvocation(issueID string, rec InvocationRecord) {
	data, err := json.Marshal(rec)
	if err != nil {
		logf("recordInvocation: marshal error: %v", err)
		return
	}
	if err := bdCommentAdd(issueID, string(data)); err != nil {
		logf("recordInvocation: bd comment error for %s: %v", issueID, err)
	}
}

// progressWriter wraps a bytes.Buffer, logging concise one-line summaries
// of Claude stream-json events (tool calls, result) via logf(). All bytes
// pass through to the underlying buffer unchanged.
type progressWriter struct {
	buf       *bytes.Buffer
	start     time.Time
	lastEvent time.Time
	partial   []byte
	turn      int
	gotFirst  bool
}

func newProgressWriter(dst *bytes.Buffer, start time.Time) *progressWriter {
	return &progressWriter{buf: dst, start: start, lastEvent: start}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	if !pw.gotFirst {
		pw.gotFirst = true
		logf("claude: [%s] first output", time.Since(pw.start).Round(time.Second))
	}
	n, err := pw.buf.Write(p)
	if err != nil {
		return n, err
	}
	pw.partial = append(pw.partial, p...)
	for {
		idx := bytes.IndexByte(pw.partial, '\n')
		if idx < 0 {
			break
		}
		pw.logLine(pw.partial[:idx])
		pw.partial = pw.partial[idx+1:]
	}
	return n, nil
}

// logLine parses a single JSON line and logs assistant turns, tool calls,
// and the final result event.
func (pw *progressWriter) logLine(line []byte) {
	if len(line) == 0 {
		return
	}
	var msg struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	now := time.Now()
	step := now.Sub(pw.lastEvent).Round(time.Second)
	total := now.Sub(pw.start).Round(time.Second)
	pw.lastEvent = now

	switch msg.Type {
	case "assistant":
		pw.turn++
		// Find first text snippet for log context.
		snippet := ""
		for _, b := range msg.Message.Content {
			if b.Type == "text" && b.Text != "" {
				snippet = b.Text
				if len(snippet) > 120 {
					snippet = snippet[:120] + "..."
				}
				snippet = strings.ReplaceAll(snippet, "\n", " ")
				break
			}
		}
		// Always log the turn header with timing.
		if snippet != "" {
			logf("claude: [%s +%s] turn %d: %s", total, step, pw.turn, snippet)
		} else {
			logf("claude: [%s +%s] turn %d", total, step, pw.turn)
		}
		// Log each tool call.
		for _, b := range msg.Message.Content {
			if b.Type == "tool_use" {
				logf("claude: [%s] turn %d: tool %s %s", total, pw.turn, b.Name, toolSummary(b.Input))
			}
		}
	case "user":
		logf("claude: [%s +%s] tools done, waiting for LLM", total, step)
	case "rate_limit_event":
		logf("claude: [%s] rate_limit", total)
	case "system":
		logf("claude: [%s] ready", total)
	case "result":
		u := msg.Usage
		totalIn := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
		logf("claude: [%s] done: %d turn(s), in=%d (base=%d cache_create=%d cache_read=%d) out=%d cost=$%.4f",
			total, pw.turn, totalIn, u.InputTokens, u.CacheCreationInputTokens,
			u.CacheReadInputTokens, u.OutputTokens, msg.TotalCostUSD)
	}
}

// toolSummary extracts a concise context string from tool input JSON
// (file_path, command, pattern, etc.).
func toolSummary(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return ""
	}
	for _, key := range []string{"file_path", "path", "pattern", "command"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var val string
		if json.Unmarshal(raw, &val) != nil {
			continue
		}
		if key == "command" && len(val) > 80 {
			val = val[:80] + "..."
		}
		return val
	}
	return ""
}

// parseClaudeTokens extracts token usage from Claude's stream-json output.
// It scans backwards for the "result" event and parses the usage object,
// which includes cache_creation_input_tokens and cache_read_input_tokens
// in addition to the base input_tokens and output_tokens.
//
// The total input tokens is: input_tokens + cache_creation + cache_read.
func parseClaudeTokens(output []byte) ClaudeResult {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(lines[i], &raw); err != nil {
			continue
		}
		typeField, ok := raw["type"]
		if !ok {
			continue
		}
		var eventType string
		if json.Unmarshal(typeField, &eventType) != nil || eventType != "result" {
			continue
		}

		var result struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
			Usage        struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(lines[i], &result); err != nil {
			logf("parseClaudeTokens: unmarshal error: %v", err)
			return ClaudeResult{}
		}

		u := result.Usage
		totalInput := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens

		logf("parseClaudeTokens: in=%d (base=%d cache_create=%d cache_read=%d) out=%d cost=$%.4f",
			totalInput, u.InputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens,
			u.OutputTokens, result.TotalCostUSD)

		return ClaudeResult{
			InputTokens:         totalInput,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationInputTokens,
			CacheReadTokens:     u.CacheReadInputTokens,
			CostUSD:             result.TotalCostUSD,
		}
	}
	return ClaudeResult{}
}

// checkClaude verifies that Claude can be invoked: podman is available,
// the container image exists, and credentials are present.
func (o *Orchestrator) checkClaude() error {
	if err := o.checkPodman(); err != nil {
		return err
	}
	return o.ensureCredentials()
}

// ensureCredentials checks that the credential file exists in SecretsDir.
// If missing, it attempts to extract credentials from the macOS Keychain.
// Returns an error if the file still does not exist after the attempt.
func (o *Orchestrator) ensureCredentials() error {
	credPath := filepath.Join(o.cfg.Claude.SecretsDir, o.cfg.EffectiveTokenFile())
	if _, err := os.Stat(credPath); err == nil {
		return nil
	}

	logf("ensureCredentials: %s not found, attempting keychain extraction", credPath)
	if err := o.ExtractCredentials(); err != nil {
		logf("ensureCredentials: keychain extraction failed: %v", err)
	}

	if _, err := os.Stat(credPath); err != nil {
		return fmt.Errorf("claude credentials not found at %s; "+
			"run 'mage credentials' on the host or place a valid credential file at %s",
			credPath, credPath)
	}
	return nil
}

// checkPodman verifies that podman is available and that the configured
// image exists locally. If the image is missing, it builds it from the
// embedded Dockerfile.
func (o *Orchestrator) checkPodman() error {
	if _, err := exec.LookPath(binPodman); err != nil {
		return fmt.Errorf("podman not found on PATH; see README.md")
	}
	return o.ensureImage()
}

// runClaude executes Claude inside a podman container and returns token
// usage. The process is killed if ClaudeMaxTimeSec is exceeded.
func (o *Orchestrator) runClaude(prompt, dir string, silence bool) (ClaudeResult, error) {
	logf("runClaude: promptLen=%d dir=%q silence=%v", len(prompt), dir, silence)

	// Refresh credentials from macOS Keychain before each invocation.
	// OAuth tokens expire periodically; extracting just before launch
	// ensures the container always gets a valid token.
	if err := o.ExtractCredentials(); err != nil {
		logf("runClaude: credential refresh warning: %v", err)
	}

	workDir := dir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return ClaudeResult{}, fmt.Errorf("getting working directory: %w", err)
		}
	}

	timeout := o.cfg.ClaudeTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := o.buildPodmanCmd(ctx, workDir)

	cmd.Stdin = strings.NewReader(prompt)

	var stdoutBuf bytes.Buffer
	if silence {
		cmd.Stdout = newProgressWriter(&stdoutBuf, time.Now())
	} else {
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
		cmd.Stderr = os.Stderr
	}

	start := time.Now()
	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		logf("runClaude: killed after %s (max time %s exceeded)", time.Since(start).Round(time.Second), timeout)
		return ClaudeResult{}, fmt.Errorf("claude max time exceeded (%s)", timeout)
	}

	rawOutput := stdoutBuf.Bytes()
	result := parseClaudeTokens(rawOutput)
	result.RawOutput = make([]byte, len(rawOutput))
	copy(result.RawOutput, rawOutput)
	logf("runClaude: finished in %s in=%d (cache_create=%d cache_read=%d) out=%d cost=$%.4f (err=%v)",
		time.Since(start).Round(time.Second), result.InputTokens,
		result.CacheCreationTokens, result.CacheReadTokens,
		result.OutputTokens, result.CostUSD, err)
	return result, err
}

// buildPodmanCmd constructs the exec.Cmd for running Claude inside a
// podman container. It mounts the working directory and the credential
// file so Claude Code can authenticate.
func (o *Orchestrator) buildPodmanCmd(ctx context.Context, workDir string) *exec.Cmd {
	args := []string{"run", "--rm", "-i",
		"-v", workDir + ":" + workDir,
		"-w", workDir,
	}

	// Mount credentials into the container at the path Claude Code expects.
	credPath := filepath.Join(o.cfg.Claude.SecretsDir, o.cfg.EffectiveTokenFile())
	if absCredPath, err := filepath.Abs(credPath); err == nil {
		if _, err := os.Stat(absCredPath); err == nil {
			args = append(args,
				"-v", absCredPath+":"+o.cfg.Claude.ContainerCredentialsPath+":ro")
		}
	}

	args = append(args, o.cfg.Podman.Args...)
	args = append(args, o.cfg.Podman.Image)
	args = append(args, binClaude)
	args = append(args, o.cfg.Claude.Args...)

	logf("runClaude: exec %s %v (timeout=%s)", binPodman, args, o.cfg.ClaudeTimeout())
	return exec.CommandContext(ctx, binPodman, args...)
}

// logConfig prints the resolved configuration for debugging.
func (o *Orchestrator) logConfig(target string) {
	logf("%s config: silence=%v stitchTotal=%d stitchPerCycle=%d measure=%d generationBranch=%q",
		target, o.cfg.Silence(), o.cfg.Cobbler.MaxStitchIssues, o.cfg.Cobbler.MaxStitchIssuesPerCycle, o.cfg.Cobbler.MaxMeasureIssues, o.cfg.Generation.Branch)
	if o.cfg.Cobbler.UserPrompt != "" {
		logf("%s config: userPrompt=%q", target, o.cfg.Cobbler.UserPrompt)
	}
}

// worktreeBasePath returns the directory used for stitch worktrees.
func worktreeBasePath() string {
	repoRoot, _ := os.Getwd()
	return filepath.Join(os.TempDir(), filepath.Base(repoRoot)+"-worktrees")
}

// hasOpenIssues returns true if there are tasks available for work in beads.
func (o *Orchestrator) hasOpenIssues() bool {
	out, err := bdListReadyTasks()
	if err != nil {
		return false
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(out, &tasks); err != nil {
		return false
	}
	return len(tasks) > 0
}

// CobblerReset removes the cobbler scratch directory.
func (o *Orchestrator) CobblerReset() error {
	logf("cobblerReset: removing %s", o.cfg.Cobbler.Dir)
	if err := os.RemoveAll(o.cfg.Cobbler.Dir); err != nil {
		return fmt.Errorf("removing %s: %w", o.cfg.Cobbler.Dir, err)
	}
	logf("cobblerReset: done")
	return nil
}

// beadsCommit syncs beads state and commits the beads directory.
func (o *Orchestrator) beadsCommit(msg string) {
	logf("beadsCommit: %s", msg)
	if err := bdSync(); err != nil {
		logf("beadsCommit: bdSync warning: %v", err)
	}
	if err := gitStageDir(o.cfg.Cobbler.BeadsDir); err != nil {
		logf("beadsCommit: gitStageDir warning: %v", err)
	}
	if err := gitCommitAllowEmpty(msg); err != nil {
		logf("beadsCommit: gitCommit warning: %v", err)
	}
	logf("beadsCommit: done")
}
