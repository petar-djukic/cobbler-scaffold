// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

// --- parseStitchComment (GH-571) ---

func TestParseStitchComment_Completed(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
	d := parseStitchComment(body)
	if d.costUSD != 0.42 {
		t.Errorf("costUSD = %v, want 0.42", d.costUSD)
	}
	if d.durationS != 5*60+32 {
		t.Errorf("durationS = %d, want %d", d.durationS, 5*60+32)
	}
}

func TestParseStitchComment_Failed(t *testing.T) {
	t.Parallel()
	body := "Stitch failed after 2m 10s. Error: Claude failure."
	d := parseStitchComment(body)
	if d.durationS != 2*60+10 {
		t.Errorf("durationS = %d, want %d", d.durationS, 2*60+10)
	}
	if d.costUSD != 0 {
		t.Errorf("costUSD = %v, want 0", d.costUSD)
	}
}

func TestParseStitchComment_SubMinuteDuration(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 45s. LOC delta: +0 prod, +0 test. Cost: $0.10."
	d := parseStitchComment(body)
	if d.durationS != 45 {
		t.Errorf("durationS = %d, want 45", d.durationS)
	}
	if d.costUSD != 0.10 {
		t.Errorf("costUSD = %v, want 0.10", d.costUSD)
	}
}

func TestParseStitchComment_NoMatch(t *testing.T) {
	t.Parallel()
	d := parseStitchComment("unrelated comment text")
	if d.costUSD != 0 || d.durationS != 0 {
		t.Errorf("expected zero values, got cost=%v dur=%d", d.costUSD, d.durationS)
	}
}

// --- extractPRDRefs (GH-571) ---

func TestExtractPRDRefs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text string
		want []string
	}{
		{
			text: "Implement prd-auth-flow for login",
			want: []string{"prd-auth-flow"},
		},
		{
			text: "Covers prd-user-model, prd-auth-flow.",
			want: []string{"prd-user-model", "prd-auth-flow"},
		},
		{
			text: "no prd references here",
			want: nil,
		},
		{
			text: "prd- alone is not a ref",
			want: nil,
		},
		{
			// Duplicates should be deduplicated.
			text: "prd-foo prd-bar prd-foo",
			want: []string{"prd-foo", "prd-bar"},
		},
	}
	for _, tc := range tests {
		got := extractPRDRefs(tc.text)
		if len(got) != len(tc.want) {
			t.Errorf("extractPRDRefs(%q): got %v, want %v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("extractPRDRefs(%q)[%d]: got %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}
}

// --- listAllCobblerIssues / fetchIssueComments (GH-571) ---

// TestListAllCobblerIssues_FakeRepo_Error verifies listAllCobblerIssues returns
// an error (not panic) when the GitHub CLI fails on a fake repo (GH-571).
func TestListAllCobblerIssues_FakeRepo_Error(t *testing.T) {
	t.Parallel()
	_, err := listAllCobblerIssues("fake/repo-that-does-not-exist", "gen-test")
	if err == nil {
		t.Error("listAllCobblerIssues with fake repo must return an error")
	}
}

// TestFetchIssueComments_FakeRepo_Error verifies fetchIssueComments returns
// an error (not panic) when the GitHub CLI fails on a fake repo (GH-571).
func TestFetchIssueComments_FakeRepo_Error(t *testing.T) {
	t.Parallel()
	_, err := fetchIssueComments("fake/repo-that-does-not-exist", 99999)
	if err == nil {
		t.Error("fetchIssueComments with fake repo must return an error")
	}
}

// TestParseCobblerIssuesJSON_State verifies that the State field is populated
// from the JSON response (GH-571).
func TestParseCobblerIssuesJSON_State(t *testing.T) {
	t.Parallel()
	data := []byte(`[
		{"number": 1, "title": "Open task", "state": "open", "body": "", "labels": []},
		{"number": 2, "title": "Done task", "state": "closed", "body": "", "labels": []}
	]`)
	issues, err := parseCobblerIssuesJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("want 2 issues, got %d", len(issues))
	}
	if issues[0].State != "open" {
		t.Errorf("issues[0].State = %q, want \"open\"", issues[0].State)
	}
	if issues[1].State != "closed" {
		t.Errorf("issues[1].State = %q, want \"closed\"", issues[1].State)
	}
}

// --- countTotalPRDRequirements (GH-989) ---

func TestCountTotalPRDRequirements(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	if err := os.MkdirAll(prdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prdContent := `name: test-prd
requirements:
  group-a:
    description: Group A
    items:
      - id: REQ-001
        text: First requirement
      - id: REQ-002
        text: Second requirement
  group-b:
    description: Group B
    items:
      - id: REQ-003
        text: Third requirement
`
	if err := os.WriteFile(filepath.Join(prdDir, "prd001-test.yaml"), []byte(prdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	total, byPRD := countTotalPRDRequirements()
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if byPRD["prd-001"] != 3 {
		t.Errorf("byPRD[prd-001] = %d, want 3", byPRD["prd-001"])
	}
}

func TestCountTotalPRDRequirements_NoPRDs(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	total, byPRD := countTotalPRDRequirements()
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(byPRD) != 0 {
		t.Errorf("byPRD = %v, want empty", byPRD)
	}
}
