// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"context"
	"os/exec"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

// ClaudeRunner encapsulates Claude execution infrastructure: running Claude,
// capturing LOC, saving history, and checking credentials. Both Measure and
// Stitch depend on this struct.
type ClaudeRunner struct {
	cfg                Config
	git                gitops.GitOps
	tracker            gh.WorkTracker
	sdkQueryFn         claude.SdkQueryFunc
	logf               func(string, ...any)
	extractCredentials func() error
	collectStats       func() (StatsRecord, error)
}

// NewClaudeRunner creates a ClaudeRunner with explicit dependencies.
func NewClaudeRunner(
	cfg Config,
	git gitops.GitOps,
	tracker gh.WorkTracker,
	sdkQueryFn claude.SdkQueryFunc,
	logf func(string, ...any),
	extractCredentials func() error,
	collectStats func() (StatsRecord, error),
) *ClaudeRunner {
	return &ClaudeRunner{
		cfg:                cfg,
		git:                git,
		tracker:            tracker,
		sdkQueryFn:         sdkQueryFn,
		logf:               logf,
		extractCredentials: extractCredentials,
		collectStats:       collectStats,
	}
}

// historyDir returns the resolved history directory path.
func (cr *ClaudeRunner) historyDir() string {
	return claude.HistoryDir(cr.cfg.Cobbler.Dir, cr.cfg.Cobbler.HistoryDir)
}

// saveHistoryReport writes a stitch report YAML file to the history directory.
func (cr *ClaudeRunner) saveHistoryReport(ts string, report claude.StitchReport) {
	claude.SaveHistoryReport(cr.historyDir(), ts, report)
}

// saveHistoryStats writes a stats YAML file to the history directory.
func (cr *ClaudeRunner) saveHistoryStats(ts, phase string, stats claude.HistoryStats) {
	claude.SaveHistoryStats(cr.historyDir(), ts, phase, stats)
}

// saveHistoryPrompt writes the prompt to the history directory.
func (cr *ClaudeRunner) saveHistoryPrompt(ts, phase, prompt string) {
	claude.SaveHistoryPrompt(cr.historyDir(), ts, phase, prompt)
}

// saveHistoryLog writes the raw Claude output to the history directory.
func (cr *ClaudeRunner) saveHistoryLog(ts, phase string, rawOutput []byte) {
	claude.SaveHistoryLog(cr.historyDir(), ts, phase, rawOutput)
}

// captureLOC returns the current Go LOC counts.
func (cr *ClaudeRunner) captureLOC() claude.LocSnapshot {
	rec, err := cr.collectStats()
	if err != nil {
		cr.logf("captureLOC: collectStats error: %v", err)
		return claude.LocSnapshot{}
	}
	return claude.LocSnapshot{Production: rec.GoProdLOC, Test: rec.GoTestLOC}
}

// captureLOCAt returns Go LOC counts measured in dir.
func (cr *ClaudeRunner) captureLOCAt(dir string) claude.LocSnapshot {
	return claude.CaptureLOCAt(dir, cr.captureLOC)
}

// checkClaude verifies that Claude can be invoked.
func (cr *ClaudeRunner) checkClaude() error {
	return claude.CheckClaude(claude.CheckClaudeDeps{
		EffectiveMode:       cr.cfg.Cobbler.effectiveMode(),
		EnsureCredentialsFn: cr.ensureCredentials,
	})
}

// ensureCredentials checks that the credential file exists in SecretsDir.
func (cr *ClaudeRunner) ensureCredentials() error {
	return claude.EnsureCredentials(
		cr.cfg.Claude.SecretsDir,
		cr.cfg.EffectiveTokenFile(),
		cr.extractCredentials,
	)
}

// logConfig prints the resolved configuration for debugging.
func (cr *ClaudeRunner) logConfig(target string) {
	claude.LogConfig(target, claude.LogConfigDeps{
		Silence:                 cr.cfg.Silence(),
		MaxStitchIssues:         cr.cfg.Cobbler.MaxStitchIssues,
		MaxStitchIssuesPerCycle: cr.cfg.Cobbler.MaxStitchIssuesPerCycle,
		MaxMeasureIssues:        cr.cfg.Cobbler.MaxMeasureIssues,
		GenerationBranch:        cr.cfg.Generation.Branch,
		UserPrompt:              cr.cfg.Cobbler.UserPrompt,
	})
}

