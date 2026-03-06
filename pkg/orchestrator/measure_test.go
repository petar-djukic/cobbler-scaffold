// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateMeasureOutput_CodeP9InRange(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Valid code task",
		Description: `deliverable_type: code
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if vr.HasErrors() {
		t.Errorf("expected no errors for valid code task, got: %v", vr.Errors)
	}
}

func TestValidateMeasureOutput_CodeP9TooFewRequirements(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Underconstrained task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: req1
  - id: R2
    text: req2
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if !vr.HasErrors() {
		t.Error("expected errors for code task with 2 requirements (P9 range 5-8)")
	}
	if len(vr.Errors) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(vr.Errors), vr.Errors)
	}
}

func TestValidateMeasureOutput_CodeP9TooManyRequirements(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Overconstrained task",
		Description: `deliverable_type: code
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
  - id: R6
    text: req6
  - id: R7
    text: req7
  - id: R8
    text: req8
  - id: R9
    text: req9
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if !vr.HasErrors() {
		t.Error("expected errors for code task with 9 requirements (P9 range 5-8)")
	}
}

func TestValidateMeasureOutput_DocP9InRange(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Valid doc task",
		Description: `deliverable_type: documentation
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
    text: ac3
`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if vr.HasErrors() {
		t.Errorf("expected no errors for valid doc task, got: %v", vr.Errors)
	}
}

func TestValidateMeasureOutput_DocP9TooManyRequirements(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Over-specified doc",
		Description: `deliverable_type: documentation
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
`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if !vr.HasErrors() {
		t.Error("expected errors for doc task with 5 requirements (P9 range 2-4)")
	}
}

func TestValidateMeasureOutput_P7ViolationFileNameMatchesPackage(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "P7 violation task",
		Description: `deliverable_type: code
files:
  - path: pkg/testutils/testutils.go
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if !vr.HasErrors() {
		t.Error("expected errors for file named after package (P7 violation)")
	}
	found := false
	for _, e := range vr.Errors {
		if contains(e, "P7 violation") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected P7 violation error, got: %v", vr.Errors)
	}
}

func TestValidateMeasureOutput_P7NoViolation(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Good naming task",
		Description: `deliverable_type: code
files:
  - path: pkg/difftest/runner.go
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}

	// runner.go in pkg/difftest/ is NOT a P7 violation because
	// the file name does not match the parent directory name.
	vr := validateMeasureOutput(issues, 0, nil)
	p7Errors := 0
	for _, e := range vr.Errors {
		if contains(e, "P7 violation") {
			p7Errors++
		}
	}
	if p7Errors > 0 {
		t.Errorf("expected no P7 violation for difftest/runner.go, got %d: %v", p7Errors, vr.Errors)
	}
}

func TestValidateMeasureOutput_UnparseableDescription(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{{
		Index: 0,
		Title: "Bad YAML task",
		Description: `{{{not valid yaml`,
	}}

	vr := validateMeasureOutput(issues, 0, nil)
	if len(vr.Warnings) == 0 {
		t.Error("expected warning for unparseable description")
	}
}

func TestValidateMeasureOutput_MultipleIssues(t *testing.T) {
	t.Parallel()
	issues := []proposedIssue{
		{
			Index: 0,
			Title: "Valid task",
			Description: `deliverable_type: code
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
		},
		{
			Index: 1,
			Title: "Invalid task",
			Description: `deliverable_type: code
requirements:
  - id: R1
    text: req1
acceptance_criteria:
  - id: AC1
    text: ac1
`,
		},
	}

	vr := validateMeasureOutput(issues, 0, nil)
	if !vr.HasErrors() {
		t.Error("expected errors from invalid second issue")
	}
}

func TestValidationResult_HasErrors(t *testing.T) {
	t.Parallel()

	empty := validationResult{}
	if empty.HasErrors() {
		t.Error("empty result should not have errors")
	}

	warningsOnly := validationResult{Warnings: []string{"warn"}}
	if warningsOnly.HasErrors() {
		t.Error("warnings-only result should not have errors")
	}

	withErrors := validationResult{Errors: []string{"err"}}
	if !withErrors.HasErrors() {
		t.Error("result with errors should have errors")
	}
}


// --- MaxRequirementsPerTask limit ---

func TestValidateMeasureOutput_MaxReqs_ZeroIsUnlimited(t *testing.T) {
	t.Parallel()
	// maxReqs=0 must never trigger an error, even with 10 requirements.
	var reqs string
	for i := 1; i <= 10; i++ {
		reqs += "  - id: R" + fmt.Sprintf("%d", i) + "\n    text: req\n"
	}
	issues := []proposedIssue{{
		Index:       0,
		Title:       "Huge task",
		Description: "deliverable_type: code\nrequirements:\n" + reqs,
	}}
	vr := validateMeasureOutput(issues, 0, nil)
	for _, e := range vr.Errors {
		if contains(e, "max is") {
			t.Errorf("maxReqs=0 should not produce max-requirements error, got: %s", e)
		}
	}
}

func TestValidateMeasureOutput_MaxReqs_ExactlyAtLimit_NoError(t *testing.T) {
	t.Parallel()
	// 5 requirements with maxReqs=5 must not trigger the limit error.
	issues := []proposedIssue{{
		Index: 0,
		Title: "At-limit task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: req
  - id: R2
    text: req
  - id: R3
    text: req
  - id: R4
    text: req
  - id: R5
    text: req
`,
	}}
	vr := validateMeasureOutput(issues, 5, nil)
	for _, e := range vr.Errors {
		if contains(e, "max is") {
			t.Errorf("5 requirements at maxReqs=5 should not error, got: %s", e)
		}
	}
}

