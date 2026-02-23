//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc006_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

var (
	orchRoot    string
	snapshotDir string
)

func TestMain(m *testing.M) {
	var err error
	orchRoot, err = testutil.FindOrchestratorRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: resolving orchRoot: %v\n", err)
		os.Exit(1)
	}
	snapshot, cleanup, prepErr := testutil.PrepareSnapshot(orchRoot)
	if prepErr != nil {
		fmt.Fprintf(os.Stderr, "e2e: preparing snapshot: %v\n", prepErr)
		os.Exit(1)
	}
	snapshotDir = snapshot
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestRel01_UC006_ConstitutionFiles(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	for _, name := range []string{"planning.yaml", "execution.yaml", "design.yaml", "go-style.yaml"} {
		rel := filepath.Join("docs", "constitutions", name)
		if !testutil.FileExists(dir, rel) {
			t.Errorf("expected %s to exist after scaffold", rel)
		}
	}
}

func TestRel01_UC006_PromptFiles(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	for _, name := range []string{"measure.yaml", "stitch.yaml"} {
		rel := filepath.Join("docs", "prompts", name)
		if !testutil.FileExists(dir, rel) {
			t.Errorf("expected %s to exist after scaffold", rel)
		}
	}
}

func TestRel01_UC006_ConfigAndMagefile(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	for _, rel := range []string{"configuration.yaml", filepath.Join("magefiles", "orchestrator.go")} {
		if !testutil.FileExists(dir, rel) {
			t.Errorf("expected %s to exist after scaffold", rel)
		}
	}
}

func TestRel01_UC006_RejectSelfTarget(t *testing.T) {
	t.Parallel()
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

func TestRel01_UC006_PushIdempotent(t *testing.T) {
	t.Parallel()
	cfg, err := orchestrator.LoadConfig(filepath.Join(orchRoot, "configuration.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	orch := orchestrator.New(cfg)

	dir := t.TempDir()
	for _, args := range [][]string{
		{"go", "mod", "init", "example.com/idempotent-test"},
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
	if err := os.MkdirAll(filepath.Join(dir, "magefiles"), 0o755); err != nil {
		t.Fatalf("mkdir magefiles: %v", err)
	}

	// First scaffold.
	if err := orch.Scaffold(dir, orchRoot); err != nil {
		t.Fatalf("first Scaffold: %v", err)
	}

	// Second scaffold (idempotent overwrite).
	if err := orch.Scaffold(dir, orchRoot); err != nil {
		t.Fatalf("second Scaffold: %v", err)
	}

	// Verify scaffolded files still exist.
	for _, rel := range []string{
		"configuration.yaml",
		filepath.Join("magefiles", "orchestrator.go"),
		filepath.Join("docs", "constitutions", "design.yaml"),
	} {
		if !testutil.FileExists(dir, rel) {
			t.Errorf("after second push: expected %s to exist", rel)
		}
	}
}

func TestRel01_UC006_PopOnNonScaffolded(t *testing.T) {
	t.Parallel()
	cfg, err := orchestrator.LoadConfig(filepath.Join(orchRoot, "configuration.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	orch := orchestrator.New(cfg)

	dir := t.TempDir()
	for _, args := range [][]string{
		{"go", "mod", "init", "example.com/nonscaffolded-test"},
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

	// Uninstall on a repo that was never scaffolded should succeed as a
	// no-op: all the files it tries to remove are already absent.
	if err := orch.Uninstall(dir); err != nil {
		t.Fatalf("Uninstall on non-scaffolded repo should be a no-op, got error: %v", err)
	}

	// Verify the empty repo was not modified.
	if testutil.FileExists(dir, "configuration.yaml") {
		t.Error("expected no configuration.yaml after no-op Uninstall")
	}
	if testutil.FileExists(dir, filepath.Join("docs", "constitutions")) {
		t.Error("expected no docs/constitutions after no-op Uninstall")
	}
}

func TestRel01_UC006_PushPopRoundTrip(t *testing.T) {
	t.Parallel()
	cfg, err := orchestrator.LoadConfig(filepath.Join(orchRoot, "configuration.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	orch := orchestrator.New(cfg)

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

	if err := os.MkdirAll(filepath.Join(dir, "magefiles"), 0o755); err != nil {
		t.Fatalf("mkdir magefiles: %v", err)
	}

	// --- Push: scaffold the orchestrator into the empty repo ---
	if err := orch.Scaffold(dir, orchRoot); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

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
		if !testutil.FileExists(dir, rel) {
			t.Errorf("after push: expected %s to exist", rel)
		}
	}

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

	popRemoved := []string{
		filepath.Join("magefiles", "orchestrator.go"),
		"configuration.yaml",
		filepath.Join("docs", "constitutions"),
		filepath.Join("docs", "prompts"),
	}
	for _, rel := range popRemoved {
		if testutil.FileExists(dir, rel) {
			t.Errorf("after pop: expected %s to be removed", rel)
		}
	}

	if !testutil.FileExists(dir, filepath.Join("magefiles", "go.mod")) {
		t.Error("after pop: expected magefiles/go.mod to be preserved")
	}
}
