// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
)

// ---------------------------------------------------------------------------
// Dependency injection: wire the parent package's logf, binary paths,
// and helper functions into the internal/claude package at init time.
// ---------------------------------------------------------------------------

func init() {
	claude.Log = logf
	claude.BinGit = binGit
	claude.BinClaude = binClaude
	claude.BinPodman = binPodman
}

// ---------------------------------------------------------------------------
// Type aliases for backward compatibility
// ---------------------------------------------------------------------------

// Runner executes Claude in a specific mode (CLI, Podman, or SDK).
type Runner = claude.Runner

// CLIRunner executes Claude by running the claude binary directly.
type CLIRunner = claude.CLIRunner

// PodmanRunner executes Claude inside a podman container.
type PodmanRunner = claude.PodmanRunner

// SDKRunner executes Claude via the Go Agent SDK.
type SDKRunner = claude.SDKRunner

// ClaudeResult holds token usage from a Claude invocation.
type ClaudeResult = claude.ClaudeResult

// LocSnapshot holds a point-in-time LOC count.
type LocSnapshot = claude.LocSnapshot

// InvocationRecord is the JSON blob recorded as a GitHub issue comment.
type InvocationRecord = claude.InvocationRecord

// HistoryStats is the YAML-serializable stats file.
type HistoryStats = claude.HistoryStats

// StitchReport is the YAML-serializable stitch report.
type StitchReport = claude.StitchReport

// claudeTokens holds token counts for an invocation record.
type claudeTokens = claude.ClaudeTokens

// diffRecord holds file-level diff statistics.
type diffRecord = claude.DiffRecord

// historyTokens holds token counts in the history stats YAML.
type historyTokens = claude.HistoryTokens

// historyDiff holds diff statistics in the history stats YAML.
type historyDiff = claude.HistoryDiff

// FileChange holds per-file diff information.
type FileChange = claude.FileChange

// ---------------------------------------------------------------------------
// Orchestrator wrapper methods
// ---------------------------------------------------------------------------

// historyDir returns the resolved history directory path.
func (o *Orchestrator) historyDir() string {
	return claude.HistoryDir(o.cfg.Cobbler.Dir, o.cfg.Cobbler.HistoryDir)
}

// saveHistoryReport writes a stitch report YAML file to the history directory.
func (o *Orchestrator) saveHistoryReport(ts string, report StitchReport) {
	claude.SaveHistoryReport(o.historyDir(), ts, report)
}

// saveHistoryStats writes a stats YAML file to the history directory.
func (o *Orchestrator) saveHistoryStats(ts, phase string, stats HistoryStats) {
	claude.SaveHistoryStats(o.historyDir(), ts, phase, stats)
}

// saveHistoryPrompt writes the prompt to the history directory.
func (o *Orchestrator) saveHistoryPrompt(ts, phase, prompt string) {
	claude.SaveHistoryPrompt(o.historyDir(), ts, phase, prompt)
}

// saveHistoryLog writes the raw Claude output to the history directory.
func (o *Orchestrator) saveHistoryLog(ts, phase string, rawOutput []byte) {
	claude.SaveHistoryLog(o.historyDir(), ts, phase, rawOutput)
}

// captureLOC returns the current Go LOC counts.
func (o *Orchestrator) captureLOC() LocSnapshot {
	rec, err := o.CollectStats()
	if err != nil {
		logf("captureLOC: collectStats error: %v", err)
		return LocSnapshot{}
	}
	return LocSnapshot{Production: rec.GoProdLOC, Test: rec.GoTestLOC}
}

// captureLOCAt returns Go LOC counts measured in dir.
func (o *Orchestrator) captureLOCAt(dir string) LocSnapshot {
	return claude.CaptureLOCAt(dir, o.captureLOC)
}

// checkClaude verifies that Claude can be invoked.
func (o *Orchestrator) checkClaude() error {
	return claude.CheckClaude(claude.CheckClaudeDeps{
		EffectiveMode:      o.cfg.Cobbler.effectiveMode(),
		EnsureCredentialsFn: o.ensureCredentials,
		CheckPodmanFn:      o.checkPodman,
	})
}

