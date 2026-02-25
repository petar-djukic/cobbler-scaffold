//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc002_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

var (
	orchRoot    string
	snapshotDir string
)

func TestMain(m *testing.M) {
	var err error
	orchRoot, err = testutil.FindOrchestratorRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: resolving orchRoot: %v\n", err)
		os.Exit(1)
	}
	snapshot, cleanup, prepErr := testutil.PrepareSnapshot(orchRoot)
	if prepErr != nil {
		fmt.Fprintf(os.Stderr, "e2e: preparing snapshot: %v\n", prepErr)
		os.Exit(1)
	}
	snapshotDir = snapshot
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestRel01_UC002_StartCreatesGenBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	t.Cleanup(func() { testutil.RunMage(t, dir, "reset") }) //nolint:errcheck

	branch := testutil.GitBranch(t, dir)
	if !strings.HasPrefix(branch, "generation-") {
		t.Errorf("expected branch to start with 'generation-', got %q", branch)
	}
}

func TestRel01_UC002_StopMergesAndTags(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	genBranch := testutil.GitBranch(t, dir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after stop, got %q", branch)
	}

	if branches := testutil.GitListBranchesMatching(t, dir, genBranch); len(branches) > 0 {
		t.Errorf("generation branch %q should be deleted after stop, got %v", genBranch, branches)
	}

	for _, suffix := range []string{"-start", "-finished", "-merged"} {
		tag := genBranch + suffix
		if !testutil.GitTagExists(t, dir, tag) {
			t.Errorf("expected tag %q to exist after stop", tag)
		}
	}
}

func TestRel01_UC002_ListShowsMerged(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	out, err := testutil.RunMageOut(t, dir, "generator:list")
	if err != nil {
		t.Fatalf("generator:list: %v", err)
	}
	if !strings.Contains(out, "merged") {
		t.Errorf("expected 'merged' in generator:list output, got:\n%s", out)
	}
}

func TestRel01_UC002_ResetReturnsToCleanMain(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("mage generator:reset: %v", err)
	}

	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main branch after generator:reset, got %q", branch)
	}
	if branches := testutil.GitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after reset, got %v", branches)
	}
}

func TestRel01_UC002_SwitchSavesAndChangesBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = "main"
	})
	if err := testutil.RunMage(t, dir, "generator:switch"); err != nil {
		t.Fatalf("generator:switch: %v", err)
	}
	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after switch, got %q", branch)
	}
}

func TestRel01_UC002_StartFailsWhenNotOnMain(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("first generator:start: %v", err)
	}

	// Dirty a tracked file so ensureOnBranch("main") cannot checkout main.
	gomod := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	if err := os.WriteFile(gomod, append(data, []byte("\n// dirty\n")...), 0o644); err != nil {
		t.Fatalf("dirtying go.mod: %v", err)
	}

	if err := testutil.RunMage(t, dir, "generator:start"); err == nil {
		t.Fatal("expected second generator:start to fail with dirty worktree on generation branch")
	}
}

func TestRel01_UC002_StopFailsWhenOnMain(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := testutil.RunMage(t, dir, "generator:stop"); err == nil {
		t.Fatal("expected generator:stop to fail when on main with no generation branches")
	}
}

func TestRel01_UC002_ListWhenEmpty(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	out, err := testutil.RunMageOut(t, dir, "generator:list")
	if err != nil {
		t.Fatalf("generator:list: %v", err)
	}
	if !strings.Contains(out, "No generations found") {
		t.Errorf("expected 'No generations found' in output, got:\n%s", out)
	}
}

func TestRel01_UC002_StartStopStartAgain(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// First generation cycle.
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("first generator:start: %v", err)
	}
	firstGen := testutil.GitBranch(t, dir)
	if !strings.HasPrefix(firstGen, "generation-") {
		t.Fatalf("expected generation branch, got %q", firstGen)
	}

	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("first generator:stop: %v", err)
	}
	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Fatalf("expected main after first stop, got %q", branch)
	}

	// Second generation cycle.
	// Sleep briefly to ensure different timestamp in generation branch name.
	cmd := exec.Command("sleep", "1")
	cmd.Run()

	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("second generator:start: %v", err)
	}
	secondGen := testutil.GitBranch(t, dir)
	if !strings.HasPrefix(secondGen, "generation-") {
		t.Errorf("expected generation branch after second start, got %q", secondGen)
	}
	if secondGen == firstGen {
		t.Errorf("second generation branch should differ from first, both are %q", firstGen)
	}
}

func TestRel01_UC002_ResetFromGenBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Create a commit on the generation branch so it has diverged from main.
	if err := os.WriteFile(filepath.Join(dir, "gen-file.txt"), []byte("work"), 0o644); err != nil {
		t.Fatalf("writing gen-file.txt: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "gen-file.txt"},
		{"git", "commit", "-m", "generation work"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	if err := testutil.RunMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("generator:reset from gen branch: %v", err)
	}

	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after generator:reset, got %q", branch)
	}
	if branches := testutil.GitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after reset, got %v", branches)
	}
}

