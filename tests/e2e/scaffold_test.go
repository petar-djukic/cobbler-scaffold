// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
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

// TestScaffold_RejectSelfTarget verifies that scaffold:push and scaffold:pop
// refuse to operate on the orchestrator repository itself. Both targets should
// exit non-zero with an error mentioning "refusing to scaffold".
func TestScaffold_RejectSelfTarget(t *testing.T) {
	for _, target := range []string{"scaffold:push", "scaffold:pop"} {
		t.Run(target, func(t *testing.T) {
			cmd := exec.Command("mage", "-d", ".", target, ".")
			cmd.Dir = orchRoot
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("%s . should have failed but succeeded:\n%s", target, out)
			}
			if !strings.Contains(string(out), "refusing to scaffold") {
				t.Errorf("expected 'refusing to scaffold' in error output, got:\n%s", out)
			}
		})
	}
}

// TestScaffold_PushPopRoundTrip creates an empty Go repository, scaffolds the
// orchestrator into it, verifies all expected files exist and mage targets are
// available, then removes the scaffold with Uninstall and verifies all
// orchestrator files are gone. This exercises the full push/pop lifecycle
// without depending on an external module download.
func TestScaffold_PushPopRoundTrip(t *testing.T) {
	// Load config from the orchestrator repo root.
	cfg, err := orchestrator.LoadConfig(filepath.Join(orchRoot, "configuration.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	orch := orchestrator.New(cfg)

	// Create an empty Go repository in a temp directory.
	dir := t.TempDir()
	for _, args := range [][]string{
		{"go", "mod", "init", "example.com/roundtrip-test"},
		{"git", "init"},
		{"git", "config", "user.email", "test@test.local"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}

	// Create a minimal magefiles directory so Scaffold has somewhere to write.
	if err := os.MkdirAll(filepath.Join(dir, "magefiles"), 0o755); err != nil {
		t.Fatalf("mkdir magefiles: %v", err)
	}

	// --- Push: scaffold the orchestrator into the empty repo ---
	if err := orch.Scaffold(dir, orchRoot); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// Verify all expected files exist after push.
	pushExpected := []string{
		filepath.Join("magefiles", "orchestrator.go"),
		"configuration.yaml",
		filepath.Join("docs", "constitutions", "design.yaml"),
		filepath.Join("docs", "constitutions", "planning.yaml"),
		filepath.Join("docs", "constitutions", "execution.yaml"),
		filepath.Join("docs", "constitutions", "go-style.yaml"),
		filepath.Join("docs", "prompts", "measure.yaml"),
		filepath.Join("docs", "prompts", "stitch.yaml"),
		filepath.Join("magefiles", "go.mod"),
	}
	for _, rel := range pushExpected {
		if !fileExists(dir, rel) {
			t.Errorf("after push: expected %s to exist", rel)
		}
	}

	// Verify mage -l works in the scaffolded repo.
	mageCmd := exec.Command("mage", "-d", ".", "-l")
	mageCmd.Dir = dir
	mageOut, err := mageCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mage -l after push: %v\n%s", err, mageOut)
	}
	if !strings.Contains(string(mageOut), "scaffold:pop") {
		t.Errorf("expected scaffold:pop in mage -l output, got:\n%s", mageOut)
	}

	// --- Pop: remove the scaffold ---
	if err := orch.Uninstall(dir); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// Verify scaffolded files are removed after pop.
	popRemoved := []string{
		filepath.Join("magefiles", "orchestrator.go"),
		"configuration.yaml",
		filepath.Join("docs", "constitutions"),
		filepath.Join("docs", "prompts"),
	}
	for _, rel := range popRemoved {
		if fileExists(dir, rel) {
			t.Errorf("after pop: expected %s to be removed", rel)
		}
	}

	// Verify magefiles/go.mod is preserved (pop does not delete it).
	if !fileExists(dir, filepath.Join("magefiles", "go.mod")) {
		t.Error("after pop: expected magefiles/go.mod to be preserved")
	}
}

