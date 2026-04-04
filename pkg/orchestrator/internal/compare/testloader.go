// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// testloader.go loads test cases from YAML spec test suites and runs
// differential comparisons between two binaries.
// srd: srd004-differential-comparison R4, R5, R6
package compare

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CompareTestCase holds a single test case for differential comparison.
// Test cases with concrete stdin/args and expected stdout are loaded from
// YAML test suite specs.
type CompareTestCase struct {
	UseCase  string         `yaml:"use_case"`
	Name     string         `yaml:"name"`
	Utility  string         `yaml:"utility"`
	Stdin    string         `yaml:"stdin"`
	Args     []string       `yaml:"args"`
	Expected CompareExpected `yaml:"expected"`
}

// CompareExpected holds expected outputs for a comparison test case.
type CompareExpected struct {
	Stdout   string `yaml:"stdout"`
	Stderr   string `yaml:"stderr"`
	ExitCode int    `yaml:"exit_code"`
}

// testSuiteFile is the top-level structure of a test suite YAML file.
// We parse only what we need for comparison.
type testSuiteFile struct {
	TestCases []compareTestCaseRaw `yaml:"test_cases"`
}

// compareTestCaseRaw matches the YAML structure in test suite files.
// Fields are extracted into CompareTestCase when they contain enough
// data for differential comparison.
type compareTestCaseRaw struct {
	UseCase  string         `yaml:"use_case"`
	Name     string         `yaml:"name"`
	GoTest   string         `yaml:"go_test"`
	Inputs   map[string]any `yaml:"inputs"`
	Expected map[string]any `yaml:"expected"`
}

// LoadCompareTestCases reads YAML test suite files from specsDir and
// extracts test cases that have concrete comparison data (stdin/args
// and expected stdout). Go-test-only entries are skipped.
func LoadCompareTestCases(specsDir string) ([]CompareTestCase, error) {
	pattern := filepath.Join(specsDir, "test-*.yaml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing test suites: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no test suite files found matching %s", pattern)
	}

	var cases []CompareTestCase
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		var suite testSuiteFile
		if err := yaml.Unmarshal(data, &suite); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f, err)
		}
		for _, raw := range suite.TestCases {
			tc, ok := ExtractCompareCase(raw)
			if ok {
				cases = append(cases, tc)
			}
		}
	}
	return cases, nil
}

// ExtractCompareCase converts a raw test case into a CompareTestCase
// if it has enough data for comparison. Returns false if the test case
// is Go-test-only or lacks comparison fields.
func ExtractCompareCase(raw compareTestCaseRaw) (CompareTestCase, bool) {
	// Skip entries that only have a go_test reference and no comparison data.
	if raw.Inputs == nil || raw.Expected == nil {
		return CompareTestCase{}, false
	}

	tc := CompareTestCase{
		UseCase: raw.UseCase,
		Name:    raw.Name,
	}

	// Extract utility from inputs.
	if v, ok := raw.Inputs["utility"]; ok {
		tc.Utility = fmt.Sprint(v)
	}

	// Extract stdin.
	if v, ok := raw.Inputs["stdin"]; ok {
		tc.Stdin = fmt.Sprint(v)
	}

	// Extract args.
	if v, ok := raw.Inputs["args"]; ok {
		switch args := v.(type) {
		case []any:
			for _, a := range args {
				tc.Args = append(tc.Args, fmt.Sprint(a))
			}
		case string:
			tc.Args = strings.Fields(args)
		}
	}

	// Extract expected stdout.
	if v, ok := raw.Expected["stdout"]; ok {
		tc.Expected.Stdout = fmt.Sprint(v)
	}

	// Extract expected stderr.
	if v, ok := raw.Expected["stderr"]; ok {
		tc.Expected.Stderr = fmt.Sprint(v)
	}

	// Extract expected exit code.
	if v, ok := raw.Expected["exit_code"]; ok {
		switch code := v.(type) {
		case int:
			tc.Expected.ExitCode = code
		case float64:
			tc.Expected.ExitCode = int(code)
		}
	}

	// A test case is usable for comparison if it has a utility and
	// at least stdin or args (something to feed to the binary).
	if tc.Utility == "" || (tc.Stdin == "" && len(tc.Args) == 0) {
		return CompareTestCase{}, false
	}

	return tc, true
}

