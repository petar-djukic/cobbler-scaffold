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

// MeasureCreatesIssues runs a single measure invocation with
// MaxMeasureIssues=1 and verifies at least one issue is created.
func TestRel01_UC003_MeasureCreatesIssues(t *testing.T) {
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

// MeasureRecordsInvocation runs measure and verifies that an InvocationRecord
// is saved in the history stats file with token data.
func TestRel01_UC003_MeasureRecordsInvocation(t *testing.T) {
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

	files := testutil.HistoryStatsFiles(t, dir, "measure")
	if len(files) == 0 {
		t.Fatal("expected at least one measure stats file in .cobbler/history/, got none")
	}

	// Verify the stats file contains token data (evidence of InvocationRecord).
	found := false
	for _, f := range files {
		if testutil.ReadFileContains(f, "tokens:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected measure stats file to contain 'tokens:' field (InvocationRecord)")
	}
}

// BeadsResetClearsAfterMeasure runs measure, then beads:reset, and verifies
// that no ready issues remain.
func TestRel01_UC003_BeadsResetClearsAfterMeasure(t *testing.T) {
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
		t.Fatal("expected at least 1 ready issue after measure, got 0")
	}

	if err := testutil.RunMage(t, dir, "beads:reset"); err != nil {
		t.Fatalf("beads:reset: %v", err)
	}

	if n := testutil.CountReadyIssues(t, dir); n != 0 {
		t.Errorf("expected 0 ready issues after beads:reset, got %d", n)
	}
}