// ensureCredentials checks that the credential file exists in SecretsDir.
func (o *Orchestrator) ensureCredentials() error {
	return claude.EnsureCredentials(
		o.cfg.Claude.SecretsDir,
		o.cfg.EffectiveTokenFile(),
		o.ExtractCredentials,
	)
}

// checkPodman verifies that podman is available and the image exists.
func (o *Orchestrator) checkPodman() error {
	return claude.CheckPodman(o.ensureImage)
}

// logConfig prints the resolved configuration for debugging.
func (o *Orchestrator) logConfig(target string) {
	claude.LogConfig(target, claude.LogConfigDeps{
		Silence:                 o.cfg.Silence(),
		MaxStitchIssues:         o.cfg.Cobbler.MaxStitchIssues,
		MaxStitchIssuesPerCycle: o.cfg.Cobbler.MaxStitchIssuesPerCycle,
		MaxMeasureIssues:        o.cfg.Cobbler.MaxMeasureIssues,
		GenerationBranch:        o.cfg.Generation.Branch,
		UserPrompt:              o.cfg.Cobbler.UserPrompt,
	})
}

// runner returns a Runner for the configured execution mode.
func (o *Orchestrator) runner() Runner {
	return claude.NewRunner(o.runClaudeDeps())
}

// runClaude executes Claude and returns token usage.
func (o *Orchestrator) runClaude(prompt, dir string, silence bool, extraClaudeArgs ...string) (ClaudeResult, error) {
	return claude.RunClaude(o.runClaudeDeps(), prompt, dir, silence, extraClaudeArgs...)
}

// runClaudeDeps builds the dependency struct for RunClaude.
func (o *Orchestrator) runClaudeDeps() claude.RunClaudeDeps {
	return claude.RunClaudeDeps{
		EffectiveMode:        o.cfg.Cobbler.effectiveMode(),
		ClaudeTimeout:        o.cfg.ClaudeTimeout(),
		Temperature:          o.cfg.Claude.Temperature,
		Silence:              o.cfg.Silence(),
		ClaudeArgs:           o.cfg.Claude.Args,
		PodmanArgs:           o.cfg.Podman.Args,
		PodmanImage:          o.cfg.Podman.Image,
		SecretsDir:           o.cfg.Claude.SecretsDir,
		TokenFile:            o.cfg.EffectiveTokenFile(),
		ContainerCreds:       o.cfg.Claude.ContainerCredentialsPath,
		IdleTimeoutS:         o.cfg.Cobbler.IdleTimeoutSeconds,
		SdkQueryFn:           o.sdkQueryFn,
		ExtractCredentialsFn: o.ExtractCredentials,
		BuildPodmanCmdFn:     o.buildPodmanCmd,
	}
}

// runClaudeSDK executes Claude via the Go Agent SDK.
func (o *Orchestrator) runClaudeSDK(ctx context.Context, prompt, workDir string, silence bool, extraClaudeArgs ...string) (ClaudeResult, error) {
	return claude.RunClaudeSDK(o.runClaudeDeps(), ctx, prompt, workDir, silence, extraClaudeArgs...)
}

// buildPodmanCmd constructs the exec.Cmd for running Claude inside a
// podman container. It mounts the working directory and the credential
// file so Claude Code can authenticate.
func (o *Orchestrator) buildPodmanCmd(ctx context.Context, workDir string, extraClaudeArgs ...string) *exec.Cmd {
	return buildPodmanCmdWithCfg(ctx, workDir, o.cfg, extraClaudeArgs...)
}

// buildDirectCmd constructs the exec.Cmd for running claude directly.
func (o *Orchestrator) buildDirectCmd(ctx context.Context, workDir string, extraClaudeArgs ...string) *exec.Cmd {
	return claude.BuildDirectCmd(ctx, workDir, o.cfg.Claude.Args, extraClaudeArgs...)
}

