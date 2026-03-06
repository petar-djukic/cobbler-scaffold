// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

// --- InvalidReleases validation ---

func TestCollectAnalyzeResult_InvalidReleases(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Create minimal doc structure so analysis doesn't fail.
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// Road-map with only release 01.0.
	roadmap := `id: test-roadmap
title: Test Roadmap
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        summary: Init
        status: done
`
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)

	// Use case file and PRD so no orphan errors.
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  - id: R1\n    title: Req 1\n"), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: Tests\nrelease: rel01.0\ntraces:\n  - rel01.0-uc001-init\n"), 0o644)

	// Configure releases with one that doesn't exist.
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			Releases: []string{"01.0", "99.0"},
		},
	}}

	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}

	if len(result.InvalidReleases) != 1 {
		t.Fatalf("expected 1 invalid release, got %d: %v", len(result.InvalidReleases), result.InvalidReleases)
	}
	if !strings.Contains(result.InvalidReleases[0], "99.0") {
		t.Errorf("expected invalid release to mention 99.0, got %q", result.InvalidReleases[0])
	}
}

func TestCollectAnalyzeResult_ValidReleases(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	roadmap := `id: test-roadmap
title: Test Roadmap
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        summary: Init
        status: done
`
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  - id: R1\n    title: Req 1\n"), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: Tests\nrelease: rel01.0\ntraces:\n  - rel01.0-uc001-init\n"), 0o644)

	// All configured releases exist in roadmap.
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			Releases: []string{"01.0"},
		},
	}}

	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}

	if len(result.InvalidReleases) != 0 {
		t.Errorf("expected 0 invalid releases, got %d: %v", len(result.InvalidReleases), result.InvalidReleases)
	}
}

// --- PRDsSpanningMultipleReleases ---

