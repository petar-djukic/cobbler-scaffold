//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// --- Renamed existing tests ---

// TestRel01_UC003_MeasureCreatesIssues verifies that mage cobbler:measure
// produces at least one ready issue in beads.
func TestRel01_UC003_MeasureCreatesIssues(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	n := countReadyIssues(t, dir)
	if n == 0 {
		t.Error("expected at least 1 ready issue after cobbler:measure, got 0")
	}
	t.Logf("cobbler:measure created %d issue(s)", n)
}

// TestRel01_UC003_BeadsResetClearsAfterMeasure verifies that beads:reset clears
// issues created by measure.
func TestRel01_UC003_BeadsResetClearsAfterMeasure(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}
	if err := runMage(t, dir, "beads:reset"); err != nil {
		t.Fatalf("beads:reset: %v", err)
	}

	if n := countReadyIssues(t, dir); n != 0 {
		t.Errorf("expected 0 ready issues after beads:reset, got %d", n)
	}
}

// TestRel01_UC003_TimingByLimit runs measure with limits 1 through 5 and logs
// the wall-clock time and issue count for each.
//
//	go test -v -count=1 -timeout 0 -run TestRel01_UC003_TimingByLimit ./tests/rel01.0/...
func TestRel01_UC003_TimingByLimit(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	for limit := 1; limit <= 5; limit++ {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			// Reset beads between runs so each starts with zero issues.
			if err := runMage(t, dir, "beads:reset"); err != nil {
				t.Fatalf("beads:reset: %v", err)
			}
			if err := runMage(t, dir, "init"); err != nil {
				t.Fatalf("init: %v", err)
			}

			writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
				cfg.Cobbler.MaxMeasureIssues = limit
			})

			start := time.Now()
			if err := runMage(t, dir, "cobbler:measure"); err != nil {
				t.Fatalf("cobbler:measure (limit=%d): %v", limit, err)
			}
			elapsed := time.Since(start).Round(time.Second)

			n := countReadyIssues(t, dir)
			t.Logf("limit=%d issues=%d time=%s", limit, n, elapsed)
		})
	}
}

// --- New tests ---

// TestRel01_UC003_MeasureFailsWithoutBeads verifies that cobbler:measure fails
// when the .beads/ directory does not exist.
func TestRel01_UC003_MeasureFailsWithoutBeads(t *testing.T) {
	dir := setupRepo(t)
	os.RemoveAll(filepath.Join(dir, ".beads"))
	if err := runMage(t, dir, "cobbler:measure"); err == nil {
		t.Fatal("expected cobbler:measure to fail without .beads")
	}
}

// TestRel01_UC003_MeasureRecordsInvocation verifies that measure records an
// InvocationRecord on created issues. Requires Claude.
func TestRel01_UC003_MeasureRecordsInvocation(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	if !issueHasField(t, dir, "invocation_record") {
		t.Error("expected invocation_record field on at least one issue")
	}
}
