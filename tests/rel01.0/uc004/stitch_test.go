//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package uc004_test

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

func TestRel01_UC004_StitchFailsWithoutBeads(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	os.RemoveAll(filepath.Join(dir, ".beads"))
	if err := testutil.RunMage(t, dir, "cobbler:stitch"); err == nil {
		t.Fatal("expected cobbler:stitch to fail without .beads")
	}
}

func TestRel01_UC004_StitchStopsWhenNoReadyTasks(t *testing.T) {
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

func TestRel01_UC004_StitchWithManualIssue(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)

	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	// Create a task via bd create so stitch has work to pick up.
	bdCreate := exec.Command("bd", "create", "--type", "task",
		"--title", "e2e stitch test task", "--description", "created by e2e test")
	bdCreate.Dir = dir
	if out, err := bdCreate.CombinedOutput(); err != nil {
		t.Fatalf("bd create: %v\n%s", err, out)
	}

	// Point credentials to an impossible path so checkClaude always fails.
	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Claude.SecretsDir = "/dev/null/impossible"
	})

	if err := testutil.RunMage(t, dir, "cobbler:stitch"); err == nil {
		t.Fatal("expected cobbler:stitch to fail without Claude credentials when tasks exist")
	}
}
