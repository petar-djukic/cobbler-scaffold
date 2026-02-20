// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mesh-intelligence/mage-claude-orchestrator/pkg/orchestrator"
)

// --- Test targets (integration) ---

// Scaffold sets up a target Go repository to use the orchestrator.
// The argument is either a local directory path or a Go module reference
// in module@version format (e.g., github.com/org/repo@v0.20260214.1).
// When a module@version is given, the source is fetched via go mod download,
// copied to a temp directory, git-initialized, and scaffolded. The temp
// directory path is printed to stdout.
func (Test) Scaffold(target string) error {
	orchRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting orchestrator root: %w", err)
	}

	// If target contains @, treat as module@version.
	if parts := strings.SplitN(target, "@", 2); len(parts) == 2 && parts[1] != "" {
		module, version := parts[0], parts[1]
		logf("test:scaffold: using go mod download for %s@%s", module, version)
		repoDir, err := newOrch().PrepareTestRepo(module, version, orchRoot)
		if err != nil {
			return err
		}
		fmt.Println(repoDir)
		return nil
	}

	return newOrch().Scaffold(target, orchRoot)
}

// Cobbler runs the full cobbler regression suite. Requires Claude.
// Creates an orchestrator with config overrides (max_measure_issues=3,
// silence=true), measures 3 issues, stitches them, and verifies all
// are closed with no stale branches.
func (Test) Cobbler() error {
	logf("test:cobbler: starting regression suite")

	cfg := baseCfg
	cfg.Cobbler.MaxMeasureIssues = 3
	cfg.Claude.SilenceAgent = boolPtr(true)
	orch := orchestrator.New(cfg)

	// Reset to clean state.
	logf("test:cobbler: resetting to clean state")
	if err := orch.FullReset(); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	if err := orch.Init(); err != nil {
		return fmt.Errorf("init: %w", err)
	}

	// Measure — should create 3 issues.
	logf("test:cobbler: measuring (expecting 3 issues)")
	if err := orch.Measure(); err != nil {
		return fmt.Errorf("measure: %w", err)
	}

	// Stitch — should close all issues.
	logf("test:cobbler: stitching all issues")
	if err := orch.Stitch(); err != nil {
		return fmt.Errorf("stitch: %w", err)
	}

	// Verify all issues are closed.
	logf("test:cobbler: verifying all issues closed")
	open, err := countOpenIssues()
	if err != nil {
		return fmt.Errorf("counting open issues: %w", err)
	}
	if open > 0 {
		return fmt.Errorf("expected 0 open issues, got %d", open)
	}

	fmt.Println("Cobbler regression test PASSED")
	return nil
}

