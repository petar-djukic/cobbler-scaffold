//go:build usecase && claude

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Tests in this file require Claude API access (via podman container).
// They are excluded from test:usecase:local and only run with
// -tags=usecase,claude (i.e., mage test:usecase).

package uc004_test

import (
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

// StitchExecutesTask runs 1 measure (MaxMeasureIssues=1) then 1 stitch
// (MaxStitchIssuesPerCycle=1) and verifies the task was processed.
// Requires Claude: invokes cobbler:measure and cobbler:stitch which call Claude via podman.
func TestRel01_UC004_StitchExecutesTask(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	headBefore := testutil.GitHead(t, dir)

	if err := testutil.RunMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}
	if n := testutil.CountReadyIssues(t, dir); n == 0 {
		t.Fatal("expected at least 1 ready issue after measure, got 0")
	}

	if err := testutil.RunMage(t, dir, "cobbler:stitch"); err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}

	headAfter := testutil.GitHead(t, dir)
	if headAfter == headBefore {
		t.Error("expected git HEAD to advance after stitch, but it did not")
	}
}

// StitchRecordsInvocation runs measure+stitch and verifies that the stitch
// history contains an InvocationRecord with diff stats and LOC data.
// Requires Claude: invokes cobbler:measure and cobbler:stitch which call Claude via podman.
func TestRel01_UC004_StitchRecordsInvocation(t *testing.T) {
	t.Parallel()
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := testutil.RunMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := testutil.RunMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}
	if err := testutil.RunMage(t, dir, "cobbler:stitch"); err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}

	// Verify stitch stats file exists and contains diff data.
	statsFiles := testutil.HistoryStatsFiles(t, dir, "stitch")
	if len(statsFiles) == 0 {
		t.Fatal("expected at least one stitch stats file in .cobbler/history/, got none")
	}

	hasDiff := false
	hasLOC := false
	for _, f := range statsFiles {
		if testutil.ReadFileContains(f, "diff:") {
			hasDiff = true
		}
		if testutil.ReadFileContains(f, "loc_before:") {
			hasLOC = true
		}
	}
	if !hasDiff {
		t.Error("expected stitch stats file to contain 'diff:' (InvocationRecord with diff stats)")
	}
	if !hasLOC {
		t.Error("expected stitch stats file to contain 'loc_before:' (InvocationRecord with LOC)")
	}

	// Verify stitch report file exists.
	reportFiles := testutil.HistoryReportFiles(t, dir, "stitch")
	if len(reportFiles) == 0 {
		t.Fatal("expected at least one stitch report file in .cobbler/history/, got none")
	}
}
