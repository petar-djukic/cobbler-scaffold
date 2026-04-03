// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// --- parseStitchComment (delegation sanity check) ---

func TestParseStitchComment_Completed(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
	d := st.ParseStitchComment(body)
	if d.CostUSD != 0.42 {
		t.Errorf("CostUSD = %v, want 0.42", d.CostUSD)
	}
	if d.DurationS != 5*60+32 {
		t.Errorf("DurationS = %d, want %d", d.DurationS, 5*60+32)
	}
	if d.LocDeltaProd != 45 {
		t.Errorf("LocDeltaProd = %d, want 45", d.LocDeltaProd)
	}
	if d.LocDeltaTest != 17 {
		t.Errorf("LocDeltaTest = %d, want 17", d.LocDeltaTest)
	}
}

func TestParseStitchComment_Failed(t *testing.T) {
	t.Parallel()
	body := "Stitch failed after 2m 10s. Error: Claude failure."
	d := st.ParseStitchComment(body)
	if d.DurationS != 2*60+10 {
		t.Errorf("DurationS = %d, want %d", d.DurationS, 2*60+10)
	}
	if d.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want 0", d.CostUSD)
	}
}

func TestParseStitchComment_SubMinuteDuration(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 45s. LOC delta: +0 prod, +0 test. Cost: $0.10."
	d := st.ParseStitchComment(body)
	if d.DurationS != 45 {
		t.Errorf("DurationS = %d, want 45", d.DurationS)
	}
	if d.CostUSD != 0.10 {
		t.Errorf("CostUSD = %v, want 0.10", d.CostUSD)
	}
}

func TestParseStitchComment_WithTurns(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 3m 15s. LOC delta: +20 prod, +10 test. Cost: $0.55. Turns: 12."
	d := st.ParseStitchComment(body)
	if d.NumTurns != 12 {
		t.Errorf("NumTurns = %d, want 12", d.NumTurns)
	}
	if d.CostUSD != 0.55 {
		t.Errorf("CostUSD = %v, want 0.55", d.CostUSD)
	}
	if d.LocDeltaProd != 20 {
		t.Errorf("LocDeltaProd = %d, want 20", d.LocDeltaProd)
	}
	if d.LocDeltaTest != 10 {
		t.Errorf("LocDeltaTest = %d, want 10", d.LocDeltaTest)
	}
}

func TestParseStitchComment_NegativeLOC(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 1m 5s. LOC delta: -12 prod, +30 test. Cost: $0.20. Turns: 5."
	d := st.ParseStitchComment(body)
	if d.LocDeltaProd != -12 {
		t.Errorf("LocDeltaProd = %d, want -12", d.LocDeltaProd)
	}
	if d.LocDeltaTest != 30 {
		t.Errorf("LocDeltaTest = %d, want 30", d.LocDeltaTest)
	}
}

func TestParseStitchComment_NoMatch(t *testing.T) {
	t.Parallel()
	d := st.ParseStitchComment("unrelated comment text")
	if d.CostUSD != 0 || d.DurationS != 0 || d.NumTurns != 0 {
		t.Errorf("expected zero values, got cost=%v dur=%d turns=%d", d.CostUSD, d.DurationS, d.NumTurns)
	}
}

func TestParseStitchComment_PromptBytes(t *testing.T) {
	t.Parallel()
	body := "Stitch started. Branch: `generation-main`, prompt: 524288 bytes."
	d := st.ParseStitchComment(body)
	if d.PromptBytes != 524288 {
		t.Errorf("PromptBytes = %d, want 524288", d.PromptBytes)
	}
}

func TestParseStitchComment_PromptBytes_NoMatch(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
	d := st.ParseStitchComment(body)
	if d.PromptBytes != 0 {
		t.Errorf("PromptBytes = %d, want 0", d.PromptBytes)
	}
}

func TestParseStitchComment_Tokens(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 3m 15s. LOC delta: +20 prod, +10 test. Cost: $0.55. Turns: 12. Tokens: 125000in 5000out."
	d := st.ParseStitchComment(body)
	if d.InputTokens != 125000 {
		t.Errorf("InputTokens = %d, want 125000", d.InputTokens)
	}
	if d.OutputTokens != 5000 {
		t.Errorf("OutputTokens = %d, want 5000", d.OutputTokens)
	}
}

