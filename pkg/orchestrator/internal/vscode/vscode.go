// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package vscode handles VS Code extension packaging and installation.
// prd: prd006-vscode-extension R10
package vscode

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// Binary names for VS Code extension tooling.
const (
	BinNpm  = "npm"
	BinCode = "code"
)

// ExtensionDir is the directory containing the VS Code extension source,
// relative to the orchestrator repository root.
const ExtensionDir = "vscode-extension"

// ExtensionID is the publisher-qualified extension identifier used by
// the code CLI for install and uninstall operations.
const ExtensionID = "mesh-intelligence.mage-orchestrator"

// Push compiles the VS Code extension from source, packages it as a
// .vsix archive, and installs it into VS Code. When profile is non-empty the
// extension is installed into that VS Code profile via --profile. It verifies
// that npm and the code CLI are available before proceeding.
// prd: prd006-vscode-extension R10.6, R10.7
func Push(profile string, log Logger) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("vscode:push: getting working directory: %w", err)
	}
	extDir := filepath.Join(root, ExtensionDir)

	// Verify required tools.
	if _, err := exec.LookPath(BinNpm); err != nil {
		return fmt.Errorf("vscode:push: npm is not installed or not on PATH; install Node.js from https://nodejs.org")
	}
	if _, err := exec.LookPath(BinCode); err != nil {
		return fmt.Errorf("vscode:push: VS Code CLI (code) is not installed or not on PATH; open VS Code and run 'Shell Command: Install code command in PATH'")
	}

	// Step 1: npm install.
	log("vscode:push: installing dependencies")
	installCmd := exec.Command(BinNpm, "install")
	installCmd.Dir = extDir
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("vscode:push: npm install failed: %w", err)
	}

	// Step 2: compile TypeScript.
	log("vscode:push: compiling TypeScript")
	compileCmd := exec.Command(BinNpm, "run", "compile")
	compileCmd.Dir = extDir
	compileCmd.Stdout = os.Stdout
	compileCmd.Stderr = os.Stderr
	if err := compileCmd.Run(); err != nil {
		return fmt.Errorf("vscode:push: TypeScript compilation failed: %w", err)
	}

	// Step 3: determine .vsix filename from package.json.
	vsixName, err := VsixFilename(extDir)
	if err != nil {
		return fmt.Errorf("vscode:push: %w", err)
	}

	// Step 4: package as .vsix.
	// Use the local node_modules/.bin/vsce binary directly rather than npx so
	// that we are guaranteed to use the version installed by npm install above
	// and avoid any npx resolution ambiguity when Dir differs from os.Getwd().
	log("vscode:push: packaging extension as %s", vsixName)
	vscebin := filepath.Join(extDir, "node_modules", ".bin", "vsce")
	packageCmd := exec.Command(vscebin, "package", "--allow-missing-repository")
	packageCmd.Dir = extDir
	packageCmd.Stdout = os.Stdout
	packageCmd.Stderr = os.Stderr
	if err := packageCmd.Run(); err != nil {
		return fmt.Errorf("vscode:push: vsce package failed: %w", err)
	}

	// Verify the .vsix file was produced before attempting install.
	vsixPath := filepath.Join(extDir, vsixName)
	if _, err := os.Stat(vsixPath); err != nil {
		return fmt.Errorf("vscode:push: expected %s but file not found after packaging", vsixPath)
	}

	// Step 5: install the extension.
	codeArgs := CodeInstallArgs(vsixPath, profile)
	log("vscode:push: installing extension from %s", vsixPath)
	if profile != "" {
		log("vscode:push: targeting profile %q", profile)
	} else {
		log("vscode:push: installing to default profile (use 'mage vscode:push <profile>' to target a specific profile)")
	}
	codeCmd := exec.Command(BinCode, codeArgs...)
	codeCmd.Stdout = os.Stdout
	codeCmd.Stderr = os.Stderr
	if err := codeCmd.Run(); err != nil {
		return fmt.Errorf("vscode:push: code --install-extension failed: %w", err)
	}

	log("vscode:push: done")
	return nil
}

// CodeInstallArgs builds the argument list for code --install-extension,
// optionally adding --profile when profile is non-empty.
func CodeInstallArgs(vsixPath, profile string) []string {
	args := []string{"--install-extension", vsixPath}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return args
}

// CodeUninstallArgs builds the argument list for code --uninstall-extension,
// optionally adding --profile when profile is non-empty.
func CodeUninstallArgs(extensionID, profile string) []string {
	args := []string{"--uninstall-extension", extensionID}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return args
}

// CodeListArgs builds the argument list for code --list-extensions,
// optionally adding --profile when profile is non-empty.
func CodeListArgs(profile string) []string {
	args := []string{"--list-extensions"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return args
}

// Pop uninstalls the VS Code extension. When profile is non-empty the
// extension is removed from that VS Code profile. The operation is idempotent:
// it succeeds even if the extension is not currently installed.
// prd: prd006-vscode-extension R10.6
func Pop(profile string, log Logger) error {
	if _, err := exec.LookPath(BinCode); err != nil {
		return fmt.Errorf("vscode:pop: VS Code CLI (code) is not installed or not on PATH; open VS Code and run 'Shell Command: Install code command in PATH'")
	}

	log("vscode:pop: uninstalling extension %s", ExtensionID)
	if profile != "" {
		log("vscode:pop: targeting profile %q", profile)
	}
	cmd := exec.Command(BinCode, CodeUninstallArgs(ExtensionID, profile)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// The code CLI exits non-zero when the extension is not installed.
		// Check if it is actually installed before treating this as an error.
		listOut, listErr := exec.Command(BinCode, CodeListArgs(profile)...).Output()
		if listErr == nil && slices.Contains(SplitLines(string(listOut)), ExtensionID) {
			return fmt.Errorf("vscode:pop: uninstall failed: %w", err)
		}
		log("vscode:pop: extension was not installed (nothing to do)")
		return nil
	}

	log("vscode:pop: done")
	return nil
}

// PackageJSON holds the fields we need from a VS Code extension package.json.
type PackageJSON struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// VsixFilename reads package.json in extDir and returns the expected .vsix
// filename: <name>-<version>.vsix.
func VsixFilename(extDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(extDir, "package.json"))
	if err != nil {
		return "", fmt.Errorf("reading package.json: %w", err)
	}
	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", fmt.Errorf("parsing package.json: %w", err)
	}
	if pkg.Name == "" || pkg.Version == "" {
		return "", fmt.Errorf("package.json missing name or version field")
	}
	return pkg.Name + "-" + pkg.Version + ".vsix", nil
}

// SplitLines splits s into non-empty trimmed lines.
func SplitLines(s string) []string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
