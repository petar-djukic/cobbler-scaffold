//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc004_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

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

// StitchNoReadyIssues verifies that cobbler:stitch exits cleanly with
// "completed 0 task(s)" when no cobbler-ready issues exist for the generation.
func TestRel01_UC004_StitchNoReadyIssues(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	out, err := testutil.RunMageOut(t, dir, "cobbler:stitch")
	if err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}
	if !strings.Contains(out, "completed 0 task(s)") {
		t.Errorf("expected 'completed 0 task(s)' in output, got:\n%s", out)
	}
}
