// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// --- resolveTargetRepo ---

func TestResolveTargetRepo_ExplicitTargetRepo(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Project.TargetRepo = "owner/target-project"
	cfg.Project.ModulePath = "github.com/owner/other" // ignored when TargetRepo set

	got := resolveTargetRepo(cfg)
	if got != "owner/target-project" {
		t.Errorf("got %q, want %q", got, "owner/target-project")
	}
}

func TestResolveTargetRepo_FallbackToModulePath(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Project.ModulePath = "github.com/acme/sdd-hello-world"

	got := resolveTargetRepo(cfg)
	if got != "acme/sdd-hello-world" {
		t.Errorf("got %q, want %q", got, "acme/sdd-hello-world")
	}
}

func TestResolveTargetRepo_NonGitHub(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Project.ModulePath = "gitlab.com/org/project"

	got := resolveTargetRepo(cfg)
	if got != "" {
		t.Errorf("got %q, want empty for non-github module path", got)
	}
}

func TestResolveTargetRepo_Empty(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	got := resolveTargetRepo(cfg)
	if got != "" {
		t.Errorf("got %q, want empty when nothing configured", got)
	}
}

// --- parseIssueURL ---

func TestParseIssueURL_ValidURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{"standard URL", "https://github.com/owner/repo/issues/123\n", 123},
		{"no trailing newline", "https://github.com/owner/repo/issues/42", 42},
		{"whitespace padded", "  https://github.com/owner/repo/issues/7  \n", 7},
		{"large number", "https://github.com/org/project/issues/99999\n", 99999},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseIssueURL(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseIssueURL_InvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"only whitespace", "  \n  "},
		{"error message", "Error: could not create issue"},
		{"short path", "github.com/issues/123"},
		{"no number at end", "https://github.com/owner/repo/issues/abc"},
		{"zero issue number", "https://github.com/owner/repo/issues/0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseIssueURL(tc.raw)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

// --- parseCobblerIssuesJSON ---

func TestParseIssuesJSON_ValidJSON(t *testing.T) {
	t.Parallel()

	input := `[
		{
			"number": 10,
			"title": "Task 1",
			"body": "---\ncobbler_generation: gen-001\ncobbler_index: 1\n---\n\nDo something",
			"labels": [{"name": "cobbler-gen-gen-001"}, {"name": "cobbler-ready"}]
		},
		{
			"number": 11,
			"title": "Task 2",
			"body": "---\ncobbler_generation: gen-001\ncobbler_index: 2\ncobbler_depends_on: 1\n---\n\nDo something else",
			"labels": [{"name": "cobbler-gen-gen-001"}]
		}
	]`

	issues, err := parseCobblerIssuesJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2", len(issues))
	}

	// Check first issue.
	if issues[0].Number != 10 {
		t.Errorf("issue[0].Number = %d, want 10", issues[0].Number)
	}
	if issues[0].Index != 1 {
		t.Errorf("issue[0].Index = %d, want 1", issues[0].Index)
	}
	if issues[0].DependsOn != -1 {
		t.Errorf("issue[0].DependsOn = %d, want -1", issues[0].DependsOn)
	}
	if issues[0].Description != "Do something" {
		t.Errorf("issue[0].Description = %q, want %q", issues[0].Description, "Do something")
	}
	if len(issues[0].Labels) != 2 {
		t.Errorf("issue[0].Labels = %v, want 2 labels", issues[0].Labels)
	}

	// Check second issue with dependency.
	if issues[1].DependsOn != 1 {
		t.Errorf("issue[1].DependsOn = %d, want 1", issues[1].DependsOn)
	}
}

func TestParseIssuesJSON_EmptyArray(t *testing.T) {
	t.Parallel()
	issues, err := parseCobblerIssuesJSON([]byte("[]"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("got %d issues, want 0", len(issues))
	}
}

func TestParseIssuesJSON_MalformedJSON(t *testing.T) {
	t.Parallel()
	_, err := parseCobblerIssuesJSON([]byte("{not valid json"))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseIssuesJSON_NoFrontMatter(t *testing.T) {
	t.Parallel()
	input := `[{"number": 5, "title": "Plain issue", "body": "No front matter here", "labels": []}]`
	issues, err := parseCobblerIssuesJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(issues))
	}
	if issues[0].Index != 0 {
		t.Errorf("Index = %d, want 0 for missing front-matter", issues[0].Index)
	}
	if issues[0].Generation != "" {
		t.Errorf("Generation = %q, want empty for missing front-matter", issues[0].Generation)
	}
}