// runner returns a Runner for the configured execution mode.
func (cr *ClaudeRunner) runner() claude.Runner {
	return claude.NewRunner(cr.runClaudeDeps())
}

// runClaude executes Claude and returns token usage.
func (cr *ClaudeRunner) runClaude(prompt, dir string, silence bool, extraClaudeArgs ...string) (claude.ClaudeResult, error) {
	return claude.RunClaude(cr.runClaudeDeps(), prompt, dir, silence, extraClaudeArgs...)
}

// runClaudeDeps builds the dependency struct for RunClaude.
func (cr *ClaudeRunner) runClaudeDeps() claude.RunClaudeDeps {
	return claude.RunClaudeDeps{
		EffectiveMode:        cr.cfg.Cobbler.effectiveMode(),
		ClaudeTimeout:        cr.cfg.ClaudeTimeout(),
		Temperature:          cr.cfg.Claude.Temperature,
		Silence:              cr.cfg.Silence(),
		Model:                cr.cfg.Claude.Model,
		ClaudeArgs:           cr.cfg.Claude.Args,
		SecretsDir:           cr.cfg.Claude.SecretsDir,
		TokenFile:            cr.cfg.EffectiveTokenFile(),
		IdleTimeoutS:         cr.cfg.Cobbler.IdleTimeoutSeconds,
		SdkQueryFn:           cr.sdkQueryFn,
		ExtractCredentialsFn: cr.extractCredentials,
	}
}

// runMeasureClaude executes Claude with the measure-specific idle timeout.
func (cr *ClaudeRunner) runMeasureClaude(prompt, dir string, silence bool, extraClaudeArgs ...string) (claude.ClaudeResult, error) {
	deps := cr.runClaudeDeps()
	deps.IdleTimeoutS = cr.cfg.Cobbler.MeasureIdleTimeoutSeconds
	return claude.RunClaude(deps, prompt, dir, silence, extraClaudeArgs...)
}

// runClaudeSDK executes Claude via the Go Agent SDK.
func (cr *ClaudeRunner) runClaudeSDK(ctx context.Context, prompt, workDir string, silence bool, extraClaudeArgs ...string) (claude.ClaudeResult, error) {
	return claude.RunClaudeSDK(cr.runClaudeDeps(), ctx, prompt, workDir, silence, extraClaudeArgs...)
}

// buildDirectCmd constructs the exec.Cmd for running claude directly.
func (cr *ClaudeRunner) buildDirectCmd(ctx context.Context, workDir string, extraClaudeArgs ...string) *exec.Cmd {
	return claude.BuildDirectCmd(ctx, workDir, cr.cfg.Claude.Model, cr.cfg.Claude.Args, extraClaudeArgs...)
}

// hasOpenIssues returns true if there are open orchestrator issues.
func (cr *ClaudeRunner) hasOpenIssues() (bool, error) {
	return claude.HasOpenIssues(claude.HasOpenIssuesDeps{
		DetectGitHubRepoFn: func(repoRoot string) (string, error) {
			return cr.tracker.DetectGitHubRepo(repoRoot)
		},
		GitReader: cr.git,
		ListOpenCobblerIssuesFn: func(repo, branch string) (int, error) {
			issues, err := cr.tracker.ListOpenCobblerIssues(repo, branch)
			return len(issues), err
		},
	})
}

// hasOnlySkippedIssues returns true when there are open cobbler issues but
// every one of them carries the cobbler-skipped label (GH-1699).
func (cr *ClaudeRunner) hasOnlySkippedIssues() (bool, error) {
	repo, err := cr.tracker.DetectGitHubRepo(".")
	if err != nil {
		return false, err
	}
	generation, err := cr.git.CurrentBranch(".")
	if err != nil {
		return false, err
	}
	issues, err := cr.tracker.ListOpenCobblerIssues(repo, generation)
	if err != nil {
		return false, err
	}
	if len(issues) == 0 {
		return false, nil
	}
	for _, iss := range issues {
		if !gh.HasLabel(iss, gh.LabelSkipped) {
			return false, nil
		}
	}
	return true, nil
}

// HistoryClean removes the history subdirectory.
func (cr *ClaudeRunner) HistoryClean() error {
	return claude.HistoryClean(cr.historyDir())
}

// CobblerReset removes the cobbler scratch directory.
func (cr *ClaudeRunner) CobblerReset() error {
	return claude.CobblerReset(cr.cfg.Cobbler.Dir)
}