func TestCollectAnalyzeResult_PRDsSpanningMultipleReleases_Pass(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// Two use cases in the same release both reference prd001-core → no violation.
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  R1:\n    title: Req 1\n    items:\n      - R1.1: Do X\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-a.yaml",
		[]byte("id: rel01.0-uc001-a\ntitle: A\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc002-b.yaml",
		[]byte("id: rel01.0-uc002-b\ntitle: B\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)
	os.WriteFile("docs/road-map.yaml", []byte("id: rm\ntitle: RM\nreleases: []\n"), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.PRDsSpanningMultipleReleases) != 0 {
		t.Errorf("expected no violations, got %v", result.PRDsSpanningMultipleReleases)
	}
}

func TestCollectAnalyzeResult_PRDsSpanningMultipleReleases_Fail(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// prd003-workflows referenced by rel01.0 and rel03.0 → one violation.
	os.WriteFile("docs/specs/product-requirements/prd003-workflows.yaml",
		[]byte("id: prd003-workflows\ntitle: Workflows\nrequirements:\n  R1:\n    title: Req 1\n    items:\n      - R1.1: Do X\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-measure.yaml",
		[]byte("id: rel01.0-uc001-measure\ntitle: Measure\ntouchpoints:\n  - T1: prd003-workflows R1\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel03.0-uc001-compare.yaml",
		[]byte("id: rel03.0-uc001-compare\ntitle: Compare\ntouchpoints:\n  - T1: prd003-workflows R1\n"), 0o644)
	os.WriteFile("docs/road-map.yaml", []byte("id: rm\ntitle: RM\nreleases: []\n"), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.PRDsSpanningMultipleReleases) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.PRDsSpanningMultipleReleases), result.PRDsSpanningMultipleReleases)
	}
	msg := result.PRDsSpanningMultipleReleases[0]
	if !strings.Contains(msg, "prd003-workflows") {
		t.Errorf("expected message to mention prd003-workflows, got %q", msg)
	}
	if !strings.Contains(msg, "01.0") || !strings.Contains(msg, "03.0") {
		t.Errorf("expected message to mention both releases, got %q", msg)
	}
}

// --- Validate() methods on document structs ---

func TestVisionDoc_Validate_AllPresent(t *testing.T) {
	d := &VisionDoc{
		ID:               "vision-01",
		Title:            "Test Vision",
		ExecutiveSummary: "Summary text",
		Problem:          "Problem text",
		WhatThisDoes:     "What it does",
		WhyWeBuildThis:   "Why we build",
	}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestVisionDoc_Validate_MissingFields(t *testing.T) {
	d := &VisionDoc{ID: "vision-01"}
	errs := d.Validate()
	wantCount := 5 // title, executive_summary, problem, what_this_does, why_we_build_this
	if len(errs) != wantCount {
		t.Fatalf("got %d errors, want %d: %v", len(errs), wantCount, errs)
	}
}

func TestArchitectureDoc_Validate_AllPresent(t *testing.T) {
	d := &ArchitectureDoc{ID: "arch-01", Title: "Architecture"}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestArchitectureDoc_Validate_MissingID(t *testing.T) {
	d := &ArchitectureDoc{Title: "Architecture"}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "id is required" {
		t.Errorf("got %v, want [id is required]", errs)
	}
}

func TestSpecificationsDoc_Validate_AllPresent(t *testing.T) {
	d := &SpecificationsDoc{ID: "spec-01", Title: "Specs", Overview: "Overview text"}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestSpecificationsDoc_Validate_MissingOverview(t *testing.T) {
	d := &SpecificationsDoc{ID: "spec-01", Title: "Specs"}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "overview is required" {
		t.Errorf("got %v, want [overview is required]", errs)
	}
}

func TestRoadmapDoc_Validate_AllPresent(t *testing.T) {
	d := &RoadmapDoc{ID: "rm-01", Title: "Roadmap"}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestRoadmapDoc_Validate_MissingTitle(t *testing.T) {
	d := &RoadmapDoc{ID: "rm-01"}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "title is required" {
		t.Errorf("got %v, want [title is required]", errs)
	}
}

func TestPRDDoc_Validate_AllPresent(t *testing.T) {
	d := &PRDDoc{
		ID:      "prd001-core",
		Title:   "Core",
		Problem: "The problem",
		Requirements: map[string]PRDRequirementGroup{
			"R1": {Title: "Group 1", Items: []map[string]string{{"R1.1": "Do X"}}},
		},
	}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestPRDDoc_Validate_MissingProblem(t *testing.T) {
	d := &PRDDoc{ID: "prd001-core", Title: "Core"}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "problem is required" {
		t.Errorf("got %v, want [problem is required]", errs)
	}
}

func TestPRDDoc_Validate_RequirementGroupMissingTitle(t *testing.T) {
	d := &PRDDoc{
		ID:      "prd001-core",
		Title:   "Core",
		Problem: "The problem",
		Requirements: map[string]PRDRequirementGroup{
			"R1": {Items: []map[string]string{{"R1.1": "Do X"}}},
		},
	}
	errs := d.Validate()
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if errs[0] != "requirements.R1.title is required" {
		t.Errorf("got %q, want %q", errs[0], "requirements.R1.title is required")
	}
}

func TestPRDDoc_Validate_RequirementGroupEmptyItems(t *testing.T) {
	d := &PRDDoc{
		ID:      "prd001-core",
		Title:   "Core",
		Problem: "The problem",
		Requirements: map[string]PRDRequirementGroup{
			"R1": {Title: "Group 1"},
		},
	}
	errs := d.Validate()
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if errs[0] != "requirements.R1.items is required" {
		t.Errorf("got %q, want %q", errs[0], "requirements.R1.items is required")
	}
}

func TestPRDDoc_Validate_ItemIDLetterSuffix_Error(t *testing.T) {
	t.Parallel()
	// R2a, R2b are letter-suffix IDs — not valid; must use R2.1, R2.2 (GH-536).
	d := &PRDDoc{
		ID:      "prd001-core",
		Title:   "Core",
		Problem: "The problem",
		Requirements: map[string]PRDRequirementGroup{
			"R2": {Title: "Group 2", Items: []map[string]string{
				{"R2a": "Do A"},
				{"R2b": "Do B"},
			}},
		},
	}
	errs := d.Validate()
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors for R2a and R2b, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if !contains(e, "numeric dotted format") {
			t.Errorf("error %q should mention numeric dotted format", e)
		}
	}
}

func TestPRDDoc_Validate_ItemIDDotted_Valid(t *testing.T) {
	t.Parallel()
	// R1.1, R2.3 are valid numeric dotted IDs (GH-536).
	d := &PRDDoc{
		ID:      "prd001-core",
		Title:   "Core",
		Problem: "The problem",
		Requirements: map[string]PRDRequirementGroup{
			"R1": {Title: "Group 1", Items: []map[string]string{
				{"R1.1": "Do X"},
				{"R1.2": "Do Y"},
			}},
			"R2": {Title: "Group 2", Items: []map[string]string{
				{"R2.3": "Do Z"},
			}},
		},
	}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors for valid dotted IDs, got: %v", errs)
	}
}

func TestUseCaseDoc_Validate_AllPresent(t *testing.T) {
	d := &UseCaseDoc{
		ID:      "rel01.0-uc001-init",
		Title:   "Init",
		Summary: "Summary text",
		Actor:   "Developer",
		Trigger: "Runs mage init",
	}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestUseCaseDoc_Validate_MissingSummary(t *testing.T) {
	d := &UseCaseDoc{
		ID:      "rel01.0-uc001-init",
		Title:   "Init",
		Actor:   "Developer",
		Trigger: "Runs mage init",
	}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "summary is required" {
		t.Errorf("got %v, want [summary is required]", errs)
	}
}

func TestUseCaseDoc_Validate_MissingMultipleFields(t *testing.T) {
	d := &UseCaseDoc{ID: "rel01.0-uc001-init"}
	errs := d.Validate()
	wantCount := 4 // title, summary, actor, trigger
	if len(errs) != wantCount {
		t.Errorf("got %d errors, want %d: %v", len(errs), wantCount, errs)
	}
}

func TestTestSuiteDoc_Validate_AllPresent(t *testing.T) {
	d := &TestSuiteDoc{ID: "test-rel01.0", Title: "Tests", Release: "01.0"}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestTestSuiteDoc_Validate_MissingRelease(t *testing.T) {
	d := &TestSuiteDoc{ID: "test-rel01.0", Title: "Tests"}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "release is required" {
		t.Errorf("got %v, want [release is required]", errs)
	}
}

func TestEngineeringDoc_Validate_AllPresent(t *testing.T) {
	d := &EngineeringDoc{
		ID:           "eng01-style",
		Title:        "Style Guide",
		Introduction: "Intro text",
		Sections: []DocSection{
			{Title: "Section 1", Content: "Content text"},
		},
	}
	if errs := d.Validate(); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestEngineeringDoc_Validate_MissingIntroduction(t *testing.T) {
	d := &EngineeringDoc{ID: "eng01-style", Title: "Style Guide"}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "introduction is required" {
		t.Errorf("got %v, want [introduction is required]", errs)
	}
}

func TestEngineeringDoc_Validate_SectionMissingContent(t *testing.T) {
	d := &EngineeringDoc{
		ID:           "eng01-style",
		Title:        "Style Guide",
		Introduction: "Intro",
		Sections:     []DocSection{{Title: "Sec 1"}},
	}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "sections[0].content is required" {
		t.Errorf("got %v, want [sections[0].content is required]", errs)
	}
}

func TestEngineeringDoc_Validate_SectionMissingTitle(t *testing.T) {
	d := &EngineeringDoc{
		ID:           "eng01-style",
		Title:        "Style Guide",
		Introduction: "Intro",
		Sections:     []DocSection{{Content: "Content"}},
	}
	errs := d.Validate()
	if len(errs) != 1 || errs[0] != "sections[0].title is required" {
		t.Errorf("got %v, want [sections[0].title is required]", errs)
	}
}

// --- validateYAMLStrict with required-field validation ---

func TestValidateYAMLStrict_UseCaseDoc_MissingSummary(t *testing.T) {
	// A use case missing summary should produce a required-field error.
	content := `id: rel01.0-uc001-init
title: Init
actor: Developer
trigger: Runs mage init
`
	dir := t.TempDir()
	path := filepath.Join(dir, "rel01.0-uc001-init.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	errs := validateYAMLStrict[UseCaseDoc](path)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing summary, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "summary is required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error containing 'summary is required', got %v", errs)
	}
}

func TestValidateYAMLStrict_PRDDoc_MissingProblem(t *testing.T) {
	content := `id: prd001-core
title: Core PRD
`
	dir := t.TempDir()
	path := filepath.Join(dir, "prd001-core.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	errs := validateYAMLStrict[PRDDoc](path)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing problem, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "problem is required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error containing 'problem is required', got %v", errs)
	}
	// Error must include the file path.
	if !strings.Contains(errs[0], path) {
		t.Errorf("expected error to contain file path %q, got %q", path, errs[0])
	}
}

func TestValidateYAMLStrict_EngineeringDoc_SectionMissingContent(t *testing.T) {
	content := `id: eng01-style
title: Style Guide
introduction: Intro text
sections:
  - title: Section 1
    content: ""
`
	dir := t.TempDir()
	path := filepath.Join(dir, "eng01-style.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	errs := validateYAMLStrict[EngineeringDoc](path)
	if len(errs) == 0 {
		t.Fatal("expected errors for empty section content, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e, "sections[0].content is required") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error containing 'sections[0].content is required', got %v", errs)
	}
}

func TestValidateYAMLStrict_DesignDoc_NoRequiredFieldValidation(t *testing.T) {
	// DesignDoc does not implement docValidator so Validate() should not be called.
	// An empty DesignDoc should produce no required-field errors (only unknown-field
	// errors matter for constitution types).
	content := "articles: []\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "design.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	errs := validateYAMLStrict[DesignDoc](path)
	if len(errs) != 0 {
		t.Errorf("DesignDoc should not trigger required-field errors, got %v", errs)
	}
}

func TestCollectAnalyzeResult_EmptyReleasesNoValidation(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	roadmap := `id: test-roadmap
title: Test Roadmap
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        summary: Init
        status: done
`
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  - id: R1\n    title: Req 1\n"), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: Tests\nrelease: rel01.0\ntraces:\n  - rel01.0-uc001-init\n"), 0o644)

	// No releases configured → no validation.
	o := &Orchestrator{cfg: Config{}}

	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}

	if len(result.InvalidReleases) != 0 {
		t.Errorf("expected 0 invalid releases for empty config, got %d", len(result.InvalidReleases))
	}
}

// captureStdout redirects os.Stdout to a pipe, runs fn, and returns the
// captured output. This is used for testing print* functions.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = origStdout

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(out)
}

// --- printSection ---

func TestPrintSection_EmptyItems(t *testing.T) {
	out := captureStdout(t, func() {
		got := printSection("label", nil)
		if got {
			t.Error("printSection returned true for empty items")
		}
	})
	if out != "" {
		t.Errorf("expected no output for empty items, got %q", out)
	}
}

func TestPrintSection_WithItems(t *testing.T) {
	out := captureStdout(t, func() {
		got := printSection("Errors", []string{"err1", "err2"})
		if !got {
			t.Error("printSection returned false for non-empty items")
		}
	})
	if !strings.Contains(out, "Errors") {
		t.Errorf("output missing label, got %q", out)
	}
	if !strings.Contains(out, "  - err1") {
		t.Errorf("output missing item err1, got %q", out)
	}
	if !strings.Contains(out, "  - err2") {
		t.Errorf("output missing item err2, got %q", out)
	}
}

// --- printReport ---

func TestPrintReport_AllClear(t *testing.T) {
	r := AnalyzeResult{}
	out := captureStdout(t, func() {
		err := r.printReport(5, 10, 3, 0)
		if err != nil {
			t.Errorf("expected nil error for clean report, got %v", err)
		}
	})
	if !strings.Contains(out, "All consistency checks passed") {
		t.Errorf("output missing success message, got %q", out)
	}
	if !strings.Contains(out, "5 PRDs") {
		t.Errorf("output missing PRD count, got %q", out)
	}
	if !strings.Contains(out, "10 use cases") {
		t.Errorf("output missing use case count, got %q", out)
	}
	if !strings.Contains(out, "3 test suites") {
		t.Errorf("output missing test suite count, got %q", out)
	}
	if !strings.Contains(out, "0 semantic models") {
		t.Errorf("output missing semantic model count, got %q", out)
	}
}

func TestPrintReport_WithIssues(t *testing.T) {
	r := AnalyzeResult{
		OrphanedPRDs:    []string{"prd099-unused"},
		BrokenCitations: []string{"uc001 T1: prd001 R99 not found"},
	}
	out := captureStdout(t, func() {
		err := r.printReport(2, 3, 1, 0)
		if err == nil {
			t.Error("expected error for report with issues")
		}
		if !strings.Contains(err.Error(), "consistency issues") {
			t.Errorf("error should mention consistency issues, got %v", err)
		}
	})
	if !strings.Contains(out, "Orphaned PRDs") {
		t.Errorf("output missing orphaned PRDs section, got %q", out)
	}
	if !strings.Contains(out, "prd099-unused") {
		t.Errorf("output missing orphaned PRD item, got %q", out)
	}
	if !strings.Contains(out, "Broken citations") {
		t.Errorf("output missing broken citations section, got %q", out)
	}
}

func TestPrintReport_AllSections(t *testing.T) {
	r := AnalyzeResult{
		OrphanedPRDs:                 []string{"a"},
		ReleasesWithoutTestSuites:    []string{"b"},
		OrphanedTestSuites:           []string{"c"},
		BrokenTouchpoints:            []string{"d"},
		UseCasesNotInRoadmap:         []string{"e"},
		SchemaErrors:                 []string{"f"},
		ConstitutionDrift:            []string{"g"},
		BrokenCitations:              []string{"h"},
		InvalidReleases:              []string{"i"},
		PRDsSpanningMultipleReleases: []string{"j"},
	}
	out := captureStdout(t, func() {
		err := r.printReport(1, 1, 1, 0)
		if err == nil {
			t.Error("expected error when all sections have issues")
		}
	})
	// Each section should appear in output.
	for _, want := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		if !strings.Contains(out, "  - "+want) {
			t.Errorf("output missing item %q", want)
		}
	}
	if strings.Contains(out, "All consistency checks passed") {
		t.Error("should not show success message when issues exist")
	}
}

// --- Analyze (end-to-end through collectAnalyzeResult + printReport) ---

func TestAnalyze_WithIssues(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	// PRD with no use cases referencing it → orphaned.
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.WriteFile("docs/road-map.yaml", []byte("releases: []\n"), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd001-orphan.yaml",
		[]byte("id: prd001-orphan\ntitle: Orphan\nrequirements:\n  - id: R1\n    title: Req 1\n"), 0o644)

	o := &Orchestrator{cfg: Config{}}

	out := captureStdout(t, func() {
		err := o.Analyze()
		if err == nil {
			t.Error("expected error for orphaned PRDs")
		}
	})
	if !strings.Contains(out, "Orphaned PRDs") {
		t.Errorf("expected orphaned PRDs section, got:\n%s", out)
	}
}

func TestAnalyze_EmptyDocs(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	// No docs at all — should return an error from collectAnalyzeResult
	// but not panic.
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)

	o := &Orchestrator{cfg: Config{}}
	captureStdout(t, func() {
		// We don't check the error — just verify it runs without panicking.
		// Without a road-map, it can't find releases.
		o.Analyze()
	})
}

// --- OOD Check 10: depends_on violations ---

// setupMinimalOODDir creates the minimal directory structure for OOD tests
// and returns an *Orchestrator. The caller must os.Chdir to dir first.
func setupMinimalOODDir(t *testing.T) {
	t.Helper()
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)
	os.WriteFile("docs/road-map.yaml", []byte("id: rm\ntitle: RM\nreleases: []\n"), 0o644)
}

func TestCollectAnalyzeResult_DependsOnViolation_MissingPRD(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// prd002-cmd depends_on prd001-pkg, which does not exist.
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
depends_on:
  - prd_id: prd001-pkg
    symbols_used:
      - SomeFunc
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.DependsOnViolations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.DependsOnViolations), result.DependsOnViolations)
	}
	if !strings.Contains(result.DependsOnViolations[0], "prd001-pkg") {
		t.Errorf("violation should mention prd001-pkg, got %q", result.DependsOnViolations[0])
	}
}

