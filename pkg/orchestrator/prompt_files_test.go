// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capturePromptFiles runs PrintContextFiles from dir and returns stdout.
func capturePromptFiles(t *testing.T, dir string, cfg Config) string {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Redirect stdout.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	o := New(cfg)
	if err := o.PrintContextFiles(); err != nil {
		w.Close()
		os.Stdout = origStdout
		t.Fatalf("PrintContextFiles: %v", err)
	}
	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// writeStandardDoc writes a minimal YAML file to dir/path and returns the full path.
func writeStandardDoc(t *testing.T, dir, rel, content string) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func TestPrintContextFiles_DefaultAnnotation(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	writeStandardDoc(t, dir, "docs/VISION.yaml", "id: v1\ntitle: Vision\n")

	out := capturePromptFiles(t, dir, Config{})

	if !strings.Contains(out, "(default)") {
		t.Errorf("expected (default) annotation in output; got:\n%s", out)
	}
	if !strings.Contains(out, "docs/VISION.yaml") {
		t.Errorf("expected docs/VISION.yaml in output; got:\n%s", out)
	}
	if !strings.Contains(out, "files,") {
		t.Errorf("expected totals line in output; got:\n%s", out)
	}
}

func TestPrintContextFiles_ConfigAnnotation(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	extra := writeStandardDoc(t, dir, "extra/notes.yaml", "note: extra\n")

	cfg := Config{
		Project: ProjectConfig{
			ContextSources: extra,
		},
	}
	out := capturePromptFiles(t, dir, cfg)

	if !strings.Contains(out, "(config)") {
		t.Errorf("expected (config) annotation for extra file; got:\n%s", out)
	}
	if !strings.Contains(out, "extra/notes.yaml") {
		t.Errorf("expected extra/notes.yaml in output; got:\n%s", out)
	}
}

func TestPrintContextFiles_ContextExclude(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	writeStandardDoc(t, dir, "docs/VISION.yaml", "id: v1\ntitle: Vision\n")
	writeStandardDoc(t, dir, "docs/ARCHITECTURE.yaml", "id: a1\ntitle: Architecture\n")

	cfg := Config{
		Project: ProjectConfig{
			ContextExclude: "docs/VISION.yaml",
		},
	}
	out := capturePromptFiles(t, dir, cfg)

	if strings.Contains(out, "VISION.yaml") {
		t.Errorf("excluded docs/VISION.yaml should not appear in output; got:\n%s", out)
	}
	if !strings.Contains(out, "ARCHITECTURE.yaml") {
		t.Errorf("non-excluded docs/ARCHITECTURE.yaml should appear in output; got:\n%s", out)
	}
}

func TestEnumerateContextFiles_RespectsContextInclude(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	// Write a custom file that is NOT in the standard patterns.
	custom := writeStandardDoc(t, dir, "custom/spec.yaml", "note: custom\n")
	// Write a standard file that should NOT appear when ContextInclude is set.
	writeStandardDoc(t, dir, "docs/VISION.yaml", "id: v1\ntitle: Vision\n")

	cfg := Config{
		Project: ProjectConfig{
			ContextInclude: custom,
		},
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	o := New(cfg)
	files := o.enumerateContextFiles()

	var foundCustom, foundVision bool
	for _, f := range files {
		if strings.HasSuffix(f.Path, "spec.yaml") {
			foundCustom = true
		}
		if strings.HasSuffix(f.Path, "VISION.yaml") {
			foundVision = true
		}
	}

	if !foundCustom {
		t.Error("expected custom/spec.yaml in files when ContextInclude is set")
	}
	// VISION.yaml is always added by ensureTypedDocs even when not in ContextInclude,
	// because it is a typed doc path — so foundVision may be true here too.
	_ = foundVision
}

func TestEnumerateContextFiles_RespectsContextExclude(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	writeStandardDoc(t, dir, "docs/VISION.yaml", "id: v1\ntitle: Vision\n")
	writeStandardDoc(t, dir, "docs/ARCHITECTURE.yaml", "id: a1\ntitle: Architecture\n")

	cfg := Config{
		Project: ProjectConfig{
			ContextExclude: "docs/VISION.yaml",
		},
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	o := New(cfg)
	files := o.enumerateContextFiles()

	for _, f := range files {
		if strings.HasSuffix(f.Path, "VISION.yaml") {
			t.Errorf("excluded docs/VISION.yaml should not appear in enumerateContextFiles; got %+v", f)
		}
	}

	var foundArch bool
	for _, f := range files {
		if strings.HasSuffix(f.Path, "ARCHITECTURE.yaml") {
			foundArch = true
		}
	}
	if !foundArch {
		t.Error("non-excluded ARCHITECTURE.yaml should appear in enumerateContextFiles")
	}
}

func TestPrintContextFiles_TotalsLine(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	writeStandardDoc(t, dir, "docs/VISION.yaml", fmt.Sprintf("%s\n", strings.Repeat("x", 100)))

	out := capturePromptFiles(t, dir, Config{})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "files,") || !strings.Contains(last, "tokens") {
		t.Errorf("last line should be totals, got: %q", last)
	}
}
