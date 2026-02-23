//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// --- Renamed existing tests ---

// TestRel01_UC004_StitchExecutesTask verifies that cobbler:stitch picks a ready
// issue created by measure and executes it: the task is closed, code is
// committed, and the task branch is cleaned up.
//
//	go test -v -count=1 -timeout 900s -run TestRel01_UC004_StitchExecutesTask ./tests/rel01.0/...
func TestRel01_UC004_StitchExecutesTask(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
		cfg.Claude.MaxTimeSec = 600
	})

	// Full reset and start a generation branch.
	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	genBranch := gitBranch(t, dir)
	t.Logf("generation branch: %s", genBranch)

	// Measure: create 1 issue.
	if err := runMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}
	nBefore := countReadyIssues(t, dir)
	if nBefore == 0 {
		t.Fatal("expected at least 1 ready issue after measure, got 0")
	}
	t.Logf("after measure: %d ready issue(s)", nBefore)

	// Record git HEAD before stitch.
	headBefore := gitHead(t, dir)

	// Stitch: execute the issue.
	if err := runMage(t, dir, "cobbler:stitch"); err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}

	// Verify: no ready issues remain.
	nAfter := countReadyIssues(t, dir)
	if nAfter != 0 {
		t.Errorf("expected 0 ready issues after stitch, got %d", nAfter)
	}

	// Verify: git HEAD advanced (stitch merged code).
	headAfter := gitHead(t, dir)
	if headAfter == headBefore {
		t.Error("expected git HEAD to advance after stitch, but it did not")
	}
	t.Logf("HEAD before=%s after=%s", headBefore[:8], headAfter[:8])

	// Verify: no leftover task branches.
	taskBranches := gitListBranchesMatching(t, dir, "task/")
	if len(taskBranches) > 0 {
		t.Errorf("expected no task branches after stitch, got %v", taskBranches)
	}
}

// TestRel01_UC004_TimingByCycle runs alternating measure/stitch cycles and logs
// the wall-clock time for each step.
//
//	go test -v -count=1 -timeout 0 -run TestRel01_UC004_TimingByCycle ./tests/rel01.0/...
func TestRel01_UC004_TimingByCycle(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	const cycles = 5

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
		cfg.Claude.MaxTimeSec = 600
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	genBranch := gitBranch(t, dir)
	t.Logf("generation branch: %s", genBranch)

	type result struct {
		Cycle       int
		MeasureTime time.Duration
		StitchTime  time.Duration
		Issues      int
	}
	results := make([]result, 0, cycles)
	totalStart := time.Now()

	for i := 1; i <= cycles; i++ {
		t.Run(fmt.Sprintf("cycle_%d", i), func(t *testing.T) {
			// Measure
			mStart := time.Now()
			if err := runMage(t, dir, "cobbler:measure"); err != nil {
				t.Fatalf("cycle %d measure: %v", i, err)
			}
			mElapsed := time.Since(mStart).Round(time.Second)

			n := countReadyIssues(t, dir)
			if n == 0 {
				t.Fatalf("cycle %d: expected at least 1 ready issue after measure, got 0", i)
			}
			t.Logf("cycle %d measure: %d issue(s) in %s", i, n, mElapsed)

			headBefore := gitHead(t, dir)

			// Stitch
			sStart := time.Now()
			if err := runMage(t, dir, "cobbler:stitch"); err != nil {
				t.Fatalf("cycle %d stitch: %v", i, err)
			}
			sElapsed := time.Since(sStart).Round(time.Second)

			headAfter := gitHead(t, dir)
			if headAfter == headBefore {
				t.Errorf("cycle %d: HEAD did not advance after stitch", i)
			}
			t.Logf("cycle %d stitch: %s (HEAD %s -> %s)", i, sElapsed, headBefore[:8], headAfter[:8])

			results = append(results, result{
				Cycle:       i,
				MeasureTime: mElapsed,
				StitchTime:  sElapsed,
				Issues:      n,
			})
		})
	}

	totalElapsed := time.Since(totalStart).Round(time.Second)

	// Summary table.
	t.Logf("\n=== Stitch Timing Summary ===")
	t.Logf("%-6s %-12s %-12s %-8s", "Cycle", "Measure", "Stitch", "Issues")
	var totalMeasure, totalStitch time.Duration
	for _, r := range results {
		t.Logf("%-6d %-12s %-12s %-8d", r.Cycle, r.MeasureTime, r.StitchTime, r.Issues)
		totalMeasure += r.MeasureTime
		totalStitch += r.StitchTime
	}
	t.Logf("%-6s %-12s %-12s", "Total", totalMeasure.Round(time.Second), totalStitch.Round(time.Second))
	t.Logf("Wall clock: %s", totalElapsed)
}

// --- New tests ---

// TestRel01_UC004_StitchFailsWithoutBeads verifies that cobbler:stitch fails
// when the .beads/ directory does not exist.
func TestRel01_UC004_StitchFailsWithoutBeads(t *testing.T) {
	dir := setupRepo(t)
	os.RemoveAll(filepath.Join(dir, ".beads"))
	if err := runMage(t, dir, "cobbler:stitch"); err == nil {
		t.Fatal("expected cobbler:stitch to fail without .beads")
	}
}

// TestRel01_UC004_StitchStopsWhenNoReadyTasks verifies that stitch completes
// with 0 tasks when no ready issues exist.
func TestRel01_UC004_StitchStopsWhenNoReadyTasks(t *testing.T) {
	dir := setupRepo(t)

	if err := runMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}

	out, err := runMageOut(t, dir, "cobbler:stitch")
	if err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}
	if !strings.Contains(out, "completed 0 task(s)") {
		t.Errorf("expected 'completed 0 task(s)' in output, got:\n%s", out)
	}
}

// TestRel01_UC004_StitchRecordsInvocation verifies that stitch records an
// InvocationRecord with diff stats on the closed issue. Requires Claude.
func TestRel01_UC004_StitchRecordsInvocation(t *testing.T) {
	dir := setupRepo(t)
	setupClaude(t, dir)

	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
		cfg.Cobbler.MaxStitchIssuesPerCycle = 1
		cfg.Claude.MaxTimeSec = 600
	})

	if err := runMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := runMage(t, dir, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	if err := runMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}
	if err := runMage(t, dir, "cobbler:stitch"); err != nil {
		t.Fatalf("cobbler:stitch: %v", err)
	}

	if !issueHasField(t, dir, "diff_stats") {
		t.Error("expected diff_stats in invocation_record on closed issue")
	}
}
