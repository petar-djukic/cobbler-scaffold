// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseIssueFrontMatter verifies round-trip parsing of the YAML
// front-matter block embedded in issue bodies.
func TestParseIssueFrontMatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantGen    string
		wantIndex  int
		wantDep    int
		wantDesc   string
	}{
		{
			name: "no dependency",
			body: "---\ncobbler_generation: gen-2026-02-28-001\ncobbler_index: 1\n---\n\nSome description",
			wantGen:   "gen-2026-02-28-001",
			wantIndex: 1,
			wantDep:   -1,
			wantDesc:  "Some description",
		},
		{
			name: "with dependency",
			body: "---\ncobbler_generation: gen-2026-02-28-001\ncobbler_index: 3\ncobbler_depends_on: 2\n---\n\nAnother description",
			wantGen:   "gen-2026-02-28-001",
			wantIndex: 3,
			wantDep:   2,
			wantDesc:  "Another description",
		},
		{
			name:      "no front-matter",
			body:      "Plain body without front-matter",
			wantGen:   "",
			wantIndex: 0,
			wantDep:   -1,
			wantDesc:  "Plain body without front-matter",
		},
		{
			name: "empty description",
			body: "---\ncobbler_generation: gen-abc\ncobbler_index: 5\n---\n\n",
			wantGen:   "gen-abc",
			wantIndex: 5,
			wantDep:   -1,
			wantDesc:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fm, desc := parseIssueFrontMatter(tc.body)
			if fm.Generation != tc.wantGen {
				t.Errorf("Generation: got %q want %q", fm.Generation, tc.wantGen)
			}
			if fm.Index != tc.wantIndex {
				t.Errorf("Index: got %d want %d", fm.Index, tc.wantIndex)
			}
			if fm.DependsOn != tc.wantDep {
				t.Errorf("DependsOn: got %d want %d", fm.DependsOn, tc.wantDep)
			}
			if desc != tc.wantDesc {
				t.Errorf("Description: got %q want %q", desc, tc.wantDesc)
			}
		})
	}
}

// TestFormatIssueFrontMatter verifies that formatted front-matter round-trips
// through parseIssueFrontMatter correctly.
func TestFormatIssueFrontMatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		generation string
		index      int
		dependsOn  int
	}{
		{"no dep", "gen-2026-02-28-001", 1, -1},
		{"with dep", "gen-2026-02-28-001", 3, 2},
		{"dep zero", "gen-abc", 2, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			desc := "Test description content"
			body := formatIssueFrontMatter(tc.generation, tc.index, tc.dependsOn) + desc
			fm, parsedDesc := parseIssueFrontMatter(body)

			if fm.Generation != tc.generation {
				t.Errorf("Generation round-trip: got %q want %q", fm.Generation, tc.generation)
			}
			if fm.Index != tc.index {
				t.Errorf("Index round-trip: got %d want %d", fm.Index, tc.index)
			}
			if fm.DependsOn != tc.dependsOn {
				t.Errorf("DependsOn round-trip: got %d want %d", fm.DependsOn, tc.dependsOn)
			}
			if parsedDesc != desc {
				t.Errorf("Description round-trip: got %q want %q", parsedDesc, desc)
			}
		})
	}
}

