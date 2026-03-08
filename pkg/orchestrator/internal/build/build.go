// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package build implements Go project compilation, linting, installation,
// cleanup, and credential extraction. The parent orchestrator package
// provides thin receiver-method wrappers around these functions.
package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ---------------------------------------------------------------------------
// Injected dependencies
// ---------------------------------------------------------------------------

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// Package-level variables set by the parent package at init time.
var (
	Log         Logger = func(string, ...any) {}
	BinGo              = "go"
	BinLint            = "golangci-lint"
	BinSecurity        = "security"
)

// ---------------------------------------------------------------------------
// Build / BuildAll / Lint / Install / Clean
// ---------------------------------------------------------------------------

// BuildConfig holds the project configuration fields needed by Build,
// BuildAll, Install, and Clean.
type BuildConfig struct {
	MainPackage string
	BinaryDir   string
	BinaryName  string
}

// Build compiles the project binary. If MainPackage is empty, the
// target is skipped.
func Build(cfg BuildConfig) error {
	if cfg.MainPackage == "" {
		Log("build: skipping (no main_package configured)")
		return nil
	}
	outPath := filepath.Join(cfg.BinaryDir, cfg.BinaryName)
	Log("build: go build -o %s %s", outPath, cfg.MainPackage)
	if err := os.MkdirAll(cfg.BinaryDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	cmd := exec.Command(BinGo, "build", "-o", outPath, cfg.MainPackage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	Log("build: done")
	return nil
}

// BuildAll compiles all cmd/ sub-packages to BinaryDir when MainPackage
// is empty. It discovers every cmd/*/main.go package and builds each to
// bin/<name>. If MainPackage is set, it delegates to Build. If no cmd/
// directory exists the target is skipped.
func BuildAll(cfg BuildConfig) error {
	if cfg.MainPackage != "" {
		return Build(cfg)
	}

	pkgs, err := DiscoverCmdPackages(".")
	if err != nil {
		return fmt.Errorf("discovering cmd packages: %w", err)
	}
	if len(pkgs) == 0 {
		Log("build:all: no cmd/ packages found, skipping")
		return nil
	}

	if err := os.MkdirAll(cfg.BinaryDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	for _, pkg := range pkgs {
		name := filepath.Base(pkg)
		outPath := filepath.Join(cfg.BinaryDir, name)
		Log("build:all: go build -o %s %s", outPath, pkg)
		cmd := exec.Command(BinGo, "build", "-o", outPath, pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build %s: %w", pkg, err)
		}
	}

	Log("build:all: built %d package(s) to %s", len(pkgs), cfg.BinaryDir)
	return nil
}

// DiscoverCmdPackages returns the import paths of all packages under cmd/
// that contain a main.go file, relative to root.
func DiscoverCmdPackages(root string) ([]string, error) {
	cmdDir := filepath.Join(root, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cmd/: %w", err)
	}

	var pkgs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mainGo := filepath.Join(cmdDir, e.Name(), "main.go")
		if _, err := os.Stat(mainGo); err == nil {
			pkgs = append(pkgs, "./cmd/"+e.Name()+"/")
		}
	}
	return pkgs, nil
}

// Lint runs golangci-lint on the project.
func Lint() error {
	Log("lint: running golangci-lint")
	cmd := exec.Command(BinLint, "run", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("golangci-lint: %w", err)
	}
	Log("lint: done")
	return nil
}

// Install runs go install for the main package. If MainPackage is empty,
// the target is skipped.
func Install(cfg BuildConfig) error {
	if cfg.MainPackage == "" {
		Log("install: skipping (no main_package configured)")
		return nil
	}
	Log("install: go install %s", cfg.MainPackage)
	cmd := exec.Command(BinGo, "install", cfg.MainPackage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install: %w", err)
	}
	Log("install: done")
	return nil
}

// Clean removes the build artifact directory.
func Clean(binaryDir string) error {
	Log("clean: removing %s", binaryDir)
	if err := os.RemoveAll(binaryDir); err != nil {
		return fmt.Errorf("removing %s: %w", binaryDir, err)
	}
	Log("clean: done")
	return nil
}

// ---------------------------------------------------------------------------
// Credentials
// ---------------------------------------------------------------------------

// ExtractCredentials reads Claude credentials from the macOS Keychain
// and writes them to the given secretsDir/tokenFile path.
func ExtractCredentials(secretsDir, tokenFile string) error {
	outPath := filepath.Join(secretsDir, tokenFile)
	Log("credentials: extracting to %s", outPath)
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return fmt.Errorf("creating secrets directory: %w", err)
	}
	out, err := exec.Command(BinSecurity, "find-generic-password",
		"-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return fmt.Errorf("extracting credentials from keychain: %w", err)
	}
	if err := os.WriteFile(outPath, out, 0o600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	Log("credentials: written to %s", outPath)
	return nil
}

