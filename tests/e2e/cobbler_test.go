// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"testing"

	"github.com/mesh-intelligence/mage-claude-orchestrator/pkg/orchestrator"
)

// TestCobbler_MeasureCreatesIssues verifies that mage cobbler:measure
// produces at least one ready issue in beads.
func TestCobbler_MeasureCreatesIssues(t *testing.T) {
	requiresClaude(t)
	dir := setupRepo(t)

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

// TestCobbler_BeadsResetClearsAfterMeasure verifies that beads:reset clears
// issues created by measure.
func TestCobbler_BeadsResetClearsAfterMeasure(t *testing.T) {
	requiresClaude(t)
	dir := setupRepo(t)

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

// TestGenerator_RunOneCycle verifies that a complete start/run/stop cycle
// with cycles=1 returns to main with the expected tags.
func TestGenerator_RunOneCycle(t *testing.T) {
	requiresClaude(t)
	dir := setupRepo(t)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Generation.Cycles = 1
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	genBranch := gitBranch(t, dir)

	if err := runMage(t, dir, "generator:run"); err != nil {
		t.Fatalf("generator:run: %v", err)
	}
	if err := runMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after stop, got %q", branch)
	}
	for _, suffix := range []string{"-start", "-finished", "-merged"} {
		tag := genBranch + suffix
		if !gitTagExists(t, dir, tag) {
			t.Errorf("expected tag %q to exist after stop", tag)
		}
	}
}

// TestGenerator_Resume verifies that generator:resume recovers from an
// interrupted run and completes cleanly.
func TestGenerator_Resume(t *testing.T) {
	requiresClaude(t)
	dir := setupRepo(t)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Generation.Cycles = 1
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Measure one issue, then simulate interruption by switching to main.
	if err := runMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	// Switch back to main to simulate interruption.
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = "main"
	})
	if err := runMage(t, dir, "generator:switch"); err != nil {
		t.Fatalf("generator:switch to main: %v", err)
	}
	if branch := gitBranch(t, dir); branch != "main" {
		t.Fatalf("expected main after switch, got %q", branch)
	}

	// Clear generation branch override so resume auto-detects.
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = ""
	})
	if err := runMage(t, dir, "generator:resume"); err != nil {
		t.Fatalf("generator:resume: %v", err)
	}

	// Resume runs cycles then stops. If still on a generation branch, stop.
	if branch := gitBranch(t, dir); branch != "main" {
		if err := runMage(t, dir, "generator:stop"); err != nil {
			t.Errorf("generator:stop after resume: %v", err)
		}
	}

	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after resume+stop, got %q", branch)
	}
	if branches := gitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after resume+stop, got %v", branches)
	}
}
