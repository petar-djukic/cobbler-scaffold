// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
)

// Cobbler groups the measure and stitch targets.
type Cobbler mg.Namespace

// Generator groups the code-generation trail lifecycle targets.
type Generator mg.Namespace

// Scaffold groups the scaffold install/uninstall targets.
type Scaffold mg.Namespace

// Beads groups issue-tracker lifecycle targets.
type Beads mg.Namespace

// Stats groups the stats targets (LOC, tokens).
type Stats mg.Namespace

// Test groups the testing targets.
type Test mg.Namespace

// baseCfg holds the configuration loaded from configuration.yaml.
var baseCfg orchestrator.Config

func init() {
	if _, err := os.Stat(orchestrator.DefaultConfigFile); errors.Is(err, os.ErrNotExist) {
		if err := orchestrator.WriteDefaultConfig(orchestrator.DefaultConfigFile); err != nil {
			panic(fmt.Sprintf("creating %s: %v", orchestrator.DefaultConfigFile, err))
		}
		fmt.Fprintf(os.Stderr, "created default %s\n", orchestrator.DefaultConfigFile)
	}
	var err error
	baseCfg, err = orchestrator.LoadConfig(orchestrator.DefaultConfigFile)
	if err != nil {
		panic(fmt.Sprintf("loading %s: %v", orchestrator.DefaultConfigFile, err))
	}
}

// newOrch creates an Orchestrator from the base config.
func newOrch() *orchestrator.Orchestrator {
	return orchestrator.New(baseCfg)
}

// logf prints a timestamped log line to stderr.
func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] %s\n", time.Now().Format(time.RFC3339), msg)
}

// --- Top-level targets ---

// Init initializes the project (beads).
func Init() error { return newOrch().Init() }

// Reset performs a full reset: cobbler, generator, beads.
func Reset() error { return newOrch().FullReset() }

// Build compiles the project binary.
func Build() error { return newOrch().Build() }

// Lint runs golangci-lint on the project.
func Lint() error { return newOrch().Lint() }

// Install runs go install for the main package.
func Install() error { return newOrch().Install() }

// Clean removes build artifacts.
func Clean() error { return newOrch().Clean() }

// Credentials extracts Claude credentials from the macOS Keychain.
func Credentials() error { return newOrch().ExtractCredentials() }

// Analyze performs cross-artifact consistency checks (PRDs, use cases, test suites, roadmap).
func Analyze() error { return newOrch().Analyze() }

// Tag creates a documentation release tag (v0.YYYYMMDD.N) and builds the container image.
func Tag() error { return newOrch().Tag() }

// --- Scaffold targets ---

// Pop removes orchestrator-managed files from the target repository:
// magefiles/orchestrator.go, docs/constitutions/, docs/prompts/, and
// configuration.yaml. Pass "." for the current directory.
func (Scaffold) Pop(target string) error { return newOrch().Uninstall(target) }

// --- Test targets ---

// Unit runs go test on all packages.
func (Test) Unit() error { return newOrch().TestUnit() }

// E2e runs all E2E tests in tests/rel01.0/.
func (Test) E2e() error { return newOrch().TestE2E() }

// All runs unit and E2E tests.
func (Test) All() error { return newOrch().TestAll() }

// Uc001OrchestratorInitialization runs UC001 E2E tests (init, reset, defaults).
func (Test) Uc001OrchestratorInitialization() error { return newOrch().TestE2EByUseCase("001") }

// Uc002GenerationLifecycle runs UC002 E2E tests (start, stop, list, run, switch).
func (Test) Uc002GenerationLifecycle() error { return newOrch().TestE2EByUseCase("002") }

// Uc003MeasureWorkflow runs UC003 E2E tests (measure creates issues).
func (Test) Uc003MeasureWorkflow() error { return newOrch().TestE2EByUseCase("003") }

// Uc004StitchWorkflow runs UC004 E2E tests (stitch executes tasks).
func (Test) Uc004StitchWorkflow() error { return newOrch().TestE2EByUseCase("004") }

// Uc005ResumeRecovery runs UC005 E2E tests (resume from interruption).
func (Test) Uc005ResumeRecovery() error { return newOrch().TestE2EByUseCase("005") }

// Uc006ScaffoldOperations runs UC006 E2E tests (scaffold push/pop).
func (Test) Uc006ScaffoldOperations() error { return newOrch().TestE2EByUseCase("006") }

// Uc007BuildTooling runs UC007 E2E tests (build, install, clean, stats).
func (Test) Uc007BuildTooling() error { return newOrch().TestE2EByUseCase("007") }

// --- Cobbler targets ---

// Measure assesses project state and proposes new tasks via Claude.
func (Cobbler) Measure() error { return newOrch().Measure() }

// Stitch picks ready tasks and invokes Claude to execute them.
func (Cobbler) Stitch() error { return newOrch().Stitch() }

// Reset removes the cobbler scratch directory.
func (Cobbler) Reset() error { return newOrch().CobblerReset() }

// --- Generator targets ---

// Start begins a new generation trail.
func (Generator) Start() error { return newOrch().GeneratorStart() }

// Run executes N cycles of measure + stitch within the current generation.
func (Generator) Run() error { return newOrch().GeneratorRun() }

// Resume recovers from an interrupted run and continues.
func (Generator) Resume() error { return newOrch().GeneratorResume() }

// Stop completes a generation trail and merges it into main.
func (Generator) Stop() error { return newOrch().GeneratorStop() }

// List shows active branches and past generations.
func (Generator) List() error { return newOrch().GeneratorList() }

// Switch commits current work and checks out another generation branch.
func (Generator) Switch() error { return newOrch().GeneratorSwitch() }

// Reset destroys generation branches, worktrees, and Go source directories.
func (Generator) Reset() error { return newOrch().GeneratorReset() }

// --- Stats targets ---

// Loc prints Go lines of code and documentation word counts.
func (Stats) Loc() error { return newOrch().Stats() }

// Tokens enumerates prompt-attached files and counts tokens via the Anthropic API.
func (Stats) Tokens() error { return newOrch().TokenStats() }

// --- Beads targets ---

// Init initializes the beads issue tracker.
func (Beads) Init() error { return newOrch().BeadsInit() }

// Reset clears beads issue history.
func (Beads) Reset() error { return newOrch().BeadsReset() }
