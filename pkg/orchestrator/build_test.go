// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Build ---

func TestBuild_SkipsWhenNoMainPackage(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			MainPackage: "",
			BinaryDir:   t.TempDir(),
			BinaryName:  "mybin",
		},
	}}
	if err := o.Build(); err != nil {
		t.Errorf("Build() with empty MainPackage should not error, got: %v", err)
	}
}

func TestBuild_CreatesBinaryDir(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin", "nested")

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			MainPackage: "nonexistent/package/that/will/fail",
			BinaryDir:   binDir,
			BinaryName:  "mybin",
		},
	}}

	// Build will fail because the package doesn't exist, but the directory
	// should have been created before the go build attempt.
	_ = o.Build()

	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		t.Error("Build() should create binary directory even on build failure")
	}
}

// --- Install ---

func TestInstall_SkipsWhenNoMainPackage(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			MainPackage: "",
		},
	}}
	if err := o.Install(); err != nil {
		t.Errorf("Install() with empty MainPackage should not error, got: %v", err)
	}
}

func TestInstall_ErrorsWhenGoInstallFails(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			MainPackage: "nonexistent/package/that/will/fail",
		},
	}}
	if err := o.Install(); err == nil {
		t.Error("Install() with nonexistent package should return error")
	}
}

// --- BuildAll ---

func TestBuildAll_SkipsWhenNoCmdDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{BinaryDir: filepath.Join(dir, "bin")},
	}}
	if err := o.BuildAll(); err != nil {
		t.Errorf("BuildAll() with no cmd/ should not error, got: %v", err)
	}
	// bin/ should not be created when there are no packages.
	if _, err := os.Stat(filepath.Join(dir, "bin")); !os.IsNotExist(err) {
		t.Error("BuildAll() should not create bin/ when no cmd/ packages exist")
	}
}

func TestBuildAll_DelegatesToBuildWhenMainPackageSet(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			MainPackage: "nonexistent/pkg",
			BinaryDir:   t.TempDir(),
			BinaryName:  "mybin",
		},
	}}
	// Should attempt Build() and fail because package doesn't exist.
	// Key is it does NOT fall through to multi-cmd path.
	err := o.BuildAll()
	if err == nil {
		t.Error("BuildAll() with nonexistent MainPackage should fail")
	}
	if !strings.Contains(err.Error(), "go build") {
		t.Errorf("error = %q, want to contain 'go build'", err.Error())
	}
}

func TestDiscoverCmdPackages_NoCmdDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	pkgs, err := discoverCmdPackages(dir)
	if err != nil {
		t.Fatalf("discoverCmdPackages error = %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("pkgs = %v, want empty", pkgs)
	}
}

func TestDiscoverCmdPackages_FindsMainPackages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create cmd/foo/main.go and cmd/bar/main.go; cmd/notmain/ has no main.go.
	for _, path := range []string{
		"cmd/foo/main.go",
		"cmd/bar/main.go",
	} {
		full := filepath.Join(dir, path)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("package main\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "cmd/notmain"), 0o755)

	pkgs, err := discoverCmdPackages(dir)
	if err != nil {
		t.Fatalf("discoverCmdPackages error = %v", err)
	}
	if len(pkgs) != 2 {
		t.Errorf("pkgs = %v, want 2 entries", pkgs)
	}
	for _, p := range pkgs {
		if !strings.HasPrefix(p, "./cmd/") || !strings.HasSuffix(p, "/") {
			t.Errorf("pkg %q should be of form ./cmd/<name>/", p)
		}
	}
}

// --- Clean ---

func TestClean_RemovesBinaryDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0o755)
	os.WriteFile(filepath.Join(binDir, "mybin"), []byte("binary"), 0o755)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			BinaryDir: binDir,
		},
	}}

	if err := o.Clean(); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	if _, err := os.Stat(binDir); !os.IsNotExist(err) {
		t.Error("Clean() should have removed binary directory")
	}
}

func TestClean_NonExistentDir(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			BinaryDir: "/nonexistent/dir/that/does/not/exist/build_test",
		},
	}}
	if err := o.Clean(); err != nil {
		t.Errorf("Clean() on nonexistent dir should not error, got: %v", err)
	}
}

func TestClean_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0o755)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			BinaryDir: binDir,
		},
	}}

	if err := o.Clean(); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	if _, err := os.Stat(binDir); !os.IsNotExist(err) {
		t.Error("Clean() should have removed empty binary directory")
	}
}

// --- DumpMeasurePrompt / DumpStitchPrompt ---

func TestDumpMeasurePrompt_ReturnsError_WhenPromptBuildFails(t *testing.T) {
	// DumpMeasurePrompt calls buildMeasurePrompt which requires a valid prompt template.
	// With an invalid template it should return an error, not panic.
	cfg := Config{}
	cfg.Cobbler.MeasurePrompt = "role: [unclosed bracket"
	o := New(cfg)

	err := o.DumpMeasurePrompt()
	if err == nil {
		t.Error("DumpMeasurePrompt() expected error for invalid template, got nil")
	}
}

func TestDumpStitchPrompt_ProducesOutput(t *testing.T) {
	// DumpStitchPrompt should succeed with default embedded templates.
	o := New(Config{})

	// Redirect stdout so the test doesn't pollute output.
	oldStdout := os.Stdout
	null, _ := os.Open(os.DevNull)
	os.Stdout = null
	defer func() {
		os.Stdout = oldStdout
		null.Close()
	}()

	err := o.DumpStitchPrompt()
	if err != nil {
		t.Errorf("DumpStitchPrompt() unexpected error: %v", err)
	}
}
