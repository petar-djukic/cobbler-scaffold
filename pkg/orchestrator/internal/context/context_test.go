// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// PhaseContext tests (prd003 R9)
// ---------------------------------------------------------------------------

func TestLoadPhaseContext_MissingFile(t *testing.T) {
	pc, err := LoadPhaseContext("/nonexistent/measure_context.yaml")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if pc != nil {
		t.Fatalf("expected nil PhaseContext for missing file, got: %+v", pc)
	}
}

func TestLoadPhaseContext_ValidFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "measure_context.yaml")
	content := `include: "docs/VISION.yaml"
exclude: "docs/engineering/*"
sources: "docs/constitutions/*.yaml"
release: "01.0"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pc, err := LoadPhaseContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc == nil {
		t.Fatal("expected non-nil PhaseContext")
	}
	if pc.Include != "docs/VISION.yaml" {
		t.Errorf("Include: got %q, want %q", pc.Include, "docs/VISION.yaml")
	}
	if pc.Exclude != "docs/engineering/*" {
		t.Errorf("Exclude: got %q, want %q", pc.Exclude, "docs/engineering/*")
	}
	if pc.Sources != "docs/constitutions/*.yaml" {
		t.Errorf("Sources: got %q, want %q", pc.Sources, "docs/constitutions/*.yaml")
	}
	if pc.Release != "01.0" {
		t.Errorf("Release: got %q, want %q", pc.Release, "01.0")
	}
}

func TestLoadPhaseContext_ExcludeSourceFields(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "measure_context.yaml")
	content := `exclude_source: true