func TestValidateMeasureOutput_MaxReqs_ExceedsLimit_Error(t *testing.T) {
	t.Parallel()
	// 6 requirements with maxReqs=5 must produce a max-requirements error.
	issues := []proposedIssue{{
		Index: 0,
		Title: "Oversized task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: req
  - id: R2
    text: req
  - id: R3
    text: req
  - id: R4
    text: req
  - id: R5
    text: req
  - id: R6
    text: req
`,
	}}
	vr := validateMeasureOutput(issues, 5, nil)
	found := false
	for _, e := range vr.Errors {
		if contains(e, "max is") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected max-requirements error for 6 reqs with limit 5, got errors: %v", vr.Errors)
	}
}

func TestValidateMeasureOutput_MaxReqs_ErrorMentionsCountAndLimit(t *testing.T) {
	t.Parallel()
	// Error message must include both the actual count and the configured limit.
	issues := []proposedIssue{{
		Index: 1,
		Title: "Task Title",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: req
  - id: R2
    text: req
  - id: R3
    text: req
  - id: R4
    text: req
  - id: R5
    text: req
  - id: R6
    text: req
  - id: R7
    text: req
  - id: R8
    text: req
`,
	}}
	vr := validateMeasureOutput(issues, 5, nil)
	found := false
	for _, e := range vr.Errors {
		if contains(e, "8") && contains(e, "5") && contains(e, "Task Title") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("error should mention count (8), limit (5), and title; got: %v", vr.Errors)
	}
}


// --- expandedRequirementCount ---

func TestExpandedRequirementCount_NilSubItemCounts(t *testing.T) {
	t.Parallel()
	reqs := []issueDescItem{
		{ID: "R1", Text: "Implement prd003 R2"},
		{ID: "R2", Text: "Something else"},
	}
	got := expandedRequirementCount(reqs, nil)
	if got != 2 {
		t.Errorf("expandedRequirementCount with nil map = %d, want 2", got)
	}
}

func TestExpandedRequirementCount_GroupExpansion(t *testing.T) {
	t.Parallel()
	subItems := map[string]map[string]int{
		"prd003": {"R2": 4, "R5": 2},
	}
	reqs := []issueDescItem{
		{ID: "R1", Text: "Implement prd003 R2"},       // expands to 4
		{ID: "R2", Text: "Handle prd003 R5"},           // expands to 2
		{ID: "R3", Text: "Something with no PRD ref"},  // counts as 1
	}
	got := expandedRequirementCount(reqs, subItems)
	if got != 7 {
		t.Errorf("expandedRequirementCount = %d, want 7 (4+2+1)", got)
	}
}

func TestExpandedRequirementCount_SubItemRefCountsAsOne(t *testing.T) {
	t.Parallel()
	subItems := map[string]map[string]int{
		"prd003": {"R2": 4},
	}
	reqs := []issueDescItem{
		{ID: "R1", Text: "Implement prd003 R2.3"}, // specific sub-item = 1
	}
	got := expandedRequirementCount(reqs, subItems)
	if got != 1 {
		t.Errorf("expandedRequirementCount for sub-item ref = %d, want 1", got)
	}
}

func TestExpandedRequirementCount_UnknownPRDCountsAsOne(t *testing.T) {
	t.Parallel()
	subItems := map[string]map[string]int{
		"prd003": {"R2": 4},
	}
	reqs := []issueDescItem{
		{ID: "R1", Text: "Implement prd999 R1"}, // unknown PRD
	}
	got := expandedRequirementCount(reqs, subItems)
	if got != 1 {
		t.Errorf("expandedRequirementCount for unknown PRD = %d, want 1", got)
	}
}

func TestExpandedRequirementCount_FuzzyPRDStemMatch(t *testing.T) {
	t.Parallel()
	// "prd003-cobbler-workflows" mapped under both full stem and "prd003".
	subItems := map[string]map[string]int{
		"prd003-cobbler-workflows": {"R2": 4},
		"prd003":                   {"R2": 4},
	}
	reqs := []issueDescItem{
		{ID: "R1", Text: "Implement prd003 R2"}, // matches short prefix
	}
	got := expandedRequirementCount(reqs, subItems)
	if got != 4 {
		t.Errorf("expandedRequirementCount with fuzzy match = %d, want 4", got)
	}
}

// --- validateMeasureOutput expanded count enforcement (GH-535) ---