func TestCollectAnalyzeResult_DependsOnViolation_SymbolMissing(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// prd001-pkg has a package_contract exporting FuncA; prd002-cmd depends on FuncB.
	os.WriteFile("docs/specs/product-requirements/prd001-pkg.yaml", []byte(`id: prd001-pkg
title: Pkg
package_contract:
  exports:
    - name: FuncA
      signature: "func FuncA() error"
`), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
depends_on:
  - prd_id: prd001-pkg
    symbols_used:
      - FuncA
      - FuncB
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.DependsOnViolations) != 1 {
		t.Fatalf("expected 1 violation (FuncB), got %d: %v", len(result.DependsOnViolations), result.DependsOnViolations)
	}
	if !strings.Contains(result.DependsOnViolations[0], "FuncB") {
		t.Errorf("violation should mention FuncB, got %q", result.DependsOnViolations[0])
	}
}

func TestCollectAnalyzeResult_DependsOnViolation_AllSymbolsPresent(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// Both symbols_used are in package_contract — no violation.
	os.WriteFile("docs/specs/product-requirements/prd001-pkg.yaml", []byte(`id: prd001-pkg
title: Pkg
package_contract:
  exports:
    - name: FuncA
    - name: FuncB
`), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
depends_on:
  - prd_id: prd001-pkg
    symbols_used:
      - FuncA
      - FuncB
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.DependsOnViolations) != 0 {
		t.Errorf("expected no violations, got %v", result.DependsOnViolations)
	}
}

