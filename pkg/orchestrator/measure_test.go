// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"testing"
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

	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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
	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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

	vr := validateMeasureOutput(issues)
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

// contains checks if substr is in s. Avoids importing strings in test.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
