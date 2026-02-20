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

// TestIntegration runs go test in the tests/ directory. If the
// directory does not exist, the target prints a skip message.
func (o *Orchestrator) TestIntegration() error {
	if _, err := os.Stat("tests"); os.IsNotExist(err) {
		fmt.Println("No integration test directory found")
		return nil
	}
	logf("test:integration: running go test ./tests/...")
	cmd := exec.Command(binGo, "test", "./tests/...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test integration: %w", err)
	}
	logf("test:integration: done")
	return nil
}

// TestAll runs unit and integration tests.
func (o *Orchestrator) TestAll() error {
	if err := o.TestUnit(); err != nil {
		return err
	}
	return o.TestIntegration()
}

// TestE2E runs the E2E test suite in tests/e2e/ against real target
// repositories with Claude enabled (E2E_CLAUDE=1). If the directory does
// not exist, the target prints a skip message and returns nil.
func (o *Orchestrator) TestE2E() error {
	if _, err := os.Stat("tests/e2e"); os.IsNotExist(err) {
		fmt.Println("No E2E test directory found (tests/e2e/)")
		return nil
	}
	logf("test:e2e: running go test -v -timeout 1800s ./tests/e2e/...")
	cmd := exec.Command(binGo, "test", "-v", "-timeout", "1800s", "./tests/e2e/...")
	cmd.Env = append(os.Environ(), "E2E_CLAUDE=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("e2e tests failed: %w", err)
	}
	return nil
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
