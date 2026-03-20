// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"context"
	"os/exec"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
)

// ---------------------------------------------------------------------------
// Orchestrator methods — each wires Config fields into dependency structs
// before delegating to internal/claude.
// ---------------------------------------------------------------------------

// historyDir returns the resolved history directory path.
func (o *Orchestrator) historyDir() string {
	return claude.HistoryDir(o.cfg.Cobbler.Dir, o.cfg.Cobbler.HistoryDir)
}

// saveHistoryReport writes a stitch report YAML file to the history directory.
func (o *Orchestrator) saveHistoryReport(ts string, report claude.StitchReport) {
	claude.SaveHistoryReport(o.historyDir(), ts, report)
}

// saveHistoryStats writes a stats YAML file to the history directory.
func (o *Orchestrator) saveHistoryStats(ts, phase string, stats claude.HistoryStats) {
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
func (o *Orchestrator) captureLOC() claude.LocSnapshot {
	rec, err := o.CollectStats()
	if err != nil {
		logf("captureLOC: collectStats error: %v", err)
		return claude.LocSnapshot{}
	}
	return claude.LocSnapshot{Production: rec.GoProdLOC, Test: rec.GoTestLOC}
}

// captureLOCAt returns Go LOC counts measured in dir.
func (o *Orchestrator) captureLOCAt(dir string) claude.LocSnapshot {
	return claude.CaptureLOCAt(dir, o.captureLOC)
}

// checkClaude verifies that Claude can be invoked.
func (o *Orchestrator) checkClaude() error {
	return claude.CheckClaude(claude.CheckClaudeDeps{
		EffectiveMode:       o.cfg.Cobbler.effectiveMode(),
		EnsureCredentialsFn: o.ensureCredentials,
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
func (o *Orchestrator) runner() claude.Runner {
	return claude.NewRunner(o.runClaudeDeps())
}

// runClaude executes Claude and returns token usage.
func (o *Orchestrator) runClaude(prompt, dir string, silence bool, extraClaudeArgs ...string) (claude.ClaudeResult, error) {
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
		SecretsDir:           o.cfg.Claude.SecretsDir,
		TokenFile:            o.cfg.EffectiveTokenFile(),
		IdleTimeoutS:         o.cfg.Cobbler.IdleTimeoutSeconds,
		SdkQueryFn:           o.sdkQueryFn,
		ExtractCredentialsFn: o.ExtractCredentials,
	}
}

// runMeasureClaude executes Claude with the measure-specific idle timeout,
// which is higher than the default to accommodate large prompts that cause
// extended thinking time (GH-1509).
func (o *Orchestrator) runMeasureClaude(prompt, dir string, silence bool, extraClaudeArgs ...string) (claude.ClaudeResult, error) {
	deps := o.runClaudeDeps()
	deps.IdleTimeoutS = o.cfg.Cobbler.MeasureIdleTimeoutSeconds
	return claude.RunClaude(deps, prompt, dir, silence, extraClaudeArgs...)
}

// runClaudeSDK executes Claude via the Go Agent SDK.
func (o *Orchestrator) runClaudeSDK(ctx context.Context, prompt, workDir string, silence bool, extraClaudeArgs ...string) (claude.ClaudeResult, error) {
	return claude.RunClaudeSDK(o.runClaudeDeps(), ctx, prompt, workDir, silence, extraClaudeArgs...)
}

// buildDirectCmd constructs the exec.Cmd for running claude directly.
func (o *Orchestrator) buildDirectCmd(ctx context.Context, workDir string, extraClaudeArgs ...string) *exec.Cmd {
	return claude.BuildDirectCmd(ctx, workDir, o.cfg.Claude.Args, extraClaudeArgs...)
}

// hasOpenIssues returns true if there are open orchestrator issues.
func (o *Orchestrator) hasOpenIssues() (bool, error) {
	return claude.HasOpenIssues(claude.HasOpenIssuesDeps{
		DetectGitHubRepoFn: func(repoRoot string) (string, error) {
			return ghTrackerWithCfg(o.cfg).DetectGitHubRepo(repoRoot)
		},
		GitReader: defaultGitOps,
		ListOpenCobblerIssuesFn: func(repo, branch string) (int, error) {
			issues, err := defaultGhTracker.ListOpenCobblerIssues(repo, branch)
			return len(issues), err
		},
	})
}

// hasOnlySkippedIssues returns true when there are open cobbler issues but
// every one of them carries the cobbler-skipped label (GH-1699). This lets
// the generator stop cleanly instead of looping on tasks that cannot succeed.
func (o *Orchestrator) hasOnlySkippedIssues() (bool, error) {
	repo, err := ghTrackerWithCfg(o.cfg).DetectGitHubRepo(".")
	if err != nil {
		return false, err
	}
	generation, err := defaultGitOps.CurrentBranch(".")
	if err != nil {
		return false, err
	}
	issues, err := defaultGhTracker.ListOpenCobblerIssues(repo, generation)
	if err != nil {
		return false, err
	}
	if len(issues) == 0 {
		return false, nil // no open issues at all
	}
	for _, iss := range issues {
		if !gh.HasLabel(iss, gh.LabelSkipped) {
			return false, nil // at least one non-skipped issue
		}
	}
	return true, nil
}

// HistoryClean removes the history subdirectory.
func (o *Orchestrator) HistoryClean() error {
	return claude.HistoryClean(o.historyDir())
}

// CobblerReset removes the cobbler scratch directory.
func (o *Orchestrator) CobblerReset() error {
	return claude.CobblerReset(o.cfg.Cobbler.Dir)
}
