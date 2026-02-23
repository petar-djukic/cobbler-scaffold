// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Build compiles the project binary. If MainPackage is empty, the
// target is skipped.
func (o *Orchestrator) Build() error {
	if o.cfg.Project.MainPackage == "" {
		logf("build: skipping (no main_package configured)")
		return nil
	}
	outPath := filepath.Join(o.cfg.Project.BinaryDir, o.cfg.Project.BinaryName)
	logf("build: go build -o %s %s", outPath, o.cfg.Project.MainPackage)
	if err := os.MkdirAll(o.cfg.Project.BinaryDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	cmd := exec.Command(binGo, "build", "-o", outPath, o.cfg.Project.MainPackage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	logf("build: done")
	return nil
}

// Lint runs golangci-lint on the project.
func (o *Orchestrator) Lint() error {
	logf("lint: running golangci-lint")
	cmd := exec.Command(binLint, "run", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("golangci-lint: %w", err)
	}
	logf("lint: done")
	return nil
}

// TestUnit runs go test on all packages.
func (o *Orchestrator) TestUnit() error {
	logf("test:unit: running go test ./...")
	cmd := exec.Command(binGo, "test", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test: %w", err)
	}
	logf("test:unit: done")
	return nil
}

// TestE2E runs go test in the tests/rel01.0/ directory. If the directory
// does not exist, the target prints a skip message.
func (o *Orchestrator) TestE2E() error {
	if _, err := os.Stat("tests/rel01.0"); os.IsNotExist(err) {
		fmt.Println("No E2E test directory found (tests/rel01.0/)")
		return nil
	}
	logf("test:e2e: running go test -v -count=1 ./tests/rel01.0/...")
	cmd := exec.Command(binGo, "test", "-v", "-count=1", "-timeout", "1800s", "./tests/rel01.0/...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test e2e: %w", err)
	}
	logf("test:e2e: done")
	return nil
}

// TestE2EByUseCase runs E2E tests matching a single use case. The uc
// parameter is the zero-padded use case number (e.g., "001"). Tests
// are selected via -run matching the Test.*_UC{uc} pattern.
func (o *Orchestrator) TestE2EByUseCase(uc string) error {
	if _, err := os.Stat("tests/rel01.0"); os.IsNotExist(err) {
		fmt.Println("No E2E test directory found (tests/rel01.0/)")
		return nil
	}
	pattern := fmt.Sprintf("Test.*_UC%s", uc)
	logf("test:e2e: running go test -v -count=1 -run %s ./tests/rel01.0/...", pattern)
	cmd := exec.Command(binGo, "test", "-v", "-count=1", "-run", pattern, "-timeout", "1800s", "./tests/rel01.0/...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test e2e (UC%s): %w", uc, err)
	}
	logf("test:e2e: done")
	return nil
}

// TestAll runs unit and E2E tests.
func (o *Orchestrator) TestAll() error {
	if err := o.TestUnit(); err != nil {
		return err
	}
	return o.TestE2E()
}

// Install runs go install for the main package. If MainPackage
// is empty, the target is skipped.
func (o *Orchestrator) Install() error {
	if o.cfg.Project.MainPackage == "" {
		logf("install: skipping (no main_package configured)")
		return nil
	}
	logf("install: go install %s", o.cfg.Project.MainPackage)
	cmd := exec.Command(binGo, "install", o.cfg.Project.MainPackage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install: %w", err)
	}
	logf("install: done")
	return nil
}

// Clean removes the build artifact directory.
func (o *Orchestrator) Clean() error {
	logf("clean: removing %s", o.cfg.Project.BinaryDir)
	if err := os.RemoveAll(o.cfg.Project.BinaryDir); err != nil {
		return fmt.Errorf("removing %s: %w", o.cfg.Project.BinaryDir, err)
	}
	logf("clean: done")
	return nil
}

// DumpMeasurePrompt assembles and prints the measure prompt to stdout.
func (o *Orchestrator) DumpMeasurePrompt() error {
	prompt, err := o.buildMeasurePrompt("", "[]", 1, "/dev/null")
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
	outPath := filepath.Join(o.cfg.Claude.SecretsDir, o.cfg.EffectiveTokenFile())
	logf("credentials: extracting to %s", outPath)
	if err := os.MkdirAll(o.cfg.Claude.SecretsDir, 0o700); err != nil {
		return fmt.Errorf("creating secrets directory: %w", err)
	}
	out, err := exec.Command(binSecurity, "find-generic-password",
		"-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return fmt.Errorf("extracting credentials from keychain: %w", err)
	}
	if err := os.WriteFile(outPath, out, 0o600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	logf("credentials: written to %s", outPath)
	return nil
}
