// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
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
		"docs/engineering/eng01-guide.yaml",
	}
	for _, f := range standardFiles {
		os.WriteFile(f, []byte("id: test"), 0o644)
	}

	// Create files that should NOT be included.
	excluded := []string{
		"docs/constitutions/planning.yaml",
		"docs/constitutions/go-style.yaml",
		"docs/utilities.yaml",
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