func TestValidateMeasureOutput_ExpandedCount_ExceedsLimit_HardError(t *testing.T) {
	t.Parallel()
	// 4 listed requirements (within limit), but prd003 R2 expands to 10 sub-items.
	// Expanded total = 10+3 = 13, maxReqs = 8 → hard error.
	subItems := map[string]map[string]int{
		"prd003": {"R2": 10},
	}
	issues := []proposedIssue{{
		Index: 0,
		Title: "Expanded task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: Implement prd003 R2
  - id: R2
    text: plain req
  - id: R3
    text: another req
  - id: R4
    text: yet another
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}
	vr := validateMeasureOutput(issues, 8, subItems)
	found := false
	for _, e := range vr.Errors {
		if contains(e, "expanded sub-item count") && contains(e, "max is") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected hard error for expanded count 13 > maxReqs 8, got errors: %v", vr.Errors)
	}
}

func TestValidateMeasureOutput_ExpandedCount_WithinLimit_NoError(t *testing.T) {
	t.Parallel()
	// prd003 R2 has 2 sub-items; total expanded = 2+3 = 5, maxReqs = 8 → no error.
	subItems := map[string]map[string]int{
		"prd003": {"R2": 2},
	}
	issues := []proposedIssue{{
		Index: 0,
		Title: "Small task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: Implement prd003 R2
  - id: R2
    text: plain req
  - id: R3
    text: another req
  - id: R4
    text: yet another
  - id: R5
    text: last one
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}
	// expanded = 2+4 = 6, maxReqs = 8 → no expanded-count error.
	vr := validateMeasureOutput(issues, 8, subItems)
	for _, e := range vr.Errors {
		if contains(e, "expanded sub-item count") {
			t.Errorf("should not error when expanded count under limit, got: %s", e)
		}
	}
}

func TestValidateMeasureOutput_NoExpandedErrorWhenUnderLimit(t *testing.T) {
	t.Parallel()
	subItems := map[string]map[string]int{
		"prd003": {"R2": 2},
	}
	issues := []proposedIssue{{
		Index: 0,
		Title: "Small task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: Implement prd003 R2
  - id: R2
    text: plain req
  - id: R3
    text: another req
  - id: R4
    text: yet another
  - id: R5
    text: last one
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
  - id: D1
    text: d1
  - id: D2
    text: d2
  - id: D3
    text: d3
`,
	}}
	// 5 listed, expanded = 2+4 = 6. maxReqs = 8. Under limit — no error.
	vr := validateMeasureOutput(issues, 8, subItems)
	for _, e := range vr.Errors {
		if contains(e, "expanded sub-item count") {
			t.Errorf("should not error when expanded count under limit, got: %s", e)
		}
	}
}

// GH-847: filterImplementedReleases / filterImplementedRelease ----------------

func TestFilterImplementedReleases_DropsImplemented(t *testing.T) {
	_, cleanup := setupMeasureRoadmapDir(t, `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    status: implemented
    use_cases: []
  - version: "02.0"
    status: spec_complete
    use_cases: []
`, "", "")
	defer cleanup()

	got := filterImplementedReleases([]string{"01.0", "02.0"})
	if len(got) != 1 || got[0] != "02.0" {
		t.Errorf("filterImplementedReleases = %v; want [02.0]", got)
	}
}

func TestFilterImplementedReleases_AllImplemented_ReturnsNil(t *testing.T) {
	_, cleanup := setupMeasureRoadmapDir(t, `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    status: implemented
    use_cases: []
`, "", "")
	defer cleanup()

	got := filterImplementedReleases([]string{"01.0"})
	if len(got) != 0 {
		t.Errorf("filterImplementedReleases all implemented = %v; want nil/empty", got)
	}
}

func TestFilterImplementedReleases_UnknownReleaseKept(t *testing.T) {
	_, cleanup := setupMeasureRoadmapDir(t, `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    status: implemented
    use_cases: []
`, "", "")
	defer cleanup()

	// "99.0" not in road-map → unknown status → kept.
	got := filterImplementedReleases([]string{"99.0"})
	if len(got) != 1 || got[0] != "99.0" {
		t.Errorf("filterImplementedReleases unknown = %v; want [99.0]", got)
	}
}

func TestFilterImplementedRelease_ImplementedReturnsEmpty(t *testing.T) {
	_, cleanup := setupMeasureRoadmapDir(t, `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    status: implemented
    use_cases: []
`, "", "")
	defer cleanup()

	got := filterImplementedRelease("01.0")
	if got != "" {
		t.Errorf("filterImplementedRelease implemented = %q; want empty", got)
	}
}

func TestFilterImplementedRelease_NotImplementedKept(t *testing.T) {
	_, cleanup := setupMeasureRoadmapDir(t, `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    status: spec_complete
    use_cases: []
`, "", "")
	defer cleanup()

	got := filterImplementedRelease("01.0")
	if got != "01.0" {
		t.Errorf("filterImplementedRelease spec_complete = %q; want 01.0", got)
	}
}

func TestMeasureReleasesConstraint_WithReleases(t *testing.T) {
	t.Parallel()
	got := measureReleasesConstraint([]string{"01.0", "02.0"}, "")
	if !contains(got, "01.0, 02.0") {
		t.Errorf("expected releases list in constraint, got %q", got)
	}
	if !contains(got, "MUST") {
		t.Errorf("expected hard constraint keyword, got %q", got)
	}
}

func TestMeasureReleasesConstraint_WithRelease(t *testing.T) {
	t.Parallel()
	got := measureReleasesConstraint(nil, "01.0")
	if !contains(got, "01.0") {
		t.Errorf("expected release in constraint, got %q", got)
	}
	if !contains(got, "MUST") {
		t.Errorf("expected hard constraint keyword, got %q", got)
	}
}

func TestMeasureReleasesConstraint_None(t *testing.T) {
	t.Parallel()
	got := measureReleasesConstraint(nil, "")
	if got != "" {
		t.Errorf("expected empty constraint, got %q", got)
	}
}

func TestMeasureReleasesConstraint_ReleasesTakePrecedence(t *testing.T) {
	t.Parallel()
	got := measureReleasesConstraint([]string{"01.0"}, "00.5")
	if !contains(got, "01.0") {
		t.Errorf("expected releases list in constraint, got %q", got)
	}
	if contains(got, "00.5") {
		t.Errorf("expected legacy release to be ignored when releases is set, got %q", got)
	}
}

// --- truncateSHA ---

func TestTruncateSHA_LongSHA(t *testing.T) {
	t.Parallel()
	got := truncateSHA("abc123def456789")
	if got != "abc123de" {
		t.Errorf("truncateSHA(long) = %q, want %q", got, "abc123de")
	}
}

func TestTruncateSHA_ExactlyEight(t *testing.T) {
	t.Parallel()
	got := truncateSHA("12345678")
	if got != "12345678" {
		t.Errorf("truncateSHA(8 chars) = %q, want %q", got, "12345678")
	}
}

func TestTruncateSHA_ShortSHA(t *testing.T) {
	t.Parallel()
	got := truncateSHA("abc")
	if got != "abc" {
		t.Errorf("truncateSHA(short) = %q, want %q", got, "abc")
	}
}

func TestTruncateSHA_Empty(t *testing.T) {
	t.Parallel()
	got := truncateSHA("")
	if got != "" {
		t.Errorf("truncateSHA(\"\") = %q, want \"\"", got)
	}
}

// --- appendMeasureLog ---

func TestAppendMeasureLog_NewFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	issues := []proposedIssue{
		{Index: 1, Title: "Task A", Description: "desc-a"},
		{Index: 2, Title: "Task B", Description: "desc-b"},
	}

	appendMeasureLog(dir, issues)

	data, err := os.ReadFile(filepath.Join(dir, "measure.yaml"))
	if err != nil {
		t.Fatalf("measure.yaml not created: %v", err)
	}

	var loaded []proposedIssue
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("measure.yaml unmarshal: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 issues in measure.yaml, got %d", len(loaded))
	}
}

func TestAppendMeasureLog_AppendsToExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Seed with one existing issue.
	seed := []proposedIssue{{Index: 1, Title: "Existing"}}
	seedData, _ := yaml.Marshal(seed)
	os.WriteFile(filepath.Join(dir, "measure.yaml"), seedData, 0o644)

	// Append a new issue.
	appendMeasureLog(dir, []proposedIssue{{Index: 2, Title: "New"}})

	data, err := os.ReadFile(filepath.Join(dir, "measure.yaml"))
	if err != nil {
		t.Fatalf("measure.yaml read: %v", err)
	}
	var loaded []proposedIssue
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("measure.yaml unmarshal: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 issues after append, got %d", len(loaded))
	}
	if loaded[0].Title != "Existing" || loaded[1].Title != "New" {
		t.Errorf("unexpected order: %v", loaded)
	}
}

func TestAppendMeasureLog_CorruptExistingStartsFresh(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write corrupt YAML.
	os.WriteFile(filepath.Join(dir, "measure.yaml"), []byte("{{{not yaml"), 0o644)

	// Append should recover and write just the new issues.
	appendMeasureLog(dir, []proposedIssue{{Index: 1, Title: "Fresh"}})

	data, _ := os.ReadFile(filepath.Join(dir, "measure.yaml"))
	var loaded []proposedIssue
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("measure.yaml unmarshal: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Title != "Fresh" {
		t.Errorf("expected fresh start with 1 issue, got %v", loaded)
	}
}

func TestAppendMeasureLog_EmptyNewIssues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Seed with one issue, then append nothing.
	seed := []proposedIssue{{Index: 1, Title: "Existing"}}
	seedData, _ := yaml.Marshal(seed)
	os.WriteFile(filepath.Join(dir, "measure.yaml"), seedData, 0o644)

	appendMeasureLog(dir, nil)

	data, _ := os.ReadFile(filepath.Join(dir, "measure.yaml"))
	var loaded []proposedIssue
	yaml.Unmarshal(data, &loaded)
	if len(loaded) != 1 {
		t.Errorf("expected 1 issue after appending nil, got %d", len(loaded))
	}
}

// --- saveHistory ---

func TestSaveHistory_WritesIssuesFile(t *testing.T) {
	t.Parallel()
	histDir := t.TempDir()
	cobblerDir := t.TempDir()

	o := New(Config{})
	o.cfg.Cobbler.Dir = cobblerDir
	o.cfg.Cobbler.HistoryDir = histDir

	// Create the issues file that saveHistory reads.
	issuesFile := filepath.Join(cobblerDir, "measure-test.yaml")
	os.WriteFile(issuesFile, []byte("- title: test issue\n"), 0o644)

	o.saveHistory("2026-02-28-12-00-00", []byte("raw output"), issuesFile)

	// Check that the issues file was copied to history.
	histIssues := filepath.Join(histDir, "2026-02-28-12-00-00-measure-issues.yaml")
	data, err := os.ReadFile(histIssues)
	if err != nil {
		t.Fatalf("history issues file not created: %v", err)
	}
	if string(data) != "- title: test issue\n" {
		t.Errorf("history issues content = %q, want %q", string(data), "- title: test issue\n")
	}
}

func TestSaveHistory_NoHistoryDir(t *testing.T) {
	t.Parallel()
	o := New(Config{})
	// HistoryDir is empty — saveHistory should be a no-op.
	o.saveHistory("2026-02-28-12-00-00", []byte("output"), "/nonexistent/file")
	// No panic is the assertion.
}

func TestSaveHistory_MissingIssuesFile(t *testing.T) {
	t.Parallel()
	histDir := t.TempDir()
	o := New(Config{})
	o.cfg.Cobbler.HistoryDir = histDir

	// Call with nonexistent issues file — should not panic.
	o.saveHistory("2026-02-28-12-00-00", []byte("output"), "/nonexistent/file.yaml")

	// The issues file should not have been created.
	matches, _ := filepath.Glob(filepath.Join(histDir, "*issues*"))
	if len(matches) > 0 {
		t.Errorf("expected no issues file in history, got %v", matches)
	}
}

// --- buildMeasurePrompt ---

func TestBuildMeasurePrompt_DefaultConfig(t *testing.T) {
	t.Parallel()
	o := New(Config{})

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "role:") {
		t.Error("prompt missing 'role:' field")
	}
	if !strings.Contains(prompt, "planning_constitution:") {
		t.Error("prompt missing 'planning_constitution:' field")
	}
	if !strings.Contains(prompt, "issue_format_constitution:") {
		t.Error("prompt missing 'issue_format_constitution:' field")
	}
}

func TestBuildMeasurePrompt_PlaceholderSubstitution(t *testing.T) {
	t.Parallel()
	o := New(Config{})
	o.cfg.Cobbler.EstimatedLinesMin = 100
	o.cfg.Cobbler.EstimatedLinesMax = 500
	o.cfg.Cobbler.MaxRequirementsPerTask = 8

	prompt, err := o.buildMeasurePrompt("", "", 3)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}

	// The limit placeholder should be substituted with "3".
	if !strings.Contains(prompt, "3") {
		t.Error("prompt should contain the limit value")
	}
}

func TestBuildMeasurePrompt_WithUserInput(t *testing.T) {
	t.Parallel()
	o := New(Config{})

	prompt, err := o.buildMeasurePrompt("Focus on testing", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Focus on testing") {
		t.Error("prompt should contain user input")
	}
}

func TestBuildMeasurePrompt_WithExistingIssues(t *testing.T) {
	t.Parallel()
	o := New(Config{})

	existingIssues := `[{"id":"42","title":"Existing task","status":"ready","type":""}]`
	prompt, err := o.buildMeasurePrompt("", existingIssues, 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Existing task") {
		t.Error("prompt should contain existing issues context")
	}
}

func TestBuildMeasurePrompt_InvalidTemplate(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Cobbler.MeasurePrompt = "role: [unclosed bracket"
	o := New(cfg)

	_, err := o.buildMeasurePrompt("", "", 1)
	if err == nil {
		t.Error("expected error for invalid template, got nil")
	}
}

func TestBuildMeasurePrompt_ReleasesConstraintAppended(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Project.Releases = []string{"01.0", "02.0"}
	o := New(cfg)

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "01.0, 02.0") {
		t.Error("prompt should contain releases constraint")
	}
	if !strings.Contains(prompt, "MUST") {
		t.Error("prompt should contain hard constraint keyword")
	}
}

func TestBuildMeasurePrompt_GoldenExample(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Cobbler.GoldenExample = "This is a golden example issue"
	o := New(cfg)

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "golden example issue") {
		t.Error("prompt should contain golden example")
	}
}

// --- importIssuesImpl YAML parsing ---

func TestImportIssuesImpl_NonexistentFile(t *testing.T) {
	t.Parallel()
	o := New(Config{})
	_, _, err := o.importIssuesImpl("/nonexistent/file.yaml", "owner/repo", "gen", false, 0)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestImportIssuesImpl_InvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "bad.yaml")
	os.WriteFile(yamlFile, []byte("{{{not valid yaml"), 0o644)

	o := New(Config{})
	_, _, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", false, 0)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "YAML") {
		t.Errorf("error should mention YAML, got: %v", err)
	}
}

func TestImportIssuesImpl_EmptyIssueList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "empty.yaml")
	os.WriteFile(yamlFile, []byte("[]\n"), 0o644)

	cfg := Config{}
	cfg.Cobbler.Dir = dir
	o := New(cfg)

	// Empty list should not error — no issues to create, no GitHub calls.
	ids, _, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", false, 0)
	if err != nil {
		t.Fatalf("importIssuesImpl() error = %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids for empty issue list, got %d", len(ids))
	}
}

func TestImportIssuesImpl_ValidationRejectsInEnforcingMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "issues.yaml")

	// Create a code issue with only 1 requirement — violates P9 range 5-8.
	issues := []proposedIssue{{
		Index: 1,
		Title: "Bad task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: req1
acceptance_criteria:
  - id: AC1
    text: ac1
`,
	}}
	data, _ := yaml.Marshal(issues)
	os.WriteFile(yamlFile, data, 0o644)

	cfg := Config{}
	cfg.Cobbler.Dir = dir
	cfg.Cobbler.EnforceMeasureValidation = true
	o := New(cfg)

	_, validationErrs, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", false, 0)
	if err == nil {
		t.Error("expected validation error in enforcing mode")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error should mention validation, got: %v", err)
	}
	if len(validationErrs) == 0 {
		t.Error("expected non-empty validationErrs slice when validation fails")
	}
}

func TestImportIssuesImpl_ValidationPassesWhenSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "issues.yaml")

	// Same invalid issue but with skipEnforcement=true.
	issues := []proposedIssue{{
		Index: 1,
		Title: "Bad task",
		Description: `deliverable_type: code
requirements:
  - id: R1
    text: req1
acceptance_criteria:
  - id: AC1
    text: ac1
`,
	}}
	data, _ := yaml.Marshal(issues)
	os.WriteFile(yamlFile, data, 0o644)

	cfg := Config{}
	cfg.Cobbler.Dir = dir
	cfg.Cobbler.EnforceMeasureValidation = true
	o := New(cfg)

	// skipEnforcement=true should bypass validation errors.
	// This will fail at createCobblerIssue (no real GitHub), but should NOT
	// fail at validation.
	ids, _, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", true, 0)
	if err != nil {
		t.Fatalf("importIssuesImpl() with skipEnforcement should not return validation error, got: %v", err)
	}
	// ids will be empty because createCobblerIssue fails (no GitHub), but no error returned.
	_ = ids
}

