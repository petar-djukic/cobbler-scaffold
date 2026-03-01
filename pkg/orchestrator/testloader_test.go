// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import "testing"

// --- extractCompareCase ---

func TestExtractCompareCase_NilInputs(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{Name: "test", Inputs: nil, Expected: map[string]any{"stdout": "x"}}
	_, ok := extractCompareCase(raw)
	if ok {
		t.Error("expected false when Inputs is nil")
	}
}

func TestExtractCompareCase_NilExpected(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{Name: "test", Inputs: map[string]any{"utility": "cat"}, Expected: nil}
	_, ok := extractCompareCase(raw)
	if ok {
		t.Error("expected false when Expected is nil")
	}
}

func TestExtractCompareCase_MissingUtility(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		Name:     "no-utility",
		Inputs:   map[string]any{"stdin": "hello"},
		Expected: map[string]any{"stdout": "hello"},
	}
	_, ok := extractCompareCase(raw)
	if ok {
		t.Error("expected false when utility is missing")
	}
}

func TestExtractCompareCase_MissingStdinAndArgs(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		Name:     "no-input",
		Inputs:   map[string]any{"utility": "cat"},
		Expected: map[string]any{"stdout": "hello"},
	}
	_, ok := extractCompareCase(raw)
	if ok {
		t.Error("expected false when both stdin and args are missing")
	}
}

func TestExtractCompareCase_WithStdin(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		UseCase:  "uc001",
		Name:     "cat-stdin",
		Inputs:   map[string]any{"utility": "cat", "stdin": "hello world"},
		Expected: map[string]any{"stdout": "hello world"},
	}
	tc, ok := extractCompareCase(raw)
	if !ok {
		t.Fatal("expected true for valid stdin case")
	}
	if tc.UseCase != "uc001" {
		t.Errorf("UseCase = %q, want uc001", tc.UseCase)
	}
	if tc.Utility != "cat" {
		t.Errorf("Utility = %q, want cat", tc.Utility)
	}
	if tc.Stdin != "hello world" {
		t.Errorf("Stdin = %q, want 'hello world'", tc.Stdin)
	}
	if tc.Expected.Stdout != "hello world" {
		t.Errorf("Expected.Stdout = %q, want 'hello world'", tc.Expected.Stdout)
	}
}

func TestExtractCompareCase_WithArgs(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		Name:     "echo-args",
		Inputs:   map[string]any{"utility": "echo", "args": []any{"hello", "world"}},
		Expected: map[string]any{"stdout": "hello world\n"},
	}
	tc, ok := extractCompareCase(raw)
	if !ok {
		t.Fatal("expected true for valid args case")
	}
	if len(tc.Args) != 2 || tc.Args[0] != "hello" || tc.Args[1] != "world" {
		t.Errorf("Args = %v, want [hello world]", tc.Args)
	}
}

func TestExtractCompareCase_ArgsAsString(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		Name:     "echo-string-args",
		Inputs:   map[string]any{"utility": "echo", "args": "-n hello"},
		Expected: map[string]any{"stdout": "hello"},
	}
	tc, ok := extractCompareCase(raw)
	if !ok {
		t.Fatal("expected true for string args case")
	}
	if len(tc.Args) != 2 || tc.Args[0] != "-n" || tc.Args[1] != "hello" {
		t.Errorf("Args = %v, want [-n hello]", tc.Args)
	}
}

func TestExtractCompareCase_WithStderrAndExitCode(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		Name:     "fail-case",
		Inputs:   map[string]any{"utility": "false", "args": []any{"x"}},
		Expected: map[string]any{"stderr": "error msg", "exit_code": 1},
	}
	tc, ok := extractCompareCase(raw)
	if !ok {
		t.Fatal("expected true")
	}
	if tc.Expected.Stderr != "error msg" {
		t.Errorf("Stderr = %q, want 'error msg'", tc.Expected.Stderr)
	}
	if tc.Expected.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", tc.Expected.ExitCode)
	}
}

func TestExtractCompareCase_ExitCodeAsFloat(t *testing.T) {
	t.Parallel()
	raw := compareTestCaseRaw{
		Name:     "float-exit",
		Inputs:   map[string]any{"utility": "cmd", "stdin": "x"},
		Expected: map[string]any{"exit_code": float64(2)},
	}
	tc, ok := extractCompareCase(raw)
	if !ok {
		t.Fatal("expected true")
	}
	if tc.Expected.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", tc.Expected.ExitCode)
	}
}

// --- FilterByUtility ---

func TestFilterByUtility_Empty(t *testing.T) {
	t.Parallel()
	result := FilterByUtility(nil, "cat")
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestFilterByUtility_Matches(t *testing.T) {
	t.Parallel()
	cases := []CompareTestCase{
		{Utility: "cat", Name: "a"},
		{Utility: "echo", Name: "b"},
		{Utility: "cat", Name: "c"},
		{Utility: "wc", Name: "d"},
	}
	result := FilterByUtility(cases, "cat")
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].Name != "a" || result[1].Name != "c" {
		t.Errorf("got names %q and %q, want a and c", result[0].Name, result[1].Name)
	}
}

func TestFilterByUtility_NoMatch(t *testing.T) {
	t.Parallel()
	cases := []CompareTestCase{
		{Utility: "cat", Name: "a"},
		{Utility: "echo", Name: "b"},
	}
	result := FilterByUtility(cases, "wc")
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}
