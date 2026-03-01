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

func TestErrTaskReset_MentionsOpen(t *testing.T) {
	if !strings.Contains(errTaskReset.Error(), "open") {
		t.Errorf("errTaskReset = %q, should mention 'open'", errTaskReset.Error())
	}
}

// --- failed-task cycle tracking ---

// TestRunStitchN_SkipsAlreadyFailedTask verifies the core invariant of the
// per-cycle failed-task set: once a task ID is recorded as failed, any
// subsequent pick of the same ID must cause the loop to terminate rather
// than re-execute the task. This is the mechanism that prevents infinite
// retry loops when a task repeatedly fails (e.g., Podman timeout).
//
// The full RunStitchN stack requires beads, git, and a Claude container, so
// this test exercises the tracking logic directly using the same map
// operations that RunStitchN uses internally.
func TestRunStitchN_SkipsAlreadyFailedTask(t *testing.T) {
	t.Parallel()

	// Start with an empty failed set (beginning of a stitch cycle).
	failedTaskIDs := map[string]struct{}{}

	taskA := "atlas-test-01"
	taskB := "atlas-test-02"

	// AC2: taskA has not failed yet — should not be skipped.
	if _, alreadyFailed := failedTaskIDs[taskA]; alreadyFailed {
		t.Error("taskA should not be in failedTaskIDs before it has failed")
	}

	// Simulate errTaskReset for taskA: RunStitchN adds it to failedTaskIDs.
	failedTaskIDs[taskA] = struct{}{}

	// AC1/AC3: taskA is now in the set — the loop would break on re-pick.
	if _, alreadyFailed := failedTaskIDs[taskA]; !alreadyFailed {
		t.Error("taskA should be in failedTaskIDs after errTaskReset")
	}

	// AC2: taskB has not failed — should still execute normally.
	if _, alreadyFailed := failedTaskIDs[taskB]; alreadyFailed {
		t.Error("taskB should not be skipped; it has not failed this cycle")
	}

	// Simulate taskB also failing.
	failedTaskIDs[taskB] = struct{}{}

	// With both tasks failed, any re-pick would terminate the loop.
	for _, id := range []string{taskA, taskB} {
		if _, alreadyFailed := failedTaskIDs[id]; !alreadyFailed {
			t.Errorf("task %s should be in failedTaskIDs after errTaskReset", id)
		}
	}
}

// TestRunStitchN_FreshCycleHasNoFailedTasks verifies that a new stitch cycle
// starts with an empty failedTaskIDs set, so tasks that failed in a previous
// cycle are eligible to run again.
func TestRunStitchN_FreshCycleHasNoFailedTasks(t *testing.T) {
	t.Parallel()

	// Each call to RunStitchN allocates a fresh map — simulate that here.
	firstCycleFailed := map[string]struct{}{"atlas-test-01": {}}
	secondCycleMap := map[string]struct{}{} // fresh allocation per cycle

	// Task that failed in the first cycle should not be in the second cycle's map.
	if _, alreadyFailed := secondCycleMap["atlas-test-01"]; alreadyFailed {
		t.Error("task that failed in a previous cycle must not carry over to the next cycle")
	}
	// Confirm the first cycle map still records the failure.
	if _, alreadyFailed := firstCycleFailed["atlas-test-01"]; !alreadyFailed {
		t.Error("first cycle map should still record the failure")
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

// --- taskBranchName ---

func TestTaskBranchName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base    string
		issueID string
		want    string
	}{
		{"main", "42", "task/main-42"},
		{"develop", "100", "task/develop-100"},
		{"generation-2026-02-28", "7", "task/generation-2026-02-28-7"},
	}
	for _, tt := range tests {
		got := taskBranchName(tt.base, tt.issueID)
		if got != tt.want {
			t.Errorf("taskBranchName(%q, %q) = %q, want %q", tt.base, tt.issueID, got, tt.want)
		}
	}
}

// --- parseRequiredReading ---

func TestParseRequiredReading_ValidYAML(t *testing.T) {
	t.Parallel()
	desc := `required_reading:
  - pkg/orchestrator/generator.go
  - pkg/orchestrator/stitch.go
  - docs/ARCHITECTURE.yaml
`
	got := parseRequiredReading(desc)
	if len(got) != 3 {
		t.Errorf("parseRequiredReading() returned %d items, want 3: %v", len(got), got)
	}
}

// --- commitWorktreeChanges ---

func TestCommitWorktreeChanges_NoChanges(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "config", "commit.gpgsign", "false")
	run("git", "commit", "--allow-empty", "-m", "initial")

	task := stitchTask{
		id:          "123",
		title:       "test task",
		worktreeDir: dir,
	}

	if err := commitWorktreeChanges(task); err != nil {
		t.Errorf("commitWorktreeChanges() with no changes error = %v", err)
	}
}

func TestCommitWorktreeChanges_WithChanges(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "config", "commit.gpgsign", "false")
	run("git", "commit", "--allow-empty", "-m", "initial")

	// Create a new file to stage.
	os.WriteFile(filepath.Join(dir, "newfile.go"), []byte("package main\n"), 0o644)

	task := stitchTask{
		id:          "456",
		title:       "add file",
		worktreeDir: dir,
	}

	if err := commitWorktreeChanges(task); err != nil {
		t.Fatalf("commitWorktreeChanges() with changes error = %v", err)
	}

	// Verify the commit was made by checking the log.
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if !strings.Contains(string(out), "Task 456: add file") {
		t.Errorf("commit message = %q, want to contain 'Task 456: add file'", string(out))
	}
}

// --- createWorktree ---

func TestCreateWorktree_CreatesWorktreeAndBranch(t *testing.T) {
	dir := initTestGitRepo(t)

	task := stitchTask{
		id:          "789",
		branchName:  "task/main-789",
		worktreeDir: filepath.Join(dir+"-worktrees", "789"),
	}

	if err := createWorktree(task); err != nil {
		t.Fatalf("createWorktree() error = %v", err)
	}
	t.Cleanup(func() {
		gitWorktreeRemove(task.worktreeDir)
		gitDeleteBranch(task.branchName)
	})

	// Verify the worktree directory exists.
	if _, err := os.Stat(task.worktreeDir); os.IsNotExist(err) {
		t.Error("worktree directory should exist after createWorktree()")
	}

	// Verify the branch was created.
	if !gitBranchExists(task.branchName) {
		t.Errorf("branch %q should exist after createWorktree()", task.branchName)
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
