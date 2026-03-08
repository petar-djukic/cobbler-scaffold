// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package vscode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVsixFilename_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `{"name": "mage-orchestrator", "version": "0.0.1"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := VsixFilename(dir)
	if err != nil {
		t.Fatalf("VsixFilename: unexpected error: %v", err)
	}
	want := "mage-orchestrator-0.0.1.vsix"
	if got != want {
		t.Errorf("VsixFilename = %q, want %q", got, want)
	}
}

func TestVsixFilename_MissingPackageJSON(t *testing.T) {
	dir := t.TempDir()
	_, err := VsixFilename(dir)
	if err == nil {
		t.Fatal("VsixFilename: expected error for missing package.json, got nil")
	}
}

func TestVsixFilename_EmptyFields(t *testing.T) {
	dir := t.TempDir()
	content := `{"name": "", "version": "1.0.0"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := VsixFilename(dir)
	if err == nil {
		t.Fatal("VsixFilename: expected error for empty name, got nil")
	}
}

func TestVsixFilename_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := VsixFilename(dir)
	if err == nil {
		t.Fatal("VsixFilename: expected error for invalid JSON, got nil")
	}
}

func TestCodeInstallArgs_WithProfile(t *testing.T) {
	got := CodeInstallArgs("/path/to/ext.vsix", "GO")
	want := []string{"--install-extension", "/path/to/ext.vsix", "--profile", "GO"}
	if len(got) != len(want) {
		t.Fatalf("CodeInstallArgs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CodeInstallArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodeInstallArgs_NoProfile(t *testing.T) {
	got := CodeInstallArgs("/path/to/ext.vsix", "")
	want := []string{"--install-extension", "/path/to/ext.vsix"}
	if len(got) != len(want) {
		t.Fatalf("CodeInstallArgs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CodeInstallArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodeUninstallArgs_WithProfile(t *testing.T) {
	got := CodeUninstallArgs("publisher.ext", "Work")
	want := []string{"--uninstall-extension", "publisher.ext", "--profile", "Work"}
	if len(got) != len(want) {
		t.Fatalf("CodeUninstallArgs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CodeUninstallArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodeUninstallArgs_NoProfile(t *testing.T) {
	got := CodeUninstallArgs("publisher.ext", "")
	want := []string{"--uninstall-extension", "publisher.ext"}
	if len(got) != len(want) {
		t.Fatalf("CodeUninstallArgs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CodeUninstallArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodeListArgs_WithProfile(t *testing.T) {
	got := CodeListArgs("GO")
	want := []string{"--list-extensions", "--profile", "GO"}
	if len(got) != len(want) {
		t.Fatalf("CodeListArgs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CodeListArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodeListArgs_NoProfile(t *testing.T) {
	got := CodeListArgs("")
	want := []string{"--list-extensions"}
	if len(got) != len(want) {
		t.Fatalf("CodeListArgs: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CodeListArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitLines_MultiLine(t *testing.T) {
	got := SplitLines("alpha\nbeta\ngamma")
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("SplitLines: got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SplitLines[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitLines_EmptyString(t *testing.T) {
	got := SplitLines("")
	if len(got) != 0 {
		t.Errorf("SplitLines: got %d lines, want 0", len(got))
	}
}

func TestSplitLines_SkipsBlanks(t *testing.T) {
	got := SplitLines("alpha\n\n  \nbeta\n")
	want := []string{"alpha", "beta"}
	if len(got) != len(want) {
		t.Fatalf("SplitLines: got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SplitLines[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitLines_TrailingNewline(t *testing.T) {
	got := SplitLines("one\ntwo\n")
	want := []string{"one", "two"}
	if len(got) != len(want) {
		t.Fatalf("SplitLines: got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SplitLines[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
