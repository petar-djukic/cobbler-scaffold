//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// --- New / DefaultConfig tests (pure Go, no repo setup) ---

// TestRel01_UC001_NewAppliesDefaults verifies that DefaultConfig populates
// expected default values for zero-value fields.
func TestRel01_UC001_NewAppliesDefaults(t *testing.T) {
	cfg := orchestrator.DefaultConfig()
	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"Project.BinaryDir", cfg.Project.BinaryDir, "bin"},
		{"Generation.Prefix", cfg.Generation.Prefix, "generation-"},
		{"Cobbler.BeadsDir", cfg.Cobbler.BeadsDir, ".beads/"},
		{"Cobbler.Dir", cfg.Cobbler.Dir, ".cobbler/"},
		{"Project.MagefilesDir", cfg.Project.MagefilesDir, "magefiles"},
		{"Claude.SecretsDir", cfg.Claude.SecretsDir, ".secrets"},
		{"Claude.DefaultTokenFile", cfg.Claude.DefaultTokenFile, "claude.json"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
}

// TestRel01_UC001_NewPreservesValues verifies that explicitly set Config
// values are not overwritten by defaults.
func TestRel01_UC001_NewPreservesValues(t *testing.T) {
	cfg := orchestrator.Config{
		Project:    orchestrator.ProjectConfig{ModulePath: "example.com/test", BinaryName: "mybin", BinaryDir: "out"},
		Generation: orchestrator.GenerationConfig{Prefix: "gen-"},
		Cobbler:    orchestrator.CobblerConfig{BeadsDir: ".issues/"},
	}
	o := orchestrator.New(cfg)
	got := o.Config()
	if got.Project.BinaryDir != "out" {
		t.Errorf("BinaryDir = %q, want %q", got.Project.BinaryDir, "out")
	}
	if got.Generation.Prefix != "gen-" {
		t.Errorf("Prefix = %q, want %q", got.Generation.Prefix, "gen-")
	}
	if got.Cobbler.BeadsDir != ".issues/" {
		t.Errorf("BeadsDir = %q, want %q", got.Cobbler.BeadsDir, ".issues/")
	}
}

// TestRel01_UC001_NewReturnsNonNil verifies that New returns a non-nil
// Orchestrator for a minimal Config.
func TestRel01_UC001_NewReturnsNonNil(t *testing.T) {
	o := orchestrator.New(orchestrator.Config{
		Project: orchestrator.ProjectConfig{ModulePath: "example.com/test", BinaryName: "test"},
	})
	if o == nil {
		t.Fatal("expected non-nil Orchestrator from New()")
	}
}

// --- Init and reset tests ---

// TestRel01_UC001_InitCreatesBD verifies that mage init creates the .beads/ directory.
func TestRel01_UC001_InitCreatesBD(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("mage init: %v", err)
	}
	if !fileExists(dir, ".beads") {
		t.Error("expected .beads/ to exist after mage init")
	}
}

// TestRel01_UC001_InitIdempotent verifies that running mage init twice does not fail.
func TestRel01_UC001_InitIdempotent(t *testing.T) {
	dir := setupRepo(t)
	for i := 1; i <= 2; i++ {
		if err := runMage(t, dir, "init"); err != nil {
			t.Fatalf("mage init (attempt %d): %v", i, err)
		}
	}
}

// TestRel01_UC001_CobblerReset verifies that mage cobbler:reset removes .cobbler/
// but leaves .beads/ untouched.
func TestRel01_UC001_CobblerReset(t *testing.T) {
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

// TestRel01_UC001_BeadsResetKeepsDir verifies that mage beads:reset keeps .beads/ present.
func TestRel01_UC001_BeadsResetKeepsDir(t *testing.T) {
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

// TestRel01_UC001_BeadsResetClearsIssues verifies that beads:reset removes previously
// created issues (countReadyIssues returns 0 after reset).
func TestRel01_UC001_BeadsResetClearsIssues(t *testing.T) {
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

// TestRel01_UC001_FullResetClearsState verifies that mage reset clears cobbler dir,
// generation branches, and beads issues.
func TestRel01_UC001_FullResetClearsState(t *testing.T) {
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

// --- Edge cases ---

// TestRel01_UC001_CobblerResetWhenMissing verifies cobbler:reset is a no-op when
// .cobbler/ does not exist.
func TestRel01_UC001_CobblerResetWhenMissing(t *testing.T) {
	dir := setupRepo(t)
	os.RemoveAll(filepath.Join(dir, ".cobbler"))
	if err := runMage(t, dir, "cobbler:reset"); err != nil {
		t.Fatalf("cobbler:reset with missing .cobbler/: %v", err)
	}
}

// TestRel01_UC001_BeadsResetWhenMissing verifies beads:reset is a no-op when
// .beads/ does not exist.
func TestRel01_UC001_BeadsResetWhenMissing(t *testing.T) {
	dir := setupRepo(t)
	os.RemoveAll(filepath.Join(dir, ".beads"))
	if err := runMage(t, dir, "beads:reset"); err != nil {
		t.Fatalf("beads:reset with missing .beads/: %v", err)
	}
}

// TestRel01_UC001_GeneratorResetWhenClean verifies generator:reset exits 0 when
// already on main with no generation branches.
func TestRel01_UC001_GeneratorResetWhenClean(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "generator:reset"); err != nil {
		t.Fatalf("generator:reset from clean state: %v", err)
	}
	if branch := gitBranch(t, dir); branch != "main" {
		t.Errorf("expected main branch, got %q", branch)
	}
}
