// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ExtractID ---

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
		if got := ExtractID(tc.path); got != tc.want {
			t.Errorf("ExtractID(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// --- ExtractPRDsFromTouchpoints ---

func TestExtractPRDsFromTouchpoints(t *testing.T) {
	tps := []string{
		"T1: Calculator component (prd001-core R1, R2)",
		"T2: Parser subsystem (prd002-parser)",
		"T3: No PRD reference here",
	}
	got := ExtractPRDsFromTouchpoints(tps)
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
	got := ExtractPRDsFromTouchpoints(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestExtractPRDsFromTouchpoints_NoPRDs(t *testing.T) {
	tps := []string{"T1: Some component", "T2: Another component"}
	got := ExtractPRDsFromTouchpoints(tps)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- ExtractUseCaseIDsFromTraces ---

func TestExtractUseCaseIDsFromTraces(t *testing.T) {
	traces := []string{
		"rel01.0-uc001-init",
		"rel01.0-uc002-lifecycle",
		"prd001-core R4",
	}
	got := ExtractUseCaseIDsFromTraces(traces)
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
	got := ExtractUseCaseIDsFromTraces(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- LoadUseCase ---

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
	uc, err := LoadUseCase(path)
	if err != nil {
		t.Fatalf("LoadUseCase: %v", err)
	}
	if uc.ID != "rel01.0-uc001-init" {
		t.Errorf("ID: got %q, want %q", uc.ID, "rel01.0-uc001-init")
	}
	if len(uc.Touchpoints) != 2 {
		t.Errorf("Touchpoints: got %d, want 2", len(uc.Touchpoints))
	}
}

func TestLoadUseCase_MissingFile(t *testing.T) {
	_, err := LoadUseCase("/nonexistent/uc.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// --- LoadTestSuite ---

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
	ts, err := LoadTestSuite(path)
	if err != nil {
		t.Fatalf("LoadTestSuite: %v", err)
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
	_, err := LoadTestSuite("/nonexistent/test.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// --- ExtractReqGroup ---

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
		if got := ExtractReqGroup(tc.input); got != tc.want {
			t.Errorf("ExtractReqGroup(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- ExtractCitationsFromTouchpoints ---

func TestExtractCitationsFromTouchpoints_SinglePRD(t *testing.T) {
	tps := []string{"T1: GeneratorStart: prd002-lifecycle R2"}
	got := ExtractCitationsFromTouchpoints(tps)
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
	got := ExtractCitationsFromTouchpoints(tps)
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
	got := ExtractCitationsFromTouchpoints(tps)
	if len(got) != 2 {
		t.Fatalf("got %d citations, want 2", len(got))
	}
	if got[0].Groups[0] != "R2" {
		t.Errorf("citation[0] group: got %q, want R2", got[0].Groups[0])
	}
	if got[1].Groups[0] != "R1" {
		t.Errorf("citation[1] group: got %q, want R1", got[1].Groups[0])
	}
}

func TestExtractCitationsFromTouchpoints_Parenthetical(t *testing.T) {
	tps := []string{"T1: Start: prd002-lifecycle R2 (including R2.8 base branch)"}
	got := ExtractCitationsFromTouchpoints(tps)
	if len(got) != 1 {
		t.Fatalf("got %d citations, want 1", len(got))
	}
	if len(got[0].Groups) != 1 || got[0].Groups[0] != "R2" {
		t.Errorf("Groups: got %v, want [R2]", got[0].Groups)
	}
}

func TestExtractCitationsFromTouchpoints_Empty(t *testing.T) {
	got := ExtractCitationsFromTouchpoints(nil)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestExtractCitationsFromTouchpoints_NoPRD(t *testing.T) {
	tps := []string{"T1: Some component with no PRD reference"}
	got := ExtractCitationsFromTouchpoints(tps)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// --- ExtractFileRelease ---

func TestExtractFileRelease(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"docs/specs/use-cases/rel01.0-uc003-measure-workflow.yaml", "01.0"},
		{"docs/specs/use-cases/rel02.0-uc001-init.yaml", "02.0"},
		{"docs/specs/use-cases/something-else.yaml", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ExtractFileRelease(tc.path); got != tc.want {
			t.Errorf("ExtractFileRelease(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// --- DetectConstitutionDrift ---

func TestDetectConstitutionDrift_Matching(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs", "constitutions")
	embeddedDir := filepath.Join(dir, "pkg", "orchestrator", "constitutions")
	os.MkdirAll(docsDir, 0o755)
	os.MkdirAll(embeddedDir, 0o755)

	content := []byte("articles:\n  - id: T1\n    title: Test\n    rule: test\n")
	os.WriteFile(filepath.Join(docsDir, "testing.yaml"), content, 0o644)
	os.WriteFile(filepath.Join(embeddedDir, "testing.yaml"), content, 0o644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	noop := func(string, ...any) {}
	got := DetectConstitutionDrift(noop)
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

	noop := func(string, ...any) {}
	got := DetectConstitutionDrift(noop)
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

	os.WriteFile(filepath.Join(docsDir, "extra.yaml"), []byte("data: true\n"), 0o644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	noop := func(string, ...any) {}
	got := DetectConstitutionDrift(noop)
	if len(got) != 0 {
		t.Errorf("got %v, want no drift", got)
	}
}

// --- CollectAnalyzeResult ---

func setupMinimalAnalyzeDir(t *testing.T) {
	t.Helper()
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)
	os.WriteFile("docs/road-map.yaml", []byte("id: rm\ntitle: RM\nreleases: []\n"), 0o644)
}

func noopDeps() AnalyzeDeps {
	return AnalyzeDeps{
		Log:                    func(string, ...any) {},
		ValidateDocSchemas:     func() []string { return nil },
		ValidatePromptTemplate: func(string) []string { return nil },
	}
}

func TestCollectAnalyzeResult_InvalidReleases(t *testing.T) {
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

	deps := noopDeps()
	deps.Releases = []string{"01.0", "99.0"}

	result, _, err := CollectAnalyzeResult(deps)
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
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

	deps := noopDeps()
	deps.Releases = []string{"01.0"}

	result, _, err := CollectAnalyzeResult(deps)
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}

	if len(result.InvalidReleases) != 0 {
		t.Errorf("expected 0 invalid releases, got %d: %v", len(result.InvalidReleases), result.InvalidReleases)
	}
}

func TestCollectAnalyzeResult_PRDsSpanningMultipleReleases_Pass(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  R1:\n    title: Req 1\n    items:\n      - R1.1: Do X\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-a.yaml",
		[]byte("id: rel01.0-uc001-a\ntitle: A\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc002-b.yaml",
		[]byte("id: rel01.0-uc002-b\ntitle: B\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
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
	setupMinimalAnalyzeDir(t)

	os.WriteFile("docs/specs/product-requirements/prd003-workflows.yaml",
		[]byte("id: prd003-workflows\ntitle: Workflows\nrequirements:\n  R1:\n    title: Req 1\n    items:\n      - R1.1: Do X\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-measure.yaml",
		[]byte("id: rel01.0-uc001-measure\ntitle: Measure\ntouchpoints:\n  - T1: prd003-workflows R1\n"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel03.0-uc001-compare.yaml",
		[]byte("id: rel03.0-uc001-compare\ntitle: Compare\ntouchpoints:\n  - T1: prd003-workflows R1\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}

	if len(result.InvalidReleases) != 0 {
		t.Errorf("expected 0 invalid releases for empty config, got %d", len(result.InvalidReleases))
	}
}

// captureStdout redirects os.Stdout to a pipe, runs fn, and returns the
// captured output.
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

// --- PrintSection ---

func TestPrintSection_EmptyItems(t *testing.T) {
	out := captureStdout(t, func() {
		got := PrintSection("label", nil)
		if got {
			t.Error("PrintSection returned true for empty items")
		}
	})
	if out != "" {
		t.Errorf("expected no output for empty items, got %q", out)
	}
}

func TestPrintSection_WithItems(t *testing.T) {
	out := captureStdout(t, func() {
		got := PrintSection("Errors", []string{"err1", "err2"})
		if !got {
			t.Error("PrintSection returned false for non-empty items")
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

// --- PrintReport ---

func TestPrintReport_AllClear(t *testing.T) {
	r := AnalyzeResult{}
	out := captureStdout(t, func() {
		err := r.PrintReport(5, 10, 3, 0)
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
		err := r.PrintReport(2, 3, 1, 0)
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
		err := r.PrintReport(1, 1, 1, 0)
		if err == nil {
			t.Error("expected error when all sections have issues")
		}
	})
	for _, want := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		if !strings.Contains(out, "  - "+want) {
			t.Errorf("output missing item %q", want)
		}
	}
	if strings.Contains(out, "All consistency checks passed") {
		t.Error("should not show success message when issues exist")
	}
}

// --- Analyze (end-to-end) ---

func TestAnalyze_WithIssues(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.WriteFile("docs/road-map.yaml", []byte("releases: []\n"), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd001-orphan.yaml",
		[]byte("id: prd001-orphan\ntitle: Orphan\nrequirements:\n  - id: R1\n    title: Req 1\n"), 0o644)

	out := captureStdout(t, func() {
		err := Analyze(noopDeps())
		if err == nil {
			t.Error("expected error for orphaned PRDs")
		}
	})
	if !strings.Contains(out, "Orphaned PRDs") {
		t.Errorf("expected orphaned PRDs section, got:\n%s", out)
	}
}

func TestAnalyze_EmptyDocs(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)

	captureStdout(t, func() {
		Analyze(noopDeps())
	})
}

// --- OOD Check 10: depends_on violations ---

func TestCollectAnalyzeResult_DependsOnViolation_MissingPRD(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
depends_on:
  - prd_id: prd001-pkg
    symbols_used:
      - SomeFunc
`), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.DependsOnViolations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.DependsOnViolations), result.DependsOnViolations)
	}
	if !strings.Contains(result.DependsOnViolations[0], "prd001-pkg") {
		t.Errorf("violation should mention prd001-pkg, got %q", result.DependsOnViolations[0])
	}
}

func TestCollectAnalyzeResult_DependsOnViolation_SymbolMissing(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.DependsOnViolations) != 1 {
		t.Fatalf("expected 1 violation (FuncB), got %d: %v", len(result.DependsOnViolations), result.DependsOnViolations)
	}
	if !strings.Contains(result.DependsOnViolations[0], "FuncB") {
		t.Errorf("violation should mention FuncB, got %q", result.DependsOnViolations[0])
	}
}

func TestCollectAnalyzeResult_DependsOnViolation_AllSymbolsPresent(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.DependsOnViolations) != 0 {
		t.Errorf("expected no violations, got %v", result.DependsOnViolations)
	}
}

// --- OOD Check 11: dependency_rule violations ---

func TestCollectAnalyzeResult_DependencyRuleViolation(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.DependencyRuleViolations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.DependencyRuleViolations), result.DependencyRuleViolations)
	}
	if !strings.Contains(result.DependencyRuleViolations[0], "cmd/a") {
		t.Errorf("violation should mention cmd/a, got %q", result.DependencyRuleViolations[0])
	}
}

func TestCollectAnalyzeResult_DependencyRuleNoViolation(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.DependencyRuleViolations) != 0 {
		t.Errorf("expected no violations, got %v", result.DependencyRuleViolations)
	}
}

// --- OOD Check 12: broken struct_refs ---

func TestCollectAnalyzeResult_BrokenStructRef_MissingPRD(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	os.WriteFile("docs/specs/product-requirements/prd002-cmd.yaml", []byte(`id: prd002-cmd
title: Cmd
struct_refs:
  - prd_id: prd999-missing
    requirement: R1
`), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.BrokenStructRefs) != 1 {
		t.Fatalf("expected 1 broken ref, got %d: %v", len(result.BrokenStructRefs), result.BrokenStructRefs)
	}
	if !strings.Contains(result.BrokenStructRefs[0], "prd999-missing") {
		t.Errorf("broken ref should mention prd999-missing, got %q", result.BrokenStructRefs[0])
	}
}

func TestCollectAnalyzeResult_BrokenStructRef_MissingRequirement(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.BrokenStructRefs) != 1 {
		t.Fatalf("expected 1 broken ref, got %d: %v", len(result.BrokenStructRefs), result.BrokenStructRefs)
	}
	if !strings.Contains(result.BrokenStructRefs[0], "R9") {
		t.Errorf("broken ref should mention R9, got %q", result.BrokenStructRefs[0])
	}
}

func TestCollectAnalyzeResult_StructRef_Valid(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.BrokenStructRefs) != 0 {
		t.Errorf("expected no broken refs, got %v", result.BrokenStructRefs)
	}
}

// --- OOD Check 13: component_dependencies gaps ---

func TestCollectAnalyzeResult_ComponentDepViolation_MissingFromArch(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.ComponentDepViolations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(result.ComponentDepViolations), result.ComponentDepViolations)
	}
	if !strings.Contains(result.ComponentDepViolations[0], "prd001-pkg") {
		t.Errorf("violation should mention prd001-pkg, got %q", result.ComponentDepViolations[0])
	}
}

func TestCollectAnalyzeResult_ComponentDepViolation_NoArchDeps(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

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
`), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.ComponentDepViolations) != 0 {
		t.Errorf("expected no violations when no component_dependencies, got %v", result.ComponentDepViolations)
	}
}

// --- Semantic model validation ---

func TestSmValidateSections_AllPresent(t *testing.T) {
	t.Parallel()
	sm := map[string]interface{}{
		"data_sources":  []interface{}{},
		"features":      []interface{}{},
		"algorithm":     map[string]interface{}{"type": "gap"},
		"output_format": map[string]interface{}{"type": "list"},
	}
	errs := SmValidateSections("test", sm)
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
		errs := SmValidateSections("prefix", sm)
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
	errs := SmValidateSM7("test", sm)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid name/version, got %v", errs)
	}
}

func TestSmValidateSM7_InvalidName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"cobbler", "Cobbler-Measure", "cobbler_measure", ""} {
		sm := map[string]interface{}{"name": name, "version": "1.0.0"}
		errs := SmValidateSM7("test", sm)
		if name == "" {
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
		errs := SmValidateSM7("test", sm)
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
		got := SmSourceRefs(tt.source)
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
	errs := SmValidateSM3("test", sm)
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
	errs := SmValidateSM3("test", sm)
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
	errs := ValidateStandaloneSemanticModel(path)
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
	errs := ValidateStandaloneSemanticModel(path)
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
	errs := ValidatePRDSemanticModel(path)
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
	errs := ValidatePRDSemanticModel(path)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid shorthand model, got %v", errs)
	}
}

func TestValidatePRDSemanticModel_MissingShorthandKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/prd001.yaml"
	os.WriteFile(path, []byte(`id: prd001
title: Test PRD
semantic_model:
  observe: input data
  reason: apply logic
`), 0o644)
	errs := ValidatePRDSemanticModel(path)
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
	errs := ValidatePromptSemanticModel(path)
	if len(errs) != 0 {
		t.Errorf("expected no errors for prompt without semantic_model, got %v", errs)
	}
}

func TestValidatePromptSemanticModel_MissingSection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := dir + "/measure.yaml"
	os.WriteFile(path, []byte(`role: analyst
semantic_model:
  data_sources:
    - id: repo
  features:
    - name: gap
  algorithm:
    type: gap_ordered
`), 0o644)
	errs := ValidatePromptSemanticModel(path)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for missing output_format, got %d: %v", len(errs), errs)
	}
	if len(errs) > 0 && !strings.Contains(errs[0], "output_format") {
		t.Errorf("error should mention missing section, got %q", errs[0])
	}
}

func TestValidateSemanticModels_Count(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/semantic-models", 0o755)
	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/prompts", 0o755)

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

	errs, count := ValidateSemanticModels(nil)
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid files, got %v", errs)
	}
}

func TestCollectAnalyzeResult_SemanticModelErrors(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)
	os.MkdirAll("docs/specs/semantic-models", 0o755)
	os.MkdirAll("docs/prompts", 0o755)

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

	result, counts, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
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

// --- Check 15: R-item coverage by acceptance criteria ---

func TestCollectAnalyzeResult_RItemCoverage_FullCoverage(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	prd := `id: prd001-core
title: Core
requirements:
  R1:
    title: Config
    items:
      - R1.1: Field A
      - R1.2: Field B
acceptance_criteria:
  - id: AC1
    criterion: All fields present
    traces:
      - R1.1
      - R1.2
`
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml", []byte(prd), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UncoveredRItems) != 0 {
		t.Errorf("expected no uncovered R-items, got %v", result.UncoveredRItems)
	}
}

func TestCollectAnalyzeResult_RItemCoverage_MissingCoverage(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	prd := `id: prd001-core
title: Core
requirements:
  R1:
    title: Config
    items:
      - R1.1: Field A
      - R1.2: Field B
      - R1.3: Field C
acceptance_criteria:
  - id: AC1
    criterion: Some fields
    traces:
      - R1.1
`
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml", []byte(prd), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UncoveredRItems) != 2 {
		t.Fatalf("expected 2 uncovered R-items, got %d: %v", len(result.UncoveredRItems), result.UncoveredRItems)
	}
	// Check that R1.2 and R1.3 are reported
	combined := strings.Join(result.UncoveredRItems, " ")
	if !strings.Contains(combined, "R1.2") {
		t.Errorf("expected R1.2 in uncovered items, got %v", result.UncoveredRItems)
	}
	if !strings.Contains(combined, "R1.3") {
		t.Errorf("expected R1.3 in uncovered items, got %v", result.UncoveredRItems)
	}
}

// --- Check 16: AC coverage by test cases ---

func TestCollectAnalyzeResult_ACCoverage_Covered(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	prd := `id: prd001-core
title: Core
requirements:
  R1:
    title: Config
    items:
      - R1.1: Field A
acceptance_criteria:
  - id: AC1
    criterion: All fields present
    traces:
      - R1.1
`
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml", []byte(prd), 0o644)

	ts := `id: test-rel01.0
title: Tests
release: rel01.0
traces:
  - rel01.0-uc001-init
test_cases:
  - use_case: rel01.0-uc001-init
    name: Config fields test
    traces:
      - prd001-core AC1
`
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml", []byte(ts), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UncoveredACs) != 0 {
		t.Errorf("expected no uncovered ACs, got %v", result.UncoveredACs)
	}
}

func TestCollectAnalyzeResult_ACCoverage_Uncovered(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	prd := `id: prd001-core
title: Core
requirements:
  R1:
    title: Config
    items:
      - R1.1: Field A
acceptance_criteria:
  - id: AC1
    criterion: Field A present
    traces:
      - R1.1
  - id: AC2
    criterion: Field B present
    traces:
      - R1.1
`
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml", []byte(prd), 0o644)

	// No test suite traces to ACs
	ts := `id: test-rel01.0
title: Tests
release: rel01.0
traces:
  - rel01.0-uc001-init
test_cases:
  - use_case: rel01.0-uc001-init
    name: Basic test
`
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml", []byte(ts), 0o644)
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UncoveredACs) != 2 {
		t.Fatalf("expected 2 uncovered ACs, got %d: %v", len(result.UncoveredACs), result.UncoveredACs)
	}
	combined := strings.Join(result.UncoveredACs, " ")
	if !strings.Contains(combined, "AC1") || !strings.Contains(combined, "AC2") {
		t.Errorf("expected AC1 and AC2 in uncovered ACs, got %v", result.UncoveredACs)
	}
}

// --- Check 17: S-item traces to AC ---

func TestCollectAnalyzeResult_SItemTraces_Valid(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  R1:\n    title: Req\n    items:\n      - R1.1: X\n"), 0o644)

	uc := `id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
success_criteria:
  - id: S1
    criterion: Init works
    traces:
      - prd001-core AC1
  - id: S2
    criterion: Defaults applied
    traces:
      - prd001-core AC2
`
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(uc), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UntracedSuccessCriteria) != 0 {
		t.Errorf("expected no untraced success criteria, got %v", result.UntracedSuccessCriteria)
	}
}

func TestCollectAnalyzeResult_SItemTraces_MissingACTrace(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	setupMinimalAnalyzeDir(t)

	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml",
		[]byte("id: prd001-core\ntitle: Core\nrequirements:\n  R1:\n    title: Req\n    items:\n      - R1.1: X\n"), 0o644)

	uc := `id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
success_criteria:
  - id: S1
    criterion: Init works
    traces:
      - prd001-core AC1
  - id: S2
    criterion: Something else
    traces: []
  - id: S3
    criterion: No traces at all
`
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(uc), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UntracedSuccessCriteria) != 2 {
		t.Fatalf("expected 2 untraced success criteria, got %d: %v", len(result.UntracedSuccessCriteria), result.UntracedSuccessCriteria)
	}
	combined := strings.Join(result.UntracedSuccessCriteria, " ")
	if !strings.Contains(combined, "S2") {
		t.Errorf("expected S2 in untraced criteria, got %v", result.UntracedSuccessCriteria)
	}
	if !strings.Contains(combined, "S3") {
		t.Errorf("expected S3 in untraced criteria, got %v", result.UntracedSuccessCriteria)
	}
}

// --- Check 18: Unreachable UCs (GH-1378) ---

func TestCollectAnalyzeResult_UnreachableUC_NoRItems(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// PRD with no R-items (empty requirements).
	prd := "id: prd001-core\ntitle: Core\nrequirements: {}\n"
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml", []byte(prd), 0o644)

	// UC that references this PRD.
	uc := "id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(uc), 0o644)

	roadmap := "id: rm\ntitle: R\nreleases:\n  - version: \"01.0\"\n    name: R1\n    status: pending\n    use_cases:\n      - id: rel01.0-uc001-init\n        summary: Init\n"
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: T\ntraces:\n  - rel01.0-uc001-init\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UnreachableUCs) != 1 {
		t.Fatalf("expected 1 unreachable UC, got %d: %v", len(result.UnreachableUCs), result.UnreachableUCs)
	}
	if !strings.Contains(result.UnreachableUCs[0], "rel01.0-uc001-init") {
		t.Errorf("expected unreachable UC to mention uc001, got %q", result.UnreachableUCs[0])
	}
}

func TestCollectAnalyzeResult_ReachableUC_HasRItems(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// PRD with R-items.
	prd := "id: prd001-core\ntitle: Core\nrequirements:\n  R1:\n    title: Config\n    items:\n      - R1.1: first\n"
	os.WriteFile("docs/specs/product-requirements/prd001-core.yaml", []byte(prd), 0o644)

	uc := "id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd001-core R1\n"
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(uc), 0o644)

	roadmap := "id: rm\ntitle: R\nreleases:\n  - version: \"01.0\"\n    name: R1\n    status: pending\n    use_cases:\n      - id: rel01.0-uc001-init\n        summary: Init\n"
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: T\ntraces:\n  - rel01.0-uc001-init\ntest_cases:\n  - name: tc1\n    use_case: rel01.0-uc001-init\n    traces:\n      - prd001-core AC1\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UnreachableUCs) != 0 {
		t.Errorf("expected 0 unreachable UCs, got %d: %v", len(result.UnreachableUCs), result.UnreachableUCs)
	}
}

func TestCollectAnalyzeResult_UnreachableUC_MissingPRD(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// UC references a PRD that doesn't exist.
	uc := "id: rel01.0-uc001-init\ntitle: Init\ntouchpoints:\n  - T1: prd999-missing R1\n"
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(uc), 0o644)

	roadmap := "id: rm\ntitle: R\nreleases:\n  - version: \"01.0\"\n    name: R1\n    status: pending\n    use_cases:\n      - id: rel01.0-uc001-init\n        summary: Init\n"
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: T\ntraces:\n  - rel01.0-uc001-init\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.UnreachableUCs) != 1 {
		t.Fatalf("expected 1 unreachable UC, got %d: %v", len(result.UnreachableUCs), result.UnreachableUCs)
	}
}

// --- Check 6b: Bare touchpoints (GH-1961) ---

func TestCollectAnalyzeResult_BareTouchpoint_FlagsMissingRGroups(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	// PRD with R-items.
	prd := "id: prd096-users\ntitle: Users\nrequirements:\n  R1:\n    title: Core\n    items:\n      - R1.1: Must print users\n"
	os.WriteFile("docs/specs/product-requirements/prd096-users.yaml", []byte(prd), 0o644)

	// UC that cites PRD WITHOUT R-group references.
	uc := "id: rel01.0-uc001-users\ntitle: Users\ntouchpoints:\n  - T1: \"cmd/users — prints logged-in usernames (prd096-users)\"\n"
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-users.yaml", []byte(uc), 0o644)

	roadmap := "id: rm\ntitle: R\nreleases:\n  - version: \"01.0\"\n    name: R1\n    status: pending\n    use_cases:\n      - id: rel01.0-uc001-users\n        summary: Users\n"
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: T\ntraces:\n  - rel01.0-uc001-users\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.BareTouchpoints) != 1 {
		t.Fatalf("expected 1 bare touchpoint, got %d: %v", len(result.BareTouchpoints), result.BareTouchpoints)
	}
	if !strings.Contains(result.BareTouchpoints[0], "prd096-users") {
		t.Errorf("expected bare touchpoint to mention prd096-users, got %q", result.BareTouchpoints[0])
	}
}

func TestCollectAnalyzeResult_BareTouchpoint_NotFlaggedWithRGroups(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/product-requirements", 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.MkdirAll("docs/specs/test-suites", 0o755)
	os.MkdirAll("docs/constitutions", 0o755)
	os.MkdirAll("pkg/orchestrator/constitutions", 0o755)

	prd := "id: prd096-users\ntitle: Users\nrequirements:\n  R1:\n    title: Core\n    items:\n      - R1.1: Must print users\n"
	os.WriteFile("docs/specs/product-requirements/prd096-users.yaml", []byte(prd), 0o644)

	// UC that cites PRD WITH R-group references — no warning expected.
	uc := "id: rel01.0-uc001-users\ntitle: Users\ntouchpoints:\n  - T1: \"cmd/users — prints usernames (prd096-users R1)\"\n"
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-users.yaml", []byte(uc), 0o644)

	roadmap := "id: rm\ntitle: R\nreleases:\n  - version: \"01.0\"\n    name: R1\n    status: pending\n    use_cases:\n      - id: rel01.0-uc001-users\n        summary: Users\n"
	os.WriteFile("docs/road-map.yaml", []byte(roadmap), 0o644)
	os.WriteFile("docs/specs/test-suites/test-rel01.0.yaml",
		[]byte("id: test-rel01.0\ntitle: T\ntraces:\n  - rel01.0-uc001-users\n"), 0o644)

	result, _, err := CollectAnalyzeResult(noopDeps())
	if err != nil {
		t.Fatalf("CollectAnalyzeResult: %v", err)
	}
	if len(result.BareTouchpoints) != 0 {
		t.Errorf("expected 0 bare touchpoints when R-groups present, got %d: %v",
			len(result.BareTouchpoints), result.BareTouchpoints)
	}
}
