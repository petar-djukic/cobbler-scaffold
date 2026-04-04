// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
)

// --- totalIssues ---

func TestTotalIssues_Zero(t *testing.T) {
	doc := AnalysisDoc{}
	if got := doc.TotalIssues(); got != 0 {
		t.Errorf("totalIssues() = %d, want 0", got)
	}
}

func TestTotalIssues_ConsistencyOnly(t *testing.T) {
	doc := AnalysisDoc{ConsistencyErrors: 3}
	if got := doc.TotalIssues(); got != 3 {
		t.Errorf("totalIssues() = %d, want 3", got)
	}
}

func TestTotalIssues_GapsOnly(t *testing.T) {
	doc := AnalysisDoc{
		CodeStatus: &CodeStatusReport{
			Gaps: []string{"gap1", "gap2"},
		},
	}
	if got := doc.TotalIssues(); got != 2 {
		t.Errorf("totalIssues() = %d, want 2", got)
	}
}

func TestTotalIssues_Combined(t *testing.T) {
	doc := AnalysisDoc{
		ConsistencyErrors: 5,
		CodeStatus: &CodeStatusReport{
			Gaps: []string{"gap1", "gap2", "gap3"},
		},
	}
	if got := doc.TotalIssues(); got != 8 {
		t.Errorf("totalIssues() = %d, want 8", got)
	}
}

// --- collectConsistencyDetails ---

func TestCollectConsistencyDetails_Empty(t *testing.T) {
	r := &AnalyzeResult{}
	details := an.CollectConsistencyDetails(r)
	if len(details) != 0 {
		t.Errorf("got %d details, want 0", len(details))
	}
}

func TestCollectConsistencyDetails_AllFields(t *testing.T) {
	r := &AnalyzeResult{
		OrphanedSRDs:              []string{"srd-orphan"},
		ReleasesWithoutTestSuites: []string{"rel01.0"},
		OrphanedTestSuites:        []string{"test-rel99.0"},
		BrokenTouchpoints:         []string{"uc001->srd-missing"},
		UseCasesNotInRoadmap:      []string{"rel01.0-uc099"},
		SchemaErrors:              []string{"bad-field.yaml"},   // excluded from details
		ConstitutionDrift:         []string{"design.yaml"},      // excluded from details
		BrokenCitations:           []string{"uc001->srd001:R99"},
	}
	details := an.CollectConsistencyDetails(r)

	// SchemaErrors and ConstitutionDrift are excluded (srd003 R11.2).
	if len(details) != 6 {
		t.Fatalf("got %d details, want 6", len(details))
	}

	// Verify prefixes to ensure correct categorization.
	prefixes := []string{
		"orphaned SRD:",
		"release without test suite:",
		"orphaned test suite:",
		"broken touchpoint:",
		"use case not in roadmap:",
		"broken citation:",
	}
	for i, prefix := range prefixes {
		if !strings.HasPrefix(details[i], prefix) {
			t.Errorf("details[%d] = %q, want prefix %q", i, details[i], prefix)
		}
	}
}

func TestCollectConsistencyDetails_MultiplePerField(t *testing.T) {
	r := &AnalyzeResult{
		OrphanedSRDs:    []string{"srd-a", "srd-b"},
		SchemaErrors:    []string{"err1", "err2", "err3"}, // excluded from details
		BrokenCitations: []string{"cite1"},
	}
	details := an.CollectConsistencyDetails(r)
	// SchemaErrors excluded; 2 orphaned + 1 citation = 3 (srd003 R11.2).
	if len(details) != 3 {
		t.Errorf("got %d details, want 3", len(details))
	}
}

// --- collectDefects ---

func TestCollectDefects_Empty(t *testing.T) {
	r := &AnalyzeResult{}
	defects := an.CollectDefects(r)
	if len(defects) != 0 {
		t.Errorf("got %d defects, want 0", len(defects))
	}
}

func TestCollectDefects_SchemaAndDrift(t *testing.T) {
	r := &AnalyzeResult{
		SchemaErrors:      []string{"docs/VISION.yaml: type mismatch at line 31"},
		ConstitutionDrift: []string{"design.yaml"},
		OrphanedSRDs:      []string{"srd-x"}, // must NOT appear in defects
	}
	defects := an.CollectDefects(r)

	if len(defects) != 2 {
		t.Fatalf("got %d defects, want 2", len(defects))
	}
	if !strings.HasPrefix(defects[0], "schema error: ") {
		t.Errorf("defects[0] = %q, want prefix 'schema error: '", defects[0])
	}
	if !strings.HasPrefix(defects[1], "constitution drift: ") {
		t.Errorf("defects[1] = %q, want prefix 'constitution drift: '", defects[1])
	}
}

func TestCollectDefects_ExcludedFromConsistencyDetails(t *testing.T) {
	// Schema errors and constitution drift must not appear in ConsistencyDetails.
	r := &AnalyzeResult{
		SchemaErrors:      []string{"docs/VISION.yaml: err"},
		ConstitutionDrift: []string{"design.yaml"},
	}
	details := an.CollectConsistencyDetails(r)
	defects := an.CollectDefects(r)

	if len(details) != 0 {
		t.Errorf("ConsistencyDetails should be empty, got %v", details)
	}
	if len(defects) != 2 {
		t.Errorf("Defects should have 2 entries, got %d", len(defects))
	}
}

func TestAnalysisDocDefectsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analysis.yaml")

	doc := &AnalysisDoc{
		ConsistencyErrors: 1,
		ConsistencyDetails: []string{"orphaned SRD: srd-x"},
		Defects:           []string{"schema error: docs/VISION.yaml: bad field"},
	}

	if err := an.WriteAnalysisDoc(doc, path); err != nil {
		t.Fatalf("writeAnalysisDoc: %v", err)
	}

	loaded := an.LoadAnalysisDoc(dir)
	if loaded == nil {
		t.Fatal("loadAnalysisDoc returned nil")
	}
	if len(loaded.Defects) != 1 {
		t.Errorf("Defects len = %d, want 1", len(loaded.Defects))
	}
	if loaded.Defects[0] != doc.Defects[0] {
		t.Errorf("Defects[0] = %q, want %q", loaded.Defects[0], doc.Defects[0])
	}
	if len(loaded.ConsistencyDetails) != 1 {
		t.Errorf("ConsistencyDetails len = %d, want 1", len(loaded.ConsistencyDetails))
	}
}

// --- writeAnalysisDoc / loadAnalysisDoc ---

func TestWriteAndLoadAnalysisDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analysis.yaml")

	doc := &AnalysisDoc{
		ConsistencyErrors:  2,
		ConsistencyDetails: []string{"orphaned SRD: srd-x", "schema error: bad.yaml"},
		CodeStatus: &CodeStatusReport{
			Releases: []ReleaseCodeStatus{{
				Version:       "01.0",
				Name:          "Core",
				SpecStatus:    "done",
				CodeReadiness: "partial",
			}},
			Gaps: []string{"release 01.0: spec done but code partial"},
		},
	}

	if err := an.WriteAnalysisDoc(doc, path); err != nil {
		t.Fatalf("writeAnalysisDoc: %v", err)
	}

	// Verify file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Load it back.
	loaded := an.LoadAnalysisDoc(dir)
	if loaded == nil {
		t.Fatal("loadAnalysisDoc returned nil")
	}
	if loaded.ConsistencyErrors != 2 {
		t.Errorf("ConsistencyErrors = %d, want 2", loaded.ConsistencyErrors)
	}
	if len(loaded.ConsistencyDetails) != 2 {
		t.Errorf("ConsistencyDetails len = %d, want 2", len(loaded.ConsistencyDetails))
	}
	if loaded.CodeStatus == nil {
		t.Fatal("CodeStatus is nil")
	}
	if len(loaded.CodeStatus.Gaps) != 1 {
		t.Errorf("Gaps len = %d, want 1", len(loaded.CodeStatus.Gaps))
	}
}

func TestWriteAnalysisDoc_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub", "dir", "analysis.yaml")

	doc := &AnalysisDoc{ConsistencyErrors: 1}
	if err := an.WriteAnalysisDoc(doc, nested); err != nil {
		t.Fatalf("writeAnalysisDoc: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("file not created in nested directory: %v", err)
	}
}

func TestWriteAnalysisDoc_EmptyDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "analysis.yaml")

	doc := &AnalysisDoc{}
	if err := an.WriteAnalysisDoc(doc, path); err != nil {
		t.Fatalf("writeAnalysisDoc: %v", err)
	}

	loaded := an.LoadAnalysisDoc(dir)
	if loaded == nil {
		t.Fatal("loadAnalysisDoc returned nil")
	}
	if loaded.ConsistencyErrors != 0 {
		t.Errorf("ConsistencyErrors = %d, want 0", loaded.ConsistencyErrors)
	}
	if loaded.CodeStatus != nil {
		t.Error("CodeStatus should be nil for empty doc")
	}
}

func TestLoadAnalysisDoc_NoFile(t *testing.T) {
	dir := t.TempDir()
	loaded := an.LoadAnalysisDoc(dir)
	if loaded != nil {
		t.Errorf("expected nil for missing file, got %+v", loaded)
	}
}

func TestLoadAnalysisDoc_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, an.AnalysisFileName)
	os.WriteFile(path, []byte("{{invalid yaml"), 0o644)

	loaded := an.LoadAnalysisDoc(dir)
	if loaded != nil {
		t.Errorf("expected nil for invalid YAML, got %+v", loaded)
	}
}

// --- RunPreCycleAnalysis ---

func TestRunPreCycleAnalysis_WritesFile(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	// Minimal doc set.
	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.WriteFile("docs/road-map.yaml", []byte("releases:\n  - id: rel01.0\n    use_cases:\n      - id: rel01.0-uc001-init\n        summary: Init\n        status: done\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: srd001-core R1\n"), 0o644)
	os.WriteFile("docs/specs/software-requirements/srd001-core.yaml",
		[]byte("id: srd001-core\ntitle: Core\nrequirements:\n  - id: R1\n    title: Req 1\n"), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: Tests\nrelease: rel01.0\ntraces:\n  - rel01.0-uc001-init\n"), 0o644)

	scratchDir := filepath.Join(dir, ".cobbler")
	o := testOrchWithCfg(Config{Cobbler: CobblerConfig{Dir: scratchDir}})
	o.Analyzer.RunPreCycleAnalysis()

	outPath := filepath.Join(scratchDir, an.AnalysisFileName)
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Fatalf("expected %s to exist after RunPreCycleAnalysis", an.AnalysisFileName)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "consistency_errors") {
		t.Errorf("output missing consistency_errors field, got:\n%s", content)
	}
}

func TestRunPreCycleAnalysis_NoRoadmap(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	// No docs at all — collectAnalyzeResult will fail but RunPreCycleAnalysis
	// should not panic.
	scratchDir := filepath.Join(dir, ".cobbler")
	o := testOrchWithCfg(Config{Cobbler: CobblerConfig{Dir: scratchDir}})
	o.Analyzer.RunPreCycleAnalysis()

	// Should still write a file even if analysis had errors.
	outPath := filepath.Join(scratchDir, an.AnalysisFileName)
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Fatalf("expected %s even with empty docs", an.AnalysisFileName)
	}
}
