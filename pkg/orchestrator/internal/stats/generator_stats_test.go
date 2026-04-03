// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package stats

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
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
		{
			text: "prd001-testutils.yaml should strip yaml suffix",
			want: []string{"prd001-testutils"},
		},
		{
			text: "prd001-testutils and prd001-testutils.yaml deduplicate",
			want: []string{"prd001-testutils"},
		},
		{
			text: "prd-auth-flow.yml strips yml suffix too",
			want: []string{"prd-auth-flow"},
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
			name: "three requirements no subreqs",
			desc: "title: some task\nrequirements:\n  - id: R1\n    text: first\n  - id: R2\n    text: second\n  - id: R3\n    text: third\n",
			want: 3,
		},
		{
			name: "sub-requirement references counted",
			desc: "requirements:\n  - \"R1: Implement per prd003 R2.1, R2.2, R2.3\"\n  - \"R2: Add tests per prd003 R3.1\"\n",
			want: 4,
		},
		{
			name: "mixed lines with and without subreq refs",
			desc: "requirements:\n  - \"R1: Implement per prd003 R1.1\"\n  - \"R2: General cleanup\"\n",
			want: 2,
		},
		{
			name: "structured format with subreq refs",
			desc: "requirements:\n  - id: R1\n    text: \"Implement per prd003 R2.1, R2.2\"\n  - id: R2\n    text: \"Add per prd003 R3.1, R3.2, R3.3\"\n",
			want: 5,
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

// TestPrintGeneratorStats_HistoryOnly verifies that stats can be built
// entirely from local history files without GitHub API (GH-1450).
func TestPrintGeneratorStats_HistoryOnly(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	// Two stitch tasks: one succeeded, one failed.
	stitch1 := `caller: stitch
task_id: "100"
task_title: "[stitch] prd001 R1 implement feature"
status: success
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
loc_before:
  production: 500
  test: 200
loc_after:
  production: 580
  test: 240
`
	stitch2 := `caller: stitch
task_id: "101"
task_title: "[stitch] prd002 R1 another feature"
status: failed
started_at: "2026-03-08T13:00:00Z"
duration: "2m 10s"
duration_s: 130
tokens:
  input: 80000
  output: 3000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.80
num_turns: 8
loc_before:
  production: 580
  test: 240
loc_after:
  production: 0
  test: 0
`
	os.WriteFile(filepath.Join(histDir, "2026-03-08-12-00-00-stitch-stats.yaml"), []byte(stitch1), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-08-13-00-00-stitch-stats.yaml"), []byte(stitch2), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		// No DetectGitHubRepo or ListAllIssues — history-only mode.
		HistoryDir: histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both tasks should appear.
	if !strings.Contains(output, "100") {
		t.Errorf("task 100 missing from output:\n%s", output)
	}
	if !strings.Contains(output, "101") {
		t.Errorf("task 101 missing from output:\n%s", output)
	}
	// Task 100 should be "done" (status: success).
	if !strings.Contains(output, "done") {
		t.Errorf("expected 'done' status in output:\n%s", output)
	}
	// Task 101 should be "failed".
	if !strings.Contains(output, "failed") {
		t.Errorf("expected 'failed' status in output:\n%s", output)
	}
	// Should show cost totals.
	if !strings.Contains(output, "$2.30") {
		t.Errorf("expected total cost $2.30 in output:\n%s", output)
	}
}

func TestPrintGeneratorStats_WeightColumn(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	// Create a requirements.yaml with weighted R-items.
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	reqYAML := `requirements:
  prd001:
    R1.1:
      status: complete
      weight: 1
      issue: 400
    R1.2:
      status: complete
      weight: 4
      issue: 400
`
	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(reqYAML), 0o644)

	// Task that references prd001 R1.1 and R1.2 (total weight = 1+4 = 5).
	stitch1 := `caller: stitch
task_id: "400"
task_title: "[stitch] prd001 R1.1-R1.2 weighted task"
status: success
started_at: "2026-03-22T14:00:00Z"
duration: "5m 0s"
duration_s: 300
tokens:
  input: 100000
  output: 5000
cost_usd: 1.00
num_turns: 10
loc_before:
  production: 500
  test: 200
loc_after:
  production: 550
  test: 220
`
	os.WriteFile(filepath.Join(histDir, "2026-03-22-14-00-00-stitch-stats.yaml"), []byte(stitch1), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		HistoryDir:             histDir,
		CobblerDir:             cobblerDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Table header should include Weight column.
	if !strings.Contains(output, "Weight") {
		t.Errorf("expected 'Weight' column header in output:\n%s", output)
	}

	// Task 400 references prd001 R1.1 (w=1) + R1.2 (w=4) = 5.
	if !strings.Contains(output, "5") {
		t.Errorf("expected weight '5' for task 400 in output:\n%s", output)
	}
}

func TestPrintGeneratorStats_StartedColumn(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	stitch1 := `caller: stitch
task_id: "300"
task_title: "[stitch] prd001 R1 with timestamp"
status: success
started_at: "2026-03-22T14:30:00Z"
duration: "5m 0s"
duration_s: 300
tokens:
  input: 100000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 1.00
num_turns: 10
loc_before:
  production: 500
  test: 200
loc_after:
  production: 550
  test: 220
`
	os.WriteFile(filepath.Join(histDir, "2026-03-22-14-30-00-stitch-stats.yaml"), []byte(stitch1), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		HistoryDir:             histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Table header should include Started column.
	if !strings.Contains(output, "Started") {
		t.Errorf("expected 'Started' column header in output:\n%s", output)
	}

	// Task 300 should show a formatted start time (Mar 22).
	if !strings.Contains(output, "Mar 22") {
		t.Errorf("expected 'Mar 22' in Started column for task 300:\n%s", output)
	}
}

func TestPrintGeneratorStats_RateLimitColumn(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	// Task with rate limit wait time.
	stitch1 := `caller: stitch
task_id: "200"
task_title: "[stitch] prd001 R1 rate limited task"
status: success
started_at: "2026-03-20T12:00:00Z"
duration: "15m 0s"
duration_s: 900
rate_limit_wait_s: 780
tokens:
  input: 100000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 1.00
num_turns: 10
loc_before:
  production: 500
  test: 200
loc_after:
  production: 550
  test: 220
`
	// Task without rate limit.
	stitch2 := `caller: stitch
task_id: "201"
task_title: "[stitch] prd001 R2 normal task"
status: success
started_at: "2026-03-20T13:00:00Z"
duration: "3m 0s"
duration_s: 180
tokens:
  input: 80000
  output: 3000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.50
num_turns: 5
loc_before:
  production: 550
  test: 220
loc_after:
  production: 600
  test: 250
`
	os.WriteFile(filepath.Join(histDir, "2026-03-20-12-00-00-stitch-stats.yaml"), []byte(stitch1), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-20-13-00-00-stitch-stats.yaml"), []byte(stitch2), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		HistoryDir:             histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Table header should include RateLimit column.
	if !strings.Contains(output, "RateLimit") {
		t.Errorf("expected 'RateLimit' column header in output:\n%s", output)
	}

	// Task 200 should show a rate limit duration (13m0s = 780s).
	if !strings.Contains(output, "13m0s") {
		t.Errorf("expected rate limit duration '13m0s' for task 200 in output:\n%s", output)
	}

	// Summary header should mention rate-limited time.
	if !strings.Contains(output, "rate-limited") {
		t.Errorf("expected 'rate-limited' in summary header:\n%s", output)
	}
}

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
		HistoryDir: histDir,
	}

	err := PrintGeneratorStats(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
		HistoryDir: histDir,
	}

	// Just verify it doesn't error — measure output goes to stdout.
	err := PrintGeneratorStats(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPrintGeneratorStats_LOCDeltaIgnoresFailedEntries verifies that failed
// stitch entries (with zero LOCAfter) do not corrupt the LOC delta (GH-1449).
func TestPrintGeneratorStats_LOCDeltaIgnoresFailedEntries(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	// Failed stitch: LOCBefore=2000, LOCAfter=0 (not set on failure).
	failedYAML := `caller: stitch
task_id: "823"
task_title: "SetPriority"
status: failed
started_at: "2026-03-10T10:00:00Z"
duration: "2m 0s"
duration_s: 120
tokens:
  input: 80000
  output: 3000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.50
num_turns: 5
loc_before:
  production: 2000
  test: 500
loc_after:
  production: 0
  test: 0
`
	// Successful stitch: LOCBefore=694, LOCAfter=707.
	successYAML := `caller: stitch
task_id: "823"
task_title: "SetPriority"
status: done
started_at: "2026-03-10T10:05:00Z"
duration: "5m 0s"
duration_s: 300
tokens:
  input: 125000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 1.00
num_turns: 10
loc_before:
  production: 694
  test: 100
loc_after:
  production: 707
  test: 100
`
	os.WriteFile(filepath.Join(histDir, "2026-03-10-10-00-00-stitch-stats.yaml"), []byte(failedYAML), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-10-10-05-00-stitch-stats.yaml"), []byte(successYAML), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 823, Title: "[stitch] SetPriority", State: "closed", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should show +13 prod (707-694), not -1375 (0-2000 + 707-694).
	if !strings.Contains(output, "+13") {
		t.Errorf("expected LOC delta +13 in output, got:\n%s", output)
	}
	if strings.Contains(output, "-1375") || strings.Contains(output, "-1987") {
		t.Errorf("failed stitch entry corrupted LOC delta:\n%s", output)
	}
}

// TestRequirementsCountUsesPerItemState verifies that the "Requirements: X/Y"
// line counts actual non-ready R-items from requirements.yaml, not all R-items
// in any touched PRD (GH-1437).
func TestRequirementsCountUsesPerItemState(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	// Create a PRD with 6 R-items across 2 groups.
	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)
	prdYAML := `id: prd001-testutils
requirements:
  R1:
    title: "Group 1"
    items:
      - R1.1: "first"
      - R1.2: "second"
      - R1.3: "third"
  R2:
    title: "Group 2"
    items:
      - R2.1: "fourth"
      - R2.2: "fifth"
      - R2.3: "sixth"
`
	os.WriteFile(filepath.Join(prdDir, "prd001-testutils.yaml"), []byte(prdYAML), 0o644)

	// Create requirements.yaml with only 2 of 6 R-items completed.
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	reqYAML := `requirements:
  prd001-testutils:
    R1.1:
      status: complete
      issue: 100
    R1.2:
      status: complete
      issue: 100
    R1.3:
      status: ready
    R2.1:
      status: ready
    R2.2:
      status: ready
    R2.3:
      status: ready
`
	os.WriteFile(filepath.Join(cobblerDir, generate.RequirementsFileName), []byte(reqYAML), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{
					Number:      100,
					Title:       "implement R1.1-R1.2 (prd001-testutils)",
					State:       "closed",
					Labels:      []string{"cobbler-task"},
					Description: "requirements:\n  - text: \"prd001-testutils R1.1\"\n    source: test\n",
				},
			}, nil
		},
		HistoryDir: filepath.Join(cobblerDir, "history"),
		CobblerDir: cobblerDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should report 2/6, not 6/6.
	if !strings.Contains(output, "Requirements: 2/6") {
		t.Errorf("expected 'Requirements: 2/6' in output, got:\n%s", output)
	}
	if strings.Contains(output, "Requirements: 6/6") {
		t.Errorf("bug not fixed: output still shows 6/6 (all R-items in touched PRD):\n%s", output)
	}
}

// TestPrintGeneratorStats_WarnsWrongBranch verifies that a warning is printed
// to stderr when the current branch does not match the generation branch (GH-1444).
func TestPrintGeneratorStats_WarnsWrongBranch(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "test task", State: "open", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: filepath.Join(dir, "history"),
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stderr = oldStderr
	stderr := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stderr, "warning: stats:generator should be run from the generation worktree") {
		t.Errorf("expected warning on stderr, got: %q", stderr)
	}
	if !strings.Contains(stderr, "Expected branch: generation-main, current branch: main.") {
		t.Errorf("expected branch names in warning, got: %q", stderr)
	}
}

// TestPrintGeneratorStats_NoWarnWhenOnCorrectBranch verifies no warning is
// printed when the current branch matches the generation branch.
func TestPrintGeneratorStats_NoWarnWhenOnCorrectBranch(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "test task", State: "open", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: filepath.Join(dir, "history"),
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stderr = oldStderr
	stderr := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(stderr, "warning") {
		t.Errorf("expected no warning when on correct branch, got: %q", stderr)
	}
}

// TestPrintGeneratorStats_ReqsFromBranch verifies that when on the wrong
// branch, requirements.yaml is read from the generation branch via
// ReadBranchFile instead of from stale CWD data (GH-1445).
func TestPrintGeneratorStats_ReqsFromBranch(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	// Create a PRD with 4 R-items.
	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)
	prdYAML := `id: prd001-test
requirements:
  R1:
    title: "Group"
    items:
      - R1.1: "first"
      - R1.2: "second"
      - R1.3: "third"
      - R1.4: "fourth"
`
	os.WriteFile(filepath.Join(prdDir, "prd001-test.yaml"), []byte(prdYAML), 0o644)

	// Stale CWD requirements.yaml: shows 3 of 4 addressed (wrong).
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	staleReqYAML := `requirements:
  prd001-test:
    R1.1:
      status: complete
    R1.2:
      status: complete
    R1.3:
      status: complete
    R1.4:
      status: ready
`
	os.WriteFile(filepath.Join(cobblerDir, generate.RequirementsFileName), []byte(staleReqYAML), 0o644)

	// Fresh requirements from generation branch: only 1 addressed.
	freshReqYAML := `requirements:
  prd001-test:
    R1.1:
      status: complete
    R1.2:
      status: ready
    R1.3:
      status: ready
    R1.4:
      status: ready
`

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	// Capture stdout, discard stderr (warning from GH-1444).
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	_, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "main", // wrong branch — triggers ReadBranchFile
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "test task", State: "open", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: filepath.Join(cobblerDir, "history"),
		CobblerDir: cobblerDir,
		ReadBranchFile: func(branch, path string) ([]byte, error) {
			if branch == "generation-main" {
				return []byte(freshReqYAML), nil
			}
			return nil, fmt.Errorf("branch not found")
		},
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	stderrW.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should report 1/4 (from branch), not 3/4 (from stale CWD).
	if !strings.Contains(output, "Requirements: 1/4") {
		t.Errorf("expected 'Requirements: 1/4' (from branch), got:\n%s", output)
	}
	if strings.Contains(output, "Requirements: 3/4") {
		t.Errorf("bug: still reading stale CWD requirements (3/4):\n%s", output)
	}
}

// TestPrintGeneratorStats_ReqsFallbackToCWD verifies that when ReadBranchFile
// is nil (not available), requirements are still read from CWD.
func TestPrintGeneratorStats_ReqsFallbackToCWD(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)
	prdYAML := `id: prd001-test
requirements:
  R1:
    title: "Group"
    items:
      - R1.1: "first"
      - R1.2: "second"
`
	os.WriteFile(filepath.Join(prdDir, "prd001-test.yaml"), []byte(prdYAML), 0o644)

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	reqYAML := `requirements:
  prd001-test:
    R1.1:
      status: complete
    R1.2:
      status: ready
`
	os.WriteFile(filepath.Join(cobblerDir, generate.RequirementsFileName), []byte(reqYAML), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stdout = w
	_, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main", // correct branch
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "test task", State: "open", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir:     filepath.Join(cobblerDir, "history"),
		CobblerDir:     cobblerDir,
		ReadBranchFile: nil, // not available
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	stderrW.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still show 1/2 from CWD.
	if !strings.Contains(output, "Requirements: 1/2") {
		t.Errorf("expected 'Requirements: 1/2' from CWD fallback, got:\n%s", output)
	}
}

// TestMeasureTaskIDZeroDisplaysAsMN verifies that a measure entry with
// TaskID "0" (from a failed placeholder creation) displays as "M1" in the
// stats table, not as "0" (GH-1438).
func TestMeasureTaskIDZeroDisplaysAsMN(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	// Simulate a measure stats file with task_id "0" (placeholder failure).
	measureYAML := `caller: measure
started_at: "2026-03-09T18:00:04Z"
duration: "56s"
duration_s: 56
task_id: "0"
tokens:
  input: 48000
  output: 2000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.31
num_turns: 2
`
	os.WriteFile(filepath.Join(histDir, "2026-03-09-18-00-04-measure-stats.yaml"), []byte(measureYAML), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log: func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "test task", State: "closed", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The measure row should show "M1", not "0".
	if strings.Contains(output, "\n0 ") || strings.Contains(output, "\n0\t") {
		t.Errorf("measure row should display as M1, not 0:\n%s", output)
	}
	if !strings.Contains(output, "M1") {
		t.Errorf("expected M1 in output:\n%s", output)
	}
}

// --- FormatDurationLong ---

func TestFormatDurationLong(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		seconds int
		want    string
	}{
		{"sub-minute", 45, "45s"},
		{"minutes", 332, "5m 32s"},
		{"exactly 1h", 3600, "1h 0m"},
		{"hours and minutes", 39120, "10h 52m"},
		{"zero", 0, "0s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatDurationLong(tc.seconds)
			if got != tc.want {
				t.Errorf("FormatDurationLong(%d) = %q, want %q", tc.seconds, got, tc.want)
			}
		})
	}
}

