// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// compare.go delegates cross-generation differential comparison to the
// internal/compare sub-package.
// srd: srd004-differential-comparison R1, R2, R3

package orchestrator

import (
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/compare"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

// Comparer provides cross-generation differential comparison.
type Comparer struct {
	logf func(string, ...any)
	git  gitops.GitOps
}

// NewComparer creates a Comparer with explicit dependencies.
func NewComparer(logf func(string, ...any), git gitops.GitOps) *Comparer {
	return &Comparer{logf: logf, git: git}
}

// Compare runs differential comparison between two binary sources.
// argA and argB are passed to compare.ResolverFromArg (git tag, "gnu",
// or directory). When utility is non-empty, only that utility is compared;
// otherwise all common utilities between the two sources are compared.
func (c *Comparer) Compare(argA, argB, utility string) error {
	deps := compare.Deps{
		Log:            c.logf,
		GitBin:         binGit,
		GoBin:          binGo,
		RemoveWorktree: c.git.WorktreeRemove,
	}
	return compare.Run(argA, argB, utility, deps)
}

// ResolverFromArg delegates to the internal compare package.
func (c *Comparer) ResolverFromArg(arg string) BinaryResolver {
	deps := compare.Deps{
		Log:            c.logf,
		GitBin:         binGit,
		GoBin:          binGo,
		RemoveWorktree: c.git.WorktreeRemove,
	}
	return compare.ResolverFromArg(arg, deps)
}
