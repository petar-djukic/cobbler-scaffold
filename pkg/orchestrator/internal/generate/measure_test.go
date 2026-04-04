// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TruncateSHA
// ---------------------------------------------------------------------------

func TestTruncateSHA_Long(t *testing.T) {
	if got := TruncateSHA("abcdef1234567890"); got != "abcdef12" {
		t.Errorf("expected abcdef12, got %q", got)
	}
}

func TestTruncateSHA_Short(t *testing.T) {
	if got := TruncateSHA("abc"); got != "abc" {
		t.Errorf("expected abc, got %q", got)
	}
}

func TestTruncateSHA_Exact8(t *testing.T) {
	if got := TruncateSHA("12345678"); got != "12345678" {
		t.Errorf("expected 12345678, got %q", got)
	}
}

func TestTruncateSHA_Empty(t *testing.T) {
	if got := TruncateSHA(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// MeasureReleasesConstraint
// ---------------------------------------------------------------------------

func TestMeasureReleasesConstraint_MultipleReleases(t *testing.T) {
	got := MeasureReleasesConstraint([]string{"rel01.0", "rel02.0"}, "")
	if got == "" {
		t.Fatal("expected non-empty constraint")
	}
	if got[:2] != "\n\n" {
		t.Error("expected constraint to start with two newlines")
	}
}

func TestMeasureReleasesConstraint_SingleRelease(t *testing.T) {
	got := MeasureReleasesConstraint(nil, "rel01.0")
	if got == "" {
		t.Fatal("expected non-empty constraint")
	}
}

func TestMeasureReleasesConstraint_NoScope(t *testing.T) {
	got := MeasureReleasesConstraint(nil, "")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMeasureReleasesConstraint_ReleasesOverridesRelease(t *testing.T) {
	got := MeasureReleasesConstraint([]string{"rel01.0"}, "rel02.0")
	// Releases (list) takes precedence — should mention rel01.0, not rel02.0.
	if got == "" || len(got) < 10 {
		t.Fatal("expected non-empty constraint from releases list")
	}
}

// ---------------------------------------------------------------------------
// ValidationResult.HasErrors
// ---------------------------------------------------------------------------

func TestValidationResult_HasErrors_True(t *testing.T) {
	vr := ValidationResult{Errors: []string{"something"}}
	if !vr.HasErrors() {
		t.Error("expected HasErrors true")
	}
}

func TestValidationResult_HasErrors_False(t *testing.T) {
	vr := ValidationResult{Warnings: []string{"warning"}}
	if vr.HasErrors() {
		t.Error("expected HasErrors false")
	}
}

// ---------------------------------------------------------------------------
// UCStatusDone
// ---------------------------------------------------------------------------

func TestUCStatusDone_Implemented(t *testing.T) {
	if !UCStatusDone("implemented") {
		t.Error("expected true for implemented")
	}
}

func TestUCStatusDone_Done(t *testing.T) {
	if !UCStatusDone("done") {
		t.Error("expected true for done")
	}
}

func TestUCStatusDone_Closed(t *testing.T) {
	if !UCStatusDone("Closed") {
		t.Error("expected true for Closed (case-insensitive)")
	}
}

func TestUCStatusDone_SpecComplete(t *testing.T) {
	if UCStatusDone("spec_complete") {
		t.Error("expected false for spec_complete")
	}
}

func TestUCStatusDone_Empty(t *testing.T) {
	if UCStatusDone("") {
		t.Error("expected false for empty")
	}
}

// ---------------------------------------------------------------------------
// ValidateMeasureOutput
// ---------------------------------------------------------------------------

func TestValidateMeasureOutput_ValidCodeIssue(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: req1
  - id: R2
    text: req2
  - id: R3
    text: req3
  - id: R4
    text: req4
  - id: R5
    text: req5
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3
files:
  - path: pkg/foo/bar.go`

	issues := []ProposedIssue{{Index: 1, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, nil)
	if result.HasErrors() {
		t.Errorf("expected no errors for valid code issue, got: %v", result.Errors)
	}
}

func TestValidateMeasureOutput_TooFewRequirements(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: req1
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3`

	issues := []ProposedIssue{{Index: 1, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, nil)
	if !result.HasErrors() {
		t.Error("expected error for code issue with 1 requirement")
	}
}

func TestValidateMeasureOutput_P7Violation(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: req1
  - id: R2
    text: req2
  - id: R3
    text: req3
  - id: R4
    text: req4
  - id: R5
    text: req5
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3
files:
  - path: pkg/foo/foo.go`

	issues := []ProposedIssue{{Index: 1, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, nil)
	if !result.HasErrors() {
		t.Error("expected P7 violation error for foo/foo.go")
	}
}

func TestValidateMeasureOutput_MaxReqsExceeded(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: srd003 R2
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3`

	subItems := map[string]map[string]int{
		"srd003": {"R2": 10},
	}
	issues := []ProposedIssue{{Index: 1, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 5, 0, subItems, nil)
	if !result.HasErrors() {
		t.Error("expected error for expanded count exceeding max")
	}
}

func TestValidateMeasureOutput_UnparseableDescription(t *testing.T) {
	issues := []ProposedIssue{{Index: 1, Title: "bad", Description: ":::not yaml"}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, nil)
	if result.HasErrors() {
		t.Error("unparseable descriptions should produce warnings, not errors")
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning for unparseable description")
	}
}

func TestValidateMeasureOutput_EmptyIssues(t *testing.T) {
	result := ValidateMeasureOutput(nil, 0, 0, nil, nil)
	if result.HasErrors() {
		t.Error("expected no errors for empty issue list")
	}
}

func TestValidateMeasureOutput_DocType(t *testing.T) {
	desc := `deliverable_type: documentation
requirements:
  - id: R1
    text: req1
  - id: R2
    text: req2
  - id: R3
    text: req3
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3`

	issues := []ProposedIssue{{Index: 1, Title: "doc", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, nil)
	if result.HasErrors() {
		t.Errorf("expected no errors for valid doc issue, got: %v", result.Errors)
	}
}

func TestValidateMeasureOutput_CompletedRItemRejected(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: "Implement config per srd001 R1.2"
  - id: R2
    text: "Add validation per srd001 R2.1"
  - id: R3
    text: "Add logging per srd001 R3.1"
  - id: R4
    text: "Format output"
  - id: R5
    text: "Error handling"
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3
files:
  - path: pkg/foo/bar.go`

	reqStates := map[string]map[string]RequirementState{
		"srd001-core": {
			"R1.1": {Status: "ready"},
			"R1.2": {Status: "complete", Issue: 42},
			"R2.1": {Status: "ready"},
			"R3.1": {Status: "ready"},
		},
	}

	issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
	if !result.HasErrors() {
		t.Fatal("expected errors for proposal targeting completed R-item")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "R1.2") && strings.Contains(e, "already complete") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error mentioning R1.2 as complete, got: %v", result.Errors)
	}
}

func TestValidateMeasureOutput_CompletedGroupRejected(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: "Implement group per srd002 R1"
  - id: R2
    text: "Other work"
  - id: R3
    text: "More work"
  - id: R4
    text: "Even more"
  - id: R5
    text: "Last one"
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3
files:
  - path: pkg/foo/bar.go`

	reqStates := map[string]map[string]RequirementState{
		"srd002-lifecycle": {
			"R1.1": {Status: "complete", Issue: 10},
			"R1.2": {Status: "complete", Issue: 11},
		},
	}

	issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
	if !result.HasErrors() {
		t.Fatal("expected errors for proposal targeting fully complete group")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "R1") && strings.Contains(e, "fully complete") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error mentioning R1 group as fully complete, got: %v", result.Errors)
	}
}

func TestValidateMeasureOutput_NilReqStatesNoCheck(t *testing.T) {
	desc := `deliverable_type: code
requirements:
  - id: R1
    text: "Implement per srd001 R1.2"
  - id: R2
    text: r2
  - id: R3
    text: r3
  - id: R4
    text: r4
  - id: R5
    text: r5
acceptance_criteria:
  - id: AC1
    text: ac1
  - id: AC2
    text: ac2
  - id: AC3
    text: ac3
  - id: AC4
    text: ac4
  - id: AC5
    text: ac5
design_decisions:
  - id: DD1
    text: dd1
  - id: DD2
    text: dd2
  - id: DD3
    text: dd3
files:
  - path: pkg/foo/bar.go`

	issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, nil)
	if result.HasErrors() {
		t.Errorf("expected no errors when reqStates is nil, got: %v", result.Errors)
	}
}

// ---------------------------------------------------------------------------
// ExpandedRequirementCount
// ---------------------------------------------------------------------------

func TestExpandedRequirementCount_NoSubItems(t *testing.T) {
	reqs := []IssueDescItem{
		{ID: "R1", Text: "do something"},
		{ID: "R2", Text: "do another thing"},
	}
	if got := ExpandedRequirementCount(reqs, nil); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}

func TestExpandedRequirementCount_WithExpansion(t *testing.T) {
	reqs := []IssueDescItem{
		{ID: "R1", Text: "implement srd003 R2 stuff"},
	}
	subItems := map[string]map[string]int{
		"srd003": {"R2": 4},
	}
	if got := ExpandedRequirementCount(reqs, subItems); got != 4 {
		t.Errorf("expected 4, got %d", got)
	}
}

func TestExpandedRequirementCount_SpecificSubItem(t *testing.T) {
	reqs := []IssueDescItem{
		{ID: "R1", Text: "implement srd003 R2.3"},
	}
	subItems := map[string]map[string]int{
		"srd003": {"R2": 4},
	}
	// Specific sub-item reference counts as 1.
	if got := ExpandedRequirementCount(reqs, subItems); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestExpandedRequirementCount_UnknownSRD(t *testing.T) {
	reqs := []IssueDescItem{
		{ID: "R1", Text: "implement srd999 R1"},
	}
	subItems := map[string]map[string]int{
		"srd003": {"R2": 4},
	}
	if got := ExpandedRequirementCount(reqs, subItems); got != 1 {
		t.Errorf("expected 1 (unknown SRD), got %d", got)
	}
}

// ---------------------------------------------------------------------------
// AppendMeasureLog
// ---------------------------------------------------------------------------

func TestAppendMeasureLog_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	issues := []ProposedIssue{{Index: 1, Title: "task 1"}}
	AppendMeasureLog(dir, issues)

	data, err := os.ReadFile(filepath.Join(dir, "measure.yaml"))
	if err != nil {
		t.Fatalf("expected measure.yaml to exist: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty measure.yaml")
	}
}

func TestAppendMeasureLog_Appends(t *testing.T) {
	dir := t.TempDir()
	AppendMeasureLog(dir, []ProposedIssue{{Index: 1, Title: "first"}})
	AppendMeasureLog(dir, []ProposedIssue{{Index: 2, Title: "second"}})

	data, _ := os.ReadFile(filepath.Join(dir, "measure.yaml"))
	content := string(data)
	if !contains(content, "first") || !contains(content, "second") {
		t.Errorf("expected both issues in measure.yaml, got:\n%s", content)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s != "" && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// SRDRefPattern
// ---------------------------------------------------------------------------

func TestSRDRefPattern_MatchesGroup(t *testing.T) {
	matches := SRDRefPattern.FindStringSubmatch("implement srd003 R2 requirements")
	if matches == nil {
		t.Fatal("expected match")
	}
	if matches[1] != "srd003" || matches[2] != "2" || matches[3] != "" {
		t.Errorf("unexpected match: stem=%q group=%q sub=%q", matches[1], matches[2], matches[3])
	}
}

func TestSRDRefPattern_MatchesSubItem(t *testing.T) {
	matches := SRDRefPattern.FindStringSubmatch("implement srd004-ts R1.3")
	if matches == nil {
		t.Fatal("expected match")
	}
	if matches[1] != "srd004-ts" || matches[2] != "1" || matches[3] != "3" {
		t.Errorf("unexpected match: stem=%q group=%q sub=%q", matches[1], matches[2], matches[3])
	}
}

func TestSRDRefPattern_MatchesWithInterveningWord(t *testing.T) {
	// Claude sometimes writes "srd002-sys requirement R2.5" instead of "srd002-sys R2.5".
	matches := SRDRefPattern.FindStringSubmatch("Implement srd002-sys requirement R2.5 as specified")
	if matches == nil {
		t.Fatal("expected match")
	}
	if matches[1] != "srd002-sys" || matches[2] != "2" || matches[3] != "5" {
		t.Errorf("unexpected match: stem=%q group=%q sub=%q", matches[1], matches[2], matches[3])
	}
}

func TestSRDRefPattern_MatchesWithTwoInterveningWords(t *testing.T) {
	matches := SRDRefPattern.FindStringSubmatch("srd003-format requirement group R1")
	if matches == nil {
		t.Fatal("expected match")
	}
	if matches[1] != "srd003-format" || matches[2] != "1" || matches[3] != "" {
		t.Errorf("unexpected match: stem=%q group=%q sub=%q", matches[1], matches[2], matches[3])
	}
}

func TestSRDRefPattern_NoMatch(t *testing.T) {
	if SRDRefPattern.FindStringSubmatch("no srd reference here") != nil {
		t.Error("expected no match")
	}
}

// ---------------------------------------------------------------------------
// Weight-based validation (GH-1832)
// ---------------------------------------------------------------------------

func TestValidateMeasureOutput_WeightBudget(t *testing.T) {
	t.Parallel()
	reqStates := map[string]map[string]RequirementState{
		"srd001": {
			"R1.1": {Status: "ready", Weight: 1},
			"R1.2": {Status: "ready", Weight: 4},
			"R1.3": {Status: "ready", Weight: 3},
		},
	}
	issues := []ProposedIssue{{
		Index: 0,
		Title: "[stitch] srd001 R1.1-R1.3",
		Description: `deliverable_type: code
required_reading:
  - docs/specs/software-requirements/srd001.yaml
files:
  - path: pkg/foo/bar.go
    action: create
requirements:
  - id: R1
    text: "srd001 R1.1 simple"
  - id: R2
    text: "srd001 R1.2 complex parser"
  - id: R3
    text: "srd001 R1.3 moderate"
design_decisions:
  - id: D1
    text: Use standard library
  - id: D2
    text: Parse with regex
  - id: D3
    text: Return errors
acceptance_criteria:
  - id: AC1
    text: Passes tests
  - id: AC2
    text: Handles edge cases
  - id: AC3
    text: Error messages are clear
  - id: AC4
    text: No panics
  - id: AC5
    text: Documentation updated`,
	}}

	// Total weight = 1+4+3 = 8. Budget = 4 → should error.
	result := ValidateMeasureOutput(issues, 0, 4, nil, reqStates)
	if len(result.Errors) == 0 {
		t.Error("expected weight budget error, got none")
	}

	// Budget = 10 → should pass.
	result = ValidateMeasureOutput(issues, 0, 10, nil, reqStates)
	for _, e := range result.Errors {
		if contains(e, "total weight") {
			t.Errorf("unexpected weight error with budget 10: %s", e)
		}
	}
}

func TestExpandedRequirementWeight_SpecificItems(t *testing.T) {
	t.Parallel()
	reqStates := map[string]map[string]RequirementState{
		"srd001": {
			"R1.1": {Status: "ready", Weight: 2},
			"R1.2": {Status: "ready", Weight: 5},
		},
	}
	reqs := []IssueDescItem{
		{Text: "srd001 R1.1 simple"},
		{Text: "srd001 R1.2 complex"},
	}
	w := ExpandedRequirementWeight(reqs, nil, reqStates)
	if w != 7 {
		t.Errorf("ExpandedRequirementWeight = %d, want 7 (2+5)", w)
	}
}

func TestExpandedRequirementWeight_GroupReference(t *testing.T) {
	t.Parallel()
	reqStates := map[string]map[string]RequirementState{
		"srd002": {
			"R1.1": {Status: "ready", Weight: 1},
			"R1.2": {Status: "ready", Weight: 3},
			"R1.3": {Status: "ready", Weight: 2},
		},
	}
	reqs := []IssueDescItem{
		{Text: "srd002 R1 entire group"},
	}
	w := ExpandedRequirementWeight(reqs, nil, reqStates)
	if w != 6 {
		t.Errorf("ExpandedRequirementWeight = %d, want 6 (1+3+2)", w)
	}
}

func TestExpandedRequirementWeight_FallsBackToCount(t *testing.T) {
	t.Parallel()
	reqs := []IssueDescItem{
		{Text: "something without srd ref"},
		{Text: "another item"},
	}
	w := ExpandedRequirementWeight(reqs, nil, nil)
	if w != 2 {
		t.Errorf("ExpandedRequirementWeight with nil states = %d, want 2", w)
	}
}