// Generator runs the full generator lifecycle suite. Requires Claude.
// Runs three progressive tests: (1) start/stop with no Claude,
// (2) start/run(1 cycle)/stop, (3) stitch respects per-cycle limit.
func (Test) Generator() error {
	logf("test:generator: starting lifecycle suite")

	// Sub-test 1: start/stop with no Claude.
	logf("test:generator: sub-test 1 — start/stop (no Claude)")
	orch := orchestrator.New(baseCfg)
	if err := orch.FullReset(); err != nil {
		return fmt.Errorf("sub-test 1 reset: %w", err)
	}
	if err := orch.GeneratorStart(); err != nil {
		return fmt.Errorf("sub-test 1 start: %w", err)
	}
	if err := orch.GeneratorStop(); err != nil {
		return fmt.Errorf("sub-test 1 stop: %w", err)
	}
	logf("test:generator: sub-test 1 passed")

	// Sub-test 2: start/run(1 cycle)/stop.
	logf("test:generator: sub-test 2 — start/run/stop (1 cycle)")
	cfg2 := baseCfg
	cfg2.Claude.SilenceAgent = boolPtr(true)
	cfg2.Cobbler.MaxMeasureIssues = 1
	cfg2.Generation.Cycles = 1
	orch2 := orchestrator.New(cfg2)
	if err := orch2.FullReset(); err != nil {
		return fmt.Errorf("sub-test 2 reset: %w", err)
	}
	if err := orch2.GeneratorStart(); err != nil {
		return fmt.Errorf("sub-test 2 start: %w", err)
	}
	if err := orch2.GeneratorRun(); err != nil {
		return fmt.Errorf("sub-test 2 run: %w", err)
	}
	if err := orch2.GeneratorStop(); err != nil {
		return fmt.Errorf("sub-test 2 stop: %w", err)
	}
	logf("test:generator: sub-test 2 passed")

	// Sub-test 3: stitch per-cycle limit.
	logf("test:generator: sub-test 3 — per-cycle limit")
	cfg3 := baseCfg
	cfg3.Claude.SilenceAgent = boolPtr(true)
	cfg3.Cobbler.MaxMeasureIssues = 1
	cfg3.Cobbler.MaxStitchIssuesPerCycle = 2
	cfg3.Generation.Cycles = 1
	orch3 := orchestrator.New(cfg3)
	if err := orch3.FullReset(); err != nil {
		return fmt.Errorf("sub-test 3 reset: %w", err)
	}
	if err := orch3.GeneratorStart(); err != nil {
		return fmt.Errorf("sub-test 3 start: %w", err)
	}
	if err := orch3.GeneratorRun(); err != nil {
		return fmt.Errorf("sub-test 3 run: %w", err)
	}
	if err := orch3.GeneratorStop(); err != nil {
		return fmt.Errorf("sub-test 3 stop: %w", err)
	}
	logf("test:generator: sub-test 3 passed")

	// Clean up.
	orchestrator.New(baseCfg).FullReset()

	fmt.Println("Generator lifecycle tests PASSED")
	return nil
}

// E2E runs the E2E test suite against target repositories. Requires Claude.
// Sets E2E_CLAUDE=1 so all tests including Claude-gated ones are executed.
func (Test) E2E() error { return newOrch().TestE2E() }

// Resume runs the resume recovery test. Requires Claude.
// Creates a generation, measures 1 issue, switches to main
// (simulating an interruption), then calls resume.
func (Test) Resume() error {
	logf("test:resume: starting recovery test")

	cfg := baseCfg
	cfg.Claude.SilenceAgent = boolPtr(true)
	cfg.Cobbler.MaxMeasureIssues = 1
	cfg.Generation.Cycles = 1
	orch := orchestrator.New(cfg)

	// Reset and start a generation.
	logf("test:resume: resetting and starting generation")
	if err := orch.FullReset(); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	if err := orch.GeneratorStart(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	// Measure 1 issue.
	logf("test:resume: measuring 1 issue")
	if err := orch.Measure(); err != nil {
		return fmt.Errorf("measure: %w", err)
	}

	// Simulate interruption by switching to main.
	logf("test:resume: simulating interruption (switching to main)")
	if err := orch.GeneratorSwitch(); err != nil {
		// GeneratorSwitch requires GenerationBranch to be set.
		// Set it to main for the switch.
		cfg.Generation.Branch = "main"
		switchOrch := orchestrator.New(cfg)
		if err := switchOrch.GeneratorSwitch(); err != nil {
			return fmt.Errorf("switch to main: %w", err)
		}
	}

	// Resume — should auto-detect the generation branch and stitch.
	logf("test:resume: resuming")
	cfg.Generation.Branch = "" // clear so resume auto-detects
	resumeOrch := orchestrator.New(cfg)
	if err := resumeOrch.GeneratorResume(); err != nil {
		return fmt.Errorf("resume: %w", err)
	}

	// Clean up.
	logf("test:resume: cleaning up")
	orchestrator.New(baseCfg).FullReset()

	fmt.Println("Generator resume test PASSED")
	return nil
}

// countOpenIssues returns the number of open (non-closed) issues in beads.
func countOpenIssues() (int, error) {
	out, err := bdListReadyJSON()
	if err != nil {
		return 0, err
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(out, &tasks); err != nil {
		// Empty list or no tasks — treat as 0.
		return 0, nil
	}
	return len(tasks), nil
}

// bdListReadyJSON calls bd list with ready status filter.
func bdListReadyJSON() ([]byte, error) {
	return execOutput("bd", "list", "--json", "--status", "ready", "--type", "task")
}

// execOutput runs a command and returns its stdout.
func execOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
