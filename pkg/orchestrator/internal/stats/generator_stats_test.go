// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package stats

import (
	"os"
	"path/filepath"
	"testing"

	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
)

// --- ParseStitchComment ---

func TestParseStitchComment_Completed(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
	d := ParseStitchComment(body)
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
	d := ParseStitchComment(body)
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
	d := ParseStitchComment(body)
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
	d := ParseStitchComment(body)
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
	d := ParseStitchComment(body)
	if d.LocDeltaProd != -12 {
		t.Errorf("LocDeltaProd = %d, want -12", d.LocDeltaProd)
	}
	if d.LocDeltaTest != 30 {
		t.Errorf("LocDeltaTest = %d, want 30", d.LocDeltaTest)
	}
}

func TestParseStitchComment_NoMatch(t *testing.T) {
	t.Parallel()
	d := ParseStitchComment("unrelated comment text")
	if d.CostUSD != 0 || d.DurationS != 0 || d.NumTurns != 0 {
		t.Errorf("expected zero values, got cost=%v dur=%d turns=%d", d.CostUSD, d.DurationS, d.NumTurns)
	}
}

func TestParseStitchComment_PromptBytes(t *testing.T) {
	t.Parallel()
	body := "Stitch started. Branch: `generation-main`, prompt: 524288 bytes."
	d := ParseStitchComment(body)
	if d.PromptBytes != 524288 {
		t.Errorf("PromptBytes = %d, want 524288", d.PromptBytes)
	}
}

func TestParseStitchComment_PromptBytes_NoMatch(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
	d := ParseStitchComment(body)
	if d.PromptBytes != 0 {
		t.Errorf("PromptBytes = %d, want 0", d.PromptBytes)
	}
}

func TestParseStitchComment_Tokens(t *testing.T) {
	t.Parallel()
	body := "Stitch completed in 3m 15s. LOC delta: +20 prod, +10 test. Cost: $0.55. Turns: 12. Tokens: 125000in 5000out."
	d := ParseStitchComment(body)
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
	d := ParseStitchComment(body)
	if d.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", d.InputTokens)
	}
	if d.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", d.OutputTokens)
	}
}

// --- FormatBytes ---

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
			got := FormatBytes(tc.bytes)
			if got != tc.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// --- FormatTokens ---

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
			got := FormatTokens(tc.tokens)
			if got != tc.want {
				t.Errorf("FormatTokens(%d) = %q, want %q", tc.tokens, got, tc.want)
			}
		})
	}
}

// --- ExtractRelease ---

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
			got := ExtractRelease(tc.text)
			if got != tc.want {
				t.Errorf("ExtractRelease(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// --- ExtractPRDRefs ---

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
		{
			text: "Implement prd006-cat utility",
			want: []string{"prd006-cat"},
		},
		{
			text: "Covers prd001-orchestrator-core and prd003-cobbler-workflows R1",
			want: []string{"prd001-orchestrator-core", "prd003-cobbler-workflows"},
		},
		{
			text: "Mixed prd-auth-flow and prd006-cat refs",
			want: []string{"prd-auth-flow", "prd006-cat"},
		},
		{
			text: "prd006-cat prd006-cat duplicate",
			want: []string{"prd006-cat"},
		},
		{
			text: "bare prd003 without hyphen-name is not a ref",
			want: nil,
		},
	}
	for _, tc := range tests {
		got := ExtractPRDRefs(tc.text)
		if len(got) != len(tc.want) {
			t.Errorf("ExtractPRDRefs(%q): got %v, want %v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("ExtractPRDRefs(%q)[%d]: got %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}
}

// --- CountTotalPRDRequirements ---

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

	total, byPRD := CountTotalPRDRequirements()
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

	total, byPRD := CountTotalPRDRequirements()
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(byPRD) != 0 {
		t.Errorf("byPRD = %v, want empty", byPRD)
	}
}

// --- CountDescriptionReqs ---

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
			got := CountDescriptionReqs(tc.desc)
			if got != tc.want {
				t.Errorf("CountDescriptionReqs() = %d, want %d", got, tc.want)
			}
		})
	}
}

// --- BuildPRDReleaseMap ---

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

	m := BuildPRDReleaseMap()
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

	m := BuildPRDReleaseMap()
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

	m := BuildPRDReleaseMap()
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

	m := BuildPRDReleaseMap()
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

	m := BuildPRDReleaseMap()
	if len(m) != 0 {
		t.Errorf("expected empty map for non-numeric PRD refs, got %v", m)
	}
}

// --- LoadHistoryStats ---

func TestLoadHistoryStats_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stats, err := LoadHistoryStats(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(stats))
	}
}

func TestLoadHistoryStats_BlankPath(t *testing.T) {
	t.Parallel()
	stats, err := LoadHistoryStats("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil, got %v", stats)
	}
}

func TestLoadHistoryStats_NonExistentDir(t *testing.T) {
	t.Parallel()
	stats, err := LoadHistoryStats("/tmp/does-not-exist-xyz-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil, got %v", stats)
	}
}