// --- importIssuesImpl upgrade path (GH-578) ---

// singleDocIssue returns YAML for one minimal documentation issue.
func singleDocIssue(index int, title string) []proposedIssue {
	desc := "deliverable_type: documentation\nrequirements:\n  - id: R1\n    text: r1\n  - id: R2\n    text: r2\nacceptance_criteria:\n  - id: AC1\n    text: ac1\n  - id: AC2\n    text: ac2\n  - id: AC3\n    text: ac3\n"
	return []proposedIssue{{Index: index, Title: title, Description: desc}}
}

// TestImportIssuesImpl_UpgradePath_PhZero_SingleIssue verifies that ph=0
// skips the upgrade path even when exactly one issue is proposed, falling
// through to createCobblerIssue (which fails gracefully without real GitHub).
func TestImportIssuesImpl_UpgradePath_PhZero_SingleIssue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "issues.yaml")
	data, _ := yaml.Marshal(singleDocIssue(1, "only task"))
	os.WriteFile(yamlFile, data, 0o644)

	cfg := Config{}
	cfg.Cobbler.Dir = dir
	o := New(cfg)

	ids, _, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", false, 0)
	if err != nil {
		t.Fatalf("importIssuesImpl() unexpected error: %v", err)
	}
	// createCobblerIssue fails (no real GitHub); ids empty is expected.
	_ = ids
}

