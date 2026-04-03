// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Config ---

func TestOrchestratorConfig_ReturnsConfiguration(t *testing.T) {
	cfg := Config{}
	cfg.Project.ModulePath = "example.com/test"
	cfg.Project.BinaryName = "testbin"
	o := New(cfg)

	got := o.Config()
	if got.Project.ModulePath != "example.com/test" {
		t.Errorf("ModulePath = %q, want %q", got.Project.ModulePath, "example.com/test")
	}
	if got.Project.BinaryName != "testbin" {
		t.Errorf("BinaryName = %q, want %q", got.Project.BinaryName, "testbin")
	}
}

func TestOrchestratorConfig_ReturnsCopy(t *testing.T) {
	o := New(Config{})
	c1 := o.Config()
	c2 := o.Config()
	c1.Project.ModulePath = "mutated"
	if c2.Project.ModulePath == "mutated" {
		t.Error("Config() returned shared reference instead of copy")
	}
}

// --- NewFromFile ---

func TestNewFromFile_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "project:\n  module_path: example.com/proj\n  binary_name: mybin\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	o, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	if o.cfg.Project.ModulePath != "example.com/proj" {
		t.Errorf("ModulePath = %q, want %q", o.cfg.Project.ModulePath, "example.com/proj")
	}
	if o.cfg.Project.BinaryName != "mybin" {
		t.Errorf("BinaryName = %q, want %q", o.cfg.Project.BinaryName, "mybin")
	}
}

func TestNewFromFile_NonexistentPath(t *testing.T) {
	_, err := NewFromFile("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func TestNewFromFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	_, err := NewFromFile(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// --- setGeneration / clearGeneration ---

func TestSetClearGeneration(t *testing.T) {
	o := testOrch()

	o.setGeneration("gen-abc")
	o.phaseMu.RLock()
	got := o.currentGeneration
	o.phaseMu.RUnlock()
	if got != "gen-abc" {
		t.Errorf("currentGeneration = %q, want %q", got, "gen-abc")
	}

	o.clearGeneration()
	o.phaseMu.RLock()
	got = o.currentGeneration
	o.phaseMu.RUnlock()
	if got != "" {
		t.Errorf("currentGeneration = %q, want empty after clearGeneration", got)
	}
}

// --- setPhase / clearPhase ---

func TestSetClearPhase(t *testing.T) {
	o := testOrch()

	o.setPhase("stitch")
	o.phaseMu.RLock()
	phase := o.currentPhase
	start := o.phaseStart
	o.phaseMu.RUnlock()

	if phase != "stitch" {
		t.Errorf("currentPhase = %q, want %q", phase, "stitch")
	}
	if start.IsZero() {
		t.Error("phaseStart is zero after setPhase")
	}

	o.clearPhase()
	o.phaseMu.RLock()
	phase = o.currentPhase
	start = o.phaseStart
	o.phaseMu.RUnlock()

	if phase != "" {
		t.Errorf("currentPhase = %q, want empty after clearPhase", phase)
	}
	if !start.IsZero() {
		t.Error("phaseStart should be zero after clearPhase")
	}
}

// --- openLogSink ---

func TestOpenLogSink_CreatesFile(t *testing.T) {
	o := testOrch()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	t.Cleanup(func() { o.closeLogSink() })

	if err := o.openLogSink(path); err != nil {
		t.Fatalf("openLogSink: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

func TestOpenLogSink_CreatesNestedDirectory(t *testing.T) {
	o := testOrch()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "test.log")

	t.Cleanup(func() { o.closeLogSink() })

	if err := o.openLogSink(path); err != nil {
		t.Fatalf("openLogSink: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not created in nested directory: %v", err)
	}
}

func TestOpenLogSink_InvalidPath(t *testing.T) {
	o := testOrch()
	err := o.openLogSink("/dev/null/impossible/dir/log.txt")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}
