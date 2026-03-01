//go:build usecase && claude

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Tests in this file require Claude API access (via podman container).
// They are excluded from test:usecase:local and only run with
// -tags=usecase,claude (i.e., mage test:usecase).

package uc003_test

import (
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

// MeasureCreatesIssues runs a single measure invocation with
// MaxMeasureIssues=1 and verifies at least one issue is created.
// Requires Claude: invokes cobbler:measure which calls Claude via podman.
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
// Requires Claude: invokes cobbler:measure which calls Claude via podman.
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