// --- OOD Check 11: dependency_rule violations ---

func TestCollectAnalyzeResult_DependencyRuleViolation(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// ARCHITECTURE.yaml: cmd/ must not import cmd/; component_dependency cmd/a -> cmd/b violates this.
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte(`id: arch-test
title: Test Architecture
overview:
  summary: test
  lifecycle: test
  coordination_pattern: test
dependency_rules:
  - description: "cmd/ must not import cmd/"
    from: "cmd/"
    to: "cmd/"
    allowed: false
component_dependencies:
  - from: "cmd/a"
    to: "cmd/b"
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.DependencyRuleViolations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.DependencyRuleViolations), result.DependencyRuleViolations)
	}
	if !strings.Contains(result.DependencyRuleViolations[0], "cmd/a") {
		t.Errorf("violation should mention cmd/a, got %q", result.DependencyRuleViolations[0])
	}
}

func TestCollectAnalyzeResult_DependencyRuleNoViolation(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// cmd/a -> pkg/b is allowed even though cmd/ -> cmd/ is forbidden.
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte(`id: arch-test
title: Test Architecture
overview:
  summary: test
  lifecycle: test
  coordination_pattern: test
dependency_rules:
  - description: "cmd/ must not import cmd/"
    from: "cmd/"
    to: "cmd/"
    allowed: false
component_dependencies:
  - from: "cmd/a"
    to: "pkg/b"
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.DependencyRuleViolations) != 0 {
		t.Errorf("expected no violations, got %v", result.DependencyRuleViolations)
	}
}

