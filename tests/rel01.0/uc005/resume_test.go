//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc005_test

import (
	"fmt"
	"os"
	"os/exec"
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

func TestRel01_UC005_ResumeFailsWithMultipleBranches(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	for _, name := range []string{"generation-a", "generation-b"} {
		cmd := exec.Command("git", "branch", name)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch %s: %v\n%s", name, err, out)
		}
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Cycles = 0
	})
	if err := testutil.RunMage(t, dir, "generator:resume"); err == nil {
		t.Fatal("expected generator:resume to fail with multiple generation branches")
	}
}

func TestRel01_UC005_ResumeFailsWithZeroBranches(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// No generation branches exist; resolveBranch returns "main" which
	// fails the generation-prefix check in GeneratorResume.
	if err := testutil.RunMage(t, dir, "generator:resume"); err == nil {
		t.Fatal("expected generator:resume to fail with no generation branches")
	}
}

func TestRel01_UC005_ResumeFailsWhenAlreadyOnGenBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	_ = testutil.GeneratorStart(t, dir)

	// Point credentials to an impossible path so checkClaude fails in RunCycles.
	// Config is written to the main repo — the mage subprocess reads it
	// before auto-entering the worktree.
	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	if err := testutil.RunMage(t, dir, "generator:resume"); err == nil {
		t.Fatal("expected generator:resume to fail without Claude credentials")
	}
}

// Resume starts a generation, switches to main, then resumes and verifies
// the branch is resolved and switched to. GeneratorResume always calls
// RunCycles which checks credentials; recovery (branch switch) happens
// before that, so we tolerate the expected credential error.
func TestRel01_UC005_Resume(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)
	genBranch := testutil.GitBranch(t, wtDir)

	// Config is written to the main repo — the mage subprocess reads it
	// before auto-entering the worktree.
	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = genBranch
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	// Resume auto-detects the worktree from the main repo.
	// Fails at RunCycles (credential check) but recovery completes first.
	if err := testutil.RunMage(t, dir, "generator:resume"); err != nil {
		t.Logf("generator:resume (expected credential error): %v", err)
	}

	// The worktree should still be on the generation branch.
	if branch := testutil.GitBranch(t, wtDir); branch != genBranch {
		t.Errorf("expected worktree branch %q after resume, got %q", genBranch, branch)
	}
}

// ResumeRecoversStaleBranches creates a stale task branch, resumes, and
// verifies the stale branch is deleted. Recovery happens before RunCycles,
// so we tolerate the expected credential error.
func TestRel01_UC005_ResumeRecoversStaleBranches(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)
	genBranch := testutil.GitBranch(t, wtDir)
	staleBranch := "task/" + genBranch + "-stale-id"

	// Create a stale task branch (shared git DB, visible from both dirs).
	cmd := exec.Command("git", "branch", staleBranch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch %s: %v\n%s", staleBranch, err, out)
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = genBranch
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	// Resume auto-detects the worktree from the main repo.
	if err := testutil.RunMage(t, dir, "generator:resume"); err != nil {
		t.Logf("generator:resume (expected credential error): %v", err)
	}

	if branches := testutil.GitListBranchesMatching(t, dir, staleBranch); len(branches) > 0 {
		t.Errorf("expected stale branch %q to be deleted after resume, still exists", staleBranch)
	}
}

// ResumeResetsOrphanedIssues creates an in_progress issue with no
// corresponding task branch, resumes, and verifies the issue is reset to ready.
// Recovery happens before RunCycles, so we tolerate the expected credential error.
func TestRel01_UC005_ResumeResetsOrphanedIssues(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)
	genBranch := testutil.GitBranch(t, wtDir)

	// Create a task and set it to in_progress (simulating an interrupted stitch).
	// CreateIssue reads the generation label from the worktree's branch.
	issueNumber := testutil.CreateIssue(t, wtDir, "orphaned task for resume test")
	testutil.SetIssueInProgress(t, wtDir, issueNumber)

	// Verify it is in_progress before resume.
	if !testutil.IssueHasLabel(t, wtDir, issueNumber, "cobbler-in-progress") {
		t.Fatal("expected issue to have cobbler-in-progress label before resume")
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = genBranch
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	// Resume auto-detects the worktree from the main repo.
	if err := testutil.RunMage(t, dir, "generator:resume"); err != nil {
		t.Logf("generator:resume (expected credential error): %v", err)
	}

	// The orphaned in_progress issue should have its in-progress label removed.
	if testutil.IssueHasLabel(t, wtDir, issueNumber, "cobbler-in-progress") {
		t.Errorf("expected cobbler-in-progress label to be removed from issue %s after resume", issueNumber)
	}
}
