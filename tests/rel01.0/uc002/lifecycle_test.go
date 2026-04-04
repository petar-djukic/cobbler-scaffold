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
	wtDir := testutil.GeneratorStart(t, dir)
	t.Cleanup(func() { testutil.RunMage(t, dir, "generator:reset") }) //nolint:errcheck

	// The worktree should be on the generation branch.
	branch := testutil.GitBranch(t, wtDir)
	if !strings.HasPrefix(branch, "generation-") {
		t.Errorf("expected branch to start with 'generation-', got %q", branch)
	}
	// The main repo should stay on main.
	mainBranch := testutil.GitBranch(t, dir)
	if mainBranch != "main" {
		t.Errorf("expected main repo to stay on 'main', got %q", mainBranch)
	}
}

func TestRel01_UC002_StopMergesAndTags(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)

	genBranch := testutil.GitBranch(t, wtDir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	// generator:stop auto-detects the worktree from the main repo.
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
	_ = testutil.GeneratorStart(t, dir)
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
	_ = testutil.GeneratorStart(t, dir)
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
	wtDir := testutil.GeneratorStart(t, dir)
	t.Cleanup(func() { testutil.RunMage(t, dir, "generator:reset") }) //nolint:errcheck

	genBranch := testutil.GitBranch(t, wtDir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	// Create a file in the worktree so there is work to save.
	sentinel := filepath.Join(wtDir, "switch-test.txt")
	if err := os.WriteFile(sentinel, []byte("dirty"), 0o644); err != nil {
		t.Fatalf("writing sentinel: %v", err)
	}

	// Set generation.branch to main so generator:switch targets the base branch.
	testutil.PatchConfigYAML(t, dir, func(cfg map[string]any) {
		gen := cfg["generation"].(map[string]any)
		gen["branch"] = "main"
	})

	// Switch from generation worktree back to main. This exercises the
	// worktree-aware path added in GH-2043.
	if err := testutil.RunMage(t, dir, "generator:switch"); err != nil {
		t.Fatalf("generator:switch: %v", err)
	}

	// After switch, the main repo should be on main.
	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after switch, got %q", branch)
	}

	// The generation worktree should have been removed.
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("expected worktree %s to be removed after switch to main", wtDir)
	}
}

func TestRel01_UC002_StartFailsWhenDirty(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Dirty a tracked file so the clean-worktree check rejects start.
	gomod := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	if err := os.WriteFile(gomod, append(data, []byte("\n// dirty\n")...), 0o644); err != nil {
		t.Fatalf("dirtying go.mod: %v", err)
	}

	if err := testutil.RunMage(t, dir, "generator:start"); err == nil {
		t.Fatal("expected generator:start to fail with dirty worktree")
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
	env1 := []string{"COBBLER_GEN_NAME=first-gen"}
	if err := testutil.RunMageEnv(t, dir, env1, "generator:start"); err != nil {
		t.Fatalf("first generator:start: %v", err)
	}
	wtDir1 := testutil.ReadWorktreeDir(t, dir)
	firstGen := testutil.GitBranch(t, wtDir1)
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
	env2 := []string{"COBBLER_GEN_NAME=second-gen"}
	if err := testutil.RunMageEnv(t, dir, env2, "generator:start"); err != nil {
		t.Fatalf("second generator:start: %v", err)
	}
	wtDir2 := testutil.ReadWorktreeDir(t, dir)
	secondGen := testutil.GitBranch(t, wtDir2)
	if !strings.HasPrefix(secondGen, "generation-") {
		t.Errorf("expected generation branch after second start, got %q", secondGen)
	}
	if secondGen == firstGen {
		t.Errorf("second generation branch should differ from first, both are %q", firstGen)
	}
}

func TestRel01_UC002_StopResetsMainToSpecsOnly(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	wtDir := testutil.GeneratorStart(t, dir)

	// Create a Go file and a history artifact on the generation branch (worktree).
	genDir := filepath.Join(wtDir, "pkg", "gencode")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatalf("creating pkg/gencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "gen.go"), []byte("package gencode\n"), 0o644); err != nil {
		t.Fatalf("writing gen.go: %v", err)
	}
	histDir := filepath.Join(wtDir, ".cobbler", "history")
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		t.Fatalf("creating .cobbler/history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(histDir, "run.yaml"), []byte("run: 1\n"), 0o644); err != nil {
		t.Fatalf("writing history file: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "--no-verify", "-m", "generation work"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wtDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	// Main should be specs-only: no Go files outside magefiles/.
	var goFiles []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() && (rel == ".git" || rel == "magefiles") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(rel, ".go") {
			goFiles = append(goFiles, rel)
		}
		return nil
	})
	if len(goFiles) > 0 {
		t.Errorf("main should have no Go files after stop, found: %v", goFiles)
	}

	// No history directory on main.
	mainHistDir := filepath.Join(dir, ".cobbler", "history")
	if _, err := os.Stat(mainHistDir); err == nil {
		t.Error("history directory should be deleted after stop")
	}

	// Version tagging is handled by mage tag (Tag() in tag.go), not
	// GeneratorStop. The merged tag preserves the generated code.
	mergedTags, _ := exec.Command("git", "-C", dir, "tag", "--list", "*-merged").Output()
	if strings.TrimSpace(string(mergedTags)) == "" {
		t.Error("expected a -merged lifecycle tag after stop")
	}
}

func TestRel01_UC002_ResetFromGenBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)

	// Create a commit on the generation branch (in worktree) so it has diverged.
	if err := os.WriteFile(filepath.Join(wtDir, "gen-file.txt"), []byte("work"), 0o644); err != nil {
		t.Fatalf("writing gen-file.txt: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "gen-file.txt"},
		{"git", "commit", "-m", "generation work"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wtDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	// Reset from the main repo — auto-detects and removes worktree.
	if err := testutil.RunMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("generator:reset: %v", err)
	}

	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after generator:reset, got %q", branch)
	}
	if branches := testutil.GitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after reset, got %v", branches)
	}
}

