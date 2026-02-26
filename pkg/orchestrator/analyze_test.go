// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// --- extractID ---

func TestExtractID(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"docs/specs/product-requirements/prd001-feature.yaml", "prd001-feature"},
		{"docs/specs/use-cases/rel01.0-uc001-init.yaml", "rel01.0-uc001-init"},
		{"docs/specs/test-suites/test-rel01.0.yaml", "test-rel01.0"},
		{"simple.yaml", "simple"},
	}
	for _, tc := range cases {
		if got := extractID(tc.path); got != tc.want {
			t.Errorf("extractID(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// --- extractPRDsFromTouchpoints ---

func TestExtractPRDsFromTouchpoints(t *testing.T) {
	tps := []string{
		"T1: Calculator component (prd001-core R1, R2)",
		"T2: Parser subsystem (prd002-parser)",
		"T3: No PRD reference here",
	}
	got := extractPRDsFromTouchpoints(tps)
	want := map[string]bool{"prd001-core": true, "prd002-parser": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected PRD ID %q", id)
		}
	}
}

func TestExtractPRDsFromTouchpoints_Empty(t *testing.T) {
	got := extractPRDsFromTouchpoints(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestExtractPRDsFromTouchpoints_NoPRDs(t *testing.T) {
	tps := []string{"T1: Some component", "T2: Another component"}
	got := extractPRDsFromTouchpoints(tps)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- extractUseCaseIDsFromTraces ---

func TestExtractUseCaseIDsFromTraces(t *testing.T) {
	traces := []string{
		"rel01.0-uc001-init",
		"rel01.0-uc002-lifecycle",
		"prd001-core R4",
	}
	got := extractUseCaseIDsFromTraces(traces)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 use case IDs", got)
	}
	want := map[string]bool{"rel01.0-uc001-init": true, "rel01.0-uc002-lifecycle": true}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected use case ID %q", id)
		}
	}
}

func TestExtractUseCaseIDsFromTraces_Empty(t *testing.T) {
	got := extractUseCaseIDsFromTraces(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- loadUseCase ---

func TestLoadUseCase_ParsesIDAndTouchpoints(t *testing.T) {
	content := `id: rel01.0-uc001-init
title: Initialization
touchpoints:
  - T1: Core component (prd001-core R1)
  - T2: Config subsystem
`
	dir := t.TempDir()
	path := filepath.Join(dir, "rel01.0-uc001-init.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	uc, err := loadUseCase(path)
	if err != nil {
		t.Fatalf("loadUseCase: %v", err)
	}
	if uc.ID != "rel01.0-uc001-init" {
		t.Errorf("ID: got %q, want %q", uc.ID, "rel01.0-uc001-init")
	}
	if len(uc.Touchpoints) != 2 {
		t.Errorf("Touchpoints: got %d, want 2", len(uc.Touchpoints))
	}
}

func TestLoadUseCase_MissingFile(t *testing.T) {
	_, err := loadUseCase("/nonexistent/uc.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// --- loadTestSuite ---

func TestLoadTestSuite_ParsesIDAndTraces(t *testing.T) {
	content := `id: test-rel01.0
title: Release 01.0 Tests
release: rel01.0
traces:
  - rel01.0-uc001-init
  - rel01.0-uc002-lifecycle
test_cases:
  - name: Init smoke test
    inputs:
      command: mage init
    expected:
      exit_code: 0
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test-rel01.0.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := loadTestSuite(path)
	if err != nil {
		t.Fatalf("loadTestSuite: %v", err)
	}
	if ts.ID != "test-rel01.0" {
		t.Errorf("ID: got %q, want %q", ts.ID, "test-rel01.0")
	}
	if len(ts.Traces) != 2 {
		t.Errorf("Traces: got %d, want 2", len(ts.Traces))
	}
	if ts.Traces[0] != "rel01.0-uc001-init" {
		t.Errorf("Traces[0]: got %q, want %q", ts.Traces[0], "rel01.0-uc001-init")
	}
}

func TestLoadTestSuite_MissingFile(t *testing.T) {
	_, err := loadTestSuite("/nonexistent/test.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// --- extractReqGroup ---

func TestExtractReqGroup(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"R1", "R1"},
		{"R2.1", "R2"},
		{"R9.1-R9.4", "R9"},
		{"R12", "R12"},
		{"R1,", "R1"},
		{"nope", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractReqGroup(tc.input); got != tc.want {
			t.Errorf("extractReqGroup(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- extractCitationsFromTouchpoints ---

func TestExtractCitationsFromTouchpoints_SinglePRD(t *testing.T) {
	tps := []string{"T1: GeneratorStart: prd002-lifecycle R2"}
	got := extractCitationsFromTouchpoints(tps)
	if len(got) != 1 {
		t.Fatalf("got %d citations, want 1", len(got))
	}
	if got[0].PRDID != "prd002-lifecycle" {
		t.Errorf("PRDID: got %q, want %q", got[0].PRDID, "prd002-lifecycle")
	}
	if len(got[0].Groups) != 1 || got[0].Groups[0] != "R2" {
		t.Errorf("Groups: got %v, want [R2]", got[0].Groups)
	}
}

func TestExtractCitationsFromTouchpoints_MultiplePRDs(t *testing.T) {
	tps := []string{"T1: Config: prd001-core R1, prd003-workflows R1, R2"}
	got := extractCitationsFromTouchpoints(tps)
	if len(got) != 2 {
		t.Fatalf("got %d citations, want 2", len(got))
	}
	if got[0].PRDID != "prd001-core" || len(got[0].Groups) != 1 {
		t.Errorf("citation[0]: got %+v, want prd001-core [R1]", got[0])
	}
	if got[1].PRDID != "prd003-workflows" || len(got[1].Groups) != 2 {
		t.Errorf("citation[1]: got %+v, want prd003-workflows [R1, R2]", got[1])
	}
}

func TestExtractCitationsFromTouchpoints_SubItems(t *testing.T) {
	tps := []string{"T2: Git tags: prd006-vscode R2.2, prd002-lifecycle R1.2"}
	got := extractCitationsFromTouchpoints(tps)
	if len(got) != 2 {
		t.Fatalf("got %d citations, want 2", len(got))
	}
	// R2.2 should extract group R2.
	if got[0].Groups[0] != "R2" {
		t.Errorf("citation[0] group: got %q, want R2", got[0].Groups[0])
	}
	if got[1].Groups[0] != "R1" {
		t.Errorf("citation[1] group: got %q, want R1", got[1].Groups[0])
	}
}

func TestExtractCitationsFromTouchpoints_Parenthetical(t *testing.T) {
	tps := []string{"T1: Start: prd002-lifecycle R2 (including R2.8 base branch)"}
	got := extractCitationsFromTouchpoints(tps)
	if len(got) != 1 {
		t.Fatalf("got %d citations, want 1", len(got))
	}
	// R2 and R2.8 both map to group R2, so only one entry after dedup.
	if len(got[0].Groups) != 1 || got[0].Groups[0] != "R2" {
		t.Errorf("Groups: got %v, want [R2]", got[0].Groups)
	}
}

func TestExtractCitationsFromTouchpoints_Empty(t *testing.T) {
	got := extractCitationsFromTouchpoints(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestExtractCitationsFromTouchpoints_NoPRD(t *testing.T) {
	tps := []string{"T1: Some component with no PRD reference"}
	got := extractCitationsFromTouchpoints(tps)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- detectConstitutionDrift ---

func TestDetectConstitutionDrift_Matching(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs", "constitutions")
	embeddedDir := filepath.Join(dir, "pkg", "orchestrator", "constitutions")
	os.MkdirAll(docsDir, 0o755)
	os.MkdirAll(embeddedDir, 0o755)

	content := []byte("articles:\n  - id: T1\n    title: Test\n    rule: test\n")
	os.WriteFile(filepath.Join(docsDir, "testing.yaml"), content, 0o644)
	os.WriteFile(filepath.Join(embeddedDir, "testing.yaml"), content, 0o644)

	// Run from the temp dir so relative paths resolve.
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := detectConstitutionDrift()
	if len(got) != 0 {
		t.Errorf("got %v, want no drift", got)
	}
}

func TestDetectConstitutionDrift_Differs(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs", "constitutions")
	embeddedDir := filepath.Join(dir, "pkg", "orchestrator", "constitutions")
	os.MkdirAll(docsDir, 0o755)
	os.MkdirAll(embeddedDir, 0o755)

	os.WriteFile(filepath.Join(docsDir, "design.yaml"), []byte("version: 2\n"), 0o644)
	os.WriteFile(filepath.Join(embeddedDir, "design.yaml"), []byte("version: 1\n"), 0o644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := detectConstitutionDrift()
	if len(got) != 1 || got[0] != "design.yaml" {
		t.Errorf("got %v, want [design.yaml]", got)
	}
}

func TestDetectConstitutionDrift_OnlyInDocs(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs", "constitutions")
	embeddedDir := filepath.Join(dir, "pkg", "orchestrator", "constitutions")
	os.MkdirAll(docsDir, 0o755)
	os.MkdirAll(embeddedDir, 0o755)

	// File only in docs/ is not drift (no embedded copy to compare).
	os.WriteFile(filepath.Join(docsDir, "extra.yaml"), []byte("data: true\n"), 0o644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := detectConstitutionDrift()
	if len(got) != 0 {
		t.Errorf("got %v, want no drift", got)
	}
}

// --- validateYAMLStrict with constitution structs ---

func TestValidateYAMLStrict_TestingDoc_Valid(t *testing.T) {
	content := "articles:\n  - id: T1\n    title: Test\n    rule: some rule\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "testing.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	errs := validateYAMLStrict[TestingDoc](path)
	if len(errs) != 0 {
		t.Errorf("got errors: %v", errs)
	}
}

func TestValidateYAMLStrict_TestingDoc_UnknownField(t *testing.T) {
	content := "articles:\n  - id: T1\n    title: Test\n    rule: ok\nextra_field: bad\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "testing.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	errs := validateYAMLStrict[TestingDoc](path)
	if len(errs) == 0 {
		t.Error("expected error for unknown field, got none")
	}
}

func TestValidateYAMLStrict_MissingFile(t *testing.T) {
	errs := validateYAMLStrict[TestingDoc]("/nonexistent/file.yaml")
	if len(errs) != 0 {
		t.Errorf("expected nil for missing file, got %v", errs)
	}
}