// TestImportIssuesImpl_UpgradePath_PhPositive_SingleIssue verifies that when
// ph > 0 and exactly one issue is proposed, the upgrade path is attempted.
// Since gh is not available, upgradeMeasuringPlaceholder fails and the fallback
// createCobblerIssue is invoked. Both fail gracefully; no top-level error.
func TestImportIssuesImpl_UpgradePath_PhPositive_SingleIssue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "issues.yaml")
	data, _ := yaml.Marshal(singleDocIssue(1, "only task"))
	os.WriteFile(yamlFile, data, 0o644)

	cfg := Config{}
	cfg.Cobbler.Dir = dir
	o := New(cfg)

	// ph=99 triggers the upgrade path; both gh calls fail without real GitHub.
	ids, _, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", false, 99)
	if err != nil {
		t.Fatalf("importIssuesImpl() unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids when gh unavailable, got %d", len(ids))
	}
}

// TestImportIssuesImpl_UpgradePath_PhPositive_MultipleIssues verifies that when
// ph > 0 but more than one issue is proposed, the upgrade path is skipped and
// createCobblerIssue is invoked for each issue (fails gracefully without gh).
func TestImportIssuesImpl_UpgradePath_PhPositive_MultipleIssues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "issues.yaml")
	issues := append(singleDocIssue(1, "task one"), singleDocIssue(2, "task two")...)
	data, _ := yaml.Marshal(issues)
	os.WriteFile(yamlFile, data, 0o644)

	cfg := Config{}
	cfg.Cobbler.Dir = dir
	o := New(cfg)

	// ph=42 but 2 issues: upgrade path must not be taken.
	ids, _, err := o.importIssuesImpl(yamlFile, "owner/repo", "gen", false, 42)
	if err != nil {
		t.Fatalf("importIssuesImpl() unexpected error: %v", err)
	}
	// createCobblerIssue fails (no real GitHub); ids empty is expected.
	_ = ids
}