// FilterByUtility returns test cases matching the specified utility.
func FilterByUtility(cases []CompareTestCase, utility string) []CompareTestCase {
	var filtered []CompareTestCase
	for _, tc := range cases {
		if tc.Utility == utility {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

// TestResult records the outcome of a single comparison test case.
type TestResult struct {
	Utility    string
	Name       string
	Passed     bool
	StdoutDiff string
	StderrDiff string
	ExitCodeA  int
	ExitCodeB  int
}

// defaultTimeout is the per-test-case execution timeout.
const defaultTimeout = 10 * time.Second

// CompareUtility runs both binaries with identical inputs for each test
// case and compares their outputs byte-for-byte.
func CompareUtility(binA, binB string, cases []CompareTestCase) []TestResult {
	var results []TestResult
	for _, tc := range cases {
		outA, errA, codeA := runBinary(binA, tc)
		outB, errB, codeB := runBinary(binB, tc)

		r := TestResult{
			Utility:   tc.Utility,
			Name:      tc.Name,
			ExitCodeA: codeA,
			ExitCodeB: codeB,
		}

		stdoutMatch := outA == outB
		stderrMatch := errA == errB
		codeMatch := codeA == codeB

		if stdoutMatch && stderrMatch && codeMatch {
			r.Passed = true
		} else {
			if !stdoutMatch {
				r.StdoutDiff = fmt.Sprintf("A: %q\nB: %q", Truncate(outA, 200), Truncate(outB, 200))
			}
			if !stderrMatch {
				r.StderrDiff = fmt.Sprintf("A: %q\nB: %q", Truncate(errA, 200), Truncate(errB, 200))
			}
		}

		results = append(results, r)
	}
	return results
}

// runBinary executes a binary with the test case inputs and returns
// stdout, stderr, and exit code.
func runBinary(binPath string, tc CompareTestCase) (stdout, stderr string, exitCode int) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, tc.Args...)
	if tc.Stdin != "" {
		cmd.Stdin = strings.NewReader(tc.Stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return
}

// FormatResults produces structured text output showing pass/fail per
// utility per test case with summary counts.
func FormatResults(results []TestResult) string {
	if len(results) == 0 {
		return "No test results.\n"
	}

	var b strings.Builder
	passed, failed := 0, 0
	currentUtility := ""

	for _, r := range results {
		if r.Utility != currentUtility {
			if currentUtility != "" {
				fmt.Fprintln(&b)
			}
			currentUtility = r.Utility
			fmt.Fprintf(&b, "=== %s ===\n", currentUtility)
		}

		if r.Passed {
			passed++
			fmt.Fprintf(&b, "  PASS  %s\n", r.Name)
		} else {
			failed++
			fmt.Fprintf(&b, "  FAIL  %s\n", r.Name)
			if r.StdoutDiff != "" {
				fmt.Fprintf(&b, "        stdout: %s\n", r.StdoutDiff)
			}
			if r.StderrDiff != "" {
				fmt.Fprintf(&b, "        stderr: %s\n", r.StderrDiff)
			}
			if r.ExitCodeA != r.ExitCodeB {
				fmt.Fprintf(&b, "        exit: A=%d B=%d\n", r.ExitCodeA, r.ExitCodeB)
			}
		}
	}

	fmt.Fprintf(&b, "\n--- Summary: %d passed, %d failed, %d total ---\n", passed, failed, passed+failed)
	return b.String()
}

// Truncate shortens s to maxLen characters, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