// --- OOD Check 12: broken struct_refs ---

func TestCollectAnalyzeResult_BrokenStructRef_MissingPRD(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// prd002 references prd999 which doesn't exist.
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
struct_refs:
  - prd_id: prd999-missing
    requirement: R1
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.BrokenStructRefs) != 1 {
		t.Fatalf("expected 1 broken ref, got %d: %v", len(result.BrokenStructRefs), result.BrokenStructRefs)
	}
	if !strings.Contains(result.BrokenStructRefs[0], "prd999-missing") {
		t.Errorf("broken ref should mention prd999-missing, got %q", result.BrokenStructRefs[0])
	}
}

func TestCollectAnalyzeResult_BrokenStructRef_MissingRequirement(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// prd001 has R1; prd002 struct_refs prd001#R9 which doesn't exist.
	os.WriteFile("docs/specs/product-requirements/prd001-pkg.yaml", []byte(`id: prd001-pkg
title: Pkg
requirements:
  R1:
    title: Req 1
    items:
      - R1.1: Do X
`), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
struct_refs:
  - prd_id: prd001-pkg
    requirement: R9
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.BrokenStructRefs) != 1 {
		t.Fatalf("expected 1 broken ref, got %d: %v", len(result.BrokenStructRefs), result.BrokenStructRefs)
	}
	if !strings.Contains(result.BrokenStructRefs[0], "R9") {
		t.Errorf("broken ref should mention R9, got %q", result.BrokenStructRefs[0])
	}
}

