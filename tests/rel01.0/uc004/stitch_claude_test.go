//go:build usecase && claude

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Tests in this file require Claude API access (via podman container).
// They are excluded from test:usecase:local and only run with
// -tags=usecase,claude (i.e., mage test:usecase).

package uc004_test

import (
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

// claudeTimeout is the per-invocation limit for mage targets that call Claude.
var claudeTimeout = testutil.ClaudeTestTimeout

// StitchExecutesTask runs 1 measure (MaxMeasureIssues=1) then 1 stitch
// (MaxStitchIssuesPerCycle=1) and verifies the task was processed.
// Precondition: hasUnresolvedRequirements via MeasureAndExpectIssues (GH-1798).
// Requires Claude: invokes cobbler:measure and cobbler:stitch.
func TestRel01_UC004_StitchExecutesTask(t *testing.T) {
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_ = testutil.GeneratorStart(t, dir)

	headBefore := testutil.GitHead(t, dir)

	testutil.MeasureAndExpectIssues(t, dir, 60*time.Second)

	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:stitch"); err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}

	headAfter := testutil.GitHead(t, dir)
	if headAfter == headBefore {
		t.Error("expected git HEAD to advance after stitch, but it did not")
	}
}

// StitchRecordsInvocation runs measure+stitch and verifies that the stitch
// history contains an InvocationRecord with diff stats and LOC data.
// Precondition: hasUnresolvedRequirements via MeasureAndExpectIssues (GH-1798).
// Requires Claude: invokes cobbler:measure and cobbler:stitch.
func TestRel01_UC004_StitchRecordsInvocation(t *testing.T) {
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_ = testutil.GeneratorStart(t, dir)

	testutil.MeasureAndExpectIssues(t, dir, 60*time.Second)

	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:stitch"); err != nil {
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

// SecondMeasureProducesNoNewTasks runs a measure → stitch → measure cycle
// and asserts the second measure creates zero new tasks. After stitch
// implements the single proposed task and closes its issue, the second
// measure should detect that the implementation satisfies the requirement
// and return an empty task list rather than spinning with follow-up tasks.
// Precondition: hasUnresolvedRequirements via MeasureAndExpectIssues (GH-1798).
// Requires Claude: invokes cobbler:measure and cobbler:stitch.
func TestRel01_UC004_SecondMeasureProducesNoNewTasks(t *testing.T) {
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_ = testutil.GeneratorStart(t, dir)

	// First measure: propose one task (with precondition check and retry).
	testutil.MeasureAndExpectIssues(t, dir, 60*time.Second)

	// Stitch: implement the proposed task and close its issue.
	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:stitch"); err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}

	// Second measure: with the task implemented and its issue closed, measure
	// should recognise the spec is satisfied and return an empty task list.
	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:measure"); err != nil {
		t.Fatalf("second cobbler:measure: %v", err)
	}

	n := testutil.CountReadyIssues(t, dir)
	if n != 0 {
		t.Errorf("expected 0 ready issues after second measure (stitch already implemented the task), got %d", n)
	}
	t.Logf("second cobbler:measure created %d issue(s) (want 0)", n)
}