// --- ETA and Cost Estimates (GH-1545) ---

// TestPrintGeneratorStats_ETAAndCost verifies that ETA and cost estimate
// lines appear in the output when requirements data is available.
func TestPrintGeneratorStats_ETAAndCost(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	// Create a PRD with 10 R-items.
	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)
	prdYAML := `id: prd001-test
requirements:
  R1:
    title: "Group"
    items:
      - R1.1: "a"
      - R1.2: "b"
      - R1.3: "c"
      - R1.4: "d"
      - R1.5: "e"
      - R1.6: "f"
      - R1.7: "g"
      - R1.8: "h"
      - R1.9: "i"
      - R1.10: "j"
`
	os.WriteFile(filepath.Join(prdDir, "prd001-test.yaml"), []byte(prdYAML), 0o644)

	// requirements.yaml: 2 of 10 addressed.
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	reqYAML := `requirements:
  prd001-test:
    R1.1:
      status: complete
      issue: 100
    R1.2:
      status: complete
      issue: 101
    R1.3:
      status: ready
    R1.4:
      status: ready
    R1.5:
      status: ready
    R1.6:
      status: ready
    R1.7:
      status: ready
    R1.8:
      status: ready
    R1.9:
      status: ready
    R1.10:
      status: ready
`
	os.WriteFile(filepath.Join(cobblerDir, generate.RequirementsFileName), []byte(reqYAML), 0o644)

	// Create history with 2 stitch tasks (600s each) and 1 measure (60s).
	histDir := filepath.Join(cobblerDir, "history")
	os.MkdirAll(histDir, 0o755)
	stitch1 := `caller: stitch
task_id: "100"
task_title: "task 1"
status: success
started_at: "2026-03-08T12:00:00Z"
duration_s: 600
tokens:
  input: 100000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 2.00
num_turns: 10
loc_before:
  production: 500
  test: 200
loc_after:
  production: 550
  test: 220
`
	stitch2 := `caller: stitch
task_id: "101"
task_title: "task 2"
status: success
started_at: "2026-03-08T13:00:00Z"
duration_s: 600
tokens:
  input: 100000
  output: 5000
  cache_creation: 0
  cache_read: 0
cost_usd: 2.00
num_turns: 10
loc_before:
  production: 550
  test: 220
loc_after:
  production: 600
  test: 240
`
	measure1 := `caller: measure
started_at: "2026-03-08T11:50:00Z"
duration_s: 60
tokens:
  input: 50000
  output: 2000
  cache_creation: 0
  cache_read: 0
cost_usd: 0.50
num_turns: 3
`
	os.WriteFile(filepath.Join(histDir, "2026-03-08-12-00-00-stitch-stats.yaml"), []byte(stitch1), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-08-13-00-00-stitch-stats.yaml"), []byte(stitch2), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-08-11-50-00-measure-stats.yaml"), []byte(measure1), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "task 1", State: "closed", Labels: []string{"cobbler-task"}},
				{Number: 101, Title: "task 2", State: "closed", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: histDir,
		CobblerDir: cobblerDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 addressed, 8 remaining, total elapsed = 1200+60 = 1260s.
	// Avg per req = 630s, remaining = 630*8 = 5040s, total est = 1260+5040 = 6300s.
	// 5040s = 1h 24m, 6300s = 1h 45m.
	if !strings.Contains(output, "ETA: 1h 24m remaining of estimated 1h 45m total") {
		t.Errorf("expected ETA line in output, got:\n%s", output)
	}

	// Cost: total = $4.50, addressed = 2, total reqs = 10.
	// Est total = 4.50/2*10 = $22.50, remaining = $22.50-$4.50 = $18.
	if !strings.Contains(output, "Cost: $18 remaining of estimated $22 total ($4.50 spent)") {
		t.Errorf("expected Cost line in output, got:\n%s", output)
	}
}

