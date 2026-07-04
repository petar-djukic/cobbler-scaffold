// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaperCommands_Sequence(t *testing.T) {
	got := paperCommands("paper/paper.md")
	want := []paperCommand{
		{binPandoc, []string{"paper/paper.md", "-o", "paper/paper.tex"}},
		{binPdflatex, []string{latexNonstop, "paper/paper.tex"}},
		{binBibtex, []string{"paper/paper"}},
		{binPdflatex, []string{latexNonstop, "paper/paper.tex"}},
		{binPdflatex, []string{latexNonstop, "paper/paper.tex"}},
	}
	if len(got) != len(want) {
		t.Fatalf("paperCommands returned %d steps, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].bin != want[i].bin || strings.Join(got[i].args, " ") != strings.Join(want[i].args, " ") {
			t.Errorf("step %d = %s %v, want %s %v", i, got[i].bin, got[i].args, want[i].bin, want[i].args)
		}
	}
}

func TestCheckPaperToolchain_MissingBinaryNamed(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir on PATH: no binaries resolve
	err := checkPaperToolchain()
	if err == nil {
		t.Fatal("expected an error when the toolchain is absent")
	}
	if !strings.Contains(err.Error(), binPandoc) {
		t.Errorf("error %q should name the missing binary %q", err.Error(), binPandoc)
	}
}

func TestBuildPaperPDF_HardRequiresToolchain(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := BuildPaperPDF("")
	if err == nil {
		t.Fatal("expected BuildPaperPDF to fail without the toolchain")
	}
	if !strings.Contains(err.Error(), "paper:pdf requires") {
		t.Errorf("error %q should explain the toolchain requirement", err.Error())
	}
}

func TestCountPlaceholders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sec.md")
	os.WriteFile(path, []byte("A {{DATA:x.json}} and {{DATA:y.csv}} here.\n"), 0o644)
	if n := countPlaceholders(path); n != 2 {
		t.Errorf("countPlaceholders = %d, want 2", n)
	}
	if n := countPlaceholders(filepath.Join(dir, "missing.md")); n != 0 {
		t.Errorf("countPlaceholders(missing) = %d, want 0", n)
	}
}

func TestReportPaperPlaceholders(t *testing.T) {
	dir := t.TempDir()
	paperDir := filepath.Join(dir, "paper")
	os.MkdirAll(paperDir, 0o755)
	os.WriteFile(filepath.Join(paperDir, "a.md"), []byte("{{DATA:one}} {{DATA:two}}\n"), 0o644)
	os.WriteFile(filepath.Join(paperDir, "b.tex"), []byte("{{DATA:three}}\n"), 0o644)
	os.WriteFile(filepath.Join(paperDir, "notes.txt"), []byte("{{DATA:ignored}}\n"), 0o644)

	if got := paperSourceFiles(paperDir); len(got) != 2 {
		t.Errorf("paperSourceFiles matched %v, want 2 (.md and .tex only)", got)
	}
	if err := ReportPaperPlaceholders(paperDir); err != nil {
		t.Errorf("ReportPaperPlaceholders returned error: %v", err)
	}
}

func TestReportPaperPlaceholders_NoDirIsNoError(t *testing.T) {
	if err := ReportPaperPlaceholders(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Errorf("expected nil for a missing paper dir, got %v", err)
	}
}
