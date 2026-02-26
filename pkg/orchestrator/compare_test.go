// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- PathResolver tests ---

func TestPathResolver_ResolvesExistingBinary(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "cat")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := PathResolver{Dir: dir}
	path, cleanup, err := r.Resolve("cat")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	defer cleanup()
	if path != binPath {
		t.Errorf("got %s, want %s", path, binPath)
	}
}

func TestPathResolver_ErrorOnMissing(t *testing.T) {
	dir := t.TempDir()
	r := PathResolver{Dir: dir}
	_, _, err := r.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestPathResolver_ListUtilities(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cat", "echo", "ls"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	r := PathResolver{Dir: dir}
	utils, err := r.ListUtilities()
	if err != nil {
		t.Fatalf("ListUtilities failed: %v", err)
	}
	if len(utils) != 3 {
		t.Errorf("got %d utilities, want 3", len(utils))
	}
}

// --- GNUResolver tests ---

func TestGNUResolver_CoreutilsPrefix(t *testing.T) {
	r := GNUResolver{}
	if got := r.binaryName("cat"); got != "gcat" {
		t.Errorf("binaryName(cat) = %q, want gcat", got)
	}
	if got := r.binaryName("ls"); got != "gls" {
		t.Errorf("binaryName(ls) = %q, want gls", got)
	}
}

func TestGNUResolver_MoreutilsNoPrefix(t *testing.T) {
	r := GNUResolver{}
	for _, name := range []string{"ts", "sponge", "chronic", "vidir"} {
		if got := r.binaryName(name); got != name {
			t.Errorf("binaryName(%s) = %q, want %q", name, got, name)
		}
	}
}

// --- ResolverFromArg tests ---

func TestResolverFromArg_GNU(t *testing.T) {
	r := ResolverFromArg("gnu")
	if _, ok := r.(GNUResolver); !ok {
		t.Errorf("ResolverFromArg(gnu) returned %T, want GNUResolver", r)
	}

	r = ResolverFromArg("GNU")
	if _, ok := r.(GNUResolver); !ok {
		t.Errorf("ResolverFromArg(GNU) returned %T, want GNUResolver", r)
	}
}

func TestResolverFromArg_Directory(t *testing.T) {
	dir := t.TempDir()
	r := ResolverFromArg(dir)
	pr, ok := r.(PathResolver)
	if !ok {
		t.Fatalf("ResolverFromArg(%s) returned %T, want PathResolver", dir, r)
	}
	if pr.Dir != dir {
		t.Errorf("PathResolver.Dir = %s, want %s", pr.Dir, dir)
	}
}

func TestResolverFromArg_GitTag(t *testing.T) {
	r := ResolverFromArg("generation-2026-02-25-merged")
	gtr, ok := r.(*GitTagResolver)
	if !ok {
		t.Fatalf("ResolverFromArg returned %T, want *GitTagResolver", r)
	}
	if gtr.Tag != "generation-2026-02-25-merged" {
		t.Errorf("GitTagResolver.Tag = %s, want generation-2026-02-25-merged", gtr.Tag)
	}
}

// --- FormatResults tests ---

func TestFormatResults_Empty(t *testing.T) {
	out := FormatResults(nil)
	if out != "No test results.\n" {
		t.Errorf("FormatResults(nil) = %q, want %q", out, "No test results.\n")
	}
}

func TestFormatResults_PassAndFail(t *testing.T) {
	results := []TestResult{
		{Utility: "cat", Name: "basic stdin", Passed: true},
		{Utility: "cat", Name: "empty input", Passed: false, StdoutDiff: "A: \"hello\"\nB: \"world\""},
	}
	out := FormatResults(results)

	if !strings.Contains(out, "=== cat ===") {
		t.Error("output should contain utility header")
	}
	if !strings.Contains(out, "PASS  basic stdin") {
		t.Error("output should contain PASS line")
	}
	if !strings.Contains(out, "FAIL  empty input") {
		t.Error("output should contain FAIL line")
	}
	if !strings.Contains(out, "1 passed, 1 failed, 2 total") {
		t.Error("output should contain summary counts")
	}
}

func TestFormatResults_ExitCodeDiff(t *testing.T) {
	results := []TestResult{
		{Utility: "cat", Name: "exit diff", Passed: false, ExitCodeA: 0, ExitCodeB: 1},
	}
	out := FormatResults(results)
	if !strings.Contains(out, "exit: A=0 B=1") {
		t.Error("output should show exit code diff")
	}
}

// --- commonUtilities tests ---

func TestCommonUtilities_Intersection(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	for _, name := range []string{"cat", "echo", "ls"} {
		os.WriteFile(filepath.Join(dirA, name), []byte{}, 0o755)
	}
	for _, name := range []string{"cat", "ls", "wc"} {
		os.WriteFile(filepath.Join(dirB, name), []byte{}, 0o755)
	}

	rA := PathResolver{Dir: dirA}
	rB := PathResolver{Dir: dirB}
	common, err := commonUtilities(rA, rB, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(common) != 2 {
		t.Fatalf("got %d common, want 2: %v", len(common), common)
	}
	if common[0] != "cat" || common[1] != "ls" {
		t.Errorf("got %v, want [cat ls]", common)
	}
}

func TestCommonUtilities_SingleUtility(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	rA := PathResolver{Dir: dirA}
	rB := PathResolver{Dir: dirB}

	common, err := commonUtilities(rA, rB, "cat")
	if err != nil {
		t.Fatal(err)
	}
	if len(common) != 1 || common[0] != "cat" {
		t.Errorf("got %v, want [cat]", common)
	}
}

// --- CompareUtility tests ---

func TestCompareUtility_IdenticalBinaries(t *testing.T) {
	bin := createTestBinary(t, "#!/bin/sh\necho hello")
	cases := []CompareTestCase{
		{Utility: "test", Name: "echo test", Args: []string{}},
	}
	results := CompareUtility(bin, bin, cases)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !results[0].Passed {
		t.Errorf("identical binaries should pass: stdout=%q, stderr=%q", results[0].StdoutDiff, results[0].StderrDiff)
	}
}

func TestCompareUtility_DifferentBinaries(t *testing.T) {
	binA := createTestBinary(t, "#!/bin/sh\necho hello")
	binB := createTestBinary(t, "#!/bin/sh\necho world")
	cases := []CompareTestCase{
		{Utility: "test", Name: "diff test", Args: []string{}},
	}
	results := CompareUtility(binA, binB, cases)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Passed {
		t.Error("different binaries should fail")
	}
	if results[0].StdoutDiff == "" {
		t.Error("should have stdout diff")
	}
}

// --- countFailed tests ---

func TestCountFailed(t *testing.T) {
	results := []TestResult{
		{Passed: true},
		{Passed: false},
		{Passed: false},
		{Passed: true},
	}
	if got := countFailed(results); got != 2 {
		t.Errorf("countFailed = %d, want 2", got)
	}
}

// --- truncate tests ---

func TestTruncate_Short(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate(hello, 10) = %q, want hello", got)
	}
}

func TestTruncate_Long(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("truncate(hello world, 5) = %q, want hello...", got)
	}
}

// createTestBinary writes a shell script to a temp file and returns its path.
func createTestBinary(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-bin")
	if err := os.WriteFile(path, []byte(script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