func TestParseIssuesJSON_NoLabels(t *testing.T) {
	t.Parallel()
	input := `[{"number": 1, "title": "No labels", "body": "", "labels": []}]`
	issues, err := parseCobblerIssuesJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues[0].Labels) != 0 {
		t.Errorf("Labels = %v, want empty", issues[0].Labels)
	}
}

// --- DAG promotion edge cases ---

func TestDAGPromotion_DiamondDependency(t *testing.T) {
	t.Parallel()

	// Diamond: 1 has no dep, 2 and 3 depend on 1, 4 depends on both 2 and 3.
	// Since cobbler_depends_on is a single value, 4 depends on 3 (the higher index).
	// When 1 is open: 2 blocked, 3 blocked, 4 blocked.
	issues := []cobblerIssue{
		{Number: 10, Index: 1, DependsOn: -1},
		{Number: 11, Index: 2, DependsOn: 1},
		{Number: 12, Index: 3, DependsOn: 1},
		{Number: 13, Index: 4, DependsOn: 3},
	}

	openIndices := map[int]bool{1: true, 2: true, 3: true, 4: true}
	for _, iss := range issues {
		blocked := iss.DependsOn >= 0 && openIndices[iss.DependsOn]
		switch iss.Number {
		case 10:
			if blocked {
				t.Error("issue #10 (no dep) should not be blocked")
			}
		case 11, 12:
			if !blocked {
				t.Errorf("issue #%d (depends on 1, which is open) should be blocked", iss.Number)
			}
		case 13:
			if !blocked {
				t.Error("issue #13 (depends on 3, which is open) should be blocked")
			}
		}
	}
}

func TestDAGPromotion_AllDepsResolved(t *testing.T) {
	t.Parallel()

	// All dependencies are closed (not in openIndices).
	issues := []cobblerIssue{
		{Number: 20, Index: 3, DependsOn: 2},
		{Number: 21, Index: 4, DependsOn: 3},
	}

	openIndices := map[int]bool{3: true, 4: true} // 1 and 2 are closed
	for _, iss := range issues {
		blocked := iss.DependsOn >= 0 && openIndices[iss.DependsOn]
		if iss.Number == 20 && blocked {
			t.Error("issue #20 (dep 2 closed) should be unblocked")
		}
		if iss.Number == 21 && !blocked {
			t.Error("issue #21 (dep 3 still open) should be blocked")
		}
	}
}

