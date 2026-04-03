// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// testloader.go re-exports comparison types from the internal/compare
// sub-package for backward compatibility within the orchestrator package.
// prd: prd004-differential-comparison R4, R5, R6

package orchestrator

import (
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/compare"
)

// CompareTestCase is re-exported from the internal compare package.
type CompareTestCase = compare.CompareTestCase

// CompareExpected is re-exported from the internal compare package.
type CompareExpected = compare.CompareExpected

// TestResult is re-exported from the internal compare package.
type TestResult = compare.TestResult

// BinaryResolver is re-exported from the internal compare package.
type BinaryResolver = compare.BinaryResolver

// ResolverFromArg delegates to the internal compare package.
func (o *Orchestrator) ResolverFromArg(arg string) BinaryResolver {
	deps := compare.Deps{
		Log:            o.logf,
		GitBin:         binGit,
		GoBin:          binGo,
		RemoveWorktree: o.git.WorktreeRemove,
	}
	return compare.ResolverFromArg(arg, deps)
}
