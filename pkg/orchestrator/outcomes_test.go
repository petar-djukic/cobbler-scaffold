// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// --- parseOutcomeRecords ---

func TestParseOutcomeRecords_SingleRecord(t *testing.T) {
	t.Parallel()
	logOutput := outcomeSep + "\n" +
		"HEAD -> task/gen-main-atlas-001\n" +
		"Tokens-Input: 45000\n" +
		"Tokens-Output: 12000\n" +
		"Tokens-Cache-Creation: 5000\n" +
		"Tokens-Cache-Read: 3000\n" +
		"Tokens-Cost-USD: 0.7500\n" +
		"Loc-Prod-Before: 441\n" +
		"Loc-Prod-After: 520\n" +
		"Loc-Test-Before: 0\n" +
		"Loc-Test-After: 45\n" +
		"Duration-Seconds: 1234\n"

	records := parseOutcomeRecords(logOutput)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	r := records[0]
	if r.TaskBranch != "task/gen-main-atlas-001" {
		t.Errorf("TaskBranch: got %q, want %q", r.TaskBranch, "task/gen-main-atlas-001")
	}
	if r.TokensInput != 45000 {
		t.Errorf("TokensInput: got %d, want 45000", r.TokensInput)
	}
	if r.TokensOutput != 12000 {
		t.Errorf("TokensOutput: got %d, want 12000", r.TokensOutput)
	}
	if r.TokensCostUSD != 0.75 {
		t.Errorf("TokensCostUSD: got %f, want 0.75", r.TokensCostUSD)
	}
	if r.LocProdBefore != 441 || r.LocProdAfter != 520 {
		t.Errorf("LOC prod: before=%d after=%d, want 441/520", r.LocProdBefore, r.LocProdAfter)
	}
	if r.LocTestBefore != 0 || r.LocTestAfter != 45 {
		t.Errorf("LOC test: before=%d after=%d, want 0/45", r.LocTestBefore, r.LocTestAfter)
	}
	if r.DurationSeconds != 1234 {
		t.Errorf("DurationSeconds: got %d, want 1234", r.DurationSeconds)
	}
}

func TestParseOutcomeRecords_SkipsCommitsWithoutTokensInput(t *testing.T) {
	t.Parallel()
	// Block without Tokens-Input trailer should be skipped.
	logOutput := outcomeSep + "\n" +
		"HEAD -> main\n" +
		"Some-Other-Trailer: value\n" +
		outcomeSep + "\n" +
		"task/gen-main-atlas-002\n" +
		"Tokens-Input: 1000\n" +
		"Tokens-Output: 200\n" +
		"Tokens-Cache-Creation: 0\n" +
		"Tokens-Cache-Read: 0\n" +
		"Tokens-Cost-USD: 0.0500\n" +
		"Loc-Prod-Before: 100\n" +
		"Loc-Prod-After: 110\n" +
		"Loc-Test-Before: 0\n" +
		"Loc-Test-After: 0\n" +
		"Duration-Seconds: 60\n"

	records := parseOutcomeRecords(logOutput)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1 (second block only)", len(records))
	}
	if records[0].TaskBranch != "task/gen-main-atlas-002" {
		t.Errorf("TaskBranch: got %q, want %q", records[0].TaskBranch, "task/gen-main-atlas-002")
	}
}

func TestParseOutcomeRecords_EmptyInput(t *testing.T) {
	t.Parallel()
	records := parseOutcomeRecords("")
	if len(records) != 0 {
		t.Errorf("got %d records for empty input, want 0", len(records))
	}
}

// --- extractBranchFromRefs ---

func TestExtractBranchFromRefs_HeadArrow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		refs string
		want string
	}{
		{"HEAD -> task/gen-main-abc, origin/task/gen-main-abc", "task/gen-main-abc"},
		{"HEAD -> main", "main"},
		{"task/gen-main-abc", "task/gen-main-abc"},
		{"task/gen-main-abc, origin/task/gen-main-abc", "task/gen-main-abc"},
		{"", ""},
	}
	for _, tc := range tests {
		got := extractBranchFromRefs(tc.refs)
		if got != tc.want {
			t.Errorf("extractBranchFromRefs(%q) = %q, want %q", tc.refs, got, tc.want)
		}
	}
}

// --- formatDuration ---

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		seconds int
		want    string
	}{
		{0, "0s"},
		{30, "30s"},
		{59, "59s"},
		{60, "1m0s"},
		{90, "1m30s"},
		{1234, "20m34s"},
	}
	for _, tc := range cases {
		got := formatDuration(tc.seconds)
		if got != tc.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}

// --- Outcomes (git-dependent, no t.Parallel) ---

func TestOutcomes_NoRecords(t *testing.T) {
	initTestGitRepo(t)
	o := &Orchestrator{}
	if err := o.Outcomes(); err != nil {
		t.Fatalf("Outcomes() error = %v", err)
	}
}

func TestOutcomes_WithRecords(t *testing.T) {
	initTestGitRepo(t)

	// Create a commit with outcome trailers so Outcomes can parse them.
	msg := "Implement feature X\n\n" +
		"Tokens-Input: 1000\n" +
		"Tokens-Output: 200\n" +
		"Tokens-Cache-Creation: 0\n" +
		"Tokens-Cache-Read: 0\n" +
		"Tokens-Cost-USD: 0.0500\n" +
		"Loc-Prod-Before: 100\n" +
		"Loc-Prod-After: 110\n" +
		"Loc-Test-Before: 0\n" +
		"Loc-Test-After: 0\n" +
		"Duration-Seconds: 60"
	gitRun(t, "commit", "--allow-empty", "--no-verify", "-m", msg)

	// Capture stdout to verify tabular output.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	o := &Orchestrator{}
	oErr := o.Outcomes()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stdout = oldStdout

	if oErr != nil {
		t.Fatalf("Outcomes() error = %v", oErr)
	}
	if !strings.Contains(buf.String(), "main") {
		t.Errorf("Outcomes output %q does not include branch 'main'", buf.String())
	}
}