func TestIssuesContextJSON_Empty(t *testing.T) {
	t.Parallel()
	result, err := issuesContextJSON(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "[]" {
		t.Errorf("issuesContextJSON(nil) = %q, want %q", result, "[]")
	}
}

func TestIssuesContextJSON_StatusMapping(t *testing.T) {
	t.Parallel()
	issues := []cobblerIssue{
		{Number: 10, Title: "Task A", Labels: []string{cobblerLabelReady}},
		{Number: 11, Title: "Task B", Labels: []string{cobblerLabelInProgress}},
		{Number: 12, Title: "Task C", Labels: []string{}},
	}
	result, err := issuesContextJSON(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []ContextIssue
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("issuesContextJSON produced invalid JSON: %v\noutput: %s", err, result)
	}
	if len(got) != 3 {
		t.Fatalf("got %d issues, want 3", len(got))
	}

	cases := []struct{ id, title, status string }{
		{"10", "Task A", "ready"},
		{"11", "Task B", "in_progress"},
		{"12", "Task C", "backfill"},
	}
	for i, c := range cases {
		if got[i].ID != c.id {
			t.Errorf("[%d] ID = %q, want %q", i, got[i].ID, c.id)
		}
		if got[i].Title != c.title {
			t.Errorf("[%d] Title = %q, want %q", i, got[i].Title, c.title)
		}
		if got[i].Status != c.status {
			t.Errorf("[%d] Status = %q, want %q", i, got[i].Status, c.status)
		}
	}
}

func TestIssuesContextJSON_ParseableByParseIssuesJSON(t *testing.T) {
	t.Parallel()
	issues := []cobblerIssue{
		{Number: 115, Title: "cmd/wc core implementation", Labels: []string{cobblerLabelReady}},
	}
	jsonStr, err := issuesContextJSON(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the output is parseable by parseIssuesJSON (the function that was broken).
	parsed := parseIssuesJSON(jsonStr)
	if len(parsed) != 1 {
		t.Fatalf("parseIssuesJSON returned %d issues, want 1; input: %s", len(parsed), jsonStr)
	}
	if parsed[0].ID != "115" {
		t.Errorf("ID = %q, want %q", parsed[0].ID, "115")
	}
	if !strings.Contains(jsonStr, "cmd/wc core implementation") {
		t.Errorf("JSON does not contain expected title: %s", jsonStr)
	}
}

// --- pickReadyIssue label invariant ---

// TestPickReadyIssue_FilterExcludesBothLabels verifies that an issue carrying
// both cobbler-ready and cobbler-in-progress is not eligible for selection.
// This mirrors the guard that prevents re-claiming an already-in-progress task
// even if the ready label was not yet removed (GH-569).
func TestPickReadyIssue_FilterExcludesBothLabels(t *testing.T) {
	t.Parallel()

	// An issue that somehow has both labels should be excluded from the ready set.
	bothLabels := cobblerIssue{Number: 10, Labels: []string{cobblerLabelReady, cobblerLabelInProgress}}
	readyOnly := cobblerIssue{Number: 11, Labels: []string{cobblerLabelReady}}

	isEligible := func(iss cobblerIssue) bool {
		return hasLabel(iss, cobblerLabelReady) && !hasLabel(iss, cobblerLabelInProgress)
	}

	if isEligible(bothLabels) {
		t.Error("issue with both ready and in-progress labels must not be eligible for pick")
	}
	if !isEligible(readyOnly) {
		t.Error("issue with only ready label must be eligible for pick")
	}
}

// TestCloseCobblerIssue_FakeRepo_NoOp verifies closeCobblerIssue returns an
// error (not panic) when the GitHub CLI fails on a fake repo (GH-569).
func TestCloseCobblerIssue_FakeRepo_NoOp(t *testing.T) {
	t.Parallel()
	err := closeCobblerIssue("fake/repo-that-does-not-exist", 99999, "gen-test")
	if err == nil {
		t.Error("closeCobblerIssue with fake repo must return an error")
	}
}

// --- measuring placeholder (GH-568) ---

// TestCreateMeasuringPlaceholder_FakeRepo_Error verifies createMeasuringPlaceholder
// returns an error (not panic) when the GitHub CLI fails on a fake repo (GH-568).
func TestCreateMeasuringPlaceholder_FakeRepo_Error(t *testing.T) {
	t.Parallel()
	_, err := createMeasuringPlaceholder("fake/repo-that-does-not-exist", "gen-test", 1)
	if err == nil {
		t.Error("createMeasuringPlaceholder with fake repo must return an error")
	}
}

// TestCloseMeasuringPlaceholder_FakeRepo_NoOp verifies closeMeasuringPlaceholder
// does not panic when the GitHub CLI fails on a fake repo (GH-568).
func TestCloseMeasuringPlaceholder_FakeRepo_NoOp(t *testing.T) {
	t.Parallel()
	closeMeasuringPlaceholder("fake/repo-that-does-not-exist", 99999) // must not panic
}

// --- progress comments (GH-567) ---

// TestCommentCobblerIssue_FakeRepo_NoOp verifies commentCobblerIssue does not
// panic when the GitHub CLI fails on a fake repo (GH-567).
func TestCommentCobblerIssue_FakeRepo_NoOp(t *testing.T) {
	t.Parallel()
	commentCobblerIssue("fake/repo-that-does-not-exist", 99999, "test body") // must not panic
}

// TestCommentCobblerIssue_ZeroNumber_NoOp verifies commentCobblerIssue is a
// no-op for invalid inputs (GH-567).
func TestCommentCobblerIssue_ZeroNumber_NoOp(t *testing.T) {
	t.Parallel()
	commentCobblerIssue("petar-djukic/cobbler-scaffold", 0, "test body")  // must not panic
	commentCobblerIssue("", 1, "test body")                                // must not panic
}
