// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// generator_stats.go delegates generator statistics to the internal/stats
// sub-package.

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// GeneratorStats prints a status report for the current generation run.
func (o *Orchestrator) GeneratorStats() error {
	currentBranch, _ := defaultGitOps.CurrentBranch(".")
	return st.PrintGeneratorStats(st.GeneratorStatsDeps{
		Log:                    logf,
		ListGenerationBranches: o.listGenerationBranches,
		GenerationBranch:       o.cfg.Generation.Branch,
		CurrentBranch:          currentBranch,
		DetectGitHubRepo: func() (string, error) {
			return ghTrackerWithCfg(o.cfg).DetectGitHubRepo(".")
		},
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return defaultGhTracker.ListAllCobblerIssues(repo, generation)
		},
		HistoryDir: o.historyDir(),
		CobblerDir: o.cfg.Cobbler.Dir,
		ReadBranchFile: func(branch, path string) ([]byte, error) {
			return defaultGitOps.ShowFileContent(branch, path, ".")
		},
	})
}