// --- MeasurePrompt (stdout entry point) ---

func TestMeasurePrompt_ProducesOutput(t *testing.T) {
	o := New(Config{})

	// Redirect stdout to capture output.
	oldStdout := os.Stdout
	null, _ := os.Open(os.DevNull)
	os.Stdout = null
	defer func() {
		os.Stdout = oldStdout
		null.Close()
	}()

	err := o.MeasurePrompt()
	if err != nil {
		t.Errorf("MeasurePrompt() unexpected error: %v", err)
	}
}

func TestBuildMeasurePrompt_WithValidationErrors(t *testing.T) {
	t.Parallel()
	o := New(Config{})

	errs := []string{
		`[1] "My task": requirement count 9 outside P9 range 5-8`,
		`[1] "My task": design decision count 2 outside P9 range 3-5`,
	}
	prompt, err := o.buildMeasurePrompt("", "", 1, errs...)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "validation_errors:") {
		t.Error("prompt should contain validation_errors key")
	}
	if !strings.Contains(prompt, "requirement count 9") {
		t.Error("prompt should contain first validation error")
	}
	if !strings.Contains(prompt, "design decision count 2") {
		t.Error("prompt should contain second validation error")
	}
}

func TestBuildMeasurePrompt_NoValidationErrorsOnFirstAttempt(t *testing.T) {
	t.Parallel()
	o := New(Config{})

	// No validation errors passed — field must be absent from the YAML output.
	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "validation_errors:") {
		t.Error("prompt must not contain validation_errors on first attempt (no errors)")
	}
}

// contains checks if substr is in s. Avoids importing strings in test.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// warnOversizedGroups tests (GH-508 audit)
// ---------------------------------------------------------------------------

func TestWarnOversizedGroups_NoPRDs(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// No PRD files present — function must not panic.
	warnOversizedGroups(5)
}

func TestWarnOversizedGroups_WithinLimit(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	prd := `id: prd001-test
title: Test PRD
problem: test
goals: []
requirements:
  R1:
    title: Group One
    items:
      - R1.1: item one
      - R1.2: item two
non_goals: []
acceptance_criteria: []
`
	os.WriteFile("docs/specs/product-requirements/prd001-test.yaml", []byte(prd), 0o644)

	// 2 items, maxReqs=5 → no warning, no panic.
	warnOversizedGroups(5)
}

func TestWarnOversizedGroups_OversizedGroup(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	prd := `id: prd001-test
title: Test PRD
problem: test
goals: []
requirements:
  R1:
    title: Big Group
    items:
      - R1.1: a
      - R1.2: b
      - R1.3: c
      - R1.4: d
      - R1.5: e
      - R1.6: f
non_goals: []
acceptance_criteria: []
`
	os.WriteFile("docs/specs/product-requirements/prd001-test.yaml", []byte(prd), 0o644)

	// 6 items, maxReqs=3 → should log a warning but not panic.
	warnOversizedGroups(3)
}

// ---------------------------------------------------------------------------
// loadPRDSubItemCounts tests (GH-508 audit)
// ---------------------------------------------------------------------------

func TestLoadPRDSubItemCounts_NoPRDs(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	counts := loadPRDSubItemCounts()
	if counts == nil {
		t.Error("expected non-nil map")
	}
	if len(counts) != 0 {
		t.Errorf("expected empty map with no PRDs, got %d entries", len(counts))
	}
}

