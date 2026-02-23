//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRel01_UC007_Build verifies that mage build compiles the binary successfully.
func TestRel01_UC007_Build(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "bin"))
	if err != nil {
		t.Fatalf("reading bin/: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one binary in bin/ after mage build")
	}
}

// TestRel01_UC007_Install verifies that mage install exits 0.
func TestRel01_UC007_Install(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "install"); err != nil {
		t.Fatalf("mage install: %v", err)
	}
}

// TestRel01_UC007_Clean verifies that mage clean removes build artifacts.
func TestRel01_UC007_Clean(t *testing.T) {
	dir := setupRepo(t)
	if err := runMage(t, dir, "build"); err != nil {
		t.Fatalf("mage build (setup): %v", err)
	}
	if err := runMage(t, dir, "clean"); err != nil {
		t.Fatalf("mage clean: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "bin"))
	if len(entries) > 0 {
		t.Errorf("expected bin/ to be empty after mage clean, found %v", entries)
	}
}

// TestRel01_UC007_Stats verifies that mage stats exits 0 and prints go_loc.
func TestRel01_UC007_Stats(t *testing.T) {
	dir := setupRepo(t)
	out, err := runMageOut(t, dir, "stats")
	if err != nil {
		t.Fatalf("mage stats: %v", err)
	}
	if !strings.Contains(out, "go_loc") {
		t.Errorf("expected 'go_loc' in mage stats output, got:\n%s", out)
	}
}
