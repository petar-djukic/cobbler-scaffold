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
	CommitAllowEmptyFn     func(msg, dir string) error
	CommitAmendTrailersFn func(dir string, trailers []string) error
	RevParseHEADFn         func(dir string) (string, error)
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
func (m *MockGitOps) CommitAmendTrailers(dir string, trailers []string) error { return m.CommitAmendTrailersFn(dir, trailers) }
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

// --- cmdGit tests ---

func TestCmdGit_SetsDir(t *testing.T) {
	t.Parallel()
	g := &ShellGitOps{GitBin: "git"}
	cmd := g.cmdGit("/tmp", "status")
	if cmd.Dir != "/tmp" {
		t.Errorf("cmd.Dir = %q, want /tmp", cmd.Dir)
	}
	if cmd.Args[0] != "git" {
		t.Errorf("cmd.Args[0] = %q, want git", cmd.Args[0])
	}
	if cmd.Args[1] != "status" {
		t.Errorf("cmd.Args[1] = %q, want status", cmd.Args[1])
	}
}

func TestCmdGit_EmptyDir(t *testing.T) {
	t.Parallel()
	g := &ShellGitOps{}
	cmd := g.cmdGit("", "log")
	if cmd.Dir != "" {
		t.Errorf("cmd.Dir = %q, want empty for no dir", cmd.Dir)
	}
}

// --- ParseBranchList edge cases ---

func TestParseBranchList_OnlyBlanks(t *testing.T) {
	t.Parallel()
	got := ParseBranchList("\n\n  \n\t\n")
	if len(got) != 0 {
		t.Errorf("ParseBranchList blanks = %v, want empty", got)
	}
}

func TestParseBranchList_StarAndPlusPrefixes(t *testing.T) {
	t.Parallel()
	input := "* main\n+ feature/worktree\n  develop\n"
	got := ParseBranchList(input)
	want := []string{"main", "feature/worktree", "develop"}
	if len(got) != len(want) {
		t.Fatalf("ParseBranchList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- ParseDiffShortstat edge cases ---

func TestParseDiffShortstat_InsertionsOnly(t *testing.T) {
	t.Parallel()
	ds := ParseDiffShortstat(" 2 files changed, 30 insertions(+)")
	if ds.FilesChanged != 2 {
		t.Errorf("FilesChanged = %d, want 2", ds.FilesChanged)
	}
	if ds.Insertions != 30 {
		t.Errorf("Insertions = %d, want 30", ds.Insertions)
	}
	if ds.Deletions != 0 {
		t.Errorf("Deletions = %d, want 0", ds.Deletions)
	}
}

func TestParseDiffShortstat_DeletionsOnly(t *testing.T) {
	t.Parallel()
	ds := ParseDiffShortstat(" 1 file changed, 5 deletions(-)")
	if ds.FilesChanged != 1 || ds.Deletions != 5 || ds.Insertions != 0 {
		t.Errorf("got %+v", ds)
	}
}

// --- ParseNumstat edge cases ---

func TestParseNumstat_Empty(t *testing.T) {
	t.Parallel()
	m := ParseNumstat("")
	if len(m) != 0 {
		t.Errorf("ParseNumstat empty = %v, want empty map", m)
	}
}

func TestParseNumstat_MalformedLine(t *testing.T) {
	t.Parallel()
	// Line with fewer than 3 tab-separated fields is skipped
	m := ParseNumstat("10\tREADME.md\n")
	if len(m) != 0 {
		t.Errorf("ParseNumstat malformed = %v, want empty map", m)
	}
}

func TestParseNumstat_BinaryFile(t *testing.T) {
	t.Parallel()
	m := ParseNumstat("-\t-\tbinary.png\n")
	entry, ok := m["binary.png"]
	if !ok {
		t.Fatal("expected binary.png in map")
	}
	if entry.Ins != 0 || entry.Del != 0 {
		t.Errorf("binary entry = %+v, want {0, 0}", entry)
	}
}

// --- ParseNameStatus edge cases ---

func TestParseNameStatus_Empty(t *testing.T) {
	t.Parallel()
	files := ParseNameStatus("", nil)
	if len(files) != 0 {
		t.Errorf("ParseNameStatus empty = %v, want empty", files)
	}
}

func TestParseNameStatus_DeletedFile(t *testing.T) {
	t.Parallel()
	nsOutput := "D\tremoved.go\n"
	numMap := map[string]NumstatEntry{
		"removed.go": {Ins: 0, Del: 25},
	}
	files := ParseNameStatus(nsOutput, numMap)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Status != "D" || files[0].Path != "removed.go" {
		t.Errorf("got %+v, want D removed.go", files[0])
	}
	if files[0].Deletions != 25 {
		t.Errorf("Deletions = %d, want 25", files[0].Deletions)
	}
}

func TestParseNameStatus_CopyStatus(t *testing.T) {
	t.Parallel()
	nsOutput := "C100\told.go\tnew_copy.go\n"
	numMap := map[string]NumstatEntry{
		"new_copy.go": {Ins: 10, Del: 0},
	}
	files := ParseNameStatus(nsOutput, numMap)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Status != "C" {
		t.Errorf("Status = %q, want C", files[0].Status)
	}
	if files[0].Path != "new_copy.go" {
		t.Errorf("Path = %q, want new_copy.go", files[0].Path)
	}
}

func TestParseNameStatus_NoNumstat(t *testing.T) {
	t.Parallel()
	nsOutput := "M\tmodified.go\n"
	files := ParseNameStatus(nsOutput, nil)
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Insertions != 0 || files[0].Deletions != 0 {
		t.Errorf("expected zero counts with nil numMap, got %+v", files[0])
	}
}

func TestParseNameStatus_ShortLine(t *testing.T) {
	t.Parallel()
	// A line with only one tab-separated field should be skipped
	files := ParseNameStatus("M\n", nil)
	if len(files) != 0 {
		t.Errorf("expected empty for malformed line, got %v", files)
	}
}

// --- DiffStat / FileChange structs ---

func TestDiffStat_ZeroValue(t *testing.T) {
	t.Parallel()
	var ds DiffStat
	if ds.FilesChanged != 0 || ds.Insertions != 0 || ds.Deletions != 0 {
		t.Errorf("zero-value DiffStat = %+v, want all zeros", ds)
	}
}

func TestFileChange_Fields(t *testing.T) {
	t.Parallel()
	fc := FileChange{Path: "main.go", Status: "M", Insertions: 5, Deletions: 3}
	if fc.Path != "main.go" {
		t.Errorf("Path = %q, want main.go", fc.Path)
	}
}

// --- LsFiles guard ---

func TestShellGitOps_LsFiles_EmptyDir(t *testing.T) {
	t.Parallel()
	g := &ShellGitOps{}
	got := g.LsFiles("")
	if got != nil {
		t.Errorf("LsFiles(\"\") = %v, want nil", got)
	}
}
