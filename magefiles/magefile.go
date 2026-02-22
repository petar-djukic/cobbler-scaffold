// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package main provides the mage build targets for the orchestrator repository.
// This file mirrors orchestrator.go (the template copied to consuming repos)
// but is specific to building and testing the orchestrator library itself.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

// Podman groups container image and container lifecycle targets.
type Podman mg.Namespace

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

// boolPtr returns a pointer to a bool value.
func boolPtr(v bool) *bool { return &v }

// --- Top-level targets ---

// Init initializes the project (beads).
func Init() error { return newOrch().Init() }

// Reset performs a full reset: cobbler, generator, beads.
func Reset() error { return newOrch().FullReset() }

// Stats prints Go lines of code and documentation word counts.
func Stats() error { return newOrch().Stats() }

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

// Push scaffolds the orchestrator into a target Go repository. The argument
// is either a local directory path or a Go module reference in module@version
// format (e.g., github.com/org/repo@v0.20260214.1). When a module@version is
// given, the source is fetched via go mod download, copied to a temp directory,
// git-initialized, and scaffolded. The temp directory path is printed to stdout.
func (Scaffold) Push(target string) error {
	orchRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting orchestrator root: %w", err)
	}

	// If target contains @, treat as module@version.
	if parts := strings.SplitN(target, "@", 2); len(parts) == 2 && parts[1] != "" {
		module, version := parts[0], parts[1]
		logf("scaffold:push: using go mod download for %s@%s", module, version)
		repoDir, err := newOrch().PrepareTestRepo(module, version, orchRoot)
		if err != nil {
			return err
		}
		fmt.Println(repoDir)
		return nil
	}

	return newOrch().Scaffold(target, orchRoot)
}

// Pop removes orchestrator-managed files from the target repository:
// magefiles/orchestrator.go, docs/constitutions/, docs/prompts/, and
// configuration.yaml. Pass "." for the current directory.
func (Scaffold) Pop(target string) error { return newOrch().Uninstall(target) }

// --- Test targets (standard) ---

// Unit runs go test on all packages.
func (Test) Unit() error { return newOrch().TestUnit() }

// Integration runs go test in the tests/ directory.
func (Test) Integration() error { return newOrch().TestIntegration() }

// All runs unit and integration tests.
func (Test) All() error { return newOrch().TestAll() }

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

// --- Beads targets ---

// Init initializes the beads issue tracker.
func (Beads) Init() error { return newOrch().BeadsInit() }

// Reset clears beads issue history.
func (Beads) Reset() error { return newOrch().BeadsReset() }

// --- Podman targets ---

// Build builds the container image from the embedded Dockerfile with versioned and latest tags.
func (Podman) Build() error { return newOrch().BuildImage() }

// Clean removes all podman containers created from the configured image.
func (Podman) Clean() error { return newOrch().PodmanClean() }
