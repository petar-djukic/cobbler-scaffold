// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
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

// requireBd skips the test if the bd CLI is not installed.
func requireBd(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not installed, skipping integration test")
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

	// Verify the task is NOT returned by bd ready (it's in_progress).
	if n := bdReadyCount(t, dir); n != 0 {
		t.Fatalf("before reset: bd ready returned %d tasks, want 0", n)
	}

	// Call resetOrphanedIssues with baseBranch "main". The task has no
	// corresponding task branch (task/main-<id>), so it should be reset.
	recovered := resetOrphanedIssues("main")
	if !recovered {
		t.Fatal("resetOrphanedIssues returned false, expected true (should recover orphaned task)")
	}

	// Verify the task is now returned by bd ready.
	if n := bdReadyCount(t, dir); n != 1 {
		t.Fatalf("after reset: bd ready returned %d tasks, want 1", n)
	}
}

func TestResetOrphanedIssues_BugRegression_ReadyStatusInvisible(t *testing.T) {
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

	// Demonstrate the bug: setting status to "ready" (the old code path)
	// makes the task invisible to bd ready.
	id := testCreateTask(t, dir, "invisible task")
	bdSetStatus(t, dir, id, "ready")

	if n := bdReadyCount(t, dir); n != 0 {
		t.Fatalf("status 'ready' should be invisible to bd ready, but got %d tasks", n)
	}

	// Setting to "open" (the fix) makes it visible.
	bdSetStatus(t, dir, id, statusOpen)

	if n := bdReadyCount(t, dir); n != 1 {
		t.Fatalf("status 'open' should be visible to bd ready, but got %d tasks", n)
	}
}
