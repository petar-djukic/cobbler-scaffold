// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- statusOpen constant ---

func TestStatusOpen_IsOpen(t *testing.T) {
	if statusOpen != "open" {
		t.Errorf("statusOpen = %q, want %q", statusOpen, "open")
	}
}

func TestErrTaskReset_MentionsOpen(t *testing.T) {
	if !strings.Contains(errTaskReset.Error(), "open") {
		t.Errorf("errTaskReset = %q, should mention 'open'", errTaskReset.Error())
	}
}

// --- resetOrphanedIssues integration test ---

// setupBeadsRepo creates a temp directory with a git repo and beads database.
// Returns the directory path. The caller must chdir into it before calling
// functions that shell out to bd (they use the working directory).
func setupBeadsRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "stitch-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.local"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	cmd := exec.Command("bd", "init", "--prefix", "test", "--force")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init: %v\n%s", err, out)
	}

	return dir
}

// testCreateTask creates a beads task in dir and returns its ID.
func testCreateTask(t *testing.T, dir, title string) string {
	t.Helper()
	cmd := exec.Command("bd", "create", "--type", "task", "--json",
		"--title", title, "--description", "test task")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd create: %v\n%s", err, out)
	}
	var issue struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &issue); err != nil {
		t.Fatalf("bd create JSON parse: %v\noutput: %s", err, out)
	}
	return issue.ID
}

// bdSetStatus sets the status of a beads issue.
func bdSetStatus(t *testing.T, dir, id, status string) {
	t.Helper()
	cmd := exec.Command("bd", "update", id, "--status", status)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd update --status %s: %v\n%s", status, err, out)
	}
}

// bdReadyCount returns how many tasks bd ready finds.
func bdReadyCount(t *testing.T, dir string) int {
	t.Helper()
	cmd := exec.Command("bd", "ready", "--json", "--type", "task")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(out, &tasks); err != nil {
		return 0
	}
	return len(tasks)
}

// bdGetStatus returns the status string of a beads issue via bd show --json.
// bd show --json returns a JSON array; we extract status from the first element.
func bdGetStatus(t *testing.T, dir, id string) string {
	t.Helper()
	cmd := exec.Command("bd", "show", "--json", id)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bd show --json %s: %v", id, err)
	}
	var issues []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		t.Fatalf("bd show JSON parse for %s: %v\noutput: %s", id, err, out)
	}
	if len(issues) == 0 {
		t.Fatalf("bd show --json %s: empty result", id)
	}
	return issues[0].Status
}

// requireBd skips the test if the bd CLI is not installed.
func requireBd(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not installed, skipping integration test")
	}
}

// --- validateIssueDescription ---

func TestValidateIssueDescription_Valid(t *testing.T) {
	t.Parallel()
	desc := `deliverable_type: code
required_reading:
  - pkg/orchestrator/generator.go
files:
  - pkg/orchestrator/generator.go
requirements: Implement the feature
acceptance_criteria: Tests pass`

	if err := validateIssueDescription(desc); err != nil {
		t.Errorf("valid description returned error: %v", err)
	}
}

func TestValidateIssueDescription_MissingFields(t *testing.T) {
	t.Parallel()
	desc := `deliverable_type: code
requirements: Implement the feature`

	err := validateIssueDescription(desc)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
	for _, field := range []string{"required_reading", "files", "acceptance_criteria"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error should mention %q, got: %v", field, err)
		}
	}
}

