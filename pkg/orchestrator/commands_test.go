// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- parseBranchList ---

func TestParseBranchList_StripsMarkers(t *testing.T) {
	input := "  main\n* current\n+ other\n"
	got := parseBranchList(input)
	want := []string{"main", "current", "other"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("index %d: got %q, want %q", i, got[i], w)
		}
	}
}

func TestParseBranchList_EmptyInput(t *testing.T) {
	got := parseBranchList("")
	if len(got) != 0 {
		t.Errorf("got %v, want empty slice", got)
	}
}

func TestParseBranchList_SkipsBlankLines(t *testing.T) {
	got := parseBranchList("main\n\n  \nfeature\n")
	if len(got) != 2 || got[0] != "main" || got[1] != "feature" {
		t.Errorf("got %v, want [main feature]", got)
	}
}

func TestParseBranchList_GenerationPattern(t *testing.T) {
	input := "  generation-20260214.0\n  generation-20260215.1\n"
	got := parseBranchList(input)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 entries", got)
	}
	if got[0] != "generation-20260214.0" || got[1] != "generation-20260215.1" {
		t.Errorf("got %v", got)
	}
}

// --- parseDiffShortstat ---

func TestParseDiffShortstat_FullOutput(t *testing.T) {
	s := " 5 files changed, 100 insertions(+), 20 deletions(-)\n"
	ds := parseDiffShortstat(s)
	if ds.FilesChanged != 5 {
		t.Errorf("FilesChanged: got %d, want 5", ds.FilesChanged)
	}
	if ds.Insertions != 100 {
		t.Errorf("Insertions: got %d, want 100", ds.Insertions)
	}
	if ds.Deletions != 20 {
		t.Errorf("Deletions: got %d, want 20", ds.Deletions)
	}
}

func TestParseDiffShortstat_InsertionsOnly(t *testing.T) {
	s := " 3 files changed, 42 insertions(+)\n"
	ds := parseDiffShortstat(s)
	if ds.FilesChanged != 3 {
		t.Errorf("FilesChanged: got %d, want 3", ds.FilesChanged)
	}
	if ds.Insertions != 42 {
		t.Errorf("Insertions: got %d, want 42", ds.Insertions)
	}
	if ds.Deletions != 0 {
		t.Errorf("Deletions: got %d, want 0", ds.Deletions)
	}
}

func TestParseDiffShortstat_Empty(t *testing.T) {
	ds := parseDiffShortstat("")
	if ds.FilesChanged != 0 || ds.Insertions != 0 || ds.Deletions != 0 {
		t.Errorf("empty input: got %+v, want all zeros", ds)
	}
}

func TestParseDiffShortstat_SingleFile(t *testing.T) {
	s := " 1 file changed, 1 insertion(+), 1 deletion(-)\n"
	ds := parseDiffShortstat(s)
	if ds.FilesChanged != 1 {
		t.Errorf("FilesChanged: got %d, want 1", ds.FilesChanged)
	}
	if ds.Insertions != 1 {
		t.Errorf("Insertions: got %d, want 1", ds.Insertions)
	}
	if ds.Deletions != 1 {
		t.Errorf("Deletions: got %d, want 1", ds.Deletions)
	}
}

// --- orDefault ---

func TestOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		val      string
		fallback string
		want     string
	}{
		{
			name:     "non-empty val returns val unchanged",
			val:      "custom.yaml",
			fallback: "default.yaml",
			want:     "custom.yaml",
		},
		{
			name:     "empty val returns fallback",
			val:      "",
			fallback: "default.yaml",
			want:     "default.yaml",
		},
		{
			name:     "both empty returns empty string",
			val:      "",
			fallback: "",
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := orDefault(tc.val, tc.fallback)
			if got != tc.want {
				t.Errorf("orDefault(%q, %q) = %q, want %q", tc.val, tc.fallback, got, tc.want)
			}
		})
	}
}

// --- init ---

func TestCommandsInit_PopulatesPath(t *testing.T) {
	// init() runs automatically when the package is loaded. Verify that
	// PATH is non-empty and contains a directory with "bin" in its name,
	// which indicates that GOBIN or GOPATH/bin was prepended.
	path := os.Getenv("PATH")
	if path == "" {
		t.Error("PATH is empty after init()")
	}
	if !strings.Contains(path, "bin") {
		t.Errorf("PATH = %q, expected it to contain 'bin' after init()", path)
	}
}

