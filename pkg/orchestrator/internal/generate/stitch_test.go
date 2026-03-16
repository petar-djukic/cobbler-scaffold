// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"
)

// ---------------------------------------------------------------------------
// TaskBranchName / TaskBranchPattern
// ---------------------------------------------------------------------------

func TestTaskBranchName(t *testing.T) {
	tests := []struct {
		base, id, want string
	}{
		{"main", "42", "task/main-42"},
		{"generation-abc", "7", "task/generation-abc-7"},
		{"feature/x", "100", "task/feature/x-100"},
	}
	for _, tc := range tests {
		got := TaskBranchName(tc.base, tc.id)
		if got != tc.want {
			t.Errorf("TaskBranchName(%q, %q) = %q, want %q", tc.base, tc.id, got, tc.want)
		}
	}
}

func TestTaskBranchPattern(t *testing.T) {
	got := TaskBranchPattern("main")
	if got != "task/main-*" {
		t.Errorf("expected task/main-*, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ParseRequiredReading
// ---------------------------------------------------------------------------

func TestParseRequiredReading_StringFormat(t *testing.T) {
	desc := `required_reading:
  - pkg/foo/bar.go (main logic)
  - docs/README.md`
	got := ParseRequiredReading(desc)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0] != "pkg/foo/bar.go (main logic)" {
		t.Errorf("unexpected first entry: %q", got[0])
	}
}

func TestParseRequiredReading_MapFormat(t *testing.T) {
	desc := `required_reading:
  - path: pkg/foo/bar.go
    reason: main logic
  - path: docs/README.md
    reason: docs`
	got := ParseRequiredReading(desc)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0] != "pkg/foo/bar.go" {
		t.Errorf("unexpected first entry: %q", got[0])
	}
}

func TestParseRequiredReading_Empty(t *testing.T) {
	got := ParseRequiredReading("")
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestParseRequiredReading_NoField(t *testing.T) {
	got := ParseRequiredReading("deliverable_type: code")
	if len(got) != 0 {
		t.Errorf("expected empty for missing field, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// ScopeSourceDirs
// ---------------------------------------------------------------------------

func TestScopeSourceDirs_NarrowsToSubDir(t *testing.T) {
	desc := `files:
  - cmd/cat/main.go
  - cmd/cat/version.go`
	got := ScopeSourceDirs([]string{"cmd/"}, desc)
	if len(got) != 1 || got[0] != "cmd/cat" {
		t.Errorf("expected [cmd/cat], got %v", got)
	}
}

func TestScopeSourceDirs_NoScoping(t *testing.T) {
	desc := `files:
  - pkg/foo.go`
	got := ScopeSourceDirs([]string{"pkg/"}, desc)
	// Only one level deep (pkg/foo.go has 2 parts) → no scoping possible.
	if got != nil {
		t.Errorf("expected nil (no scoping), got %v", got)
	}
}

func TestScopeSourceDirs_EmptyDescription(t *testing.T) {
	got := ScopeSourceDirs([]string{"cmd/"}, "")
	if got != nil {
		t.Errorf("expected nil for empty description, got %v", got)
	}
}

func TestScopeSourceDirs_EmptyDirs(t *testing.T) {
	got := ScopeSourceDirs(nil, "files:\n  - cmd/cat/main.go")
	if got != nil {
		t.Errorf("expected nil for empty dirs, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// ValidateIssueDescription
// ---------------------------------------------------------------------------

func TestValidateIssueDescription_Valid(t *testing.T) {
	desc := `deliverable_type: code
required_reading:
  - pkg/foo.go
files:
  - pkg/foo.go
requirements:
  - id: R1
    text: req
acceptance_criteria:
  - id: AC1
    text: ac`
	if err := ValidateIssueDescription(desc); err != nil {
		t.Errorf("expected no error for valid description, got: %v", err)
	}
}

func TestValidateIssueDescription_MissingFields(t *testing.T) {
	err := ValidateIssueDescription("deliverable_type: code")
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestValidateIssueDescription_Empty(t *testing.T) {
	err := ValidateIssueDescription("")
	if err == nil {
		t.Fatal("expected error for empty description")
	}
}

func TestValidateIssueDescription_InvalidYAML(t *testing.T) {
	err := ValidateIssueDescription(":::not yaml")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// RecoverStaleBranches
// ---------------------------------------------------------------------------

func TestRecoverStaleBranches_NoBranches(t *testing.T) {
	gitDeps := StitchGitDeps{
		RepoReader:      &mockRepoReader{ListBranchesFn: func(pattern, dir string) []string { return nil }},
		BranchManager:   &mockBranchManager{},
		WorktreeManager: &mockWorktreeManager{},
	}
	got := RecoverStaleBranches("main", "/tmp/wt", "owner/repo", gitDeps, StitchIssueDeps{})
	if got {
		t.Error("expected false when no stale branches")
	}
}

func TestRecoverStaleBranches_RecoversBranch(t *testing.T) {
	var deletedBranch string
	var removedLabel int
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{ListBranchesFn: func(pattern, dir string) []string { return []string{"task/main-42"} }},
		BranchManager: &mockBranchManager{
			ForceDeleteBranchFn: func(name, dir string) error { deletedBranch = name; return nil },
		},
		WorktreeManager: &mockWorktreeManager{
			WorktreeRemoveFn: func(wtDir, dir string) error { return nil },
		},
	}
	issueDeps := StitchIssueDeps{
		RemoveInProgressLabel: func(repo string, number int) error {
			removedLabel = number
			return nil
		},
	}
	got := RecoverStaleBranches("main", t.TempDir(), "owner/repo", gitDeps, issueDeps)
	if !got {
		t.Error("expected true when branches recovered")
	}
	if deletedBranch != "task/main-42" {
		t.Errorf("expected task/main-42 deleted, got %q", deletedBranch)
	}
	if removedLabel != 42 {
		t.Errorf("expected label removed from issue 42, got %d", removedLabel)
	}
}

// ---------------------------------------------------------------------------
// ResetOrphanedIssues
// ---------------------------------------------------------------------------

func TestResetOrphanedIssues_NoOrphans(t *testing.T) {
	issueDeps := StitchIssueDeps{
		ListOpenCobblerIssues: func(repo, gen string) ([]StitchIssue, error) { return nil, nil },
		LabelInProgress:       "cobbler-in-progress",
	}
	gitDeps := StitchGitDeps{
		RepoReader:      &mockRepoReader{},
		BranchManager:   &mockBranchManager{},
		WorktreeManager: &mockWorktreeManager{},
	}
	got := ResetOrphanedIssues("main", "owner/repo", "gen-1", gitDeps, issueDeps)
	if got {
		t.Error("expected false when no orphans")
	}
}

func TestResetOrphanedIssues_ResetsOrphan(t *testing.T) {
	var resetNum int
	issueDeps := StitchIssueDeps{
		ListOpenCobblerIssues: func(repo, gen string) ([]StitchIssue, error) {
			return []StitchIssue{
				{Number: 10, Labels: []string{"cobbler-in-progress"}},
			}, nil
		},
		HasLabel: func(iss StitchIssue, label string) bool {
			for _, l := range iss.Labels {
				if l == label {
					return true
				}
			}
			return false
		},
		RemoveInProgressLabel: func(repo string, number int) error {
			resetNum = number
			return nil
		},
		LabelInProgress: "cobbler-in-progress",
	}
	gitDeps := StitchGitDeps{
		RepoReader:      &mockRepoReader{BranchExistsFn: func(name, dir string) bool { return false }},
		BranchManager:   &mockBranchManager{},
		WorktreeManager: &mockWorktreeManager{},
	}
	got := ResetOrphanedIssues("main", "owner/repo", "gen-1", gitDeps, issueDeps)
	if !got {
		t.Error("expected true when orphan reset")
	}
	if resetNum != 10 {
		t.Errorf("expected issue 10 reset, got %d", resetNum)
	}
}

// ---------------------------------------------------------------------------
// PickTask
// ---------------------------------------------------------------------------

func TestPickTask_Success(t *testing.T) {
	issueDeps := StitchIssueDeps{
		PickReadyIssue: func(repo, gen string) (StitchIssue, error) {
			return StitchIssue{
				Number:      42,
				Title:       "Test task",
				Description: "deliverable_type: code\nrequired_reading:\n  - f.go\nfiles:\n  - f.go\nrequirements:\n  - id: R1\n    text: r\nacceptance_criteria:\n  - id: AC1\n    text: a",
			}, nil
		},
	}
	task, err := PickTask("main", "/tmp/wt", "owner/repo", "gen-1", issueDeps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != "42" {
		t.Errorf("expected ID 42, got %q", task.ID)
	}
	if task.BranchName != "task/main-42" {
		t.Errorf("expected task/main-42, got %q", task.BranchName)
	}
}

func TestPickTask_NoTasks(t *testing.T) {
	issueDeps := StitchIssueDeps{
		PickReadyIssue: func(repo, gen string) (StitchIssue, error) {
			return StitchIssue{}, errors.New("no ready issues")
		},
	}
	_, err := PickTask("main", "/tmp/wt", "owner/repo", "gen-1", issueDeps)
	if err == nil {
		t.Fatal("expected error when no tasks available")
	}
}

// ---------------------------------------------------------------------------
// CreateWorktree
// ---------------------------------------------------------------------------

func TestCreateWorktree_CreatesBranchAndWorktree(t *testing.T) {
	var createdBranch string
	var worktreeAdded bool
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{BranchExistsFn: func(name, dir string) bool { return false }},
		BranchManager: &mockBranchManager{
			CreateBranchFn: func(name, dir string) error { createdBranch = name; return nil },
		},
		WorktreeManager: &mockWorktreeManager{
			WorktreeAddFn: func(wtDir, branch, dir string) *exec.Cmd {
				worktreeAdded = true
				return exec.Command("true")
			},
		},
	}
	task := StitchTask{
		BranchName:  "task/main-42",
		WorktreeDir: t.TempDir(),
	}
	if err := CreateWorktree(task, gitDeps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdBranch != "task/main-42" {
		t.Errorf("expected branch creation for task/main-42, got %q", createdBranch)
	}
	if !worktreeAdded {
		t.Error("expected worktree add to be called")
	}
}

func TestCreateWorktree_BranchAlreadyExists(t *testing.T) {
	var createdBranch string
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{BranchExistsFn: func(name, dir string) bool { return true }},
		BranchManager: &mockBranchManager{
			CreateBranchFn: func(name, dir string) error { createdBranch = name; return nil },
		},
		WorktreeManager: &mockWorktreeManager{
			WorktreeAddFn: func(wtDir, branch, dir string) *exec.Cmd {
				return exec.Command("true")
			},
		},
	}
	task := StitchTask{
		BranchName:  "task/main-42",
		WorktreeDir: t.TempDir(),
	}
	if err := CreateWorktree(task, gitDeps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdBranch != "" {
		t.Error("expected no branch creation when branch already exists")
	}
}

// ---------------------------------------------------------------------------
// MergeBranch
// ---------------------------------------------------------------------------

func TestMergeBranch_Success(t *testing.T) {
	var checkedOut string
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{},
		BranchManager: &mockBranchManager{
			CheckoutFn: func(branch, dir string) error { checkedOut = branch; return nil },
			MergeCmdFn: func(branch, dir string) *exec.Cmd {
				return exec.Command("true")
			},
		},
		WorktreeManager: &mockWorktreeManager{},
	}
	if err := MergeBranch("task/main-42", "main", ".", gitDeps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checkedOut != "main" {
		t.Errorf("expected checkout to main, got %q", checkedOut)
	}
}

func TestMergeBranch_CheckoutFails(t *testing.T) {
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{},
		BranchManager: &mockBranchManager{
			CheckoutFn: func(branch, dir string) error { return fmt.Errorf("checkout failed") },
		},
		WorktreeManager: &mockWorktreeManager{},
	}
	err := MergeBranch("task/main-42", "main", ".", gitDeps)
	if err == nil {
		t.Fatal("expected error when checkout fails")
	}
}

// ---------------------------------------------------------------------------
// CleanupWorktree
// ---------------------------------------------------------------------------

func TestCleanupWorktree_Success(t *testing.T) {
	var deletedBranch string
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{},
		BranchManager: &mockBranchManager{
			DeleteBranchFn: func(name, dir string) error { deletedBranch = name; return nil },
		},
		WorktreeManager: &mockWorktreeManager{
			WorktreeRemoveFn: func(wtDir, dir string) error { return nil },
		},
	}
	task := StitchTask{BranchName: "task/main-42", WorktreeDir: "/tmp/wt/42"}
	got := CleanupWorktree(task, gitDeps)
	if !got {
		t.Error("expected true on successful cleanup")
	}
	if deletedBranch != "task/main-42" {
		t.Errorf("expected branch task/main-42 deleted, got %q", deletedBranch)
	}
}

func TestCleanupWorktree_RemoveFails(t *testing.T) {
	gitDeps := StitchGitDeps{
		RepoReader: &mockRepoReader{},
		BranchManager: &mockBranchManager{},
		WorktreeManager: &mockWorktreeManager{
			WorktreeRemoveFn: func(wtDir, dir string) error { return fmt.Errorf("remove failed") },
		},
	}
	task := StitchTask{BranchName: "task/main-42", WorktreeDir: "/tmp/wt/42"}
	got := CleanupWorktree(task, gitDeps)
	if got {
		t.Error("expected false when worktree remove fails")
	}
}