func TestParseStitchComment_Tokens_NoMatch(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
	d := st.ParseStitchComment(body)
	if d.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", d.InputTokens)
	}
	if d.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", d.OutputTokens)
	}
}

// --- formatBytes (delegation sanity check) ---

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		bytes int
		want  string
	}{
		{"megabytes", 1_500_000, "1.5M"},
		{"exactly 1M", 1_000_000, "1.0M"},
		{"kilobytes", 524288, "524K"},
		{"small kilobytes", 1000, "1K"},
		{"bytes", 999, "999B"},
		{"zero", 0, "0B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := st.FormatBytes(tc.bytes)
			if got != tc.want {
				t.Errorf("st.FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// --- formatTokens (delegation sanity check) ---

func TestFormatTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tokens int
		want   string
	}{
		{"millions", 1_500_000, "1.5M"},
		{"exactly 1M", 1_000_000, "1.0M"},
		{"thousands", 125000, "125K"},
		{"small thousands", 1000, "1K"},
		{"small", 999, "999"},
		{"zero", 0, "0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := st.FormatTokens(tc.tokens)
			if got != tc.want {
				t.Errorf("st.FormatTokens(%d) = %q, want %q", tc.tokens, got, tc.want)
			}
		})
	}
}

// --- extractRelease (delegation sanity check) ---

func TestExtractRelease(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want string
	}{
		{"title with rel", "cmd/tee implementation (rel02.1-uc001-tee)", "02.1"},
		{"rel01.0", "[stitch] prd001: Implement Foo (rel01.0-uc003)", "01.0"},
		{"no release", "prd001: Implement Foo", ""},
		{"plain text", "no release info here", ""},
		{"multiple releases", "rel01.0 and rel02.1", "01.0"},
		{"embedded in word", "xrel03.0y", "03.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := st.ExtractRelease(tc.text)
			if got != tc.want {
				t.Errorf("st.ExtractRelease(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// --- extractPRDRefs (delegation sanity check) ---

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
			text: "prd-foo prd-bar prd-foo",
			want: []string{"prd-foo", "prd-bar"},
		},
	}
	for _, tc := range tests {
		got := st.ExtractPRDRefs(tc.text)
		if len(got) != len(tc.want) {
			t.Errorf("st.ExtractPRDRefs(%q): got %v, want %v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("st.ExtractPRDRefs(%q)[%d]: got %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}
}

// --- testOrch().tracker.ListAllCobblerIssues (delegation sanity check) ---

func TestListAllCobblerIssues_FakeRepo_Error(t *testing.T) {
	t.Parallel()
	_, err := testOrch().tracker.ListAllCobblerIssues("fake/repo-that-does-not-exist", "gen-test")
	if err == nil {
		t.Error("ListAllCobblerIssues with fake repo must return an error")
	}
}

func TestParseCobblerIssuesJSON_State(t *testing.T) {
	t.Parallel()
	data := []byte(`[
		{"number": 1, "title": "Open task", "state": "open", "body": "", "labels": []},
		{"number": 2, "title": "Done task", "state": "closed", "body": "", "labels": []}
	]`)
	issues, err := gh.ParseCobblerIssuesJSON(data)
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

// --- countTotalPRDRequirements (delegation sanity check) ---

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

	total, byPRD := st.CountTotalPRDRequirements()
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if byPRD["prd001-test"] != 3 {
		t.Errorf("byPRD[prd001-test] = %d, want 3", byPRD["prd001-test"])
	}
}

func TestCountTotalPRDRequirements_NoPRDs(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	total, byPRD := st.CountTotalPRDRequirements()
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(byPRD) != 0 {
		t.Errorf("byPRD = %v, want empty", byPRD)
	}
}

// --- countDescriptionReqs (delegation sanity check) ---

func TestCountDescriptionReqs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		desc string
		want int
	}{
		{
			name: "three requirements",
			desc: "title: some task\nrequirements:\n  - id: R1\n    text: first\n  - id: R2\n    text: second\n  - id: R3\n    text: third\n",
			want: 3,
		},
		{
			name: "no requirements key",
			desc: "title: some task\ndescription: no reqs here\n",
			want: 0,
		},
		{
			name: "empty requirements list",
			desc: "title: some task\nrequirements: []\n",
			want: 0,
		},
		{
			name: "invalid yaml",
			desc: "{{not yaml at all",
			want: 0,
		},
		{
			name: "plain text",
			desc: "Just a plain text description with no YAML structure.",
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := st.CountDescriptionReqs(tc.desc)
			if got != tc.want {
				t.Errorf("st.CountDescriptionReqs() = %d, want %d", got, tc.want)
			}
		})
	}
}

// --- buildPRDReleaseMap (delegation sanity check) ---

func TestBuildPRDReleaseMap(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	ucDir := filepath.Join(dir, "docs", "specs", "use-cases")
	if err := os.MkdirAll(ucDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ucContent := `id: rel01.0-uc003-measure-workflow
title: Measure Workflow
summary: Measure phase
actor: Orchestrator
trigger: mage cobbler:measure
flow:
  - F1: "step one"
touchpoints:
  - T1: "Config: prd001-orchestrator-core R1, prd003-cobbler-workflows R1"
  - T2: "Prompt: prd003-cobbler-workflows R5"
success_criteria:
  - SC1: "it works"
out_of_scope: []
`
	if err := os.WriteFile(filepath.Join(ucDir, "rel01.0-uc003-measure-workflow.yaml"), []byte(ucContent), 0o644); err != nil {
		t.Fatal(err)
	}

	uc2Content := `id: rel02.0-uc001-lifecycle-commands
title: Lifecycle Commands
summary: VS Code lifecycle
actor: Developer
trigger: command palette
flow:
  - F1: "step one"
touchpoints:
  - T1: "Extension: prd006-vscode-extension R1"
success_criteria:
  - SC1: "it works"
out_of_scope: []
`
	if err := os.WriteFile(filepath.Join(ucDir, "rel02.0-uc001-lifecycle-commands.yaml"), []byte(uc2Content), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	m := st.BuildPRDReleaseMap()
	if m["prd001-orchestrator-core"] != "01.0" {
		t.Errorf("prd001-orchestrator-core release = %q, want %q", m["prd001-orchestrator-core"], "01.0")
	}
	if m["prd003-cobbler-workflows"] != "01.0" {
		t.Errorf("prd003-cobbler-workflows release = %q, want %q", m["prd003-cobbler-workflows"], "01.0")
	}
	if m["prd006-vscode-extension"] != "02.0" {
		t.Errorf("prd006-vscode-extension release = %q, want %q", m["prd006-vscode-extension"], "02.0")
	}
}

func TestBuildPRDReleaseMap_NoUseCases(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	m := st.BuildPRDReleaseMap()
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestBuildPRDReleaseMap_MalformedFilename(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	ucDir := filepath.Join(dir, "docs", "specs", "use-cases")
	os.MkdirAll(ucDir, 0o755)

	content := `id: rel01.0-something
title: Bad
summary: Missing uc pattern
actor: A
trigger: T
flow:
  - F1: "step"
touchpoints:
  - T1: "prd001-core R1"
success_criteria:
  - SC1: "ok"
out_of_scope: []
`
	os.WriteFile(filepath.Join(ucDir, "rel01.0-something.yaml"), []byte(content), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	m := st.BuildPRDReleaseMap()
	if len(m) != 0 {
		t.Errorf("expected empty map for malformed filename, got %v", m)
	}
}

func TestBuildPRDReleaseMap_InvalidYAML(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	ucDir := filepath.Join(dir, "docs", "specs", "use-cases")
	os.MkdirAll(ucDir, 0o755)

	os.WriteFile(filepath.Join(ucDir, "rel01.0-uc001-broken.yaml"), []byte("{{invalid yaml"), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	m := st.BuildPRDReleaseMap()
	if len(m) != 0 {
		t.Errorf("expected empty map for invalid YAML, got %v", m)
	}
}

func TestBuildPRDReleaseMap_NonNumericPRD(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	ucDir := filepath.Join(dir, "docs", "specs", "use-cases")
	os.MkdirAll(ucDir, 0o755)

	content := `id: rel01.0-uc001-test
title: Test
summary: Non-numeric PRD refs
actor: A
trigger: T
flow:
  - F1: "step"
touchpoints:
  - T1: "Config: prd-alpha R1"
  - T2: "Short: prd R2"
success_criteria:
  - SC1: "ok"
out_of_scope: []
`
	os.WriteFile(filepath.Join(ucDir, "rel01.0-uc001-test.yaml"), []byte(content), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	m := st.BuildPRDReleaseMap()
	if len(m) != 0 {
		t.Errorf("expected empty map for non-numeric PRD refs, got %v", m)
	}
}
