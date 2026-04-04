// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// testloader.go re-exports comparison types from the internal/compare
// sub-package for backward compatibility within the orchestrator package.
// srd: srd004-differential-comparison R4, R5, R6

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

// ResolverFromArg is now a method on Comparer (see compare.go).
