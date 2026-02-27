// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
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
	pc, err := loadPhaseContext("/nonexistent/measure_context.yaml")
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

	pc, err := loadPhaseContext(path)
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

func TestLoadPhaseContext_MalformedFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	pc, err := loadPhaseContext(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if pc != nil {
		t.Errorf("expected nil PhaseContext for malformed file, got: %+v", pc)
	}
}

func TestBuildProjectContext_PhaseContextOverride(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Create a custom doc.
	os.WriteFile("docs/custom.yaml", []byte("id: custom\ntitle: Custom"), 0o644)

	project := ProjectConfig{
		ContextInclude: "docs/VISION.yaml\ndocs/ARCHITECTURE.yaml",
	}

	// PhaseContext overrides include to only load custom.yaml.
	phaseCtx := &PhaseContext{
		Include: "docs/custom.yaml",
	}

	ctx, err := buildProjectContext("", project, phaseCtx)
	if err != nil {
		t.Fatal(err)
	}

	// Vision should still be loaded: ensureTypedDocs adds it even when
	// phase include doesn't cover it.
	if ctx.Vision == nil {
		t.Error("Vision should be loaded (ensureTypedDocs adds missing typed docs)")
	}
}

func TestBuildProjectContext_NilPhaseContextUsesConfig(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	project := ProjectConfig{
		GoSourceDirs: []string{"pkg/"},
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Standard files should be loaded.
	if ctx.Vision == nil {
		t.Error("Vision should be loaded with nil PhaseContext")
	}
}

func TestBuildProjectContext_PhaseContextPartialOverride(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	project := ProjectConfig{
		GoSourceDirs:   []string{"pkg/"},
		ContextExclude: "pkg/app/util.go",
	}

	// PhaseContext sets only include (empty exclude defers to Config).
	phaseCtx := &PhaseContext{
		Include: "docs/VISION.yaml",
	}

	ctx, err := buildProjectContext("", project, phaseCtx)
	if err != nil {
		t.Fatal(err)
	}

	// Vision should be loaded (from phase include).
	if ctx.Vision == nil {
		t.Error("Vision should be loaded from PhaseContext.Include")
	}

	// Architecture should still be loaded: ensureTypedDocs adds it even
	// when phase include only specifies VISION.
	if ctx.Architecture == nil {
		t.Error("Architecture should be loaded (ensureTypedDocs adds missing typed docs)")
	}

	// util.go should still be excluded (from Config.ContextExclude,
	// because PhaseContext.Exclude is empty and defers to Config).
	for _, sf := range ctx.SourceCode {
		if sf.File == "pkg/app/util.go" {
			t.Error("pkg/app/util.go should be excluded (Config.ContextExclude still active)")
		}
	}
}

func TestNumberLines_Normal(t *testing.T) {
	input := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	got := numberLines(input)
	want := "1 | package main\n3 | import \"fmt\"\n5 | func main() {\n6 | \tfmt.Println(\"hello\")\n7 | }"
	if got != want {
		t.Errorf("numberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestNumberLines_BlankLinesOmitted(t *testing.T) {
	input := "a\n\n\nb\n"
	got := numberLines(input)
	want := "1 | a\n4 | b"
	if got != want {
		t.Errorf("numberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestNumberLines_SingleLine(t *testing.T) {
	input := "package main\n"
	got := numberLines(input)
	want := "1 | package main"
	if got != want {
		t.Errorf("numberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestNumberLines_Empty(t *testing.T) {
	got := numberLines("")
	if got != "" {
		t.Errorf("numberLines empty: got %q, want empty", got)
	}
}

func TestNumberLines_WhitespaceOnlyLines(t *testing.T) {
	input := "a\n  \n\t\nb\n"
	got := numberLines(input)
	want := "1 | a\n4 | b"
	if got != want {
		t.Errorf("numberLines:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFileMatchesRelease(t *testing.T) {
	tests := []struct {
		path    string
		release string
		want    bool
	}{
		// Empty release disables filtering.
		{"rel01.0-uc001-feature.yaml", "", true},
		{"test-rel01.0.yaml", "", true},

		// Use case filenames.
		{"rel01.0-uc001-feature.yaml", "01.0", true},
		{"rel01.0-uc002-other.yaml", "02.0", true},
		{"rel02.0-uc003-future.yaml", "01.0", false},
		{"rel02.0-uc003-future.yaml", "02.0", true},
		{"rel01.1-uc004-minor.yaml", "01.0", false},
		{"rel01.1-uc004-minor.yaml", "01.1", true},

		// Test suite filenames.
		{"test-rel01.0.yaml", "01.0", true},
		{"test-rel02.0.yaml", "01.0", false},
		{"test-rel02.0.yaml", "02.0", true},

		// Unknown format passes through.
		{"something-else.yaml", "01.0", true},

		// Subdirectory paths.
		{"docs/specs/use-cases/rel01.0-uc001-feature.yaml", "01.0", true},
		{"docs/specs/use-cases/rel02.0-uc003-future.yaml", "01.0", false},
		{"docs/specs/test-suites/test-rel01.0.yaml", "01.0", true},
	}

	for _, tt := range tests {
		rf := releaseFilter{MaxRelease: tt.release}
		got := fileMatchesRelease(tt.path, rf)
		if got != tt.want {
			t.Errorf("fileMatchesRelease(%q, maxRelease=%q) = %v, want %v",
				tt.path, tt.release, got, tt.want)
		}
	}
}

func TestFileMatchesRelease_ReleaseSet(t *testing.T) {
	set := releaseFilter{ReleaseSet: map[string]bool{"01.0": true, "03.0": true}}

	tests := []struct {
		path string
		want bool
	}{
		// In-set releases pass.
		{"rel01.0-uc001-feature.yaml", true},
		{"rel03.0-uc005-extra.yaml", true},
		{"test-rel01.0.yaml", true},
		{"test-rel03.0.yaml", true},

		// Out-of-set releases are excluded.
		{"rel02.0-uc003-skipped.yaml", false},
		{"test-rel02.0.yaml", false},

		// Unknown format passes through.
		{"something-else.yaml", true},
	}

	for _, tt := range tests {
		got := fileMatchesRelease(tt.path, set)
		if got != tt.want {
			t.Errorf("fileMatchesRelease(%q, set{01.0,03.0}) = %v, want %v",
				tt.path, got, tt.want)
		}
	}
}

func TestNewReleaseFilter(t *testing.T) {
	// Releases list takes precedence over Release string.
	rf := newReleaseFilter([]string{"01.0", "02.0"}, "03.0")
	if rf.ReleaseSet == nil {
		t.Fatal("expected ReleaseSet to be set when Releases is non-empty")
	}
	if rf.MaxRelease != "" {
		t.Error("MaxRelease should be empty when ReleaseSet is used")
	}
	if !rf.ReleaseSet["01.0"] || !rf.ReleaseSet["02.0"] {
		t.Errorf("ReleaseSet = %v, want {01.0, 02.0}", rf.ReleaseSet)
	}

	// Empty Releases falls back to Release string.
	rf2 := newReleaseFilter(nil, "02.0")
	if rf2.ReleaseSet != nil {
		t.Error("expected nil ReleaseSet when Releases is empty")
	}
	if rf2.MaxRelease != "02.0" {
		t.Errorf("MaxRelease = %q, want %q", rf2.MaxRelease, "02.0")
	}

	// Both empty → no filtering.
	rf3 := newReleaseFilter(nil, "")
	if rf3.active() {
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
		got := extractFileRelease(tt.path)
		if got != tt.want {
			t.Errorf("extractFileRelease(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestResolveStandardFiles(t *testing.T) {
	// Create a temp dir with known doc structure.
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	// Create standard files.
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

	// Create files that should NOT be included.
	excluded := []string{
		"docs/constitutions/planning.yaml",
		"docs/constitutions/go-style.yaml",
		"docs/utilities.yaml",
		"docs/engineering/eng01-guide.yaml",
	}
	for _, f := range excluded {
		os.WriteFile(f, []byte("id: test"), 0o644)
	}

	resolved := resolveStandardFiles()

	// All standard files should be included.
	resolvedSet := make(map[string]bool)
	for _, f := range resolved {
		resolvedSet[f] = true
	}
	for _, f := range standardFiles {
		if !resolvedSet[f] {
			t.Errorf("expected standard file %s to be resolved", f)
		}
	}

	// Excluded files should not be included.
	for _, f := range excluded {
		if resolvedSet[f] {
			t.Errorf("excluded file %s should not be in resolved set", f)
		}
	}
}

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
	noFilter := releaseFilter{}
	loadContextFileInto(ctx, "docs/VISION.yaml", noFilter)
	loadContextFileInto(ctx, "docs/ARCHITECTURE.yaml", noFilter)
	loadContextFileInto(ctx, "docs/road-map.yaml", noFilter)

	if ctx.Vision == nil || ctx.Vision.File != "docs/VISION.yaml" {
		t.Errorf("Vision.File = %q, want %q", ctx.Vision.File, "docs/VISION.yaml")
	}
	if ctx.Architecture == nil || ctx.Architecture.File != "docs/ARCHITECTURE.yaml" {
		t.Errorf("Architecture.File = %q, want %q", ctx.Architecture.File, "docs/ARCHITECTURE.yaml")
	}
	if ctx.Roadmap == nil || ctx.Roadmap.File != "docs/road-map.yaml" {
		t.Errorf("Roadmap.File = %q, want %q", ctx.Roadmap.File, "docs/road-map.yaml")
	}

	// Verify file: appears in marshaled YAML.
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
		{
			name:    "empty string returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:    "literal [] returns nil",
			input:   "[]",
			wantNil: true,
		},
		{
			name:    "malformed JSON returns nil",
			input:   "{not valid json",
			wantNil: true,
		},
		{
			name:    "valid JSON array returns issues",
			input:   `[{"id":"i1","title":"Fix bug","status":"open","type":"code"}]`,
			wantLen: 1,
		},
		{
			name:    "valid JSON array with multiple items",
			input:   `[{"id":"i1","title":"A","status":"open","type":"code"},{"id":"i2","title":"B","status":"done","type":"doc"}]`,
			wantLen: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseIssuesJSON(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("parseIssuesJSON(%q) = %v, want nil", tc.input, got)
				}
				return
			}
			if len(got) != tc.wantLen {
				t.Errorf("parseIssuesJSON() len = %d, want %d", len(got), tc.wantLen)
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

	// dependency-map.yaml -> ctx.Specs.DependencyMap
	os.WriteFile(filepath.Join("docs", "specs", "dependency-map.yaml"), []byte("name: depmap\n"), 0o644)
	// sources.yaml -> ctx.Specs.Sources
	os.WriteFile(filepath.Join("docs", "specs", "sources.yaml"), []byte("name: sources\n"), 0o644)
	// unknown file -> ctx.Extra
	os.WriteFile(filepath.Join("docs", "specs", "utilities.yaml"), []byte("name: utilities\n"), 0o644)

	ctx := &ProjectContext{Specs: &SpecsCollection{}}
	noFilter := releaseFilter{}
	loadContextFileInto(ctx, filepath.Join("docs", "specs", "dependency-map.yaml"), noFilter)
	loadContextFileInto(ctx, filepath.Join("docs", "specs", "sources.yaml"), noFilter)
	loadContextFileInto(ctx, filepath.Join("docs", "specs", "utilities.yaml"), noFilter)

	if ctx.Specs.DependencyMap == nil {
		t.Error("Specs.DependencyMap should be set for dependency-map.yaml")
	} else if ctx.Specs.DependencyMap.File != filepath.Join("docs", "specs", "dependency-map.yaml") {
		t.Errorf("DependencyMap.File = %q, want %q", ctx.Specs.DependencyMap.File, filepath.Join("docs", "specs", "dependency-map.yaml"))
	}
	if ctx.Specs.Sources == nil {
		t.Error("Specs.Sources should be set for sources.yaml")
	} else if ctx.Specs.Sources.File != filepath.Join("docs", "specs", "sources.yaml") {
		t.Errorf("Sources.File = %q, want %q", ctx.Specs.Sources.File, filepath.Join("docs", "specs", "sources.yaml"))
	}
	if len(ctx.Extra) != 1 {
		t.Errorf("Extra len = %d, want 1 (for utilities.yaml)", len(ctx.Extra))
	} else if ctx.Extra[0].File != filepath.Join("docs", "specs", "utilities.yaml") {
		t.Errorf("Extra[0].File = %q, want utilities.yaml path", ctx.Extra[0].File)
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
	noFilter := releaseFilter{}
	loadContextFileInto(ctx, filepath.Join("docs", "engineering", "eng01-testing.yaml"), noFilter)

	if len(ctx.Engineering) != 1 {
		t.Fatalf("Engineering len = %d, want 1", len(ctx.Engineering))
	}
	if ctx.Engineering[0].File != filepath.Join("docs", "engineering", "eng01-testing.yaml") {
		t.Errorf("Engineering[0].File = %q", ctx.Engineering[0].File)
	}
}

func TestLoadContextFileInto_Extra(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	os.WriteFile("notes.yaml", []byte("name: notes\n"), 0o644)

	ctx := &ProjectContext{Specs: &SpecsCollection{}}
	noFilter := releaseFilter{}
	loadContextFileInto(ctx, "notes.yaml", noFilter)

	if len(ctx.Extra) != 1 {
		t.Fatalf("Extra len = %d, want 1", len(ctx.Extra))
	}
	if ctx.Extra[0].File != "notes.yaml" {
		t.Errorf("Extra[0].File = %q, want %q", ctx.Extra[0].File, "notes.yaml")
	}
}

func TestBuildProjectContextNoConstitutions(t *testing.T) {
	// Build a minimal ProjectContext and verify no Constitutions field
	// appears in marshaled YAML.
	ctx := &ProjectContext{
		Vision: &VisionDoc{ID: "test", Title: "Test"},
	}
	data, err := yaml.Marshal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Check that "constitutions" key is absent.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["constitutions"]; ok {
		t.Errorf("ProjectContext YAML should not contain 'constitutions' key")
	}
}

func TestPrdIDsFromUseCases(t *testing.T) {
	useCases := []*UseCaseDoc{
		{
			ID: "rel01.0-uc001-feature",
			Touchpoints: []map[string]string{
				{"T1": "Component (prd001-core R1, R2)"},
				{"T2": "Other (prd002-extra R3)"},
			},
		},
		{
			ID: "rel01.0-uc002-other",
			Touchpoints: []map[string]string{
				{"T1": "Same (prd001-core R4)"},
			},
		},
	}

	ids := prdIDsFromUseCases(useCases)
	if !ids["prd001-core"] {
		t.Error("expected prd001-core in referenced PRDs")
	}
	if !ids["prd002-extra"] {
		t.Error("expected prd002-extra in referenced PRDs")
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 PRD IDs, got %d", len(ids))
	}

	// Nil use cases should return nil.
	if got := prdIDsFromUseCases(nil); got != nil {
		t.Errorf("expected nil for nil use cases, got %v", got)
	}
}

// setupContextTestDir creates a temp directory with standard doc structure
// and Go source files, chdir into it, and returns a cleanup function.
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

	// Create standard doc structure.
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

func TestContextExclude(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Create an extra file that will be excluded.
	os.WriteFile("docs/extra.yaml", []byte("id: extra"), 0o644)

	project := ProjectConfig{
		GoSourceDirs:   []string{"pkg/"},
		ContextSources: "docs/extra.yaml",
		ContextExclude: "docs/extra.yaml\npkg/app/util.go",
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// extra.yaml should be excluded from extras.
	for _, e := range ctx.Extra {
		if e.File == "docs/extra.yaml" {
			t.Error("docs/extra.yaml should be excluded from extras")
		}
	}

	// util.go should be excluded from source code.
	for _, sf := range ctx.SourceCode {
		if sf.File == "pkg/app/util.go" {
			t.Error("pkg/app/util.go should be excluded from source code")
		}
	}

	// main.go should still be present.
	found := false
	for _, sf := range ctx.SourceCode {
		if sf.File == "pkg/app/main.go" {
			found = true
		}
	}
	if !found {
		t.Error("pkg/app/main.go should be present in source code")
	}
}

func TestContextIncludeReplacesStandard(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Create a custom doc that is NOT in the standard set.
	os.WriteFile("docs/custom.yaml", []byte("id: custom\ntitle: Custom"), 0o644)

	project := ProjectConfig{
		ContextInclude: "docs/custom.yaml",
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Typed docs should still be loaded: ensureTypedDocs adds them even
	// when context_include doesn't cover their paths.
	if ctx.Vision == nil {
		t.Error("Vision should be loaded (ensureTypedDocs adds missing typed docs)")
	}
	if ctx.Architecture == nil {
		t.Error("Architecture should be loaded (ensureTypedDocs adds missing typed docs)")
	}

	// The custom file should be loaded as an extra (classified as "extra").
	found := false
	for _, e := range ctx.Extra {
		if e.File == "docs/custom.yaml" {
			found = true
		}
	}
	if !found {
		t.Error("docs/custom.yaml should be loaded via context_include")
	}
}

func TestContextExcludeDirectory(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Create files in a subdirectory.
	os.MkdirAll("pkg/sub", 0o755)
	os.WriteFile("pkg/sub/a.go", []byte("package sub\n"), 0o644)
	os.WriteFile("pkg/sub/b.go", []byte("package sub\n"), 0o644)

	project := ProjectConfig{
		GoSourceDirs:   []string{"pkg/"},
		ContextExclude: "pkg/sub",
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// No files from pkg/sub/ should appear in source code.
	for _, sf := range ctx.SourceCode {
		if strings.HasPrefix(sf.File, "pkg/sub/") || strings.HasPrefix(sf.File, filepath.Join("pkg", "sub")+string(filepath.Separator)) {
			t.Errorf("file %s from excluded directory should not be in source code", sf.File)
		}
	}

	// Files from pkg/app/ should still be present.
	appFound := false
	for _, sf := range ctx.SourceCode {
		if strings.HasPrefix(sf.File, "pkg/app/") {
			appFound = true
		}
	}
	if !appFound {
		t.Error("pkg/app/ files should still be present")
	}
}

func TestContextIncludeWithExclude(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Include two files, exclude one of them.
	os.WriteFile("docs/inc1.yaml", []byte("id: inc1"), 0o644)
	os.WriteFile("docs/inc2.yaml", []byte("id: inc2"), 0o644)

	project := ProjectConfig{
		ContextInclude: "docs/inc1.yaml\ndocs/inc2.yaml",
		ContextExclude: "docs/inc2.yaml",
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// inc1 should be loaded, inc2 should be excluded.
	var names []string
	for _, e := range ctx.Extra {
		names = append(names, e.File)
	}

	found1, found2 := false, false
	for _, n := range names {
		if n == "docs/inc1.yaml" {
			found1 = true
		}
		if n == "docs/inc2.yaml" {
			found2 = true
		}
	}
	if !found1 {
		t.Error("docs/inc1.yaml should be loaded via context_include")
	}
	if found2 {
		t.Error("docs/inc2.yaml should be excluded by context_exclude")
	}

	// Typed docs should still be loaded via ensureTypedDocs.
	if ctx.Vision == nil {
		t.Error("Vision should be loaded (ensureTypedDocs adds missing typed docs)")
	}
}

// ---------------------------------------------------------------------------
// Releases filtering integration tests
// ---------------------------------------------------------------------------

func TestBuildProjectContext_ReleasesFilter(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Create use cases in two releases.
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel02.0-uc002-extra.yaml",
		[]byte("id: rel02.0-uc002-extra\ntitle: Extra"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel03.0-uc003-future.yaml",
		[]byte("id: rel03.0-uc003-future\ntitle: Future"), 0o644)

	project := ProjectConfig{
		Releases: []string{"01.0", "03.0"},
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Should include rel01.0 and rel03.0 use cases, but NOT rel02.0.
	ucIDs := make(map[string]bool)
	for _, uc := range ctx.Specs.UseCases {
		ucIDs[uc.ID] = true
	}
	if !ucIDs["rel01.0-uc001-init"] {
		t.Error("expected rel01.0-uc001-init to be included")
	}
	if ucIDs["rel02.0-uc002-extra"] {
		t.Error("expected rel02.0-uc002-extra to be excluded (not in Releases)")
	}
	if !ucIDs["rel03.0-uc003-future"] {
		t.Error("expected rel03.0-uc003-future to be included")
	}
}

func TestBuildProjectContext_ReleaseLegacyBackwardCompat(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Create use cases in two releases.
	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel02.0-uc002-extra.yaml",
		[]byte("id: rel02.0-uc002-extra\ntitle: Extra"), 0o644)

	// Legacy single Release field: includes <= "01.0".
	project := ProjectConfig{
		Release: "01.0",
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	ucIDs := make(map[string]bool)
	for _, uc := range ctx.Specs.UseCases {
		ucIDs[uc.ID] = true
	}
	if !ucIDs["rel01.0-uc001-init"] {
		t.Error("expected rel01.0-uc001-init to be included (legacy Release)")
	}
	if ucIDs["rel02.0-uc002-extra"] {
		t.Error("expected rel02.0-uc002-extra to be excluded (> Release 01.0)")
	}
}

func TestBuildProjectContext_ReleasesOverridesRelease(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel02.0-uc002-extra.yaml",
		[]byte("id: rel02.0-uc002-extra\ntitle: Extra"), 0o644)

	// When both set, Releases takes precedence.
	project := ProjectConfig{
		Release:  "02.0",
		Releases: []string{"01.0"},
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	ucIDs := make(map[string]bool)
	for _, uc := range ctx.Specs.UseCases {
		ucIDs[uc.ID] = true
	}
	if !ucIDs["rel01.0-uc001-init"] {
		t.Error("expected rel01.0-uc001-init included (in Releases)")
	}
	if ucIDs["rel02.0-uc002-extra"] {
		t.Error("expected rel02.0-uc002-extra excluded (Releases takes precedence over Release)")
	}
}

func TestBuildProjectContext_EmptyReleasesIncludesAll(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel02.0-uc002-extra.yaml",
		[]byte("id: rel02.0-uc002-extra\ntitle: Extra"), 0o644)

	// No release filtering: both should be included.
	project := ProjectConfig{}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(ctx.Specs.UseCases) != 2 {
		t.Errorf("expected 2 use cases (no filtering), got %d", len(ctx.Specs.UseCases))
	}
}

func TestBuildProjectContext_PhaseContextOverridesReleases(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml",
		[]byte("id: rel01.0-uc001-init\ntitle: Init"), 0o644)
	os.WriteFile("docs/specs/use-cases/rel02.0-uc002-extra.yaml",
		[]byte("id: rel02.0-uc002-extra\ntitle: Extra"), 0o644)

	// Releases allows both, but PhaseContext narrows to 01.0.
	project := ProjectConfig{
		Releases: []string{"01.0", "02.0"},
	}
	phase := &PhaseContext{Release: "01.0"}

	ctx, err := buildProjectContext("", project, phase)
	if err != nil {
		t.Fatal(err)
	}

	ucIDs := make(map[string]bool)
	for _, uc := range ctx.Specs.UseCases {
		ucIDs[uc.ID] = true
	}
	if !ucIDs["rel01.0-uc001-init"] {
		t.Error("expected rel01.0-uc001-init included (PhaseContext release 01.0)")
	}
	if ucIDs["rel02.0-uc002-extra"] {
		t.Error("expected rel02.0-uc002-extra excluded (PhaseContext overrides to 01.0 max)")
	}
}

// ---------------------------------------------------------------------------
// ensureTypedDocs tests
// ---------------------------------------------------------------------------

func TestEnsureTypedDocs_AddsMissingDocs(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Start with an empty file list — ensureTypedDocs should add typed docs
	// that exist on disk.
	files := ensureTypedDocs(nil)

	// VISION, ARCHITECTURE, and road-map.yaml exist in the test fixture.
	found := make(map[string]bool)
	for _, f := range files {
		found[f] = true
	}
	if !found["docs/VISION.yaml"] {
		t.Error("ensureTypedDocs should add docs/VISION.yaml")
	}
	if !found["docs/ARCHITECTURE.yaml"] {
		t.Error("ensureTypedDocs should add docs/ARCHITECTURE.yaml")
	}
	if !found["docs/road-map.yaml"] {
		t.Error("ensureTypedDocs should add docs/road-map.yaml")
	}
}

func TestEnsureTypedDocs_DoesNotDuplicate(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Start with VISION already in the list.
	files := []string{"docs/VISION.yaml"}
	result := ensureTypedDocs(files)

	count := 0
	for _, f := range result {
		if f == "docs/VISION.yaml" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("VISION.yaml appears %d times, want 1 (no duplication)", count)
	}
}

func TestEnsureTypedDocs_SkipsMissingFiles(t *testing.T) {
	// Run in a temp dir where no typed docs exist.
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	files := ensureTypedDocs(nil)
	if len(files) != 0 {
		t.Errorf("got %d files, want 0 (no typed docs exist in temp dir)", len(files))
	}
}

// ---------------------------------------------------------------------------
// loadNamedDoc markdown handling tests
// ---------------------------------------------------------------------------

func TestLoadNamedDoc_MarkdownFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "do-work.md")
	// Markdown that fails YAML parsing (contains colons without proper quoting,
	// matching the real-world error reported in GH-53).
	content := "# Do Work\n\nUse this command:\n\n```bash\ncurl http://example.com\n```\n"
	os.WriteFile(mdPath, []byte(content), 0o644)

	doc := loadNamedDoc(mdPath)
	if doc == nil {
		t.Fatal("loadNamedDoc returned nil for markdown file")
	}
	if doc.Name != "do-work" {
		t.Errorf("Name = %q, want %q", doc.Name, "do-work")
	}
	if doc.Content.Kind != yaml.ScalarNode {
		t.Errorf("Content.Kind = %d, want ScalarNode (%d)", doc.Content.Kind, yaml.ScalarNode)
	}
	if !strings.Contains(doc.Content.Value, "# Do Work") {
		t.Errorf("Content.Value should contain markdown content, got %q", doc.Content.Value[:50])
	}
}

func TestLoadNamedDoc_TextFile(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "readme.txt")
	os.WriteFile(txtPath, []byte("plain text"), 0o644)

	doc := loadNamedDoc(txtPath)
	if doc == nil {
		t.Fatal("loadNamedDoc returned nil for .txt file")
	}
	if doc.Content.Value != "plain text" {
		t.Errorf("Content.Value = %q, want %q", doc.Content.Value, "plain text")
	}
}

func TestLoadNamedDoc_YAMLFileStillWorks(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(yamlPath, []byte("id: test\ntitle: Test Doc"), 0o644)

	doc := loadNamedDoc(yamlPath)
	if doc == nil {
		t.Fatal("loadNamedDoc returned nil for YAML file")
	}
	if doc.Name != "config" {
		t.Errorf("Name = %q, want %q", doc.Name, "config")
	}
	// YAML files should be parsed as mapping nodes, not scalar.
	if doc.Content.Kind == yaml.ScalarNode {
		t.Error("YAML file should not be loaded as scalar node")
	}
}

// ---------------------------------------------------------------------------
// classifyContextFile tests
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
		if got := classifyContextFile(tc.path); got != tc.want {
			t.Errorf("classifyContextFile(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// filterSourceFiles tests
// ---------------------------------------------------------------------------

func TestFilterSourceFiles_ExactMatch(t *testing.T) {
	sources := []SourceFile{
		{File: "pkg/orchestrator/stitch.go", Lines: "1 | package orchestrator"},
		{File: "pkg/orchestrator/context.go", Lines: "1 | package orchestrator"},
		{File: "pkg/orchestrator/config.go", Lines: "1 | package orchestrator"},
	}
	required := []string{"pkg/orchestrator/stitch.go", "pkg/orchestrator/context.go"}

	got := filterSourceFiles(sources, required)
	if len(got) != 2 {
		t.Fatalf("filterSourceFiles: got %d files, want 2", len(got))
	}
	if got[0].File != "pkg/orchestrator/stitch.go" {
		t.Errorf("got[0].File = %q, want stitch.go", got[0].File)
	}
	if got[1].File != "pkg/orchestrator/context.go" {
		t.Errorf("got[1].File = %q, want context.go", got[1].File)
	}
}

func TestFilterSourceFiles_SuffixMatch(t *testing.T) {
	sources := []SourceFile{
		{File: "/tmp/worktree/pkg/bar/foo.go", Lines: "1 | package bar"},
		{File: "/tmp/worktree/pkg/baz/other.go", Lines: "1 | package baz"},
	}
	required := []string{"pkg/bar/foo.go"}

	got := filterSourceFiles(sources, required)
	if len(got) != 1 {
		t.Fatalf("filterSourceFiles suffix: got %d files, want 1", len(got))
	}
	if got[0].File != "/tmp/worktree/pkg/bar/foo.go" {
		t.Errorf("got[0].File = %q, want foo.go path", got[0].File)
	}
}

func TestFilterSourceFiles_EmptyRequired(t *testing.T) {
	sources := []SourceFile{
		{File: "pkg/a.go"},
		{File: "pkg/b.go"},
		{File: "pkg/c.go"},
	}

	got := filterSourceFiles(sources, nil)
	if len(got) != 3 {
		t.Errorf("filterSourceFiles empty required: got %d, want 3 (all files)", len(got))
	}

	got2 := filterSourceFiles(sources, []string{})
	if len(got2) != 3 {
		t.Errorf("filterSourceFiles empty slice: got %d, want 3", len(got2))
	}
}

func TestFilterSourceFiles_NoMatch(t *testing.T) {
	sources := []SourceFile{
		{File: "pkg/a.go"},
		{File: "pkg/b.go"},
	}
	required := []string{"pkg/nonexistent.go"}

	got := filterSourceFiles(sources, required)
	if len(got) != 0 {
		t.Errorf("filterSourceFiles no match: got %d, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// stripParenthetical tests
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
		got := stripParenthetical(tt.input)
		if got != tt.want {
			t.Errorf("stripParenthetical(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseRequiredReading tests
// ---------------------------------------------------------------------------

func TestParseRequiredReading(t *testing.T) {
	desc := `deliverable_type: code
required_reading:
  - pkg/orchestrator/stitch.go (buildStitchPrompt)
  - pkg/orchestrator/context.go (buildProjectContext)
  - docs/engineering/eng05.md (recommendation D)
files:
  - path: pkg/orchestrator/stitch.go
    action: modify
`
	got := parseRequiredReading(desc)
	if len(got) != 3 {
		t.Fatalf("parseRequiredReading: got %d entries, want 3", len(got))
	}
	if got[0] != "pkg/orchestrator/stitch.go (buildStitchPrompt)" {
		t.Errorf("got[0] = %q", got[0])
	}
}

func TestParseRequiredReading_Empty(t *testing.T) {
	got := parseRequiredReading("")
	if got != nil {
		t.Errorf("parseRequiredReading empty: got %v, want nil", got)
	}
}

func TestParseRequiredReading_NoField(t *testing.T) {
	desc := "deliverable_type: code\nfiles: []\n"
	got := parseRequiredReading(desc)
	if len(got) != 0 {
		t.Errorf("parseRequiredReading no field: got %d, want 0", len(got))
	}
}

func TestParseRequiredReading_InvalidYAML(t *testing.T) {
	got := parseRequiredReading("not: [valid: yaml: {")
	if got != nil {
		t.Errorf("parseRequiredReading invalid: got %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// applyContextBudget tests
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

	// Set a budget smaller than the full context but large enough for one file.
	data, _ := yaml.Marshal(ctx)
	fullSize := len(data)
	budget := fullSize / 2

	applyContextBudget(ctx, budget, required)

	// a.go must be preserved (it's required).
	found := false
	for _, sf := range ctx.SourceCode {
		if sf.File == "pkg/a.go" {
			found = true
		}
	}
	if !found {
		t.Error("required file pkg/a.go was removed by budget enforcement")
	}

	// At least one non-required file should have been removed.
	if len(ctx.SourceCode) >= 3 {
		t.Errorf("expected some files to be removed, still have %d", len(ctx.SourceCode))
	}
}

func TestApplyContextBudget_ZeroBudget(t *testing.T) {
	ctx := &ProjectContext{
		SourceCode: []SourceFile{
			{File: "pkg/a.go", Lines: "package a"},
			{File: "pkg/b.go", Lines: "package b"},
		},
	}

	applyContextBudget(ctx, 0, nil)

	if len(ctx.SourceCode) != 2 {
		t.Errorf("zero budget should not remove files, got %d", len(ctx.SourceCode))
	}
}

func TestApplyContextBudget_PreservesRequired(t *testing.T) {
	// All files are required — none should be removed even if over budget.
	ctx := &ProjectContext{
		SourceCode: []SourceFile{
			{File: "pkg/a.go", Lines: strings.Repeat("x", 5000)},
			{File: "pkg/b.go", Lines: strings.Repeat("y", 5000)},
		},
	}
	required := []string{"pkg/a.go", "pkg/b.go"}

	applyContextBudget(ctx, 1, required) // impossibly small budget

	if len(ctx.SourceCode) != 2 {
		t.Errorf("all-required: expected 2 files preserved, got %d", len(ctx.SourceCode))
	}
}

func TestApplyContextBudget_UnderBudget(t *testing.T) {
	ctx := &ProjectContext{
		SourceCode: []SourceFile{
			{File: "pkg/a.go", Lines: "package a"},
		},
	}

	applyContextBudget(ctx, 1000000, nil)

	if len(ctx.SourceCode) != 1 {
		t.Errorf("under budget should not remove files, got %d", len(ctx.SourceCode))
	}
}

func TestApplyContextBudget_ExactlyAtLimit(t *testing.T) {
	ctx := &ProjectContext{
		SourceCode: []SourceFile{
			{File: "pkg/a.go", Lines: "package a"},
		},
	}
	data, _ := yaml.Marshal(ctx)
	exactSize := len(data)

	applyContextBudget(ctx, exactSize, nil)

	if len(ctx.SourceCode) != 1 {
		t.Errorf("at-limit: expected 1 file, got %d", len(ctx.SourceCode))
	}
}

func TestApplyContextBudget_NilContext(t *testing.T) {
	// Should not panic.
	applyContextBudget(nil, 100, nil)
}

func TestContextExcludeEverything(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Exclude "." — everything in the working directory.
	project := ProjectConfig{
		GoSourceDirs:   []string{},
		ContextExclude: ".",
	}

	ctx, err := buildProjectContext("", project, nil)
	if err != nil {
		t.Fatal(err)
	}

	// With "." excluded, no standard docs should be loaded.
	if ctx.Vision != nil {
		t.Error("Vision should be nil with context_exclude='.'")
	}
	if ctx.Architecture != nil {
		t.Error("Architecture should be nil with context_exclude='.'")
	}
	if ctx.Roadmap != nil {
		t.Error("Roadmap should be nil with context_exclude='.'")
	}
	if len(ctx.SourceCode) > 0 {
		t.Errorf("SourceCode should be empty with context_exclude='.', got %d", len(ctx.SourceCode))
	}
	if len(ctx.Extra) > 0 {
		t.Errorf("Extra should be empty with context_exclude='.', got %d", len(ctx.Extra))
	}
}