source_patterns: |
  pkg/orchestrator/*.go
  pkg/types/*.go
exclude_tests: true
`
	os.WriteFile(path, []byte(content), 0o644)

	pc, err := LoadPhaseContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pc.ExcludeSource {
		t.Error("ExcludeSource should be true")
	}
	if !pc.ExcludeTests {
		t.Error("ExcludeTests should be true")
	}
	if !strings.Contains(pc.SourcePatterns, "pkg/orchestrator/*.go") {
		t.Errorf("SourcePatterns should contain pkg/orchestrator/*.go, got %q", pc.SourcePatterns)
	}
}

func TestLoadPhaseContext_MalformedFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	os.WriteFile(path, []byte("not: [valid: yaml"), 0o644)

	_, err := LoadPhaseContext(path)
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

// ---------------------------------------------------------------------------
// NumberLines tests
// ---------------------------------------------------------------------------

func TestNumberLines_Normal(t *testing.T) {
	input := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	got := NumberLines(input)
	want := "1 | package main\n3 | import \"fmt\"\n5 | func main() {\n6 | \tfmt.Println(\"hello\")\n7 | }"
	if got != want {
		t.Errorf("NumberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestNumberLines_BlankLinesOmitted(t *testing.T) {
	input := "a\n\n\nb\n"
	got := NumberLines(input)
	want := "1 | a\n4 | b"
	if got != want {
		t.Errorf("NumberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestNumberLines_SingleLine(t *testing.T) {
	input := "package main\n"
	got := NumberLines(input)
	want := "1 | package main"
	if got != want {
		t.Errorf("NumberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestNumberLines_Empty(t *testing.T) {
	got := NumberLines("")
	if got != "" {
		t.Errorf("NumberLines empty: got %q, want empty", got)
	}
}

func TestNumberLines_WhitespaceOnlyLines(t *testing.T) {
	input := "a\n  \n\t\nb\n"
	got := NumberLines(input)
	want := "1 | a\n4 | b"
	if got != want {
		t.Errorf("NumberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// ReleaseFilter tests
// ---------------------------------------------------------------------------

func TestFileMatchesRelease(t *testing.T) {
	tests := []struct {
		path    string
		release string
		want    bool
	}{
		{"rel01.0-uc001-feature.yaml", "", true},
		{"test-rel01.0.yaml", "", true},
		{"rel01.0-uc001-feature.yaml", "01.0", true},
		{"rel01.0-uc002-other.yaml", "02.0", true},
		{"rel02.0-uc003-future.yaml", "01.0", false},
		{"rel02.0-uc003-future.yaml", "02.0", true},
		{"rel01.1-uc004-minor.yaml", "01.0", false},
		{"rel01.1-uc004-minor.yaml", "01.1", true},
		{"test-rel01.0.yaml", "01.0", true},
		{"test-rel02.0.yaml", "01.0", false},
		{"test-rel02.0.yaml", "02.0", true},
		{"something-else.yaml", "01.0", true},
		{"docs/specs/use-cases/rel01.0-uc001-feature.yaml", "01.0", true},
		{"docs/specs/use-cases/rel02.0-uc003-future.yaml", "01.0", false},
		{"docs/specs/test-suites/test-rel01.0.yaml", "01.0", true},
	}
	for _, tt := range tests {
		rf := ReleaseFilter{MaxRelease: tt.release}
		got := FileMatchesRelease(tt.path, rf)
		if got != tt.want {
			t.Errorf("FileMatchesRelease(%q, maxRelease=%q) = %v, want %v",
				tt.path, tt.release, got, tt.want)
		}
	}
}

func TestFileMatchesRelease_ReleaseSet(t *testing.T) {
	set := ReleaseFilter{ReleaseSet: map[string]bool{"01.0": true, "03.0": true}}
	tests := []struct {
		path string
		want bool
	}{
		{"rel01.0-uc001-feature.yaml", true},
		{"rel03.0-uc005-extra.yaml", true},
		{"test-rel01.0.yaml", true},
		{"test-rel03.0.yaml", true},
		{"rel02.0-uc003-skipped.yaml", false},
		{"test-rel02.0.yaml", false},
		{"something-else.yaml", true},
	}
	for _, tt := range tests {
		got := FileMatchesRelease(tt.path, set)
		if got != tt.want {
			t.Errorf("FileMatchesRelease(%q, set{01.0,03.0}) = %v, want %v",
				tt.path, got, tt.want)
		}
	}
}

func TestNewReleaseFilter(t *testing.T) {
	rf := NewReleaseFilter([]string{"01.0", "02.0"}, "03.0")
	if rf.ReleaseSet == nil {
		t.Fatal("expected ReleaseSet to be set when Releases is non-empty")
	}
	if rf.MaxRelease != "" {
		t.Error("MaxRelease should be empty when ReleaseSet is used")
	}
	if !rf.ReleaseSet["01.0"] || !rf.ReleaseSet["02.0"] {
		t.Errorf("ReleaseSet = %v, want {01.0, 02.0}", rf.ReleaseSet)
	}

	rf2 := NewReleaseFilter(nil, "02.0")
	if rf2.ReleaseSet != nil {
		t.Error("expected nil ReleaseSet when Releases is empty")
	}
	if rf2.MaxRelease != "02.0" {
		t.Errorf("MaxRelease = %q, want %q", rf2.MaxRelease, "02.0")
	}

	rf3 := NewReleaseFilter(nil, "")
	if rf3.Active() {
		t.Error("expected inactive filter when both are empty")
	}
}

func TestExtractFileRelease(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"rel01.0-uc001-feature.yaml", "01.0"},
		{"rel02.0-uc003-future.yaml", "02.0"},
		{"test-rel01.0.yaml", "01.0"},
		{"test-rel03.0.yaml", "03.0"},
		{"docs/specs/use-cases/rel01.0-uc001-feature.yaml", "01.0"},
		{"something-else.yaml", ""},
		{"prd001-core.yaml", ""},
	}
	for _, tt := range tests {
		got := ExtractFileRelease(tt.path)
		if got != tt.want {
			t.Errorf("ExtractFileRelease(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ResolveStandardFiles tests
// ---------------------------------------------------------------------------

func TestResolveStandardFiles(t *testing.T) {
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	dirs := []string{
		"docs",
		"docs/specs/product-requirements",
		"docs/specs/use-cases",
		"docs/specs/test-suites",
		"docs/engineering",
		"docs/constitutions",
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0o755)
	}

	standardFiles := []string{
		"docs/VISION.yaml",
		"docs/ARCHITECTURE.yaml",
		"docs/specs/product-requirements/prd001-core.yaml",
		"docs/specs/use-cases/rel01.0-uc001-feature.yaml",
		"docs/specs/test-suites/test-rel01.0.yaml",
	}
	for _, f := range standardFiles {
		os.WriteFile(f, []byte("id: test"), 0o644)
	}

	excluded := []string{
		"docs/constitutions/planning.yaml",
		"docs/constitutions/go-style.yaml",
		"docs/utilities.yaml",
		"docs/engineering/eng01-guide.yaml",
	}
	for _, f := range excluded {
		os.WriteFile(f, []byte("id: test"), 0o644)
	}

	resolved := ResolveStandardFiles()
	resolvedSet := make(map[string]bool)
	for _, f := range resolved {
		resolvedSet[f] = true
	}
	for _, f := range standardFiles {
		if !resolvedSet[f] {
			t.Errorf("expected standard file %s to be resolved", f)
		}
	}
	for _, f := range excluded {
		if resolvedSet[f] {
			t.Errorf("excluded file %s should not be in resolved set", f)
		}
	}
}

// ---------------------------------------------------------------------------
// LoadContextFileInto tests
// ---------------------------------------------------------------------------

func TestLoadContextFileIntoSetsFilePath(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	os.MkdirAll("docs", 0o755)
	os.WriteFile("docs/VISION.yaml", []byte("id: test\ntitle: Test Vision"), 0o644)
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte("id: test\ntitle: Test Arch"), 0o644)
	os.WriteFile("docs/road-map.yaml", []byte("id: test\ntitle: Test Roadmap"), 0o644)

	ctx := &ProjectContext{Specs: &SpecsCollection{}}
	noFilter := ReleaseFilter{}
	LoadContextFileInto(ctx, "docs/VISION.yaml", noFilter)
	LoadContextFileInto(ctx, "docs/ARCHITECTURE.yaml", noFilter)
	LoadContextFileInto(ctx, "docs/road-map.yaml", noFilter)

	if ctx.Vision == nil || ctx.Vision.File != "docs/VISION.yaml" {
		t.Errorf("Vision.File = %q, want %q", ctx.Vision.File, "docs/VISION.yaml")
	}
	if ctx.Architecture == nil || ctx.Architecture.File != "docs/ARCHITECTURE.yaml" {
		t.Errorf("Architecture.File = %q, want %q", ctx.Architecture.File, "docs/ARCHITECTURE.yaml")
	}
	if ctx.Roadmap == nil || ctx.Roadmap.File != "docs/road-map.yaml" {
		t.Errorf("Roadmap.File = %q, want %q", ctx.Roadmap.File, "docs/road-map.yaml")
	}

	data, err := yaml.Marshal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{"file: docs/VISION.yaml", "file: docs/ARCHITECTURE.yaml", "file: docs/road-map.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("marshaled YAML missing %q", want)
		}
	}
}

func TestParseIssuesJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantNil bool
	}{
		{"empty string returns nil", "", 0, true},
		{"literal [] returns nil", "[]", 0, true},
		{"malformed JSON returns nil", "{not valid json", 0, true},
		{"valid JSON array returns issues", `[{"id":"i1","title":"Fix bug","status":"open","type":"code"}]`, 1, false},
		{"valid JSON array with multiple items", `[{"id":"i1","title":"A","status":"open","type":"code"},{"id":"i2","title":"B","status":"done","type":"doc"}]`, 2, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseIssuesJSON(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("ParseIssuesJSON(%q) = %v, want nil", tc.input, got)
				}
				return
			}
			if len(got) != tc.wantLen {
				t.Errorf("ParseIssuesJSON() len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestLoadContextFileInto_SpecAux(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	os.MkdirAll(filepath.Join("docs", "specs"), 0o755)
	os.WriteFile(filepath.Join("docs", "specs", "dependency-map.yaml"), []byte("name: depmap\n"), 0o644)
	os.WriteFile(filepath.Join("docs", "specs", "sources.yaml"), []byte("name: sources\n"), 0o644)
	os.WriteFile(filepath.Join("docs", "specs", "utilities.yaml"), []byte("name: utilities\n"), 0o644)

	ctx := &ProjectContext{Specs: &SpecsCollection{}}
	noFilter := ReleaseFilter{}
	LoadContextFileInto(ctx, filepath.Join("docs", "specs", "dependency-map.yaml"), noFilter)
	LoadContextFileInto(ctx, filepath.Join("docs", "specs", "sources.yaml"), noFilter)
	LoadContextFileInto(ctx, filepath.Join("docs", "specs", "utilities.yaml"), noFilter)

	if ctx.Specs.DependencyMap == nil {
		t.Error("Specs.DependencyMap should be set for dependency-map.yaml")
	}
	if ctx.Specs.Sources == nil {
		t.Error("Specs.Sources should be set for sources.yaml")
	}
	if len(ctx.Extra) != 1 {
		t.Errorf("Extra len = %d, want 1 (for utilities.yaml)", len(ctx.Extra))
	}
}

func TestLoadContextFileInto_Engineering(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	os.MkdirAll(filepath.Join("docs", "engineering"), 0o755)
	os.WriteFile(filepath.Join("docs", "engineering", "eng01-testing.yaml"), []byte("id: eng01\ntitle: Testing Guide\n"), 0o644)

	ctx := &ProjectContext{Specs: &SpecsCollection{}}
	noFilter := ReleaseFilter{}
	LoadContextFileInto(ctx, filepath.Join("docs", "engineering", "eng01-testing.yaml"), noFilter)

	if len(ctx.Engineering) != 1 {
		t.Errorf("Engineering len = %d, want 1", len(ctx.Engineering))
	} else if ctx.Engineering[0].ID != "eng01" {
		t.Errorf("Engineering[0].ID = %q, want %q", ctx.Engineering[0].ID, "eng01")
	}
}

// ---------------------------------------------------------------------------
// ClassifyContextFile tests
// ---------------------------------------------------------------------------

func TestClassifyContextFile_AllTypes(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"docs/VISION.yaml", "vision"},
		{"docs/ARCHITECTURE.yaml", "architecture"},
		{"docs/SPECIFICATIONS.yaml", "specifications"},
		{"docs/road-map.yaml", "roadmap"},
		{filepath.Join("docs", "specs", "product-requirements", "prd001-feature.yaml"), "prd"},
		{filepath.Join("docs", "specs", "use-cases", "rel01.0-uc001-init.yaml"), "use_case"},
		{filepath.Join("docs", "specs", "test-suites", "test-rel-01.0.yaml"), "test_suite"},
		{filepath.Join("docs", "specs", "dependency-map.yaml"), "spec_aux"},
		{filepath.Join("docs", "engineering", "eng01-guidelines.yaml"), "engineering"},
		{filepath.Join("docs", "constitutions", "design.yaml"), "constitution"},
		{"docs/custom.yaml", "extra"},
		{"random/file.yaml", "extra"},
	}
	for _, tc := range cases {
		if got := ClassifyContextFile(tc.path); got != tc.want {
			t.Errorf("ClassifyContextFile(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FilterSourceFiles tests
// ---------------------------------------------------------------------------

func TestFilterSourceFiles_ExactMatch(t *testing.T) {
	sources := []SourceFile{
		{File: "pkg/orchestrator/stitch.go", Lines: "1 | package orchestrator"},
		{File: "pkg/orchestrator/context.go", Lines: "1 | package orchestrator"},
		{File: "pkg/orchestrator/config.go", Lines: "1 | package orchestrator"},
	}
	required := []string{"pkg/orchestrator/stitch.go", "pkg/orchestrator/context.go"}

	got := FilterSourceFiles(sources, required)
	if len(got) != 2 {
		t.Fatalf("FilterSourceFiles: got %d files, want 2", len(got))
	}
}

func TestFilterSourceFiles_EmptyRequired(t *testing.T) {
	sources := []SourceFile{{File: "pkg/a.go"}, {File: "pkg/b.go"}, {File: "pkg/c.go"}}
	got := FilterSourceFiles(sources, nil)
	if len(got) != 3 {
		t.Errorf("FilterSourceFiles empty required: got %d, want 3 (all files)", len(got))
	}
}

func TestFilterSourceFiles_NoMatch(t *testing.T) {
	sources := []SourceFile{{File: "pkg/a.go"}, {File: "pkg/b.go"}}
	required := []string{"pkg/nonexistent.go"}
	got := FilterSourceFiles(sources, required)
	if len(got) != 0 {
		t.Errorf("FilterSourceFiles no match: got %d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// StripParenthetical tests
// ---------------------------------------------------------------------------

func TestStripParenthetical(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"pkg/types/cupboard.go (CrumbTable interface)", "pkg/types/cupboard.go"},
		{"pkg/orchestrator/stitch.go (buildStitchPrompt, stitchTask)", "pkg/orchestrator/stitch.go"},
		{"docs/engineering/eng05.md (recommendation D)", "docs/engineering/eng05.md"},
		{"pkg/plain.go", "pkg/plain.go"},
		{"", ""},
		{"  pkg/spaced.go  ", "pkg/spaced.go"},
	}
	for _, tt := range tests {
		got := StripParenthetical(tt.input)
		if got != tt.want {
			t.Errorf("StripParenthetical(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ApplyContextBudget tests
// ---------------------------------------------------------------------------

func TestApplyContextBudget_RemovesNonRequired(t *testing.T) {
	ctx := &ProjectContext{
		SourceCode: []SourceFile{
			{File: "pkg/a.go", Lines: strings.Repeat("x", 1000)},
			{File: "pkg/b.go", Lines: strings.Repeat("y", 1000)},
			{File: "pkg/c.go", Lines: strings.Repeat("z", 1000)},
		},
	}
	required := []string{"pkg/a.go"}
	data, _ := yaml.Marshal(ctx)
	fullSize := len(data)
	budget := fullSize / 2

	ApplyContextBudget(ctx, budget, required)

	found := false
	for _, sf := range ctx.SourceCode {
		if sf.File == "pkg/a.go" {
			found = true
		}
	}
	if !found {
		t.Error("required file pkg/a.go was removed by budget enforcement")
	}
	if len(ctx.SourceCode) >= 3 {
		t.Errorf("expected some files to be removed, still have %d", len(ctx.SourceCode))
	}
}

func TestApplyContextBudget_NilContext(t *testing.T) {
	ApplyContextBudget(nil, 100, nil)
}

func TestApplyContextBudget_NegativeBudget(t *testing.T) {
	ctx := &ProjectContext{
		SourceCode: []SourceFile{
			{File: "pkg/a.go", Lines: strings.Repeat("x", 5000)},
			{File: "pkg/b.go", Lines: strings.Repeat("y", 5000)},
		},
	}
	ApplyContextBudget(ctx, -1, nil)
	if len(ctx.SourceCode) != 2 {
		t.Errorf("negative budget should disable enforcement, got %d files", len(ctx.SourceCode))
	}
}

// ---------------------------------------------------------------------------
// EnsureTypedDocs tests
// ---------------------------------------------------------------------------

func TestEnsureTypedDocs_SkipsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	files := EnsureTypedDocs(nil)
	if len(files) != 0 {
		t.Errorf("got %d files, want 0 (no typed docs exist in temp dir)", len(files))
	}
}

// ---------------------------------------------------------------------------
// loadNamedDoc tests
// ---------------------------------------------------------------------------

func TestLoadNamedDoc_MarkdownFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "do-work.md")
	content := "# Do Work\n\nUse this command:\n\n```bash\ncurl http://example.com\n```\n"
	os.WriteFile(mdPath, []byte(content), 0o644)

	doc := LoadNamedDoc(mdPath)
	if doc == nil {
		t.Fatal("LoadNamedDoc returned nil for markdown file")
	}
	if doc.Name != "do-work" {
		t.Errorf("Name = %q, want %q", doc.Name, "do-work")
	}
	if doc.Content.Kind != yaml.ScalarNode {
		t.Errorf("Content.Kind = %d, want ScalarNode (%d)", doc.Content.Kind, yaml.ScalarNode)
	}
}

func TestLoadNamedDoc_TextFile(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "readme.txt")
	os.WriteFile(txtPath, []byte("plain text"), 0o644)

	doc := LoadNamedDoc(txtPath)
	if doc == nil {
		t.Fatal("LoadNamedDoc returned nil for .txt file")
	}
	if doc.Content.Value != "plain text" {
		t.Errorf("Content.Value = %q, want %q", doc.Content.Value, "plain text")
	}
}

func TestLoadNamedDoc_YAMLFileStillWorks(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(yamlPath, []byte("id: test\ntitle: Test Doc"), 0o644)

	doc := LoadNamedDoc(yamlPath)
	if doc == nil {
		t.Fatal("LoadNamedDoc returned nil for YAML file")
	}
	if doc.Name != "config" {
		t.Errorf("Name = %q, want %q", doc.Name, "config")
	}
	if doc.Content.Kind == yaml.ScalarNode {
		t.Error("YAML file should not be loaded as scalar node")
	}
}

// ---------------------------------------------------------------------------
// SummarizeGoHeaders tests
// ---------------------------------------------------------------------------

func TestSummarizeGoHeaders_ExportsOnly(t *testing.T) {
	src := `package example

import "fmt"

// ExportedFunc does something.
func ExportedFunc() error {
	fmt.Println("hello")
	return nil
}

func unexportedFunc() {
	fmt.Println("hidden")
}

// ExportedType is a public type.
type ExportedType struct {
	Name string
}

type unexportedType struct {
	name string
}
`
	got := SummarizeGoHeaders(src)
	if !strings.Contains(got, "ExportedFunc") {
		t.Error("SummarizeGoHeaders should include ExportedFunc")
	}
	if strings.Contains(got, "unexportedFunc") {
		t.Error("SummarizeGoHeaders should exclude unexportedFunc")
	}
	if !strings.Contains(got, "ExportedType") {
		t.Error("SummarizeGoHeaders should include ExportedType")
	}
	if strings.Contains(got, "unexportedType") {
		t.Error("SummarizeGoHeaders should exclude unexportedType")
	}
	// Function body should be removed.
	if strings.Contains(got, "hello") {
		t.Error("SummarizeGoHeaders should remove function bodies")
	}
}

func TestSummarizeGoHeaders_InvalidInput(t *testing.T) {
	input := "not valid go code{{{}"
	got := SummarizeGoHeaders(input)
	if got != input {
		t.Error("SummarizeGoHeaders should return input unchanged for invalid Go")
	}
}

// ---------------------------------------------------------------------------
// SummarizeCustom tests
// ---------------------------------------------------------------------------

func TestSummarizeCustom_EmptyCommand(t *testing.T) {
	got := SummarizeCustom("", "file.go", "full content")
	if got != "full content" {
		t.Errorf("SummarizeCustom empty command: got %q, want %q", got, "full content")
	}
}

// ---------------------------------------------------------------------------
// ParseTouchpointPackages tests
// ---------------------------------------------------------------------------

func TestParseTouchpointPackages_EmDash(t *testing.T) {
	touchpoints := []map[string]string{
		{"T1": "pkg/orchestrator \u2014 prd003 R1, R2"},
		{"T2": "cmd/cobbler \u2014 prd001 R1"},
	}
	got := ParseTouchpointPackages(touchpoints)
	if len(got) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(got), got)
	}
}

func TestParseTouchpointPackages_Empty(t *testing.T) {
	got := ParseTouchpointPackages(nil)
	if got != nil {
		t.Errorf("expected nil for nil touchpoints, got %v", got)
	}

	got2 := ParseTouchpointPackages([]map[string]string{})
	if got2 != nil {
		t.Errorf("expected nil for empty touchpoints, got %v", got2)
	}
}

// ---------------------------------------------------------------------------
// UCStatusDone tests
// ---------------------------------------------------------------------------

func TestUCStatusDone(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"done", true},
		{"Done", true},
		{"DONE", true},
		{"implemented", true},
		{"Implemented", true},
		{"pending", false},
		{"in-progress", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := UCStatusDone(tt.status); got != tt.want {
			t.Errorf("UCStatusDone(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PRDIDsFromUseCases tests
// ---------------------------------------------------------------------------

func TestPrdIDsFromUseCases(t *testing.T) {
	useCases := []*UseCaseDoc{
		{
			Touchpoints: []map[string]string{
				{"T1": "pkg/orchestrator \u2014 prd003 R1"},
				{"T2": "cmd/cobbler \u2014 prd001 R1"},
			},
		},
		{
			Touchpoints: []map[string]string{
				{"T1": "pkg/types \u2014 prd003 R2"},
			},
		},
	}
	got := PRDIDsFromUseCases(useCases)
	if !got["prd003"] {
		t.Error("expected prd003 in set")
	}
	if !got["prd001"] {
		t.Error("expected prd001 in set")
	}
}

// ---------------------------------------------------------------------------
// BuildProjectContext tests
// ---------------------------------------------------------------------------

func setupContextTestDir(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{
		"docs",
		"docs/specs/product-requirements",
		"docs/specs/use-cases",
		"docs/specs/test-suites",
		"docs/engineering",
		"pkg/app",
	} {
		os.MkdirAll(d, 0o755)
	}

	os.WriteFile("docs/VISION.yaml", []byte("id: v1\ntitle: Vision"), 0o644)
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte("id: a1\ntitle: Arch"), 0o644)
	os.WriteFile("docs/road-map.yaml", []byte("id: r1\ntitle: Roadmap"), 0o644)
	os.WriteFile("pkg/app/main.go", []byte("package app\n"), 0o644)
	os.WriteFile("pkg/app/util.go", []byte("package app\n"), 0o644)

	return tmp, func() { os.Chdir(orig) }
}

func TestBuildProjectContext_PhaseContextOverride(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	os.WriteFile("docs/custom.yaml", []byte("id: custom\ntitle: Custom"), 0o644)

	project := ContextConfig{
		ContextInclude: "docs/VISION.yaml\ndocs/ARCHITECTURE.yaml",
	}

	phaseCtx := &PhaseContext{
		Include: "docs/custom.yaml",
	}

	ctx, err := BuildProjectContext("", project, phaseCtx, ".cobbler")
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Vision == nil {
		t.Error("Vision should be loaded (EnsureTypedDocs adds missing typed docs)")
	}
}

func TestBuildProjectContext_NilPhaseContextUsesConfig(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	project := ContextConfig{
		GoSourceDirs: []string{"pkg/"},
	}

	ctx, err := BuildProjectContext("", project, nil, ".cobbler")
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Vision == nil {
		t.Error("Vision should be loaded with nil PhaseContext")
	}
}

func TestBuildProjectContext_ExcludeSource(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	project := ContextConfig{GoSourceDirs: []string{"pkg/"}}
	phaseCtx := &PhaseContext{ExcludeSource: true}

	ctx, err := BuildProjectContext("", project, phaseCtx, ".cobbler")
	if err != nil {
		t.Fatal(err)
	}

	if len(ctx.SourceCode) != 0 {
		t.Errorf("SourceCode should be empty when ExcludeSource=true, got %d files", len(ctx.SourceCode))
	}
	if ctx.Vision == nil {
		t.Error("Vision should still be loaded even with ExcludeSource=true")
	}
}

func TestBuildProjectContextNoConstitutions(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	project := ContextConfig{}
	ctx, err := BuildProjectContext("", project, nil, ".cobbler")
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Vision == nil {
		t.Error("Vision should be loaded from standard files")
	}
	if ctx.Architecture == nil {
		t.Error("Architecture should be loaded from standard files")
	}
}

func TestContextExcludeEverything(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	project := ContextConfig{
		GoSourceDirs:   []string{},
		ContextExclude: ".",
	}

	ctx, err := BuildProjectContext("", project, nil, ".cobbler")
	if err != nil {
		t.Fatal(err)
	}

	if ctx.Vision != nil {
		t.Error("Vision should be nil with context_exclude='.'")
	}
	if ctx.Architecture != nil {
		t.Error("Architecture should be nil with context_exclude='.'")
	}
}

// ---------------------------------------------------------------------------
// SelectNextPendingUseCase tests
// ---------------------------------------------------------------------------

func TestSelectNextPendingUseCase_AllDone(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/use-cases", 0o755)
	os.WriteFile("docs/road-map.yaml", []byte(`id: rm
title: Roadmap
releases:
  - version: "01.0"
    name: First
    status: active
    use_cases:
      - id: rel01.0-uc001-first
        status: done
`), 0o644)

	doc, err := SelectNextPendingUseCase(ContextConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if doc != nil {
		t.Errorf("expected nil when all UCs are done, got %+v", doc)
	}
}

func TestSelectNextPendingUseCase_MissingRoadmap(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	doc, err := SelectNextPendingUseCase(ContextConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if doc != nil {
		t.Errorf("expected nil when roadmap is missing, got %+v", doc)
	}
}

// ---------------------------------------------------------------------------
// ApplyContextBudget with default budget
// ---------------------------------------------------------------------------

func TestApplyContextBudget_DefaultBudget(t *testing.T) {
	var sources []SourceFile
	for i := 0; i < 100; i++ {
		sources = append(sources, SourceFile{
			File:  fmt.Sprintf("pkg/file%03d.go", i),
			Lines: strings.Repeat("x", 3000),
		})
	}
	ctx := &ProjectContext{SourceCode: sources}
	required := []string{"pkg/file000.go"}
	ApplyContextBudget(ctx, 0, required)

	found := false
	for _, sf := range ctx.SourceCode {
		if sf.File == "pkg/file000.go" {
			found = true
		}
	}
	if !found {
		t.Error("required file pkg/file000.go was removed by default budget")
	}
	if len(ctx.SourceCode) >= 100 {
		t.Errorf("expected default budget to remove files, still have %d", len(ctx.SourceCode))
	}
}
