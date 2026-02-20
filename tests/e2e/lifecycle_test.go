// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"strings"
	"testing"
)

// TestGenerator_StartCreatesGenBranch verifies that after mage generator:start
// the current branch matches the "generation-" prefix.
func TestGenerator_StartCreatesGenBranch(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	t.Cleanup(func() { runMage(t, dir, "reset") }) //nolint:errcheck

	branch := gitBranch(t, dir)
	if !strings.HasPrefix(branch, "generation-") {
		t.Errorf("expected branch to start with 'generation-', got %q", branch)
	}
}

// TestGenerator_StartStop_Tags verifies the full start/stop lifecycle:
// after stop the repo is back on main, the generation branch is deleted,
// and the expected git tags (-start, -finished, -merged) exist.
func TestGenerator_StartStop_Tags(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	genBranch := gitBranch(t, dir)
	if !strings.HasPrefix(genBranch, "generation-") {
		t.Fatalf("expected generation branch after start, got %q", genBranch)
	}

	if err := runMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after stop, got %q", branch)
	}

	// Generation branch should be deleted.
	if branches := gitListBranchesMatching(t, dir, genBranch); len(branches) > 0 {
		t.Errorf("generation branch %q should be deleted after stop, got %v", genBranch, branches)
	}

	// Lifecycle tags should exist.
	for _, suffix := range []string{"-start", "-finished", "-merged"} {
		tag := genBranch + suffix
		if !gitTagExists(t, dir, tag) {
			t.Errorf("expected tag %q to exist after stop", tag)
		}
	}
}

// TestGenerator_List_ShowsMerged verifies that mage generator:list shows the
// merged generation after a complete start/stop cycle.
func TestGenerator_List_ShowsMerged(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := runMage(t, dir, "generator:stop"); err != nil {
		t.Fatalf("generator:stop: %v", err)
	}

	out, err := runMageOut(t, dir, "generator:list")
	if err != nil {
		t.Fatalf("generator:list: %v", err)
	}
	if !strings.Contains(out, "merged") {
		t.Errorf("expected 'merged' in generator:list output, got:\n%s", out)
	}
}
