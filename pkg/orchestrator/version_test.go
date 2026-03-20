// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	rel "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/release"
)

func TestReadVersionConst_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "version.go")
	os.WriteFile(path, []byte(`package main

const Version = "v0.20260225.1"
`), 0o644)

	got := rel.ReadVersionConst(path)
	if got != "v0.20260225.1" {
		t.Errorf("rel.ReadVersionConst() = %q, want %q", got, "v0.20260225.1")
	}
}

func TestReadVersionConst_MissingFile(t *testing.T) {
	t.Parallel()
	got := rel.ReadVersionConst("/nonexistent/version.go")
	if got != "" {
		t.Errorf("rel.ReadVersionConst() = %q, want empty string for missing file", got)
	}
}

func TestReadVersionConst_NoVersionConst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "version.go")
	os.WriteFile(path, []byte(`package main

const AppName = "myapp"
`), 0o644)

	got := rel.ReadVersionConst(path)
	if got != "" {
		t.Errorf("rel.ReadVersionConst() = %q, want empty string when no Version const", got)
	}
}

func TestWriteVersionConst_ValidReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "version.go")
	os.WriteFile(path, []byte(`package main

const Version = "v0.20260225.0"
`), 0o644)

	err := rel.WriteVersionConst(path, "v0.20260226.1")
	if err != nil {
		t.Fatalf("writeVersionConst: %v", err)
	}

	got := rel.ReadVersionConst(path)
	if got != "v0.20260226.1" {
		t.Errorf("after write, rel.ReadVersionConst() = %q, want %q", got, "v0.20260226.1")
	}
}

func TestWriteVersionConst_MissingFile(t *testing.T) {
	t.Parallel()
	err := rel.WriteVersionConst("/nonexistent/version.go", "v1.0.0")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestWriteVersionConst_NoVersionConst(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "version.go")
	os.WriteFile(path, []byte(`package main

const AppName = "myapp"
`), 0o644)

	err := rel.WriteVersionConst(path, "v1.0.0")
	if err == nil {
		t.Error("expected error when no Version const, got nil")
	}
}