func TestLoadPRDSubItemCounts_WithPRD(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	prd := `id: prd003-wf
title: Workflow PRD
problem: test
goals: []
requirements:
  R1:
    title: Group One
    items:
      - R1.1: item1
      - R1.2: item2
      - R1.3: item3
  R2:
    title: Group Two
    items:
      - R2.1: itemA
      - R2.2: itemB
non_goals: []
acceptance_criteria: []
`
	os.WriteFile("docs/specs/product-requirements/prd003-wf.yaml", []byte(prd), 0o644)

	counts := loadPRDSubItemCounts()

	// Full stem entry.
	if counts["prd003-wf"]["R1"] != 3 {
		t.Errorf("expected R1=3, got %d", counts["prd003-wf"]["R1"])
	}
	if counts["prd003-wf"]["R2"] != 2 {
		t.Errorf("expected R2=2, got %d", counts["prd003-wf"]["R2"])
	}
	// Short prefix entry.
	if counts["prd003"]["R1"] != 3 {
		t.Errorf("expected short prefix R1=3, got %d", counts["prd003"]["R1"])
	}
}

func TestLoadPRDSubItemCounts_EmptyItemsCountAs1(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	prd := `id: prd005-empty
title: PRD with empty group
problem: test
goals: []
requirements:
  R1:
    title: Empty Group
    items: []
non_goals: []
acceptance_criteria: []
`
	os.WriteFile("docs/specs/product-requirements/prd005-empty.yaml", []byte(prd), 0o644)

	counts := loadPRDSubItemCounts()

	// Empty items list → count defaults to 1.
	if counts["prd005-empty"]["R1"] != 1 {
		t.Errorf("expected R1=1 for empty items, got %d", counts["prd005-empty"]["R1"])
	}
}

func TestLoadPRDSubItemCounts_ShortPrefixNotDuplicated(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Two PRDs with the same prefix (prd001) — short entry maps to the first.
	prd1 := `id: prd001-alpha
title: Alpha
problem: test
goals: []
requirements:
  R1:
    title: Group
    items:
      - R1.1: x
non_goals: []
acceptance_criteria: []
`
	prd2 := `id: prd001-beta
title: Beta
problem: test
goals: []
requirements:
  R1:
    title: Group
    items:
      - R1.1: x
      - R1.2: y
non_goals: []
acceptance_criteria: []
`
	os.WriteFile("docs/specs/product-requirements/prd001-alpha.yaml", []byte(prd1), 0o644)
	os.WriteFile("docs/specs/product-requirements/prd001-beta.yaml", []byte(prd2), 0o644)

	counts := loadPRDSubItemCounts()

	// Both full stems must be present.
	if _, ok := counts["prd001-alpha"]; !ok {
		t.Error("expected prd001-alpha in counts")
	}
	if _, ok := counts["prd001-beta"]; !ok {
		t.Error("expected prd001-beta in counts")
	}
	// Short prefix must exist (set to whichever was processed first; not duplicated).
	if _, ok := counts["prd001"]; !ok {
		t.Error("expected prd001 short prefix in counts")
	}
}

// ---------------------------------------------------------------------------
// buildMeasurePrompt + MeasureRoadmapSource tests (GH-534, GH-508 audit)
// ---------------------------------------------------------------------------

// setupMeasureRoadmapDir creates a temp dir with the minimal structure needed
// to exercise the MeasureRoadmapSource code path: road-map.yaml, a use case
// file with touchpoint paths, and an optional cobbler dir for phase context.
func setupMeasureRoadmapDir(t *testing.T, roadmapYAML, ucID, ucYAML string) (cobblerDir string, cleanup func()) {
	t.Helper()
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{
		"docs/specs/use-cases",
		"docs/specs/product-requirements",
		".cobbler",
	} {
		os.MkdirAll(d, 0o755)
	}

	os.WriteFile("docs/road-map.yaml", []byte(roadmapYAML), 0o644)
	if ucID != "" && ucYAML != "" {
		os.WriteFile("docs/specs/use-cases/"+ucID+".yaml", []byte(ucYAML), 0o644)
	}

	return filepath.Join(tmp, ".cobbler"), func() {
		os.Chdir(orig)
	}
}

func TestBuildMeasurePrompt_MeasureRoadmapSource_AllDone(t *testing.T) {
	roadmap := `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    name: R1
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        status: done
`
	cobblerDir, cleanup := setupMeasureRoadmapDir(t, roadmap, "", "")
	defer cleanup()

	cfg := Config{}
	cfg.Cobbler.Dir = cobblerDir + "/"
	cfg.Cobbler.MeasureRoadmapSource = true
	o := New(cfg)

	// All done → no SourcePatterns set, build should succeed normally.
	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "role:") {
		t.Error("prompt missing role field")
	}
}

func TestBuildMeasurePrompt_MeasureRoadmapSource_PendingUC(t *testing.T) {
	roadmap := `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    name: R1
    status: in_progress
    use_cases:
      - id: rel01.0-uc001-init
        status: done
      - id: rel01.0-uc002-wf
        status: in_progress
`
	ucYAML := `id: rel01.0-uc002-wf
title: Workflow
touchpoints:
  - T1: "pkg/workflow ` + "\u2014" + ` prd003-wf R1"
`
	cobblerDir, cleanup := setupMeasureRoadmapDir(t, roadmap, "rel01.0-uc002-wf", ucYAML)
	defer cleanup()

	cfg := Config{}
	cfg.Cobbler.Dir = cobblerDir + "/"
	cfg.Cobbler.MeasureRoadmapSource = true
	o := New(cfg)

	// Pending UC with touchpoint "pkg/workflow" → SourcePatterns contains that path pattern.
	// We can verify the road-map path was selected by checking the prompt builds cleanly.
	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "role:") {
		t.Error("prompt missing role field")
	}
}