func TestRel01_UC002_StartRecordsBaseBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)
	t.Cleanup(func() { testutil.RunMage(t, dir, "generator:reset") }) //nolint:errcheck

	// Verify .cobbler/base-branch exists in the worktree and contains "main".
	bbFile := filepath.Join(wtDir, ".cobbler", "base-branch")
	data, err := os.ReadFile(bbFile)
	if err != nil {
		t.Fatalf("reading .cobbler/base-branch: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if content != "main" {
		t.Errorf("expected base-branch to be 'main', got %q", content)
	}
}

func TestRel01_UC002_StartFromFeatureBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create and switch to a feature branch.
	cmd := exec.Command("git", "checkout", "-b", "feature-test")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating feature branch: %v\n%s", err, out)
	}

	wtDir := testutil.GeneratorStart(t, dir)
	t.Cleanup(func() { testutil.RunMage(t, dir, "generator:reset") }) //nolint:errcheck

	// Should be on a generation branch in the worktree.
	branch := testutil.GitBranch(t, wtDir)
	if !strings.HasPrefix(branch, "generation-") {
		t.Errorf("expected generation branch after start, got %q", branch)
	}

	// Base branch should record "feature-test" (in worktree).
	bbFile := filepath.Join(wtDir, ".cobbler", "base-branch")
	data, err := os.ReadFile(bbFile)
	if err != nil {
		t.Fatalf("reading .cobbler/base-branch: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if content != "feature-test" {
		t.Errorf("expected base-branch to be 'feature-test', got %q", content)
	}
}

func TestRel01_UC002_StopReturnsToFeatureBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Create and switch to a feature branch.
	cmd := exec.Command("git", "checkout", "-b", "feature-test")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating feature branch: %v\n%s", err, out)
	}

	wtDir := testutil.GeneratorStart(t, dir)

	genBranch := testutil.GitBranch(t, wtDir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	// Stop from the main repo — auto-detects worktree.
	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	// Should return to feature-test (the recorded base branch).
	if branch := testutil.GitBranch(t, dir); branch != "feature-test" {
		t.Errorf("expected feature-test after stop, got %q", branch)
	}

	// Generation branch should be deleted.
	if branches := testutil.GitListBranchesMatching(t, dir, genBranch); len(branches) > 0 {
		t.Errorf("generation branch %q should be deleted after stop, got %v", genBranch, branches)
	}

	// Lifecycle tags should exist.
	for _, suffix := range []string{"-start", "-finished", "-merged"} {
		tag := genBranch + suffix
		if !testutil.GitTagExists(t, dir, tag) {
			t.Errorf("expected tag %q to exist after stop", tag)
		}
	}
}

func TestRel01_UC002_StopFallsBackToMain(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	wtDir := testutil.GeneratorStart(t, dir)

	// Remove the base-branch file from the worktree to simulate an older generation.
	bbFile := filepath.Join(wtDir, ".cobbler", "base-branch")
	if err := os.Remove(bbFile); err != nil {
		t.Fatalf("removing base-branch file: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "--no-verify", "-m", "remove base-branch file"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wtDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	// Should fall back to main when .cobbler/base-branch is absent.
	if branch := testutil.GitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after stop (fallback), got %q", branch)
	}
}