// TestCobblerGenLabel verifies label name construction.
func TestCobblerGenLabel(t *testing.T) {
	t.Parallel()
	got := cobblerGenLabel("gen-2026-02-28-001")
	want := "cobbler-gen-gen-2026-02-28-001"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestDetectGitHubRepoFromConfig verifies that IssuesRepo config override
// is returned directly without running any external commands.
func TestDetectGitHubRepoFromConfig(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Cobbler.IssuesRepo = "owner/repo"
	got, err := detectGitHubRepo(t.TempDir(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/repo" {
		t.Errorf("got %q want %q", got, "owner/repo")
	}
}

// TestDetectGitHubRepoFromModulePath verifies fallback to go.mod parsing.
func TestDetectGitHubRepoFromModulePath(t *testing.T) {
	t.Parallel()

	// Create a temp dir with a go.mod.
	dir := t.TempDir()
	gomod := "module github.com/acme/myproject\n\ngo 1.22\n"
	if err := writeFileForTest(dir+"/go.mod", gomod); err != nil {
		t.Fatal(err)
	}

	cfg := Config{}
	cfg.Project.ModulePath = "github.com/acme/myproject"
	// Pass non-existent dir so gh repo view fails → falls through to module path.
	got, err := detectGitHubRepo(dir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "acme/myproject" {
		t.Errorf("got %q want %q", got, "acme/myproject")
	}
}

// TestHasLabel verifies label lookup on a cobblerIssue.
func TestHasLabel(t *testing.T) {
	t.Parallel()
	iss := cobblerIssue{Labels: []string{"cobbler-ready", "cobbler-gen-abc"}}
	if !hasLabel(iss, "cobbler-ready") {
		t.Error("expected to find cobbler-ready")
	}
	if hasLabel(iss, "cobbler-in-progress") {
		t.Error("did not expect cobbler-in-progress")
	}
}

// TestDAGPromotion tests the DAG logic directly by simulating what
// promoteReadyIssues would decide — which issues are blocked vs. unblocked.
func TestDAGPromotion(t *testing.T) {
	t.Parallel()

	// Build a chain: 1 → 2 → 3. Only issue 1 has no dep → unblocked.
	// Issue 2 depends on 1 (open) → blocked.
	// Issue 3 depends on 2 (open) → blocked.
	issues := []cobblerIssue{
		{Number: 10, Index: 1, DependsOn: -1},
		{Number: 11, Index: 2, DependsOn: 1},
		{Number: 12, Index: 3, DependsOn: 2},
	}

	openIndices := map[int]bool{}
	for _, iss := range issues {
		openIndices[iss.Index] = true
	}

	wantBlocked := map[int]bool{10: false, 11: true, 12: true}
	for _, iss := range issues {
		blocked := iss.DependsOn >= 0 && openIndices[iss.DependsOn]
		if blocked != wantBlocked[iss.Number] {
			t.Errorf("issue #%d blocked=%v want=%v", iss.Number, blocked, wantBlocked[iss.Number])
		}
	}
}

// TestDAGPromotionDepClosed tests that once dep is closed (gone from openIndices),
// the dependent issue becomes unblocked.
func TestDAGPromotionDepClosed(t *testing.T) {
	t.Parallel()

	// Only issue 2 remains open; its dependency (index 1) is closed.
	issues := []cobblerIssue{
		{Number: 11, Index: 2, DependsOn: 1},
	}

	openIndices := map[int]bool{2: true} // index 1 is gone (closed)
	for _, iss := range issues {
		blocked := iss.DependsOn >= 0 && openIndices[iss.DependsOn]
		if blocked {
			t.Errorf("issue #%d should be unblocked when dep is closed", iss.Number)
		}
	}
}

// writeFileForTest is a test helper that writes content to path.
func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// --- goModModulePath ---

func TestGoModModulePath_ValidGoMod(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/org/repo\n\ngo 1.23\n"), 0o644)
	got := goModModulePath(dir)
	if got != "github.com/org/repo" {
		t.Errorf("goModModulePath = %q, want github.com/org/repo", got)
	}
}

func TestGoModModulePath_MissingFile(t *testing.T) {
	t.Parallel()
	got := goModModulePath(t.TempDir())
	if got != "" {
		t.Errorf("goModModulePath = %q, want empty for missing go.mod", got)
	}
}

func TestGoModModulePath_NoModuleLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.23\n"), 0o644)
	got := goModModulePath(dir)
	if got != "" {
		t.Errorf("goModModulePath = %q, want empty for go.mod without module line", got)
	}
}
