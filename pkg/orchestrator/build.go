// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/build"
)

// ---------------------------------------------------------------------------
// Dependency injection: wire the parent package's logf, binary paths,
// and helper functions into the internal/build package at init time.
// ---------------------------------------------------------------------------

func init() {
	build.Log = logf
	build.BinGo = binGo
	build.BinLint = binLint
	build.BinSecurity = binSecurity
	build.BinPodman = binPodman
	build.BinMage = binMage
	build.BinGit = binGit
	build.PodmanBuildFn = podmanBuild
	build.ReadVersionConstFn = readVersionConst
	build.GitListTagsFn = gitListTags
}

// Build compiles the project binary. If MainPackage is empty, the
// target is skipped.
func (o *Orchestrator) Build() error {
	return build.Build(o.buildConfig())
}

// BuildAll compiles all cmd/ sub-packages to BinaryDir when MainPackage is
// empty. It discovers every cmd/*/main.go package and builds each to
// bin/<name> using go build -o bin/<name> ./cmd/<name>/. If no cmd/
// directory exists the target is skipped. prd003 B1.1.
func (o *Orchestrator) BuildAll() error {
	return build.BuildAll(o.buildConfig())
}

// discoverCmdPackages returns the import paths of all packages under cmd/
// that contain a main.go file, relative to root.
func discoverCmdPackages(root string) ([]string, error) {
	return build.DiscoverCmdPackages(root)
}

// Lint runs golangci-lint on the project.
func (o *Orchestrator) Lint() error {
	return build.Lint()
}

// Install runs go install for the main package. If MainPackage
// is empty, the target is skipped.
func (o *Orchestrator) Install() error {
	return build.Install(o.buildConfig())
}

// Clean removes the build artifact directory.
func (o *Orchestrator) Clean() error {
	return build.Clean(o.cfg.Project.BinaryDir)
}

// DumpMeasurePrompt assembles and prints the measure prompt to stdout.
func (o *Orchestrator) DumpMeasurePrompt() error {
	prompt, err := o.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		return fmt.Errorf("building measure prompt: %w", err)
	}
	fmt.Print(prompt)
	return nil
}

// DumpStitchPrompt assembles and prints the stitch prompt to stdout.
// Uses a placeholder task so the template structure is visible.
func (o *Orchestrator) DumpStitchPrompt() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	prompt, err := o.buildStitchPrompt(stitchTask{
		worktreeDir: cwd,
		id:          "EXAMPLE-001",
		title:       "Example task",
		description: "Placeholder task description for prompt preview.",
		issueType:   "task",
	})
	if err != nil {
		return fmt.Errorf("building stitch prompt: %w", err)
	}
	fmt.Print(prompt)
	return nil
}

// ExtractCredentials reads Claude credentials from the macOS Keychain
// and writes them to SecretsDir/TokenFile.
func (o *Orchestrator) ExtractCredentials() error {
	return build.ExtractCredentials(o.cfg.Claude.SecretsDir, o.cfg.EffectiveTokenFile())
}

// buildConfig returns a build.BuildConfig from the orchestrator's config.
func (o *Orchestrator) buildConfig() build.BuildConfig {
	return build.BuildConfig{
		MainPackage: o.cfg.Project.MainPackage,
		BinaryDir:   o.cfg.Project.BinaryDir,
		BinaryName:  o.cfg.Project.BinaryName,
	}
}
