// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInit_CreatesBD verifies that mage init creates the .beads/ directory.
func TestInit_CreatesBD(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("mage init: %v", err)
	}
	if !fileExists(dir, ".beads") {
		t.Error("expected .beads/ to exist after mage init")
	}
}

// TestInit_Idempotent verifies that running mage init twice does not fail.
func TestInit_Idempotent(t *testing.T) {
	dir := setupRepo(t)
	for i := 1; i <= 2; i++ {
		if err := runMage(t, dir, "init"); err != nil {
			t.Fatalf("mage init (attempt %d): %v", i, err)
		}
	}
}

// TestCobblerReset verifies that mage cobbler:reset removes .cobbler/ but
// leaves .beads/ untouched.
func TestCobblerReset(t *testing.T) {
	dir := setupRepo(t)

	// Setup: init beads and create a cobbler directory with a file.
	if err := runMage(t, dir, "beads:init"); err != nil {
		t.Fatalf("beads:init: %v", err)
	}
	cobblerDir := filepath.Join(dir, ".cobbler")
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		t.Fatalf("creating .cobbler: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cobblerDir, "dummy.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("writing dummy.json: %v", err)
	}

	if err := runMage(t, dir, "cobbler:reset"); err != nil {
		t.Fatalf("mage cobbler:reset: %v", err)
	}

	if fileExists(dir, ".cobbler") {
		t.Error(".cobbler/ should not exist after cobbler:reset")
	}
	if !fileExists(dir, ".beads") {
		t.Error(".beads/ should still exist after cobbler:reset")
	}
}

// TestBeadsReset_KeepsDir verifies that mage beads:reset keeps .beads/ present.
func TestBeadsReset_KeepsDir(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "beads:init"); err != nil {
		t.Fatalf("beads:init: %v", err)
	}
	if err := runMage(t, dir, "beads:reset"); err != nil {
		t.Fatalf("mage beads:reset: %v", err)
	}
	if !fileExists(dir, ".beads") {
		t.Error(".beads/ should still exist after beads:reset")
	}
}

// TestBeadsReset_ClearsIssues verifies that beads:reset removes previously
// created issues (countReadyIssues returns 0 after reset).
func TestBeadsReset_ClearsIssues(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "beads:init"); err != nil {
		t.Fatalf("beads:init: %v", err)
	}

	// Create a task via bd.
	bdCreate := exec.Command("bd", "create", "--type", "task",
		"--title", "e2e test task", "--description", "created by e2e test")
	bdCreate.Dir = dir
	if out, err := bdCreate.CombinedOutput(); err != nil {
		t.Fatalf("bd create: %v\n%s", err, out)
	}

	if err := runMage(t, dir, "beads:reset"); err != nil {
		t.Fatalf("mage beads:reset: %v", err)
	}
	if n := countReadyIssues(t, dir); n != 0 {
		t.Errorf("expected 0 ready issues after beads:reset, got %d", n)
	}
}

// TestGeneratorReset verifies that generator:reset returns to main and
// removes generation branches.
func TestGeneratorReset(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := runMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("mage generator:reset: %v", err)
	}

	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main branch after generator:reset, got %q", branch)
	}
	if branches := gitListBranchesMatching(t, dir, "generation-"); len(branches) > 0 {
		t.Errorf("expected no generation branches after reset, got %v", branches)
	}
}

// TestFullReset verifies that mage reset clears cobbler dir, generation
// branches, and beads issues.
func TestFullReset(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	cobblerDir := filepath.Join(dir, ".cobbler")
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		t.Fatalf("creating .cobbler: %v", err)
	}

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("mage reset: %v", err)
	}

	if fileExists(dir, ".cobbler") {
		t.Error(".cobbler/ should not exist after full reset")
	}
	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main after reset, got %q", branch)
	}
	if n := countReadyIssues(t, dir); n != 0 {
		t.Errorf("expected 0 ready issues after reset, got %d", n)
	}
}

// TestEdge_CobblerResetWhenMissing verifies cobbler:reset is a no-op when
// .cobbler/ does not exist.
func TestEdge_CobblerResetWhenMissing(t *testing.T) {
	dir := setupRepo(t)
	os.RemoveAll(filepath.Join(dir, ".cobbler"))
	if err := runMage(t, dir, "cobbler:reset"); err != nil {
		t.Fatalf("cobbler:reset with missing .cobbler/: %v", err)
	}
}

// TestEdge_BeadsResetWhenMissing verifies beads:reset is a no-op when
// .beads/ does not exist.
func TestEdge_BeadsResetWhenMissing(t *testing.T) {
	dir := setupRepo(t)
	os.RemoveAll(filepath.Join(dir, ".beads"))
	if err := runMage(t, dir, "beads:reset"); err != nil {
		t.Fatalf("beads:reset with missing .beads/: %v", err)
	}
}

// TestEdge_GeneratorResetWhenClean verifies generator:reset exits 0 when
// already on main with no generation branches.
func TestEdge_GeneratorResetWhenClean(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("generator:reset from clean state: %v", err)
	}
	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main branch, got %q", branch)
	}
}
