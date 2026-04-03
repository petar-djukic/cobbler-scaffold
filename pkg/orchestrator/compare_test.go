// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/compare"
)

// TestCompare_DelegationSmoke verifies that the root-package Compare method
// and re-exported functions correctly delegate to the internal/compare
// sub-package. Exhaustive tests live in internal/compare/.

func TestResolverFromArg_Delegation(t *testing.T) {
	t.Parallel()
	r := testOrch().Comparer.ResolverFromArg("gnu")
	if r == nil {
		t.Fatal("ResolverFromArg returned nil")
	}
}

func TestFormatResults_Delegation(t *testing.T) {
	t.Parallel()
	out := compare.FormatResults(nil)
	if out != "No test results.\n" {
		t.Errorf("compare.FormatResults(nil) = %q, want %q", out, "No test results.\n")
	}
}

func TestFilterByUtility_Delegation(t *testing.T) {
	t.Parallel()
	cases := []CompareTestCase{
		{Utility: "cat", Name: "a"},
		{Utility: "echo", Name: "b"},
	}
	result := compare.FilterByUtility(cases, "cat")
	if len(result) != 1 || result[0].Name != "a" {
		t.Errorf("FilterByUtility returned %v, want [{cat a}]", result)
	}
}

func TestCompareUtility_Delegation(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "test-bin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []CompareTestCase{
		{Utility: "test", Name: "echo test", Args: []string{}},
	}
	results := compare.CompareUtility(bin, bin, cases)
	if len(results) != 1 || !results[0].Passed {
		t.Error("identical binaries should pass")
	}
}
