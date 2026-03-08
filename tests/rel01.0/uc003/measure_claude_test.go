//go:build usecase && claude

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Tests in this file require Claude API access (via podman container).
// They are excluded from test:usecase:local and only run with
// -tags=usecase,claude (i.e., mage test:usecase).

package uc003_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
)

// claudeTimeout is the per-invocation limit for mage targets that call Claude.
var claudeTimeout = testutil.ClaudeTestTimeout

// MeasureCreatesIssues runs a single measure invocation with
// MaxMeasureIssues=1 and verifies at least one issue is created.
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
	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	n := testutil.WaitForReadyIssues(t, dir, 1, 30*time.Second)
	if n == 0 {
		t.Error("expected at least 1 ready issue after cobbler:measure, got 0")
	}
	t.Logf("cobbler:measure created %d issue(s)", n)
}

// seedHelloWorldSource writes the minimal Go source files that satisfy
// prd001 (hello world binary) into dir and commits them. The sdd-hello-world
// snapshot only contains specs; without seeding, measure correctly proposes
// implementing the binary.
func seedHelloWorldSource(t *testing.T, dir string) {
	t.Helper()
	cmdDir := filepath.Join(dir, "cmd", "sdd-hello-world")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("seedHelloWorldSource: mkdir: %v", err)
	}
	mainGo := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	versionGo := `package main

// Version is the current release version.
const Version = "0.0.1"
`
	if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("seedHelloWorldSource: write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "version.go"), []byte(versionGo), 0o644); err != nil {
		t.Fatalf("seedHelloWorldSource: write version.go: %v", err)
	}
	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "Seed hello world source for spec-complete test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("seedHelloWorldSource: %v: %s", err, out)
		}
	}
}

// MeasureReturnsZeroForImplementedSpec runs cobbler:measure against a
// codebase with Go sources that satisfy prd001. The measure agent should
// return an empty task list and create zero issues. This validates the
// spec-complete detection introduced in GH-889: the measure prompt now
// explicitly permits returning [] when no meaningful work remains.
// Requires Claude: invokes cobbler:measure which calls Claude via podman.
func TestRel01_UC003_MeasureReturnsZeroForImplementedSpec(t *testing.T) {
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	// Seed the Go source files that satisfy the spec. The sdd-hello-world
	// snapshot is specs-only (generator:stop strips sources), so without
	// this seeding measure correctly proposes implementing the binary.
	seedHelloWorldSource(t, dir)

	// Allow up to 3 issues so that if measure incorrectly proposes work it
	// shows up in the assertion, rather than being artificially limited to 0.
	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = 3
	})

	// init is a no-op but follows the test convention; do NOT reset so the
	// full codebase is visible to the measure agent.
	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := testutil.RunMageTimeout(t, dir, claudeTimeout, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure: %v", err)
	}

	n := testutil.CountReadyIssues(t, dir)
	if n != 0 {
		t.Errorf("expected 0 ready issues after measure on fully-implemented codebase, got %d", n)
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

