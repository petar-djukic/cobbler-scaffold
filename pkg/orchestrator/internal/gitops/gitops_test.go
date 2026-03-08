// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package gitops

import (
	"os/exec"
	"testing"
)

// Compile-time interface satisfaction check.
var _ GitOps = (*ShellGitOps)(nil)

func TestShellGitOps_DefaultGitBin(t *testing.T) {
	t.Parallel()
	g := &ShellGitOps{}
	if g.gitBin() != "git" {
		t.Errorf("gitBin() = %q, want %q", g.gitBin(), "git")
	}
}

func TestShellGitOps_CustomGitBin(t *testing.T) {
	t.Parallel()
	g := &ShellGitOps{GitBin: "/usr/local/bin/git"}
	if g.gitBin() != "/usr/local/bin/git" {
		t.Errorf("gitBin() = %q, want %q", g.gitBin(), "/usr/local/bin/git")
	}
}

func TestParseBranchList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		want   int
	}{
		{"empty", "", 0},
		{"single", "  main\n", 1},
		{"multiple", "* main\n  feature\n  bugfix\n", 3},
		{"with_markers", "+ worktree-branch\n* main\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBranchList(tt.input)
			if len(got) != tt.want {
				t.Errorf("ParseBranchList(%q) = %d items, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestParseDiffShortstat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		files int
		ins   int
		del   int
	}{
		{"", 0, 0, 0},
		{" 5 files changed, 100 insertions(+), 20 deletions(-)", 5, 100, 20},
		{" 1 file changed, 10 insertions(+)", 1, 10, 0},
		{" 3 files changed, 50 deletions(-)", 3, 0, 50},
	}
	for _, tt := range tests {
		ds := ParseDiffShortstat(tt.input)
		if ds.FilesChanged != tt.files || ds.Insertions != tt.ins || ds.Deletions != tt.del {
			t.Errorf("ParseDiffShortstat(%q) = {%d, %d, %d}, want {%d, %d, %d}",
				tt.input, ds.FilesChanged, ds.Insertions, ds.Deletions, tt.files, tt.ins, tt.del)
		}
	}
}

func TestParseNumstat(t *testing.T) {
	t.Parallel()
	output := "10\t2\tREADME.md\n-\t-\timage.png\n"
	m := ParseNumstat(output)
	if len(m) != 2 {
		t.Fatalf("got %d entries, want 2", len(m))
	}
	if m["README.md"].Ins != 10 || m["README.md"].Del != 2 {
		t.Errorf("README.md: got %+v", m["README.md"])
	}
	if m["image.png"].Ins != 0 || m["image.png"].Del != 0 {
		t.Errorf("image.png: got %+v", m["image.png"])
	}
}

func TestParseNameStatus(t *testing.T) {
	t.Parallel()
	nsOutput := "A\tnew.go\nM\texisting.go\nR100\told.go\tnew_name.go\n"
	numMap := map[string]NumstatEntry{
		"new.go":      {Ins: 50, Del: 0},
		"existing.go": {Ins: 10, Del: 5},
		"new_name.go": {Ins: 3, Del: 1},
	}
	files := ParseNameStatus(nsOutput, numMap)
	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}
	if files[0].Path != "new.go" || files[0].Status != "A" || files[0].Insertions != 50 {
		t.Errorf("[0] = %+v", files[0])
	}
	if files[2].Path != "new_name.go" || files[2].Status != "R" {
		t.Errorf("[2] = %+v", files[2])
	}
}

// MockGitOps provides a mock implementation of GitOps for testing.
// Each field is a function that, when set, handles the corresponding method.
// Unset fields panic to catch unexpected calls.
type MockGitOps struct {
	CheckoutFn         func(branch, dir string) error
	CheckoutNewFn      func(branch, dir string) error
	CreateBranchFn     func(name, dir string) error
	DeleteBranchFn     func(name, dir string) error
	ForceDeleteBranchFn func(name, dir string) error
	BranchExistsFn     func(name, dir string) bool
	ListBranchesFn     func(pattern, dir string) []string
	CurrentBranchFn    func(dir string) (string, error)
	TagFn              func(name, dir string) error
	DeleteTagFn        func(name, dir string) error
	TagAtFn            func(name, ref, dir string) error
	RenameTagFn        func(oldName, newName, dir string) error
	ListTagsFn         func(pattern, dir string) []string
	LsFilesFn          func(dir string) []string
	StageAllFn         func(dir string) error
	StageDirFn         func(path, dir string) error
	UnstageAllFn       func(dir string) error
	HasChangesFn       func(dir string) bool
	StashFn            func(msg, dir string) error
	CommitFn           func(msg, dir string) error
	CommitAllowEmptyFn func(msg, dir string) error
	RevParseHEADFn     func(dir string) (string, error)
	ResetSoftFn        func(ref, dir string) error
	MergeCmdFn         func(branch, dir string) *exec.Cmd
	WorktreePruneFn    func(dir string) error
	WorktreeAddFn      func(worktreeDir, branch, dir string) *exec.Cmd
	WorktreeRemoveFn   func(worktreeDir, dir string) error
	DiffShortstatFn    func(ref, dir string) (DiffStat, error)
	DiffNameStatusFn   func(ref, dir string) ([]FileChange, error)
	LsTreeFilesFn      func(ref, dir string) ([]string, error)
	ShowFileContentFn  func(ref, path, dir string) ([]byte, error)
}

// Compile-time check that MockGitOps implements GitOps.
var _ GitOps = (*MockGitOps)(nil)

func (m *MockGitOps) Checkout(branch, dir string) error        { return m.CheckoutFn(branch, dir) }
func (m *MockGitOps) CheckoutNew(branch, dir string) error     { return m.CheckoutNewFn(branch, dir) }
func (m *MockGitOps) CreateBranch(name, dir string) error      { return m.CreateBranchFn(name, dir) }
func (m *MockGitOps) DeleteBranch(name, dir string) error      { return m.DeleteBranchFn(name, dir) }
func (m *MockGitOps) ForceDeleteBranch(name, dir string) error { return m.ForceDeleteBranchFn(name, dir) }
func (m *MockGitOps) BranchExists(name, dir string) bool       { return m.BranchExistsFn(name, dir) }
func (m *MockGitOps) ListBranches(pattern, dir string) []string { return m.ListBranchesFn(pattern, dir) }
func (m *MockGitOps) CurrentBranch(dir string) (string, error) { return m.CurrentBranchFn(dir) }
func (m *MockGitOps) Tag(name, dir string) error               { return m.TagFn(name, dir) }
func (m *MockGitOps) DeleteTag(name, dir string) error         { return m.DeleteTagFn(name, dir) }
func (m *MockGitOps) TagAt(name, ref, dir string) error        { return m.TagAtFn(name, ref, dir) }
func (m *MockGitOps) RenameTag(oldName, newName, dir string) error { return m.RenameTagFn(oldName, newName, dir) }
func (m *MockGitOps) ListTags(pattern, dir string) []string    { return m.ListTagsFn(pattern, dir) }
func (m *MockGitOps) LsFiles(dir string) []string              { return m.LsFilesFn(dir) }
func (m *MockGitOps) StageAll(dir string) error                { return m.StageAllFn(dir) }
func (m *MockGitOps) StageDir(path, dir string) error          { return m.StageDirFn(path, dir) }
func (m *MockGitOps) UnstageAll(dir string) error              { return m.UnstageAllFn(dir) }
func (m *MockGitOps) HasChanges(dir string) bool               { return m.HasChangesFn(dir) }
func (m *MockGitOps) Stash(msg, dir string) error              { return m.StashFn(msg, dir) }
func (m *MockGitOps) Commit(msg, dir string) error             { return m.CommitFn(msg, dir) }
func (m *MockGitOps) CommitAllowEmpty(msg, dir string) error   { return m.CommitAllowEmptyFn(msg, dir) }
func (m *MockGitOps) RevParseHEAD(dir string) (string, error)  { return m.RevParseHEADFn(dir) }
func (m *MockGitOps) ResetSoft(ref, dir string) error          { return m.ResetSoftFn(ref, dir) }
func (m *MockGitOps) MergeCmd(branch, dir string) *exec.Cmd    { return m.MergeCmdFn(branch, dir) }
func (m *MockGitOps) WorktreePrune(dir string) error           { return m.WorktreePruneFn(dir) }
func (m *MockGitOps) WorktreeAdd(worktreeDir, branch, dir string) *exec.Cmd { return m.WorktreeAddFn(worktreeDir, branch, dir) }
func (m *MockGitOps) WorktreeRemove(worktreeDir, dir string) error { return m.WorktreeRemoveFn(worktreeDir, dir) }
func (m *MockGitOps) DiffShortstat(ref, dir string) (DiffStat, error) { return m.DiffShortstatFn(ref, dir) }
func (m *MockGitOps) DiffNameStatus(ref, dir string) ([]FileChange, error) { return m.DiffNameStatusFn(ref, dir) }
func (m *MockGitOps) LsTreeFiles(ref, dir string) ([]string, error) { return m.LsTreeFilesFn(ref, dir) }
func (m *MockGitOps) ShowFileContent(ref, path, dir string) ([]byte, error) { return m.ShowFileContentFn(ref, path, dir) }
