//go:build usecase && claude

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Tests in this file require Claude API access (via podman container).
// They are excluded from test:usecase:local and only run with
// -tags=usecase,claude (i.e., mage test:usecase).

package uc003_test

import (
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

// claudeTimeout is the per-invocation limit for mage targets that call Claude.
var claudeTimeout = testutil.ClaudeTestTimeout

// MeasureCreatesIssues verifies that measure proposes tasks when unresolved
// requirements exist. Uses MeasureAndExpectIssues which checks the
// precondition (hasUnresolvedRequirements) and retries once on empty
// Claude response (GH-1798).
// Requires Claude: invokes cobbler:measure which calls Claude via podman.
func TestRel01_UC003_MeasureCreatesIssues(t *testing.T) {
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 1
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_ = testutil.GeneratorStart(t, dir)

	n := testutil.MeasureAndExpectIssues(t, dir, 60*time.Second)
	t.Logf("cobbler:measure created %d issue(s)", n)
}

// MeasureReturnsZeroForImplementedSpec marks all requirements as complete
// in requirements.yaml, then runs cobbler:measure and verifies zero tasks
// are proposed. This is a deterministic test: the orchestrator knows all
// R-items are complete, and both the prompt constraint and the validation
// layer reject proposals for completed requirements (GH-1798).
// Requires Claude: invokes cobbler:measure which calls Claude via podman.
func TestRel01_UC003_MeasureReturnsZeroForImplementedSpec(t *testing.T) {
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	// Allow up to 3 issues so that if measure incorrectly proposes work it
	// shows up in the assertion, rather than being artificially limited to 0.
	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 3
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	_ = testutil.GeneratorStart(t, dir)

	// Mark all requirements complete — deterministic precondition.
	testutil.MarkAllRequirementsComplete(t, dir)

	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	n := testutil.CountReadyIssues(t, dir)
	if n != 0 {
		t.Errorf("expected 0 ready issues after measure with all requirements complete, got %d", n)
	}
	t.Logf("cobbler:measure created %d issue(s) (want 0)", n)
}

// MeasureRecordsInvocation runs measure and verifies that an InvocationRecord
// is saved in the history stats file with token data.
// Requires Claude: invokes cobbler:measure which calls Claude via podman.
func TestRel01_UC003_MeasureRecordsInvocation(t *testing.T) {
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
	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:measure"); err != nil {
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