func TestBuildMeasurePrompt_MeasureRoadmapSource_ManualPatternsOverride(t *testing.T) {
	// When MeasureSourcePatterns is set manually, MeasureRoadmapSource must not
	// overwrite it (manual patterns have priority).
	roadmap := `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    name: R1
    status: in_progress
    use_cases:
      - id: rel01.0-uc001-init
        status: in_progress
`
	ucYAML := `id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: "pkg/init ` + "\u2014" + ` prd001-init R1"
`
	cobblerDir, cleanup := setupMeasureRoadmapDir(t, roadmap, "rel01.0-uc001-init", ucYAML)
	defer cleanup()

	cfg := Config{}
	cfg.Cobbler.Dir = cobblerDir + "/"
	cfg.Cobbler.MeasureRoadmapSource = true
	cfg.Cobbler.MeasureSourcePatterns = "cmd/tool/**/*.go"
	o := New(cfg)

	// Manual patterns set → road-map source ignored, build must succeed.
	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "role:") {
		t.Error("prompt missing role field")
	}
}

func TestBuildMeasurePrompt_MeasureRoadmapSource_UCNoTouchpointPaths(t *testing.T) {
	// UC found but touchpoints use colon-style (no em-dash) → no SourcePatterns set.
	roadmap := `id: rm1
title: Roadmap
releases:
  - version: "01.0"
    name: R1
    status: in_progress
    use_cases:
      - id: rel01.0-uc001-init
        status: in_progress
`
	ucYAML := `id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: "Config fields: prd001-init R1, R2"
`
	cobblerDir, cleanup := setupMeasureRoadmapDir(t, roadmap, "rel01.0-uc001-init", ucYAML)
	defer cleanup()

	cfg := Config{}
	cfg.Cobbler.Dir = cobblerDir + "/"
	cfg.Cobbler.MeasureRoadmapSource = true
	o := New(cfg)

	// No em-dash in touchpoints → no pkg paths → no SourcePatterns filter; build must succeed.
	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "role:") {
		t.Error("prompt missing role field")
	}
}

// --- test file exclusion wiring (GH-616) ---

// TestBuildMeasurePrompt_ExcludeTests_DefaultTrue verifies that _test.go files
// are excluded from the prompt by default (nil MeasureExcludeTests → true) (GH-616).
func TestBuildMeasurePrompt_ExcludeTests_DefaultTrue(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	os.WriteFile("pkg/app/app_test.go", []byte("package app\n"), 0o644)

	cfg := Config{}
	cfg.Project.GoSourceDirs = []string{"pkg/"}
	// MeasureExcludeTests is nil → effectiveMeasureExcludeTests() returns true.
	o := New(cfg)

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "app_test.go") {
		t.Error("_test.go should not appear in prompt when MeasureExcludeTests is unset (default true)")
	}
}

// TestBuildMeasurePrompt_ExcludeTests_DisabledFalse verifies that _test.go files
// appear in the prompt when MeasureExcludeTests is explicitly false (GH-616).
func TestBuildMeasurePrompt_ExcludeTests_DisabledFalse(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	os.WriteFile("pkg/app/app_test.go", []byte("package app\n"), 0o644)

	f := false
	cfg := Config{}
	cfg.Project.GoSourceDirs = []string{"pkg/"}
	cfg.Cobbler.MeasureExcludeTests = &f
	o := New(cfg)

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "app_test.go") {
		t.Error("_test.go should appear in prompt when MeasureExcludeTests=false")
	}
}

// --- source summarization wiring (GH-617) ---

// TestBuildMeasurePrompt_SourceMode_HeadersWired verifies that
// MeasureSourceMode="headers" strips function bodies from source files
// included in the measure prompt (prd003 R12).
func TestBuildMeasurePrompt_SourceMode_HeadersWired(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	src := "package app\n\n// Exported returns a value.\nfunc Exported() string {\n\treturn \"body-unique-marker\"\n}\n"
	os.WriteFile("pkg/app/main.go", []byte(src), 0o644)

	cfg := Config{}
	cfg.Project.GoSourceDirs = []string{"pkg/"}
	cfg.Cobbler.MeasureSourceMode = "headers"
	o := New(cfg)

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "body-unique-marker") {
		t.Error("headers mode should strip function bodies from measure prompt")
	}
	// Exported func name must still appear in the prompt.
	if !strings.Contains(prompt, "Exported") {
		t.Error("headers mode should keep exported func name in measure prompt")
	}
}

// TestBuildMeasurePrompt_SourceMode_PhaseCtxWins verifies that a SourceMode
// set in the phase context file takes precedence over the config value
// (prd003 R12.6).
func TestBuildMeasurePrompt_SourceMode_PhaseCtxWins(t *testing.T) {
	_, cleanup := setupContextTestDir(t)
	defer cleanup()

	// Write a measure_context.yaml with source_mode: full so the config
	// value of "headers" is overridden.
	os.MkdirAll(".cobbler", 0o755)
	os.WriteFile(".cobbler/measure_context.yaml", []byte("source_mode: full\n"), 0o644)
	defer os.Remove(".cobbler/measure_context.yaml")

	src := "package app\n\n// Exported returns a value.\nfunc Exported() string {\n\treturn \"body-present-marker\"\n}\n"
	os.WriteFile("pkg/app/main.go", []byte(src), 0o644)

	cfg := Config{}
	cfg.Project.GoSourceDirs = []string{"pkg/"}
	cfg.Cobbler.MeasureSourceMode = "headers"
	o := New(cfg)

	prompt, err := o.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt() error = %v", err)
	}
	// Phase context set source_mode: full → body should be present.
	if !strings.Contains(prompt, "body-present-marker") {
		t.Error("phase context source_mode=full should override config headers mode; body should be in prompt")
	}
}
