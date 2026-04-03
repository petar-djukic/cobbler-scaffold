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
// tracker or git operations. It replaces the former package-level
// defaultGhTracker global that was removed as part of GH-1709.
func testOrch() *Orchestrator {
	git := &gitops.ShellGitOps{}
	return &Orchestrator{
		git: git,
		tracker: &gh.GitHubTracker{
			GhBin:        binGh,
			BranchExists: git.BranchExists,
		},
	}
}

// testOrchWithCfg returns an Orchestrator with the given config and
// properly initialized git and tracker dependencies.
func testOrchWithCfg(cfg Config) *Orchestrator {
	git := &gitops.ShellGitOps{}
	return &Orchestrator{
		cfg: cfg,
		git: git,
		tracker: &gh.GitHubTracker{
			GhBin:        binGh,
			BranchExists: git.BranchExists,
		},
	}
}