func TestCollectAnalyzeResult_StructRef_Valid(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// prd002 references prd001#R1 which exists — no violation.
	os.WriteFile("docs/specs/product-requirements/prd001-pkg.yaml", []byte(`id: prd001-pkg
title: Pkg
requirements:
  R1:
    title: Req 1
    items:
      - R1.1: Do X
`), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
struct_refs:
  - prd_id: prd001-pkg
    requirement: R1
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.BrokenStructRefs) != 0 {
		t.Errorf("expected no broken refs, got %v", result.BrokenStructRefs)
	}
}

// --- OOD Check 13: component_dependencies gaps ---

func TestCollectAnalyzeResult_ComponentDepViolation_MissingFromArch(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	// prd002 depends_on prd001-pkg; architecture has component_dependencies but
	// "prd001-pkg" doesn't appear in any endpoint — violation.
	os.WriteFile("docs/specs/product-requirements/prd001-pkg.yaml", []byte(`id: prd001-pkg
title: Pkg
`), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
depends_on:
  - prd_id: prd001-pkg
`), 0o644)
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte(`id: arch-test
title: Test Architecture
overview:
  summary: test
  lifecycle: test
  coordination_pattern: test
component_dependencies:
  - from: "cmd/other"
    to: "pkg/other"
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.ComponentDepViolations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.ComponentDepViolations), result.ComponentDepViolations)
	}
	if !strings.Contains(result.ComponentDepViolations[0], "prd001-pkg") {
		t.Errorf("violation should mention prd001-pkg, got %q", result.ComponentDepViolations[0])
	}
}

func TestCollectAnalyzeResult_ComponentDepViolation_NoArchDeps(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// When architecture has no component_dependencies, skip check — no violation.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)

	os.WriteFile("docs/specs/product-requirements/prd001-pkg.yaml", []byte(`id: prd001-pkg
title: Pkg
`), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
depends_on:
  - prd_id: prd001-pkg
`), 0o644)
	// Architecture with no component_dependencies.
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte(`id: arch-test
title: Test Architecture
overview:
  summary: test
  lifecycle: test
  coordination_pattern: test
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if len(result.ComponentDepViolations) != 0 {
		t.Errorf("expected no violations when no component_dependencies, got %v", result.ComponentDepViolations)
	}
}

// ---------------------------------------------------------------------------
// Semantic model validation tests (R1–R6, SM1, SM3, SM7)
// ---------------------------------------------------------------------------

func TestSmValidateSections_AllPresent(t *testing.T) {
	t.Parallel()
	sm := map[string]interface{}{
		"data_sources": []interface{}{},
		"features":     []interface{}{},
		"algorithm":    map[string]interface{}{"type": "gap"},
		"output_format": map[string]interface{}{"type": "list"},
	}
	errs := smValidateSections("test", sm)
	if len(errs) != 0 {
		t.Errorf("expected no errors for full model, got %v", errs)
	}
}

func TestSmValidateSections_MissingSection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		missing string
	}{
		{"data_sources"},
		{"features"},
		{"algorithm"},
		{"output_format"},
	}
	for _, tt := range tests {
		sm := map[string]interface{}{
			"data_sources":  []interface{}{},
			"features":      []interface{}{},
			"algorithm":     map[string]interface{}{},
			"output_format": map[string]interface{}{},
		}
		delete(sm, tt.missing)
		errs := smValidateSections("prefix", sm)
		if len(errs) != 1 {
			t.Errorf("missing %q: expected 1 error, got %d: %v", tt.missing, len(errs), errs)
			continue
		}
		if !strings.Contains(errs[0], tt.missing) {
			t.Errorf("missing %q: error should mention section name, got %q", tt.missing, errs[0])
		}
		if !strings.Contains(errs[0], "SM1") {
			t.Errorf("missing %q: error should mention SM1, got %q", tt.missing, errs[0])
		}
	}
}

