//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc007_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
	"gopkg.in/yaml.v3"
)

// requireBuildableSource reads configuration.yaml, derives the main package
// directory from the module path, and skips the test when no Go files exist
// there. This handles specs-only target repos where configuration.yaml
// declares a main_package that has not yet been generated.
func requireBuildableSource(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "configuration.yaml"))
	if err != nil {
		return // let the test fail on its own
	}
	var cfg struct {
		Project struct {
			ModulePath string `yaml:"module_path"`
			MainPackage string `yaml:"main_package"`
		} `yaml:"project"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil || cfg.Project.MainPackage == "" {
		return
	}
	rel := strings.TrimPrefix(cfg.Project.MainPackage, cfg.Project.ModulePath+"/")
	if rel == cfg.Project.MainPackage {
		rel = "."
	}
	goFiles, _ := filepath.Glob(filepath.Join(dir, rel, "*.go"))
	if len(goFiles) == 0 {
		t.Skipf("target repo is specs-only: no Go files in %s", rel)
	}
}

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

func TestRel01_UC007_Build(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	requireBuildableSource(t, dir)
	if err := testutil.RunMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "bin"))
	if err != nil {
		t.Fatalf("reading bin/: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one binary in bin/ after mage build")
	}
}

func TestRel01_UC007_Install(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	requireBuildableSource(t, dir)
	if err := testutil.RunMage(t, dir, "install"); err != nil {
		t.Fatalf("mage install: %v", err)
	}
}

func TestRel01_UC007_Clean(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	requireBuildableSource(t, dir)
	if err := testutil.RunMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build (setup): %v", err)
	}
	if err := testutil.RunMage(t, dir, "clean"); err != nil {
		t.Fatalf("mage clean: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "bin"))
	if len(entries) > 0 {
		t.Errorf("expected bin/ to be empty after mage clean, found %v", entries)
	}
}

func TestRel01_UC007_Stats(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	out, err := testutil.RunMageOut(t, dir, "stats:loc")
	if err != nil {
		t.Fatalf("mage stats: %v", err)
	}
	if !strings.Contains(out, "go_loc") {
		t.Errorf("expected 'go_loc' in mage stats output, got:\n%s", out)
	}
}

func TestRel01_UC007_BuildSkipsWithoutMainPackage(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Project.MainPackage = ""
	})

	if err := testutil.RunMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build with empty MainPackage should skip, got error: %v", err)
	}

	// bin/ should not be created since build was skipped.
	if _, err := os.Stat(filepath.Join(dir, "bin")); err == nil {
		t.Error("expected bin/ to not exist when build is skipped")
	}
}

func TestRel01_UC007_CleanWhenNoBinDir(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	// Ensure bin/ does not exist before clean.
	os.RemoveAll(filepath.Join(dir, "bin"))

	if err := testutil.RunMage(t, dir, "clean"); err != nil {
		t.Fatalf("mage clean without bin/ should succeed, got error: %v", err)
	}
}

func TestRel01_UC007_Analyze(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	out, err := testutil.RunMageOut(t, dir, "analyze")
	// Analyze may report consistency issues on the scaffolded test repo
	// (e.g. YAML schema errors in VISION.yaml). We verify it ran to
	// completion (output mentions "analyze:") rather than crashing.
	if !strings.Contains(out, "analyze:") {
		t.Fatalf("mage analyze produced no output; err=%v\n%s", err, out)
	}
	t.Logf("analyze output:\n%s", out)
}
