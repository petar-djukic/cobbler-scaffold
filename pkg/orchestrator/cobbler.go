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
)

// ClaudeResult holds token usage from a Claude invocation.
type ClaudeResult struct {
	InputTokens  int
	OutputTokens int
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
	Input  int `json:"input"`
	Output int `json:"output"`
}

type diffRecord struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
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

// parseClaudeTokens extracts token usage from Claude's stream-json
// output. The final JSON line has "type":"result" with a "usage" object
// containing "input_tokens" and "output_tokens".
func parseClaudeTokens(output []byte) ClaudeResult {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var msg struct {
			Type  string `json:"type"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(lines[i], &msg); err != nil {
			continue
		}
		if msg.Type == "result" {
			return ClaudeResult{
				InputTokens:  msg.Usage.InputTokens,
				OutputTokens: msg.Usage.OutputTokens,
			}
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

// clearClaudeHistory removes Claude conversation history files from $HOME.
func clearClaudeHistory() {
	home, err := os.UserHomeDir()
	if err != nil {
		logf("clearClaudeHistory: cannot determine home dir: %v", err)
		return
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".claude.json*"))
	for _, f := range matches {
		logf("clearClaudeHistory: removing %s", f)
		os.Remove(f)
	}
}

// runClaude executes Claude inside a podman container and returns token
// usage. The process is killed if ClaudeMaxTimeSec is exceeded.
func (o *Orchestrator) runClaude(prompt, dir string, silence bool) (ClaudeResult, error) {
	logf("runClaude: promptLen=%d dir=%q silence=%v", len(prompt), dir, silence)

	clearClaudeHistory()

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
		cmd.Stdout = &stdoutBuf
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

	result := parseClaudeTokens(stdoutBuf.Bytes())
	logf("runClaude: finished in %s tokens(in=%d out=%d) (err=%v)",
		time.Since(start).Round(time.Second), result.InputTokens, result.OutputTokens, err)
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
				"-v", absCredPath+":/home/crumbs/.claude/.credentials.json:ro")
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
	os.RemoveAll(o.cfg.Cobbler.Dir)
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
