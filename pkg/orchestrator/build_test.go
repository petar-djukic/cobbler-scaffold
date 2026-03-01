// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
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