// hasOpenIssues returns true if there are open orchestrator issues.
func (o *Orchestrator) hasOpenIssues() (bool, error) {
	return claude.HasOpenIssues(claude.HasOpenIssuesDeps{
		DetectGitHubRepoFn: func(repoRoot string) (string, error) {
			return detectGitHubRepo(repoRoot, o.cfg)
		},
		GitCurrentBranchFn: gitCurrentBranch,
		ListOpenCobblerIssuesFn: func(repo, branch string) (int, error) {
			issues, err := listOpenCobblerIssues(repo, branch)
			return len(issues), err
		},
	})
}

// HistoryClean removes the history subdirectory.
func (o *Orchestrator) HistoryClean() error {
	return claude.HistoryClean(o.historyDir())
}

// CobblerReset removes the cobbler scratch directory.
func (o *Orchestrator) CobblerReset() error {
	return claude.CobblerReset(o.cfg.Cobbler.Dir)
}

// ---------------------------------------------------------------------------
// Functions delegated to internal/claude (package-level)
// ---------------------------------------------------------------------------

// parseClaudeTokens extracts token usage from Claude's stream-json output.
func parseClaudeTokens(output []byte) ClaudeResult {
	return claude.ParseClaudeTokens(output)
}

// extractTextFromStreamJSON concatenates all text blocks from assistant messages.
func extractTextFromStreamJSON(rawOutput []byte) string {
	return claude.ExtractTextFromStreamJSON(rawOutput)
}

// extractYAMLBlock finds the first ```yaml fenced code block in text.
func extractYAMLBlock(text string) ([]byte, error) {
	return claude.ExtractYAMLBlock(text)
}

// filterSDKStderr reads lines from r and replaces rate-limit warnings.
var filterSDKStderr = claude.FilterSDKStderr

// toolSummary extracts a concise context string from tool input JSON.
func toolSummary(input json.RawMessage) string {
	return claude.ToolSummary(input)
}

// intFromUsage extracts an integer from the ResultMessage usage map.
func intFromUsage(usage map[string]interface{}, key string) int {
	return claude.IntFromUsage(usage, key)
}

// formatOutcomeTrailers returns the set of git trailer strings for rec.
func formatOutcomeTrailers(rec InvocationRecord) []string {
	return claude.FormatOutcomeTrailers(rec)
}

// appendOutcomeTrailers amends the last commit with outcome trailers.
func appendOutcomeTrailers(worktreeDir string, rec InvocationRecord) error {
	return claude.AppendOutcomeTrailers(worktreeDir, rec)
}

// worktreeBasePath returns the directory used for stitch worktrees.
func worktreeBasePath() string {
	return claude.WorktreeBasePath()
}

// progressWriter wraps a bytes.Buffer, logging concise event summaries.
type progressWriter = claude.ProgressWriter

// newProgressWriter creates a progressWriter.
var newProgressWriter = claude.NewProgressWriter

// idleTrackingWriter wraps an io.Writer and records the last write timestamp.
type idleTrackingWriter = claude.IdleTrackingWriter

// buildPodmanCmdWithCfg constructs the exec.Cmd for running Claude inside a
// podman container. This stays in the parent package because it needs Config.
func buildPodmanCmdWithCfg(ctx context.Context, workDir string, cfg Config, extraClaudeArgs ...string) *exec.Cmd {
	args := []string{"run", "--rm", "-i",
		"-v", workDir + ":" + workDir,
		"-w", workDir,
	}

	// Mount credentials into the container at the path Claude Code expects.
	credPath := filepath.Join(cfg.Claude.SecretsDir, cfg.EffectiveTokenFile())
	if absCredPath, err := filepath.Abs(credPath); err == nil {
		if _, err := os.Stat(absCredPath); err == nil {
			args = append(args,
				"-v", absCredPath+":"+cfg.Claude.ContainerCredentialsPath+":ro")
		}
	}

	args = append(args, cfg.Podman.Args...)
	args = append(args, cfg.Podman.Image)
	args = append(args, binClaude)
	args = append(args, cfg.Claude.Args...)
	args = append(args, extraClaudeArgs...)

	logf("runClaude: exec %s %v (timeout=%s)", binPodman, args, cfg.ClaudeTimeout())
	return exec.CommandContext(ctx, binPodman, args...)
}
