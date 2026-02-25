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
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Point credentials to an impossible path so checkClaude fails in RunCycles.
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
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	genBranch := testutil.GitBranch(t, dir)

	// Switch back to main so resume has to resolve the branch.
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %v\n%s", err, out)
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = genBranch
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	// Resume fails at RunCycles (credential check) but recovery completes first.
	if err := testutil.RunMage(t, dir, "generator:resume"); err != nil {
		t.Logf("generator:resume (expected credential error): %v", err)
	}

	if branch := testutil.GitBranch(t, dir); branch != genBranch {
		t.Errorf("expected current branch %q after resume, got %q", genBranch, branch)
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
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	genBranch := testutil.GitBranch(t, dir)
	staleBranch := "task/" + genBranch + "-stale-id"

	// Create a stale task branch.
	cmd := exec.Command("git", "branch", staleBranch)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch %s: %v\n%s", staleBranch, err, out)
	}

	// Switch back to main so resume switches to the generation branch.
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %v\n%s", err, out)
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = genBranch
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	// Resume fails at RunCycles (credential check) but recovery completes first.
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
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	genBranch := testutil.GitBranch(t, dir)

	// Create a task and set it to in_progress (simulating an interrupted stitch).
	issueID := testutil.CreateIssue(t, dir, "orphaned task for resume test")
	cmd := exec.Command("bd", "update", issueID, "--status", "in_progress")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd update %s --status in_progress: %v\n%s", issueID, err, out)
	}

	// Verify it is in_progress before resume.
	if n := testutil.CountIssuesByStatus(t, dir, "in_progress"); n == 0 {
		t.Fatal("expected at least 1 in_progress issue before resume")
	}

	// Switch back to main so resume switches to the generation branch.
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %v\n%s", err, out)
	}

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Generation.Branch = genBranch
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	// Resume fails at RunCycles (credential check) but recovery completes first.
	if err := testutil.RunMage(t, dir, "generator:resume"); err != nil {
		t.Logf("generator:resume (expected credential error): %v", err)
	}

	// The orphaned in_progress issue should be reset to ready.
	if n := testutil.CountIssuesByStatus(t, dir, "in_progress"); n != 0 {
		t.Errorf("expected 0 in_progress issues after resume, got %d", n)
	}
}
