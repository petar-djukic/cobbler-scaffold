// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

// testGitOps returns a fresh git operations instance that uses the current
// working directory. This replaces the former package-level defaultGitOps
// global that was removed as part of GH-1709.
func testGitOps() *gitops.Repository {
	return gitops.NewRepository("")
}

// testOrch returns a minimal Orchestrator suitable for tests that need
// tracker, git, or domain struct operations.
func testOrch() *Orchestrator {
	return testOrchWithCfg(Config{})
}

// testOrchWithCfg returns an Orchestrator with the given config and
// properly initialized git, tracker, and domain struct dependencies.
func testOrchWithCfg(cfg Config) *Orchestrator {
	git := &gitops.ShellGitOps{}
	o := &Orchestrator{
		cfg: cfg,
		git: git,
		tracker: &gh.GitHubTracker{
			GhBin:        binGh,
			BranchExists: git.BranchExists,
		},
	}
	o.Builder = NewBuilder(cfg)
	o.Scaffolder = NewScaffolder(o.git, o.logf)
	o.Comparer = NewComparer(o.logf, o.git)
	o.VsCode = NewVsCode(o.logf)
	o.Stats = NewStats(cfg, o.logf, o.git, o.tracker)
	o.Releaser = NewReleaser(cfg)
	o.Analyzer = NewAnalyzer(cfg, o.logf)
	o.ClaudeRunner = NewClaudeRunner(
		cfg, o.git, o.tracker, nil, o.logf,
		o.Builder.ExtractCredentials,
		o.Stats.CollectStats,
	)
	o.Generator = NewGenerator(o)
	o.Measure = NewMeasure(o)
	o.Stitch = NewStitch(o)
	o.Generator.measure = o.Measure
	o.Generator.stitch = o.Stitch
	return o
}
