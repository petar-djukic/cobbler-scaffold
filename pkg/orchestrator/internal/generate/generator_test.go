// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStopTarget_CallerEqualsGen(t *testing.T) {
	got := ResolveStopTarget("gen-branch", "gen-branch", "main")
	if got != "main" {
		t.Errorf("expected main, got %q", got)
	}
}

func TestResolveStopTarget_CallerEqualsBase(t *testing.T) {
	got := ResolveStopTarget("main", "gen-branch", "main")
	if got != "main" {
		t.Errorf("expected main, got %q", got)
	}
}

func TestResolveStopTarget_CallerIsFeatureBranch(t *testing.T) {
	got := ResolveStopTarget("feature-x", "gen-branch", "main")
	if got != "feature-x" {
		t.Errorf("expected feature-x, got %q", got)
	}
}

func TestGenerationName_StartSuffix(t *testing.T) {
	if got := GenerationName("my-gen-start"); got != "my-gen" {
		t.Errorf("expected my-gen, got %q", got)
	}
}

func TestGenerationName_FinishedSuffix(t *testing.T) {
	if got := GenerationName("gen-finished"); got != "gen" {
		t.Errorf("expected gen, got %q", got)
	}
}

func TestGenerationName_MergedSuffix(t *testing.T) {
	if got := GenerationName("gen-merged"); got != "gen" {
		t.Errorf("expected gen, got %q", got)
	}
}

func TestGenerationName_AbandonedSuffix(t *testing.T) {
	if got := GenerationName("gen-abandoned"); got != "gen" {
		t.Errorf("expected gen, got %q", got)
	}
}

func TestGenerationName_NoSuffix(t *testing.T) {
	if got := GenerationName("plain-tag"); got != "plain-tag" {
		t.Errorf("expected plain-tag, got %q", got)
	}
}

func TestSaveAndSwitchBranch_AlreadyOnTarget(t *testing.T) {
	deps := GitDeps{
		RepoReader:    &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "target", nil }},
		BranchManager: &mockBranchManager{},
		CommitWriter:  &mockCommitWriter{},
	}
	if err := SaveAndSwitchBranch("target", deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveAndSwitchBranch_CommitSucceeds(t *testing.T) {
	var staged, committed, checkedOut bool
	deps := GitDeps{
		RepoReader: &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "current", nil }},
		BranchManager: &mockBranchManager{
			CheckoutFn: func(branch, dir string) error { checkedOut = true; return nil },
		},
		CommitWriter: &mockCommitWriter{
			StageAllFn: func(dir string) error { staged = true; return nil },
			CommitFn:   func(msg, dir string) error { committed = true; return nil },
		},
	}
	if err := SaveAndSwitchBranch("target", deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !staged {
		t.Error("expected StageAll to be called")
	}
	if !committed {
		t.Error("expected Commit to be called")
	}
	if !checkedOut {
		t.Error("expected Checkout to be called")
	}
}

func TestSaveAndSwitchBranch_CommitFailsWithDirtyTree(t *testing.T) {
	var stashed, unstaged bool
	deps := GitDeps{
		RepoReader: &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "current", nil }},
		BranchManager: &mockBranchManager{
			CheckoutFn: func(branch, dir string) error { return nil },
		},
		CommitWriter: &mockCommitWriter{
			StageAllFn:   func(dir string) error { return nil },
			CommitFn:     func(msg, dir string) error { return errors.New("nothing to commit") },
			UnstageAllFn: func(dir string) error { unstaged = true; return nil },
			HasChangesFn: func(dir string) bool { return true },
			StashFn:      func(msg, dir string) error { stashed = true; return nil },
		},
	}
	if err := SaveAndSwitchBranch("target", deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !unstaged {
		t.Error("expected UnstageAll to be called")
	}
	if !stashed {
		t.Error("expected Stash to be called on dirty tree")
	}
}

func TestSaveAndSwitchBranch_CommitFailsCleanTree(t *testing.T) {
	var stashed bool
	deps := GitDeps{
		RepoReader: &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "current", nil }},
		BranchManager: &mockBranchManager{
			CheckoutFn: func(branch, dir string) error { return nil },
		},
		CommitWriter: &mockCommitWriter{
			StageAllFn:   func(dir string) error { return nil },
			CommitFn:     func(msg, dir string) error { return errors.New("nothing to commit") },
			UnstageAllFn: func(dir string) error { return nil },
			HasChangesFn: func(dir string) bool { return false },
			StashFn:      func(msg, dir string) error { stashed = true; return nil },
		},
	}
	if err := SaveAndSwitchBranch("target", deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stashed {
		t.Error("expected Stash NOT to be called on clean tree")
	}
}

func TestEnsureOnBranch_AlreadyOnBranch(t *testing.T) {
	deps := GitDeps{
		RepoReader:    &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "main", nil }},
		BranchManager: &mockBranchManager{},
		CommitWriter:  &mockCommitWriter{},
	}
	if err := EnsureOnBranch("main", deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureOnBranch_SwitchesBranch(t *testing.T) {
	var target string
	deps := GitDeps{
		RepoReader: &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "feature", nil }},
		BranchManager: &mockBranchManager{
			CheckoutFn: func(branch, dir string) error { target = branch; return nil },
		},
		CommitWriter: &mockCommitWriter{},
	}
	if err := EnsureOnBranch("main", deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "main" {
		t.Errorf("expected checkout to main, got %q", target)
	}
}

func TestEnsureOnBranch_ErrorFromCurrentBranch(t *testing.T) {
	deps := GitDeps{
		RepoReader:    &mockRepoReader{CurrentBranchFn: func(dir string) (string, error) { return "", errors.New("detached HEAD") }},
		BranchManager: &mockBranchManager{},
		CommitWriter:  &mockCommitWriter{},
	}
	if err := EnsureOnBranch("main", deps); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveEmptyDirs(t *testing.T) {
	root := t.TempDir()
	// Create nested empty dirs.
	emptyNested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(emptyNested, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a dir with a file.
	dirWithFile := filepath.Join(root, "d")
	if err := os.MkdirAll(dirWithFile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirWithFile, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	RemoveEmptyDirs(root)

	// Empty dirs should be gone.
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Error("expected empty dir 'a' to be removed")
	}
	// Dir with file should remain.
	if _, err := os.Stat(dirWithFile); err != nil {
		t.Error("expected dir 'd' with file to remain")
	}
}

func TestRemoveEmptyDirs_NonexistentRoot(t *testing.T) {
	RemoveEmptyDirs("/nonexistent/path/that/does/not/exist")
	// Should not panic.
}

func TestAppendToGitignore_NewFile(t *testing.T) {
	dir := t.TempDir()
	if err := AppendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "bin/\n" {
		t.Errorf("expected 'bin/\\n', got %q", got)
	}
}

func TestAppendToGitignore_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.o\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if got := string(data); got != "*.o\nbin/\n" {
		t.Errorf("expected '*.o\\nbin/\\n', got %q", got)
	}
}

func TestAppendToGitignore_Duplicate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if got := string(data); got != "bin/\n" {
		t.Errorf("expected no duplicate, got %q", got)
	}
}

func TestAppendToGitignore_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.o"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AppendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if got := string(data); got != "*.o\nbin/\n" {
		t.Errorf("expected newline inserted before entry, got %q", got)
	}
}