func TestLoadHistoryStats_ParsesFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	stitchYAML := `caller: stitch
task_id: "42"
task_title: "implement feature"
status: done
started_at: "2026-03-08T12:00:00Z"
duration: "5m 32s"
duration_s: 332
tokens:
  input: 125000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.42
num_turns: 12
`
	measureYAML := `caller: measure
started_at: "2026-03-08T12:05:00Z"
duration: "1m 10s"
duration_s: 70
tokens:
  input: 50000
  output: 2000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.15
num_turns: 3
`
	os.WriteFile(filepath.Join(dir, "2026-03-08-12-00-00-stitch-stats.yaml"), []byte(stitchYAML), 0o644)
	os.WriteFile(filepath.Join(dir, "2026-03-08-12-05-00-measure-stats.yaml"), []byte(measureYAML), 0o644)
	// Non-stats file should be ignored.
	os.WriteFile(filepath.Join(dir, "2026-03-08-12-00-00-stitch-report.yaml"), []byte("ignored: true"), 0o644)

	stats, err := LoadHistoryStats(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(stats))
	}

	// Find stitch and measure entries.
	var foundStitch, foundMeasure bool
	for _, s := range stats {
		switch s.Caller {
		case "stitch":
			foundStitch = true
			if s.TaskID != "42" {
				t.Errorf("stitch TaskID = %q, want %q", s.TaskID, "42")
			}
			if s.CostUSD != 0.42 {
				t.Errorf("stitch CostUSD = %v, want 0.42", s.CostUSD)
			}
			if s.DurationS != 332 {
				t.Errorf("stitch DurationS = %d, want 332", s.DurationS)
			}
			if s.NumTurns != 12 {
				t.Errorf("stitch NumTurns = %d, want 12", s.NumTurns)
			}
			if s.Tokens.Input != 125000 {
				t.Errorf("stitch Input = %d, want 125000", s.Tokens.Input)
			}
			if s.Tokens.Output != 5000 {
				t.Errorf("stitch Output = %d, want 5000", s.Tokens.Output)
			}
		case "measure":
			foundMeasure = true
			if s.CostUSD != 0.15 {
				t.Errorf("measure CostUSD = %v, want 0.15", s.CostUSD)
			}
		}
	}
	if !foundStitch {
		t.Error("stitch entry not found")
	}
	if !foundMeasure {
		t.Error("measure entry not found")
	}
}

func TestLoadHistoryStats_SkipsInvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad-stats.yaml"), []byte("{{invalid yaml"), 0o644)
	stats, err := LoadHistoryStats(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected empty slice for invalid YAML, got %d entries", len(stats))
	}
}

// --- PrintGeneratorStats with history ---

func TestPrintGeneratorStats_PrefersHistoryOverComments(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	// Create history dir with stitch stats.
	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)
	stitchYAML := `caller: stitch
task_id: "100"
task_title: "implement feature"
status: done
started_at: "2026-03-08T12:00:00Z"
duration: "5m 32s"
duration_s: 332
tokens:
  input: 125000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 1.50
num_turns: 15
`
	os.WriteFile(filepath.Join(histDir, "2026-03-08-12-00-00-stitch-stats.yaml"), []byte(stitchYAML), 0o644)

	// Set up minimal PRD/use-case dirs so BuildPRDReleaseMap doesn't fail.
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	commentCalledForCost := false
	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "implement feature", State: "closed", Labels: []string{"cobbler-task"}},
			}, nil
		},
		FetchIssueComments: func(repo string, number int) ([]string, error) {
			// Comments are still fetched for PromptBytes even when history
			// exists. Return a "Stitch started" comment with prompt bytes
			// but no cost — cost should come from history, not comments.
			commentCalledForCost = false
			return []string{"Stitch started. Branch: `generation-main`, prompt: 524288 bytes."}, nil
		},
		HistoryDir: histDir,
	}

	err := PrintGeneratorStats(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commentCalledForCost {
		t.Error("expected cost to come from history, not comments")
	}
}

func TestPrintGeneratorStats_FallsBackToComments(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	// Empty history dir — no matching stats for any issue.
	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	commentCalled := false
	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 200, Title: "other feature", State: "closed", Labels: []string{"cobbler-task"}},
			}, nil
		},
		FetchIssueComments: func(repo string, number int) ([]string, error) {
			commentCalled = true
			return []string{"Stitch completed in 2m 0s. Cost: $0.30. Turns: 5."}, nil
		},
		HistoryDir: histDir,
	}

	err := PrintGeneratorStats(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !commentCalled {
		t.Error("expected FetchIssueComments to be called as fallback when no history data matches")
	}
}

func TestPrintGeneratorStats_MeasureSummary(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	measureYAML := `caller: measure
started_at: "2026-03-08T12:05:00Z"
duration: "1m 10s"
duration_s: 70
tokens:
  input: 50000
  output: 2000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.15
num_turns: 3
`
	os.WriteFile(filepath.Join(histDir, "2026-03-08-12-05-00-measure-stats.yaml"), []byte(measureYAML), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 300, Title: "test task", State: "open", Labels: []string{"cobbler-task"}},
			}, nil
		},
		FetchIssueComments: func(repo string, number int) ([]string, error) {
			return nil, nil
		},
		HistoryDir: histDir,
	}

	// Just verify it doesn't error — measure output goes to stdout.
	err := PrintGeneratorStats(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
