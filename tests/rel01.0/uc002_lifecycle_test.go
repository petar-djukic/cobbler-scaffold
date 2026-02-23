//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// --- Renamed existing tests ---

// TestRel01_UC002_StartCreatesGenBranch verifies that after mage generator:start
// the current branch matches the "generation-" prefix.
func TestRel01_UC002_StartCreatesGenBranch(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	t.Cleanup(func() { runMage(t, dir, "reset") }) //nolint:errcheck

	branch := gitBranch(t, dir)
	if !strings.HasPrefix(branch, "generation-") {
		t.Errorf("expected branch to start with 'generation-', got %q", branch)
	}
}

// TestRel01_UC002_StopMergesAndTags verifies the full start/stop lifecycle:
// after stop the repo is back on main, the generation branch is deleted,
// and the expected git tags (-start, -finished, -merged) exist.
func TestRel01_UC002_StopMergesAndTags(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	genBranch := gitBranch(t, dir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	if err := runMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after stop, got %q", branch)
	}

	// Generation branch should be deleted.
	if branches := gitListBranchesMatching(t, dir, genBranch); len(branches) > 0 {
		t.Errorf("generation branch %q should be deleted after stop, got %v", genBranch, branches)
	}

	// Lifecycle tags should exist.
	for _, suffix := range []string{"-start", "-finished", "-merged"} {
		tag := genBranch + suffix
		if !gitTagExists(t, dir, tag) {
			t.Errorf("expected tag %q to exist after stop", tag)
		}
	}
}

// TestRel01_UC002_ListShowsMerged verifies that mage generator:list shows the
// merged generation after a complete start/stop cycle.
func TestRel01_UC002_ListShowsMerged(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := runMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	out, err := runMageOut(t, dir, "generator:list")
	if err != nil {
		t.Fatalf("generator:list: %v", err)
	}
	if !strings.Contains(out, "merged") {
		t.Errorf("expected 'merged' in generator:list output, got:\n%s", out)
	}
}

// TestRel01_UC002_RunOneCycle verifies that a complete start/run/stop cycle
// with cycles=1 returns to main with the expected tags.
func TestRel01_UC002_RunOneCycle(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

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

// TestRel01_UC002_ResetReturnsToCleanMain verifies that generator:reset returns
// to main and removes generation branches.
func TestRel01_UC002_ResetReturnsToCleanMain(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := runMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("mage generator:reset: %v", err)
	}

	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main branch after generator:reset, got %q", branch)
	}
	if branches := gitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after reset, got %v", branches)
	}
}

// TestRel01_UC002_Stitch100 runs a full generation with 100 stitch iterations.
// This is a stress test — run it explicitly:
//
//	go test -v -count=1 -timeout 0 -run TestRel01_UC002_Stitch100 ./tests/rel01.0/...
func TestRel01_UC002_Stitch100(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 5
		cfg.Cobbler.EstimatedLinesMin = 500
		cfg.Cobbler.EstimatedLinesMax = 1000
		cfg.Cobbler.MaxStitchIssues = 100
		cfg.Cobbler.MaxStitchIssuesPerCycle = 10
		cfg.Claude.MaxTimeSec = 600
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

// --- New tests ---

// TestRel01_UC002_StartFailsWhenNotOnMain verifies that generator:start fails
// when the current branch is not main.
func TestRel01_UC002_StartFailsWhenNotOnMain(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Checkout a non-main branch.
	cmd := exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b feature: %v\n%s", err, out)
	}

	// generator:start should fail.
	if err := runMage(t, dir, "generator:start"); err == nil {
		t.Fatal("expected generator:start to fail on non-main branch")
	}
}

// TestRel01_UC002_SwitchSavesAndChangesBranch verifies that generator:switch
// saves work and switches to the configured branch.
func TestRel01_UC002_SwitchSavesAndChangesBranch(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Switch to main.
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = "main"
	})
	if err := runMage(t, dir, "generator:switch"); err != nil {
		t.Fatalf("generator:switch: %v", err)
	}
	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after switch, got %q", branch)
	}
}
