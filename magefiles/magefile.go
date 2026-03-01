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
	"os/exec"
	"path/filepath"
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

// Podman groups container image and container lifecycle targets.
type Podman mg.Namespace

// Prompt groups prompt preview targets.
type Prompt mg.Namespace

// Stats groups the stats targets (LOC, tokens).
type Stats mg.Namespace

// Test groups the testing targets.
type Test mg.Namespace

// Compare groups the cross-generation differential comparison targets.
type Compare mg.Namespace

// Vscode groups the VS Code extension build and install targets.
type Vscode mg.Namespace

// Constitution groups constitution preview targets.
type Constitution mg.Namespace

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

// Status reports code implementation status per use case and release,
// comparing road-map.yaml spec status with test file presence.
func Status() error { return newOrch().CodeStatus() }

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

	if err := rejectSelfTarget(target, orchRoot); err != nil {
		return err
	}
	return newOrch().Scaffold(target, orchRoot)
}

// Pop removes orchestrator-managed files from the target repository:
// magefiles/orchestrator.go, docs/constitutions/, docs/prompts/, and
// configuration.yaml. Pass "." for the current directory.
func (Scaffold) Pop(target string) error {
	orchRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting orchestrator root: %w", err)
	}
	if err := rejectSelfTarget(target, orchRoot); err != nil {
		return err
	}
	return newOrch().Uninstall(target)
}

// rejectSelfTarget returns an error if target resolves to orchRoot.
// Running push or pop against the orchestrator repo itself is destructive:
// push replaces the dev magefile with the template, pop deletes source
// constitutions, prompts, and configuration.
func rejectSelfTarget(target, orchRoot string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolving target path: %w", err)
	}
	if abs == orchRoot {
		return fmt.Errorf("refusing to scaffold the orchestrator repo itself (%s); use a separate target repository", orchRoot)
	}
	return nil
}

// --- Test targets ---

// Unit runs go test on all packages (excluding use-case tests).
func (Test) Unit() error {
	cmd := exec.Command("go", "test", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Usecase runs all use-case tests including Claude-dependent tests.
// Packages run in parallel. For fast local-only tests, use test:usecase:local.
func (Test) Usecase() error {
	for _, pkg := range []string{"./tests/rel01.0/...", "./tests/e2e/..."} {
		cmd := exec.Command("go", "test", "-tags=usecase,claude", "-v", "-count=1", "-timeout", "1800s", pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// Local runs use-case tests that do not require Claude API access.
// Completes in under 60 seconds. Claude-dependent tests are in files
// tagged with //go:build usecase && claude and are excluded here.
func (Test) Local() error {
	for _, pkg := range []string{"./tests/rel01.0/...", "./tests/e2e/..."} {
		cmd := exec.Command("go", "test", "-tags=usecase", "-v", "-count=1", "-timeout", "300s", pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

// Uc runs a use-case test by number (e.g., mage test:uc 001).
// Includes Claude-dependent tests.
func (Test) Uc(uc string) error {
	pkg := fmt.Sprintf("./tests/rel01.0/uc%s/", uc)
	cmd := exec.Command("go", "test", "-tags=usecase,claude", "-v", "-count=1", "-timeout", "1800s", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Benchmark runs long-running benchmark tests (e.g., Stitch100).
func (Test) Benchmark() error {
	cmd := exec.Command("go", "test", "-tags=benchmark", "-v", "-count=1", "-timeout", "7200s", "./tests/rel01.0/...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

// Run executes measure + stitch cycles using the generation.cycles value in configuration.yaml.
// Use RunN to override the cycle count for a single invocation.
func (Generator) Run() error { return newOrch().GeneratorRun(0) }

// RunN executes exactly n cycles of measure + stitch within the current generation.
// Pass n > 0 to override generation.cycles in configuration.yaml for this run only.
func (Generator) RunN(n int) error { return newOrch().GeneratorRun(n) }

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

// Outcomes prints a summary table of task outcome trailers from git history.
func (Stats) Outcomes() error { return newOrch().Outcomes() }

// --- Prompt targets ---

// Measure prints the assembled measure prompt to stdout.
func (Prompt) Measure() error { return newOrch().DumpMeasurePrompt() }

// Stitch prints the assembled stitch prompt to stdout.
func (Prompt) Stitch() error { return newOrch().DumpStitchPrompt() }

// Files lists all files that will be appended to the Claude prompt with sizes and token estimates.
func (Prompt) Files() error { return newOrch().PrintContextFiles() }

// --- Podman targets ---

// Build builds the container image from the embedded Dockerfile with versioned and latest tags.
func (Podman) Build() error { return newOrch().BuildImage() }

// Clean removes all podman containers created from the configured image.
func (Podman) Clean() error { return newOrch().PodmanClean() }

// --- Compare targets ---

// Run builds binaries from two sources and runs differential comparison.
// Each argument is a git tag, "gnu", or a directory path. Set the UTILITY
// environment variable to compare a single utility (e.g., UTILITY=cat).
func (Compare) Run(argA, argB string) error {
	utility := os.Getenv("UTILITY")
	return newOrch().Compare(argA, argB, utility)
}

// --- Vscode targets ---

// Push compiles the extension and installs it into the default VS Code profile.
func (Vscode) Push() error { return newOrch().VscodePush("") }

// PushProfile compiles the extension and installs it into a named VS Code profile
// (e.g., mage vscode:pushProfile GO).
func (Vscode) PushProfile(profile string) error { return newOrch().VscodePush(profile) }

// Pop uninstalls the extension from the default VS Code profile.
func (Vscode) Pop() error { return newOrch().VscodePop("") }

// PopProfile uninstalls the extension from a named VS Code profile
// (e.g., mage vscode:popProfile GO).
func (Vscode) PopProfile(profile string) error { return newOrch().VscodePop(profile) }

// --- Constitution targets ---

// Preview reads a constitution YAML file and prints its sections as markdown to stdout.
// Pass the path to a constitution YAML file (e.g., mage constitution:preview pkg/orchestrator/constitutions/execution.yaml).
func (Constitution) Preview(file string) error { return newOrch().ConstitutionPreviewFile(file) }