// TestPrintGeneratorStats_ETACalculating verifies that "calculating..." is
// shown when no requirements have been addressed yet.
func TestPrintGeneratorStats_ETACalculating(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)
	prdYAML := `id: prd001-test
requirements:
  R1:
    title: "Group"
    items:
      - R1.1: "a"
      - R1.2: "b"
`
	os.WriteFile(filepath.Join(prdDir, "prd001-test.yaml"), []byte(prdYAML), 0o644)

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	reqYAML := `requirements:
  prd001-test:
    R1.1:
      status: ready
    R1.2:
      status: ready
`
	os.WriteFile(filepath.Join(cobblerDir, generate.RequirementsFileName), []byte(reqYAML), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		DetectGitHubRepo:       func() (string, error) { return "owner/repo", nil },
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return []gh.CobblerIssue{
				{Number: 100, Title: "test task", State: "open", Labels: []string{"cobbler-task"}},
			}, nil
		},
		HistoryDir: filepath.Join(cobblerDir, "history"),
		CobblerDir: cobblerDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "ETA: calculating...") {
		t.Errorf("expected 'ETA: calculating...' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Cost: calculating...") {
		t.Errorf("expected 'Cost: calculating...' in output, got:\n%s", output)
	}
}

// --- Attempt tracking (GH-1967) ---

