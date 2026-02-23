//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc003_test

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestRel01_UC003_MeasureFailsWithoutBeads(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	os.RemoveAll(filepath.Join(dir, ".beads"))
	if err := testutil.RunMage(t, dir, "cobbler:measure"); err == nil {
		t.Fatal("expected cobbler:measure to fail without .beads")
	}
}

func TestRel01_UC003_MeasureFailsWithoutGeneration(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Point credentials to an impossible path so checkClaude always fails.
	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	if err := testutil.RunMage(t, dir, "cobbler:measure"); err == nil {
		t.Fatal("expected cobbler:measure to fail without Claude credentials on main")
	}
}

// MeasureCreatesOneIssue runs a single measure invocation with
// MaxMeasureIssues=1 and verifies at least one issue is created.
func TestRel01_UC003_MeasureCreatesOneIssue(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	n := testutil.CountReadyIssues(t, dir)
	if n == 0 {
		t.Error("expected at least 1 ready issue after cobbler:measure, got 0")
	}
	t.Logf("cobbler:measure created %d issue(s)", n)
}
