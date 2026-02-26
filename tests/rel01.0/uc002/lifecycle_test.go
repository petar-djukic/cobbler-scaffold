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

	// Dirty a tracked file so the clean-worktree check rejects the second start.
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

func TestRel01_UC002_StopResetsMainToSpecsOnly(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Create a Go file and a history artifact on the generation branch.
	genDir := filepath.Join(dir, "pkg", "gencode")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatalf("creating pkg/gencode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "gen.go"), []byte("package gencode\n"), 0o644); err != nil {
		t.Fatalf("writing gen.go: %v", err)
	}
	histDir := filepath.Join(dir, ".cobbler", "history")
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
		cmd.Dir = dir
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

	// No history directory.
	if _, err := os.Stat(histDir); err == nil {
		t.Error("history directory should be deleted after stop")
	}

	// V1 tag should exist and contain the generated code.
	out, err := exec.Command("git", "tag", "--list", "v1.*").Output()
	if err != nil {
		t.Fatalf("listing v1 tags: %v", err)
	}
	v1Tags := strings.TrimSpace(string(out))
	if v1Tags == "" {
		t.Fatal("expected at least one v1 tag after stop")
	}
	// Pick the first v1 tag and verify the generated file exists at it.
	v1Tag := strings.Split(v1Tags, "\n")[0]
	showCmd := exec.Command("git", "show", v1Tag+":pkg/gencode/gen.go")
	showCmd.Dir = dir
	if showOut, err := showCmd.Output(); err != nil {
		t.Errorf("generated file should exist at %s tag: %v", v1Tag, err)
	} else if !strings.Contains(string(showOut), "package gencode") {
		t.Errorf("generated file at %s tag has unexpected content: %s", v1Tag, showOut)
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

func TestRel01_UC002_StartRecordsBaseBranch(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	t.Cleanup(func() { testutil.RunMage(t, dir, "reset") }) //nolint:errcheck

	// Verify .cobbler/base-branch exists and contains "main".
	bbFile := filepath.Join(dir, ".cobbler", "base-branch")
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

	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start from feature branch: %v", err)
	}
	t.Cleanup(func() { testutil.RunMage(t, dir, "reset") }) //nolint:errcheck

	// Should be on a generation branch now.
	branch := testutil.GitBranch(t, dir)
	if !strings.HasPrefix(branch, "generation-") {
		t.Errorf("expected generation branch after start, got %q", branch)
	}

	// Base branch should record "feature-test".
	bbFile := filepath.Join(dir, ".cobbler", "base-branch")
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

	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start from feature branch: %v", err)
	}

	genBranch := testutil.GitBranch(t, dir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	if err := testutil.RunMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	// Should return to feature-test, not main.
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
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Remove the base-branch file to simulate an older generation.
	bbFile := filepath.Join(dir, ".cobbler", "base-branch")
	if err := os.Remove(bbFile); err != nil {
		t.Fatalf("removing base-branch file: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "--no-verify", "-m", "remove base-branch file"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
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

