// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestScaffold_ConstitutionFiles verifies that all four constitution files
// are written to docs/constitutions/ by PrepareTestRepo/Scaffold.
func TestScaffold_ConstitutionFiles(t *testing.T) {
	dir := setupRepo(t)
	for _, name := range []string{"planning.yaml", "execution.yaml", "design.yaml", "go-style.yaml"} {
		rel := filepath.Join("docs", "constitutions", name)
		if !fileExists(dir, rel) {
			t.Errorf("expected %s to exist after scaffold", rel)
		}
	}
}

// TestScaffold_PromptFiles verifies that prompt YAML files
// are written to docs/prompts/ by PrepareTestRepo/Scaffold.
func TestScaffold_PromptFiles(t *testing.T) {
	dir := setupRepo(t)
	for _, name := range []string{"measure.yaml", "stitch.yaml"} {
		rel := filepath.Join("docs", "prompts", name)
		if !fileExists(dir, rel) {
			t.Errorf("expected %s to exist after scaffold", rel)
		}
	}
}

// TestScaffold_ConfigAndMagefile verifies that configuration.yaml and
// magefiles/orchestrator.go are present after scaffold.
func TestScaffold_ConfigAndMagefile(t *testing.T) {
	dir := setupRepo(t)
	for _, rel := range []string{"configuration.yaml", filepath.Join("magefiles", "orchestrator.go")} {
		if !fileExists(dir, rel) {
			t.Errorf("expected %s to exist after scaffold", rel)
		}
	}
}

// TestBuild verifies that mage build compiles the binary successfully.
func TestBuild(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build: %v", err)
	}
	// mcp-calc binary name is "mcp-calc" (last segment of module path).
	// configuration.yaml sets binary_name from detectBinaryName.
	// Check that something was placed in bin/.
	entries, err := os.ReadDir(filepath.Join(dir, "bin"))
	if err != nil {
		t.Fatalf("reading bin/: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one binary in bin/ after mage build")
	}
}

// TestInstall verifies that mage install exits 0.
func TestInstall(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "install"); err != nil {
		t.Fatalf("mage install: %v", err)
	}
}

// TestClean verifies that mage clean removes build artifacts.
func TestClean(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build (setup): %v", err)
	}
	if err := runMage(t, dir, "clean"); err != nil {
		t.Fatalf("mage clean: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "bin"))
	if len(entries) > 0 {
		t.Errorf("expected bin/ to be empty after mage clean, found %v", entries)
	}
}

// TestStats verifies that mage stats exits 0 and prints go_loc.
func TestStats(t *testing.T) {
	dir := setupRepo(t)
	out, err := runMageOut(t, dir, "stats")
	if err != nil {
		t.Fatalf("mage stats: %v", err)
	}
	if !strings.Contains(out, "go_loc") {
		t.Errorf("expected 'go_loc' in mage stats output, got:\n%s", out)
	}
}

