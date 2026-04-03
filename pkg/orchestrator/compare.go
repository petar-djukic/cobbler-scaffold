// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// compare.go delegates cross-generation differential comparison to the
// internal/compare sub-package.
// prd: prd004-differential-comparison R1, R2, R3

package orchestrator

import (
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/compare"
)

// Comparer runs differential comparison between binary sources.
type Comparer interface {
	Compare(argA, argB, utility string) error
}

// Compare runs differential comparison between two binary sources.
// argA and argB are passed to compare.ResolverFromArg (git tag, "gnu",
// or directory). When utility is non-empty, only that utility is compared;
// otherwise all common utilities between the two sources are compared.
func (o *Orchestrator) Compare(argA, argB, utility string) error {
	deps := compare.Deps{
		Log:            o.logf,
		GitBin:         binGit,
		GoBin:          binGo,
		RemoveWorktree: o.git.WorktreeRemove,
	}
	return compare.Run(argA, argB, utility, deps)
}