func TestSmValidateSM7_ValidNameAndVersion(t *testing.T) {
	t.Parallel()
	sm := map[string]interface{}{
		"name":    "cobbler-measure",
		"version": "1.0.0",
	}
	errs := smValidateSM7("test", sm)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid name/version, got %v", errs)
	}
}

func TestSmValidateSM7_InvalidName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"cobbler", "Cobbler-Measure", "cobbler_measure", ""} {
		sm := map[string]interface{}{"name": name, "version": "1.0.0"}
		errs := smValidateSM7("test", sm)
		if name == "" {
			// empty name: no validation triggered
			if len(errs) != 0 {
				t.Errorf("empty name: expected no error, got %v", errs)
			}
			continue
		}
		if len(errs) != 1 {
			t.Errorf("name %q: expected 1 error, got %d: %v", name, len(errs), errs)
			continue
		}
		if !strings.Contains(errs[0], "SM7") {
			t.Errorf("name %q: error should mention SM7, got %q", name, errs[0])
		}
	}
}

func TestSmValidateSM7_InvalidVersion(t *testing.T) {
	t.Parallel()
	for _, ver := range []string{"1.0", "v1.0.0", "1.0.0.0", "latest"} {
		sm := map[string]interface{}{"name": "edge-compute", "version": ver}
		errs := smValidateSM7("test", sm)
		if len(errs) != 1 {
			t.Errorf("version %q: expected 1 error, got %d: %v", ver, len(errs), errs)
			continue
		}
		if !strings.Contains(errs[0], "SM7") {
			t.Errorf("version %q: error should mention SM7, got %q", ver, errs[0])
		}
	}
}

func TestSmSourceRefs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source string
		want   []string
	}{
		{"specification_tree.all_use_cases - project_state.implemented", []string{"specification_tree", "project_state"}},
		{"implementation_gap", []string{"implementation_gap"}},
		{"codebase.specifications + codebase.existing_code", []string{"codebase"}},
		{"task.issue_description.acceptance_criteria", []string{"task"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := smSourceRefs(tt.source)
		if len(got) != len(tt.want) {
			t.Errorf("source %q: got %v, want %v", tt.source, got, tt.want)
			continue
		}
		for i, w := range tt.want {
			if got[i] != w {
				t.Errorf("source %q: got[%d]=%q, want %q", tt.source, i, got[i], w)
			}
		}
	}
}

func TestSmValidateSM3_ValidTraceability(t *testing.T) {
	t.Parallel()
	sm := map[string]interface{}{
		"data_sources": []interface{}{
			map[string]interface{}{"id": "network_state"},
		},
		"features": []interface{}{
			map[string]interface{}{"name": "gap", "source": "network_state.capacity"},
			map[string]interface{}{"name": "derived", "source": "gap"},
		},
	}
	errs := smValidateSM3("test", sm)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid traceability, got %v", errs)
	}
}

func TestSmValidateSM3_UntetheredFeature(t *testing.T) {
	t.Parallel()
	sm := map[string]interface{}{
		"data_sources": []interface{}{
			map[string]interface{}{"id": "network_state"},
		},
		"features": []interface{}{
			map[string]interface{}{"name": "bad_feature", "source": "unknown_source.capacity"},
		},
	}
	errs := smValidateSM3("test", sm)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for untethered feature, got %d: %v", len(errs), errs)
	}
	if len(errs) > 0 && !strings.Contains(errs[0], "SM3") {
		t.Errorf("error should mention SM3, got %q", errs[0])
	}
}

func TestValidateStandaloneSemanticModel_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/model.yaml"
	os.WriteFile(path, []byte(`measure:
  semantic_model:
    name: cobbler-measure
    version: 1.0.0
    data_sources:
      - id: specification_tree
        connector: filesystem
    features:
      - name: implementation_gap
        source: specification_tree.all_use_cases
        extraction: set difference
    algorithm:
      type: gap_ordered_task_generation
    output_format:
      type: list
`), 0o644)
	errs := validateStandaloneSemanticModel(path)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid standalone model, got %v", errs)
	}
}

