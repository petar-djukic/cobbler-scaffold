// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// issues_gh.go wires the orchestrator's global dependencies into the
// internal/github sub-package via a package-level GitHubTracker singleton.
// Callers use defaultGhTracker directly for operations that do not need
// Config, or ghTrackerWithCfg(cfg) for operations that require RepoConfig.

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
)

// Type aliases so the rest of the package can refer to internal/github
// types without a qualified name.
type cobblerIssue = gh.CobblerIssue
type cobblerFrontMatter = gh.CobblerFrontMatter
type proposedIssue = gh.ProposedIssue

// defaultGhTracker is a package-level GitHubTracker for operations that
// do not require Config (label ops, issue CRUD, queries, GC, etc.).
// It is a var (not a const initializer) because the dependencies (logf,
// binGh, defaultGitOps) are themselves package-level vars that may be
// reassigned in tests.
var defaultGhTracker = &gh.GitHubTracker{
	Log:          logf,
	GhBin:        binGh,
	BranchExists: defaultGitOps.BranchExists,
}

// ghTrackerWithCfg constructs a GitHubTracker that includes RepoConfig,
// needed by DetectGitHubRepo and ResolveTargetRepo.
func ghTrackerWithCfg(cfg Config) *gh.GitHubTracker {
	return &gh.GitHubTracker{
		Log:          logf,
		GhBin:        binGh,
		BranchExists: defaultGitOps.BranchExists,
		Cfg: gh.RepoConfig{
			IssuesRepo: cfg.Cobbler.IssuesRepo,
			ModulePath: cfg.Project.ModulePath,
			TargetRepo: cfg.Project.TargetRepo,
		},
	}
}

// CobblerGenLabel returns the GitHub label name for a generation. Exported
// for use by e2e tests that need label-safe generation names (GH-1644).
func CobblerGenLabel(generation string) string {
	return gh.GenLabel(generation)
}
