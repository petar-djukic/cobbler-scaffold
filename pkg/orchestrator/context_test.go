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
		got := fileMatchesRelease(tt.path, tt.release)
		if got != tt.want {
			t.Errorf("fileMatchesRelease(%q, %q) = %v, want %v",
				tt.path, tt.release, got, tt.want)
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
	loadContextFileInto(ctx, "docs/VISION.yaml", "")
	loadContextFileInto(ctx, "docs/ARCHITECTURE.yaml", "")
	loadContextFileInto(ctx, "docs/road-map.yaml", "")

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

	ctx, err := buildProjectContext("", project)
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

	ctx, err := buildProjectContext("", project)
	if err != nil {
		t.Fatal(err)
	}

	// Standard files should NOT be loaded since context_include replaces them.
	if ctx.Vision != nil {
		t.Error("Vision should be nil when context_include replaces standard files")
	}
	if ctx.Architecture != nil {
		t.Error("Architecture should be nil when context_include replaces standard files")
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

	ctx, err := buildProjectContext("", project)
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

	ctx, err := buildProjectContext("", project)
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

	// Standard files should NOT be loaded (replaced by include).
	if ctx.Vision != nil {
		t.Error("Vision should be nil when context_include is set")
	}
}

func TestContextExcludeEverything(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Exclude "." — everything in the working directory.
	project := ProjectConfig{
		GoSourceDirs:   []string{},
		ContextExclude: ".",
	}

	ctx, err := buildProjectContext("", project)
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
