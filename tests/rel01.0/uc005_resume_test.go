//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os/exec"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// --- Renamed existing test ---

// TestRel01_UC005_Resume verifies that generator:resume recovers from an
// interrupted run (switch to main immediately after start, no prior work)
// and completes cleanly.
func TestRel01_UC005_Resume(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Generation.Cycles = 1
		cfg.Claude.MaxTimeSec = 600 // generous single-measure timeout
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Simulate interruption immediately after start — no work done yet.
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
		writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
			cfg.Generation.Branch = ""
		})
		commitCmd := exec.Command("git", "commit", "-am", "Clear generation.branch after resume")
		commitCmd.Dir = dir
		if out, err := commitCmd.CombinedOutput(); err != nil {
			t.Fatalf("committing config fix: %v\n%s", err, out)
		}
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

// --- New tests ---

// TestRel01_UC005_ResumeFailsWithMultipleBranches verifies that generator:resume
// fails when multiple generation branches exist and no branch is configured.
func TestRel01_UC005_ResumeFailsWithMultipleBranches(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create two generation branches.
	for _, name := range []string{"generation-a", "generation-b"} {
		cmd := exec.Command("git", "branch", name)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch %s: %v\n%s", name, err, out)
		}
	}

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Cycles = 0
	})
	if err := runMage(t, dir, "generator:resume"); err == nil {
		t.Fatal("expected generator:resume to fail with multiple generation branches")
	}
}

// TestRel01_UC005_ResumeRecoversStaleBranches verifies that generator:resume
// deletes stale task branches left from a previous interrupted stitch.
func TestRel01_UC005_ResumeRecoversStaleBranches(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	genBranch := gitBranch(t, dir)

	// Create a stale task branch.
	cmd := exec.Command("git", "branch", "task/"+genBranch+"-stale-id")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating stale branch: %v\n%s", err, out)
	}

	// Switch to main and resume.
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = "main"
	})
	if err := runMage(t, dir, "generator:switch"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = ""
		cfg.Generation.Cycles = 0
	})
	if err := runMage(t, dir, "generator:resume"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	// Verify stale task branch is deleted.
	if branches := gitListBranchesMatching(t, dir, "task/"); len(branches) > 0 {
		t.Errorf("expected stale task branches to be deleted, got %v", branches)
	}
}

// TestRel01_UC005_ResumeResetsOrphanedIssues verifies that generator:resume
// resets in_progress issues that have no corresponding task branch.
func TestRel01_UC005_ResumeResetsOrphanedIssues(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create an issue and set it to in_progress.
	issueID := createIssue(t, dir, "orphan task")
	updateCmd := exec.Command("bd", "update", issueID, "--status", "in_progress")
	updateCmd.Dir = dir
	if out, err := updateCmd.CombinedOutput(); err != nil {
		t.Fatalf("bd update: %v\n%s", err, out)
	}

	// Start a generation and resume with cycles=0 (just recovery, no work).
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Cycles = 0
	})
	if err := runMage(t, dir, "generator:resume"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	// Verify the orphaned in_progress issue was reset (no longer in_progress).
	showCmd := exec.Command("bd", "show", issueID, "--json")
	showCmd.Dir = dir
	out, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd show: %v\n%s", err, out)
	}
	// The issue should be reset to ready status (not in_progress).
	if containsField(string(out), "in_progress") {
		t.Errorf("expected orphaned issue %s to be reset from in_progress", issueID)
	}
}