// --- git primitive helpers (git-dependent, no t.Parallel) ---

func TestGitTagAt(t *testing.T) {
	initTestGitRepo(t)

	head, err := gitRevParseHEAD("")
	if err != nil {
		t.Fatalf("gitRevParseHEAD: %v", err)
	}

	if err := gitTagAt("v1.2.3", head, ""); err != nil {
		t.Fatalf("gitTagAt: %v", err)
	}

	tags := gitListTags("v1.2.3", "")
	if len(tags) != 1 || tags[0] != "v1.2.3" {
		t.Errorf("gitListTags after gitTagAt: got %v, want [v1.2.3]", tags)
	}
}

func TestGitStash(t *testing.T) {
	dir := initTestGitRepo(t)

	// Create and commit a tracked file so we have something to stash.
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add tracked file")

	// Modify the file without staging it.
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	const stashMsg = "my-test-stash"
	if err := gitStash(stashMsg, ""); err != nil {
		t.Fatalf("gitStash: %v", err)
	}

	if gitHasChanges("") {
		t.Error("gitHasChanges: want false after stash, got true")
	}

	out, err := exec.Command("git", "stash", "list").Output()
	if err != nil {
		t.Fatalf("git stash list: %v", err)
	}
	if !strings.Contains(string(out), stashMsg) {
		t.Errorf("stash list %q does not contain message %q", string(out), stashMsg)
	}
}

func TestGitStageDir(t *testing.T) {
	dir := initTestGitRepo(t)

	subdir := filepath.Join(dir, "mydir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := gitStageDir("mydir", ""); err != nil {
		t.Fatalf("gitStageDir: %v", err)
	}

	out, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if !strings.Contains(string(out), "mydir/file.txt") {
		t.Errorf("git status %q: want mydir/file.txt to be staged", string(out))
	}
}

func TestGitCommitAllowEmpty(t *testing.T) {
	initTestGitRepo(t)

	head1, err := gitRevParseHEAD("")
	if err != nil {
		t.Fatalf("gitRevParseHEAD before: %v", err)
	}

	if err := gitCommitAllowEmpty("empty commit", ""); err != nil {
		t.Fatalf("gitCommitAllowEmpty: %v", err)
	}

	head2, err := gitRevParseHEAD("")
	if err != nil {
		t.Fatalf("gitRevParseHEAD after: %v", err)
	}
	if head1 == head2 {
		t.Error("HEAD did not change after gitCommitAllowEmpty")
	}
}

func TestGitLsTreeFiles(t *testing.T) {
	dir := initTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add two files")

	files, err := gitLsTreeFiles("HEAD", "")
	if err != nil {
		t.Fatalf("gitLsTreeFiles: %v", err)
	}

	found := make(map[string]bool, len(files))
	for _, f := range files {
		found[f] = true
	}
	if !found["alpha.txt"] {
		t.Errorf("gitLsTreeFiles: missing alpha.txt, got %v", files)
	}
	if !found["beta.txt"] {
		t.Errorf("gitLsTreeFiles: missing beta.txt, got %v", files)
	}
}

func TestGitShowFileContent(t *testing.T) {
	dir := initTestGitRepo(t)

	const want = "hello from git\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add hello file")

	got, err := gitShowFileContent("HEAD", "hello.txt", "")
	if err != nil {
		t.Fatalf("gitShowFileContent: %v", err)
	}
	if string(got) != want {
		t.Errorf("gitShowFileContent = %q, want %q", string(got), want)
	}
}

func TestGitDiffShortstat(t *testing.T) {
	dir := initTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add data file")

	// Append a line without staging; git diff HEAD shows working-tree changes.
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := gitDiffShortstat("HEAD", "")
	if err != nil {
		t.Fatalf("gitDiffShortstat: %v", err)
	}
	if ds.Insertions < 1 {
		t.Errorf("gitDiffShortstat insertions = %d, want >= 1", ds.Insertions)
	}
}

func TestGitDiffNameStatus(t *testing.T) {
	dir := initTestGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add data file")

	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changes, err := gitDiffNameStatus("HEAD", "")
	if err != nil {
		t.Fatalf("gitDiffNameStatus: %v", err)
	}

	var found bool
	for _, fc := range changes {
		if fc.Path == "data.txt" && fc.Status == "M" {
			found = true
		}
	}
	if !found {
		t.Errorf("gitDiffNameStatus: no FileChange with Path=data.txt Status=M; got %+v", changes)
	}
}
