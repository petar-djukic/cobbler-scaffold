//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc001_test

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

// --- New / DefaultConfig tests (pure Go, no repo setup) ---

func TestRel01_UC001_NewAppliesDefaults(t *testing.T) {
	t.Parallel()
	cfg := orchestrator.DefaultConfig()
	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"Project.BinaryDir", cfg.Project.BinaryDir, "bin"},
		{"Generation.Prefix", cfg.Generation.Prefix, "generation-"},
		{"Cobbler.Dir", cfg.Cobbler.Dir, ".cobbler/"},
		{"Project.MagefilesDir", cfg.Project.MagefilesDir, "magefiles"},
		{"Claude.SecretsDir", cfg.Claude.SecretsDir, ".secrets"},
		{"Claude.DefaultTokenFile", cfg.Claude.DefaultTokenFile, "claude.json"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
}

func TestRel01_UC001_NewPreservesValues(t *testing.T) {
	t.Parallel()
	cfg := orchestrator.Config{
		Project:    orchestrator.ProjectConfig{ModulePath: "example.com/test", BinaryName: "mybin", BinaryDir: "out"},
		Generation: orchestrator.GenerationConfig{Prefix: "gen-"},
		Cobbler:    orchestrator.CobblerConfig{IssuesRepo: "owner/repo"},
	}
	o := orchestrator.New(cfg)
	got := o.Config()
	if got.Project.BinaryDir != "out" {
		t.Errorf("BinaryDir = %q, want %q", got.Project.BinaryDir, "out")
	}
	if got.Generation.Prefix != "gen-" {
		t.Errorf("Prefix = %q, want %q", got.Generation.Prefix, "gen-")
	}
	if got.Cobbler.IssuesRepo != "owner/repo" {
		t.Errorf("IssuesRepo = %q, want %q", got.Cobbler.IssuesRepo, "owner/repo")
	}
}

func TestRel01_UC001_NewReturnsNonNil(t *testing.T) {
	t.Parallel()
	o := orchestrator.New(orchestrator.Config{
		Project: orchestrator.ProjectConfig{ModulePath: "example.com/test", BinaryName: "test"},
	})
	if o == nil {
		t.Fatal("expected non-nil Orchestrator from New()")
	}
}

// --- Init and reset tests ---

func TestRel01_UC001_InitIdempotent(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	for i := 1; i <= 2; i++ {
		if err := testutil.RunMage(t, dir, "init"); err != nil {
			t.Fatalf("mage init (attempt %d): %v", i, err)
		}
	}
}

func TestRel01_UC001_CobblerReset(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	cobblerDir := filepath.Join(dir, ".cobbler")
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		t.Fatalf("creating .cobbler: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cobblerDir, "dummy.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("writing dummy.json: %v", err)
	}

	if err := testutil.RunMage(t, dir, "cobbler:reset"); err != nil {
		t.Fatalf("mage cobbler:reset: %v", err)
	}

	if testutil.FileExists(dir, ".cobbler") {
		t.Error(".cobbler/ should not exist after cobbler:reset")
	}
}

func TestRel01_UC001_FullResetClearsState(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	cobblerDir := filepath.Join(dir, ".cobbler")
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		t.Fatalf("creating .cobbler: %v", err)
	}

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("mage reset: %v", err)
	}

	if testutil.FileExists(dir, ".cobbler") {
		t.Error(".cobbler/ should not exist after full reset")
	}
	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after reset, got %q", branch)
	}
}

// --- Edge cases ---

func TestRel01_UC001_CobblerResetWhenMissing(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	os.RemoveAll(filepath.Join(dir, ".cobbler"))
	if err := testutil.RunMage(t, dir, "cobbler:reset"); err != nil {
		t.Fatalf("cobbler:reset with missing .cobbler/: %v", err)
	}
}

func TestRel01_UC001_GeneratorResetWhenClean(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	if err := testutil.RunMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("generator:reset from clean state: %v", err)
	}
	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main branch, got %q", branch)
	}
}

func TestRel01_UC001_FullResetFromGenerationBranch(t *testing.T) {
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

	// Create a file and commit on the generation branch so it has work.
	if err := os.WriteFile(filepath.Join(dir, "gen-work.txt"), []byte("generation work"), 0o644); err != nil {
		t.Fatalf("writing gen-work.txt: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "gen-work.txt"},
		{"git", "commit", "-m", "work on generation branch"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	// Reset from the generation branch.
	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("mage reset from generation branch: %v", err)
	}

	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after reset, got %q", branch)
	}
	if branches := testutil.GitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after reset, got %v", branches)
	}
}

func TestRel01_UC001_InitAfterReset(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init after reset: %v", err)
	}
}