func TestValidateStandaloneSemanticModel_MissingAlgorithm(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/model.yaml"
	os.WriteFile(path, []byte(`analyze:
  semantic_model:
    name: cobbler-analyze
    version: 1.0.0
    data_sources:
      - id: repo
        connector: filesystem
    features:
      - name: counts
        source: repo.files
    output_format:
      type: report
`), 0o644)
	errs := validateStandaloneSemanticModel(path)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for missing algorithm, got %d: %v", len(errs), errs)
	}
	if len(errs) > 0 && !strings.Contains(errs[0], "algorithm") {
		t.Errorf("error should mention missing section, got %q", errs[0])
	}
}

func TestValidatePRDSemanticModel_NoSemanticModel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/prd001.yaml"
	os.WriteFile(path, []byte(`id: prd001
title: Test PRD
problem: test
`), 0o644)
	errs := validatePRDSemanticModel(path)
	if len(errs) != 0 {
		t.Errorf("expected no errors for PRD without semantic_model, got %v", errs)
	}
}

func TestValidatePRDSemanticModel_ValidShorthand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/prd001.yaml"
	os.WriteFile(path, []byte(`id: prd001
title: Test PRD
problem: test
semantic_model:
  observe: input data
  reason: apply logic
  produce: output result
`), 0o644)
	errs := validatePRDSemanticModel(path)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid shorthand model, got %v", errs)
	}
}

func TestValidatePRDSemanticModel_MissingShorthandKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/prd001.yaml"
	// Missing "produce" key.
	os.WriteFile(path, []byte(`id: prd001
title: Test PRD
semantic_model:
  observe: input data
  reason: apply logic
`), 0o644)
	errs := validatePRDSemanticModel(path)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for missing produce key, got %d: %v", len(errs), errs)
	}
	if len(errs) > 0 && !strings.Contains(errs[0], "produce") {
		t.Errorf("error should mention missing key, got %q", errs[0])
	}
}

func TestValidatePromptSemanticModel_NoSemanticModel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/measure.yaml"
	os.WriteFile(path, []byte(`role: analyst
task: analyze
`), 0o644)
	errs := validatePromptSemanticModel(path)
	if len(errs) != 0 {
		t.Errorf("expected no errors for prompt without semantic_model, got %v", errs)
	}
}

func TestValidatePromptSemanticModel_MissingSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/measure.yaml"
	// Full format but missing output_format.
	os.WriteFile(path, []byte(`role: analyst
semantic_model:
  data_sources:
    - id: repo
  features:
    - name: gap
  algorithm:
    type: gap_ordered
`), 0o644)
	errs := validatePromptSemanticModel(path)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for missing output_format, got %d: %v", len(errs), errs)
	}
	if len(errs) > 0 && !strings.Contains(errs[0], "output_format") {
		t.Errorf("error should mention missing section, got %q", errs[0])
	}
}

func TestValidateSemanticModels_Count(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/semantic-models", 0o755)
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/prompts", 0o755)

	// Write two valid standalone semantic model files.
	writeValidSMFile := func(name, behavior string) {
		os.WriteFile("docs/specs/semantic-models/"+name, []byte(behavior+`:
  semantic_model:
    name: test-`+behavior+`
    version: 1.0.0
    data_sources:
      - id: src
        connector: filesystem
    features:
      - name: feat
        source: src.query
    algorithm:
      type: simple
    output_format:
      type: list
`), 0o644)
	}
	writeValidSMFile("model-a.yaml", "behave")
	writeValidSMFile("model-b.yaml", "analyze")

	errs, count := validateSemanticModels(nil)
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid files, got %v", errs)
	}
}

func TestCollectAnalyzeResult_SemanticModelErrors(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalOODDir(t)
	os.MkdirAll("docs/specs/semantic-models", 0o755)
	os.MkdirAll("docs/prompts", 0o755)

	// Write a standalone file missing the algorithm section.
	os.WriteFile("docs/specs/semantic-models/bad.yaml", []byte(`analyze:
  semantic_model:
    name: cobbler-analyze
    version: 1.0.0
    data_sources:
      - id: repo
    features:
      - name: counts
        source: repo.files
    output_format:
      type: report
`), 0o644)

	o := &Orchestrator{cfg: Config{}}
	result, counts, err := o.collectAnalyzeResult()
	if err != nil {
		t.Fatalf("collectAnalyzeResult: %v", err)
	}
	if counts.SemanticModels != 1 {
		t.Errorf("expected SemanticModels=1, got %d", counts.SemanticModels)
	}
	if len(result.SemanticModelErrors) == 0 {
		t.Error("expected at least one semantic model error for missing algorithm")
	}
	found := false
	for _, e := range result.SemanticModelErrors {
		if strings.Contains(e, "algorithm") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error mentioning algorithm, got %v", result.SemanticModelErrors)
	}
}