// TestPrintGeneratorStats_AttemptsColumn verifies that the Att column shows
// the number of stitch invocations per task, and the retry summary line is
// printed when retries occur.
func TestPrintGeneratorStats_AttemptsColumn(t *testing.T) {
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	// Task 200: two failed attempts, then one success = 3 attempts.
	fail1 := `caller: stitch
task_id: "200"
task_title: "[stitch] prd001 R1 flaky task"
status: failed
started_at: "2026-03-10T10:00:00Z"
duration_s: 120
tokens:
  input: 50000
  output: 2000
cost_usd: 0.40
num_turns: 5
loc_before:
  production: 500
  test: 200
loc_after:
  production: 0
  test: 0
`
	fail2 := `caller: stitch
task_id: "200"
task_title: "[stitch] prd001 R1 flaky task"
status: failed
started_at: "2026-03-10T10:05:00Z"
duration_s: 90
tokens:
  input: 50000
  output: 2000
cost_usd: 0.35
num_turns: 4
loc_before:
  production: 500
  test: 200
loc_after:
  production: 0
  test: 0
`
	success := `caller: stitch
task_id: "200"
task_title: "[stitch] prd001 R1 flaky task"
status: success
started_at: "2026-03-10T10:10:00Z"
duration_s: 200
tokens:
  input: 60000
  output: 3000
cost_usd: 0.50
num_turns: 8
loc_before:
  production: 500
  test: 200
loc_after:
  production: 530
  test: 220
`
	// Task 201: single success = 1 attempt.
	single := `caller: stitch
task_id: "201"
task_title: "[stitch] prd002 R1 clean task"
status: success
started_at: "2026-03-10T11:00:00Z"
duration_s: 150
tokens:
  input: 40000
  output: 2000
cost_usd: 0.30
num_turns: 6
loc_before:
  production: 530
  test: 220
loc_after:
  production: 560
  test: 240
`
	os.WriteFile(filepath.Join(histDir, "2026-03-10-10-00-00-stitch-stats.yaml"), []byte(fail1), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-10-10-05-00-stitch-stats.yaml"), []byte(fail2), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-10-10-10-00-stitch-stats.yaml"), []byte(success), 0o644)
	os.WriteFile(filepath.Join(histDir, "2026-03-10-11-00-00-stitch-stats.yaml"), []byte(single), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		HistoryDir:             histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Table header should include Att column.
	if !strings.Contains(output, "Att") {
		t.Errorf("expected Att column header in output:\n%s", output)
	}

	// The retry summary line should appear.
	if !strings.Contains(output, "Retries: 2 retries across 4 attempts") {
		t.Errorf("expected retry summary in output:\n%s", output)
	}
	if !strings.Contains(output, "50%") {
		t.Errorf("expected 50%% retry rate in output:\n%s", output)
	}
	// Wasted cost = $0.40 + $0.35 = $0.75
	if !strings.Contains(output, "$0.75 wasted") {
		t.Errorf("expected $0.75 wasted in output:\n%s", output)
	}
}