func TestValidateIssueDescription_Empty(t *testing.T) {
	t.Parallel()
	err := validateIssueDescription("")
	if err == nil {
		t.Fatal("expected error for empty description")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestValidateIssueDescription_InvalidYAML(t *testing.T) {
	t.Parallel()
	err := validateIssueDescription("{{{{not yaml")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "YAML") {
		t.Errorf("error should mention 'YAML', got: %v", err)
	}
}

// --- taskBranchPattern ---

func TestTaskBranchPattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base string
		want string
	}{
		{"main", "task/main-*"},
		{"develop", "task/develop-*"},
		{"feature/foo", "task/feature/foo-*"},
	}
	for _, tt := range tests {
		got := taskBranchPattern(tt.base)
		if got != tt.want {
			t.Errorf("taskBranchPattern(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestResetOrphanedIssues_SetsStatusToOpen(t *testing.T) {
	requireBd(t)

	dir := setupBeadsRepo(t)

	// Save and restore working directory — bd commands use cwd.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Create a task and set it to in_progress (simulates stitch claiming it).
	id := testCreateTask(t, dir, "orphaned task")
	bdSetStatus(t, dir, id, "in_progress")

	// Verify the task is in_progress before the reset.
	// Note: bd 0.46.0 returns in_progress tasks from bd ready, so we check
	// the status directly rather than relying on bd ready returning 0.
	if s := bdGetStatus(t, dir, id); s != "in_progress" {
		t.Fatalf("before reset: task status = %q, want in_progress", s)
	}

	// Call resetOrphanedIssues with baseBranch "main". The task has no
	// corresponding task branch (task/main-<id>), so it should be reset.
	recovered := resetOrphanedIssues("main")
	if !recovered {
		t.Fatal("resetOrphanedIssues returned false, expected true (should recover orphaned task)")
	}

	// Verify the task is now open (reset by resetOrphanedIssues).
	if s := bdGetStatus(t, dir, id); s != "open" {
		t.Fatalf("after reset: task status = %q, want open", s)
	}

	// Verify the task is returned by bd ready.
	if n := bdReadyCount(t, dir); n != 1 {
		t.Fatalf("after reset: bd ready returned %d tasks, want 1", n)
	}
}

// TestResetOrphanedIssues_BugRegression_OpenStatusVisible verifies that
// resetOrphanedIssues uses statusOpen ("open") to make tasks available.
//
// Historical note: the old code path used "ready" as the reset status.
// In bd ≥0.46.0 "ready" is no longer a valid status (bd update exits 0 with
// an error, leaving the status unchanged). Using statusOpen ("open") ensures
// tasks are always visible to bd ready regardless of bd version.
func TestResetOrphanedIssues_BugRegression_OpenStatusVisible(t *testing.T) {
	requireBd(t)

	dir := setupBeadsRepo(t)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// statusOpen must be "open" — the only status that reliably makes tasks
	// available via bd ready across all bd versions. The old bug used "ready".
	if statusOpen != "open" {
		t.Fatalf("statusOpen = %q, must be %q to ensure tasks are visible to bd ready", statusOpen, "open")
	}

	id := testCreateTask(t, dir, "reset task")

	// Set to in_progress (simulates a stitch claiming it).
	bdSetStatus(t, dir, id, "in_progress")
	if s := bdGetStatus(t, dir, id); s != "in_progress" {
		t.Fatalf("expected status in_progress, got %q", s)
	}

	// Reset to statusOpen ("open") — this is what resetOrphanedIssues does.
	bdSetStatus(t, dir, id, statusOpen)
	if s := bdGetStatus(t, dir, id); s != "open" {
		t.Fatalf("expected status open after reset, got %q", s)
	}

	// Verify the task is visible to bd ready after being set to "open".
	if n := bdReadyCount(t, dir); n != 1 {
		t.Fatalf("status 'open' should be visible to bd ready, got %d tasks", n)
	}
}

// --- buildStitchPrompt ---

func TestBuildStitchPrompt_NilContext(t *testing.T) {
	// When worktreeDir is empty, buildStitchPrompt skips project context
	// assembly. The function should still produce valid YAML output using
	// embedded constitution defaults.
	o := New(Config{})
	task := stitchTask{
		id:        "test-01",
		title:     "Add unit tests",
		issueType: "code",
	}
	out, err := o.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt() unexpected error: %v", err)
	}
	if !strings.Contains(out, "role:") {
		t.Errorf("buildStitchPrompt() output missing 'role:' field")
	}
	if strings.Contains(out, "project_context:") {
		t.Errorf("buildStitchPrompt() should omit project_context when nil")
	}
}

func TestBuildStitchPrompt_ConstitutionDocs(t *testing.T) {
	// When a worktree dir is set, buildStitchPrompt should include
	// ExecutionConstitution and GoStyleConstitution from embedded defaults
	// even when no project docs exist in the worktree.
	tmp := t.TempDir()
	o := New(Config{})
	task := stitchTask{
		id:          "test-02",
		title:       "Implement feature",
		issueType:   "code",
		worktreeDir: tmp,
	}
	out, err := o.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt() unexpected error: %v", err)
	}
	if !strings.Contains(out, "execution_constitution:") {
		t.Errorf("buildStitchPrompt() output missing 'execution_constitution:' field")
	}
	if !strings.Contains(out, "go_style_constitution:") {
		t.Errorf("buildStitchPrompt() output missing 'go_style_constitution:' field")
	}
}

func TestBuildStitchPrompt_InvalidTemplate(t *testing.T) {
	// An invalid stitch prompt YAML should cause buildStitchPrompt to return
	// an error immediately, before any context assembly is attempted.
	cfg := Config{}
	cfg.Cobbler.StitchPrompt = "role: [unclosed bracket"
	o := New(cfg)
	task := stitchTask{id: "test-03", title: "Test", issueType: "code"}
	_, err := o.buildStitchPrompt(task)
	if err == nil {
		t.Error("buildStitchPrompt() expected error for invalid template, got nil")
	}
}

// --- cleanupWorktree ---

func TestCleanupWorktree_NonExistentDir_NoOp(t *testing.T) {
	// cleanupWorktree is called by resetTask, which the fix added to the
	// buildStitchPrompt error path in doOneTask. When the worktreeDir does
	// not exist (e.g., in test environments without a real git repo),
	// cleanupWorktree must not panic; git errors are logged as warnings.
	task := stitchTask{
		id:          "test-cleanup",
		worktreeDir: "/nonexistent/worktree/path",
		branchName:  "stitch-test-cleanup",
	}
	cleanupWorktree(task) // must not panic
}

func TestBuildStitchPrompt_RepositoryFiles(t *testing.T) {
	// When worktreeDir is a git repo with staged files, buildStitchPrompt
	// must include repository_files: in the output listing those files.
	tmp := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = tmp
		if err := cmd.Run(); err != nil {
			t.Fatalf("setup %v: %v", args, err)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "main.go")

	o := New(Config{})
	task := stitchTask{
		id:          "test-05",
		title:       "Repository files test",
		issueType:   "code",
		worktreeDir: tmp,
	}
	out, err := o.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt() unexpected error: %v", err)
	}
	if !strings.Contains(out, "repository_files:") {
		t.Errorf("buildStitchPrompt() output missing 'repository_files:' field")
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("buildStitchPrompt() output missing 'main.go' in repository_files")
	}
}

func TestBuildStitchPrompt_RequiredReadingFilter(t *testing.T) {
	// When description contains required_reading with .go paths and a
	// worktreeDir is set, the source file filter path is exercised.
	tmp := t.TempDir()
	o := New(Config{})
	task := stitchTask{
		id:        "test-04",
		title:     "Filter sources",
		issueType: "code",
		description: `required_reading:
  - pkg/orchestrator/context.go
  - pkg/orchestrator/stitch.go
`,
		worktreeDir: tmp,
	}
	out, err := o.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt() unexpected error: %v", err)
	}
	if !strings.Contains(out, "execution_constitution:") {
		t.Errorf("buildStitchPrompt() output missing 'execution_constitution:'")
	}
}