// TestPrintGeneratorStats_NoRetrySummaryWhenNoRetries verifies that the retry
// summary line is omitted when all tasks succeed on the first attempt.
func TestPrintGeneratorStats_NoRetrySummaryWhenNoRetries(t *testing.T) {
	dir := t.TempDir()

	histDir := filepath.Join(dir, "history")
	os.MkdirAll(histDir, 0o755)

	single := `caller: stitch
task_id: "300"
task_title: "[stitch] prd001 R1 clean"
status: success
started_at: "2026-03-10T12:00:00Z"
duration_s: 100
tokens:
  input: 30000
  output: 1000
cost_usd: 0.20
num_turns: 4
loc_before:
  production: 100
  test: 50
loc_after:
  production: 120
  test: 60
`
	os.WriteFile(filepath.Join(histDir, "2026-03-10-12-00-00-stitch-stats.yaml"), []byte(single), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	deps := GeneratorStatsDeps{
		Log:                    func(format string, args ...any) {},
		ListGenerationBranches: func() []string { return []string{"generation-main"} },
		GenerationBranch:       "generation-main",
		CurrentBranch:          "generation-main",
		HistoryDir:             histDir,
	}

	err := PrintGeneratorStats(deps)
	w.Close()
	captured, _ := io.ReadAll(r)
	os.Stdout = oldStdout
	output := string(captured)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No retry summary when all tasks have 1 attempt.
	if strings.Contains(output, "Retries:") {
		t.Errorf("retry summary should not appear when no retries:\n%s", output)
	}
}
