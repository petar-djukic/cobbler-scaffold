// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package compare

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
	if got := r.BinaryName("cat"); got != "gcat" {
		t.Errorf("BinaryName(cat) = %q, want gcat", got)
	}
	if got := r.BinaryName("ls"); got != "gls" {
		t.Errorf("BinaryName(ls) = %q, want gls", got)
	}
}

func TestGNUResolver_MoreutilsNoPrefix(t *testing.T) {
	r := GNUResolver{}
	for _, name := range []string{"ts", "sponge", "chronic", "vidir"} {
		if got := r.BinaryName(name); got != name {
			t.Errorf("BinaryName(%s) = %q, want %q", name, got, name)
		}
	}
}

// --- ResolverFromArg tests ---

func TestResolverFromArg_GNU(t *testing.T) {
	deps := Deps{Log: func(string, ...any) {}, GitBin: "git", GoBin: "go"}
	r := ResolverFromArg("gnu", deps)
	if _, ok := r.(GNUResolver); !ok {
		t.Errorf("ResolverFromArg(gnu) returned %T, want GNUResolver", r)
	}

	r = ResolverFromArg("GNU", deps)
	if _, ok := r.(GNUResolver); !ok {
		t.Errorf("ResolverFromArg(GNU) returned %T, want GNUResolver", r)
	}
}

func TestResolverFromArg_Directory(t *testing.T) {
	deps := Deps{Log: func(string, ...any) {}, GitBin: "git", GoBin: "go"}
	dir := t.TempDir()
	r := ResolverFromArg(dir, deps)
	pr, ok := r.(PathResolver)
	if !ok {
		t.Fatalf("ResolverFromArg(%s) returned %T, want PathResolver", dir, r)
	}
	if pr.Dir != dir {
		t.Errorf("PathResolver.Dir = %s, want %s", pr.Dir, dir)
	}
}

func TestResolverFromArg_GitTag(t *testing.T) {
	deps := Deps{Log: func(string, ...any) {}, GitBin: "git", GoBin: "go"}
	r := ResolverFromArg("generation-2026-02-25-merged", deps)
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

// --- CommonUtilities tests ---

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
	common, err := CommonUtilities(rA, rB, "")
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

	common, err := CommonUtilities(rA, rB, "cat")
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

// --- CountFailed tests ---

func TestCountFailed(t *testing.T) {
	results := []TestResult{
		{Passed: true},
		{Passed: false},
		{Passed: false},
		{Passed: true},
	}
	if got := CountFailed(results); got != 2 {
		t.Errorf("CountFailed = %d, want 2", got)
	}
}

// --- Truncate tests ---

func TestTruncate_Short(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("Truncate(hello, 10) = %q, want hello", got)
	}
}

func TestTruncate_Long(t *testing.T) {
	got := Truncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("Truncate(hello world, 5) = %q, want hello...", got)
	}
}

// --- noop ---

func TestNoop(t *testing.T) {
	t.Parallel()
	// noop should not panic.
	noop()
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

// --- GitTagResolver.cleanup ---

func TestGitTagResolver_Cleanup_NoPaths(t *testing.T) {
	t.Parallel()
	r := &GitTagResolver{deps: Deps{
		Log:            func(string, ...any) {},
		RemoveWorktree: func(string, string) error { return nil },
	}}
	r.cleanup() // should be a no-op, no panic
	if r.wtDir != "" || r.buildDir != "" {
		t.Error("expected empty paths after cleanup")
	}
}

func TestGitTagResolver_Cleanup_BuildDirOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	buildDir := filepath.Join(dir, "build")
	os.MkdirAll(buildDir, 0o755)
	os.WriteFile(filepath.Join(buildDir, "bin"), []byte("x"), 0o644)

	r := &GitTagResolver{
		buildDir: buildDir,
		deps: Deps{
			Log:            func(string, ...any) {},
			RemoveWorktree: func(string, string) error { return nil },
		},
	}
	r.cleanup()
	if r.buildDir != "" {
		t.Error("buildDir should be cleared after cleanup")
	}
	if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
		t.Error("buildDir should be removed")
	}
}

// --- CompareUtility edge cases ---

func TestCompareUtility_StderrDiff(t *testing.T) {
	t.Parallel()
	binA := createTestBinary(t, "#!/bin/sh\necho hello; echo errA >&2")
	binB := createTestBinary(t, "#!/bin/sh\necho hello; echo errB >&2")
	cases := []CompareTestCase{
		{Utility: "test", Name: "stderr diff", Args: []string{}},
	}
	results := CompareUtility(binA, binB, cases)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Passed {
		t.Error("different stderr should fail")
	}
	if results[0].StderrDiff == "" {
		t.Error("should have stderr diff")
	}
}

func TestCompareUtility_ExitCodeDiff(t *testing.T) {
	t.Parallel()
	binA := createTestBinary(t, "#!/bin/sh\nexit 0")
	binB := createTestBinary(t, "#!/bin/sh\nexit 1")
	cases := []CompareTestCase{
		{Utility: "test", Name: "exit diff", Args: []string{}},
	}
	results := CompareUtility(binA, binB, cases)
	if results[0].Passed {
		t.Error("different exit codes should fail")
	}
	if results[0].ExitCodeA != 0 {
		t.Errorf("ExitCodeA = %d, want 0", results[0].ExitCodeA)
	}
	if results[0].ExitCodeB != 1 {
		t.Errorf("ExitCodeB = %d, want 1", results[0].ExitCodeB)
	}
}

func TestCompareUtility_WithStdin(t *testing.T) {
	t.Parallel()
	bin := createTestBinary(t, "#!/bin/sh\ncat")
	cases := []CompareTestCase{
		{Utility: "cat", Name: "stdin passthrough", Stdin: "hello world", Args: []string{}},
	}
	results := CompareUtility(bin, bin, cases)
	if !results[0].Passed {
		t.Errorf("identical cat with stdin should pass: %+v", results[0])
	}
}

func TestCompareUtility_MultipleTests(t *testing.T) {
	t.Parallel()
	bin := createTestBinary(t, "#!/bin/sh\necho ok")
	cases := []CompareTestCase{
		{Utility: "test", Name: "case1", Args: []string{}},
		{Utility: "test", Name: "case2", Args: []string{}},
		{Utility: "test", Name: "case3", Args: []string{}},
	}
	results := CompareUtility(bin, bin, cases)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for i, r := range results {
		if !r.Passed {
			t.Errorf("result[%d] should pass", i)
		}
	}
}

// --- CountFailed edge cases ---

func TestCountFailed_Empty(t *testing.T) {
	t.Parallel()
	if got := CountFailed(nil); got != 0 {
		t.Errorf("CountFailed(nil) = %d, want 0", got)
	}
}

func TestCountFailed_AllPass(t *testing.T) {
	t.Parallel()
	results := []TestResult{{Passed: true}, {Passed: true}}
	if got := CountFailed(results); got != 0 {
		t.Errorf("CountFailed(all pass) = %d, want 0", got)
	}
}

func TestCountFailed_AllFail(t *testing.T) {
	t.Parallel()
	results := []TestResult{{Passed: false}, {Passed: false}}
	if got := CountFailed(results); got != 2 {
		t.Errorf("CountFailed(all fail) = %d, want 2", got)
	}
}

// --- FormatResults edge cases ---

func TestFormatResults_SingleUtility(t *testing.T) {
	t.Parallel()
	results := []TestResult{
		{Utility: "echo", Name: "basic", Passed: true},
	}
	out := FormatResults(results)
	if !strings.Contains(out, "=== echo ===") {
		t.Error("output should contain utility header")
	}
	if !strings.Contains(out, "1 passed, 0 failed, 1 total") {
		t.Error("output should contain correct summary")
	}
}

func TestFormatResults_MultipleUtilities(t *testing.T) {
	t.Parallel()
	results := []TestResult{
		{Utility: "cat", Name: "a", Passed: true},
		{Utility: "echo", Name: "b", Passed: false, StdoutDiff: "diff"},
	}
	out := FormatResults(results)
	if !strings.Contains(out, "=== cat ===") {
		t.Error("output should contain cat header")
	}
	if !strings.Contains(out, "=== echo ===") {
		t.Error("output should contain echo header")
	}
}

func TestFormatResults_StderrOnly(t *testing.T) {
	t.Parallel()
	results := []TestResult{
		{Utility: "cmd", Name: "stderr-only", Passed: false, StderrDiff: "A: x\nB: y"},
	}
	out := FormatResults(results)
	if !strings.Contains(out, "stderr:") {
		t.Error("output should contain stderr diff")
	}
}

// --- Truncate edge cases ---

func TestTruncate_Empty(t *testing.T) {
	t.Parallel()
	if got := Truncate("", 10); got != "" {
		t.Errorf("Truncate empty = %q, want empty", got)
	}
}

func TestTruncate_ExactLen(t *testing.T) {
	t.Parallel()
	if got := Truncate("12345", 5); got != "12345" {
		t.Errorf("Truncate exact = %q, want 12345", got)
	}
}

func TestTruncate_ZeroMaxLen(t *testing.T) {
	t.Parallel()
	got := Truncate("hello", 0)
	if got != "..." {
		t.Errorf("Truncate(0) = %q, want ...", got)
	}
}

// --- PathResolver edge cases ---

func TestPathResolver_ListUtilities_EmptyDir(t *testing.T) {
	t.Parallel()
	r := PathResolver{Dir: t.TempDir()}
	utils, err := r.ListUtilities()
	if err != nil {
		t.Fatalf("ListUtilities: %v", err)
	}
	if len(utils) != 0 {
		t.Errorf("got %d utils, want 0", len(utils))
	}
}

func TestPathResolver_ListUtilities_SkipsDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "cat"), []byte{}, 0o755)

	r := PathResolver{Dir: dir}
	utils, err := r.ListUtilities()
	if err != nil {
		t.Fatalf("ListUtilities: %v", err)
	}
	if len(utils) != 1 || utils[0] != "cat" {
		t.Errorf("got %v, want [cat]", utils)
	}
}

func TestPathResolver_ListUtilities_NonExistentDir(t *testing.T) {
	t.Parallel()
	r := PathResolver{Dir: "/nonexistent/dir/path"}
	_, err := r.ListUtilities()
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

// --- CommonUtilities edge cases ---

func TestCommonUtilities_NoOverlap(t *testing.T) {
	t.Parallel()
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "cat"), []byte{}, 0o755)
	os.WriteFile(filepath.Join(dirB, "dog"), []byte{}, 0o755)

	common, err := CommonUtilities(PathResolver{Dir: dirA}, PathResolver{Dir: dirB}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(common) != 0 {
		t.Errorf("got %v, want empty for no overlap", common)
	}
}

func TestCommonUtilities_BothEmpty(t *testing.T) {
	t.Parallel()
	dirA := t.TempDir()
	dirB := t.TempDir()
	common, err := CommonUtilities(PathResolver{Dir: dirA}, PathResolver{Dir: dirB}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(common) != 0 {
		t.Errorf("got %v, want empty for both empty", common)
	}
}

// --- LoadCompareTestCases ---

func TestLoadCompareTestCases_ValidDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := `test_cases:
  - use_case: "uc001"
    name: "cat-basic"
    inputs:
      utility: "cat"
      stdin: "hello"
    expected:
      stdout: "hello"
  - use_case: "uc002"
    name: "go-test-only"
    go_test: "TestSomething"
`
	if err := os.WriteFile(filepath.Join(dir, "test-rel01.0.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cases, err := LoadCompareTestCases(dir)
	if err != nil {
		t.Fatalf("LoadCompareTestCases: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("got %d cases, want 1 (go_test-only should be skipped)", len(cases))
	}
	if cases[0].Utility != "cat" {
		t.Errorf("Utility = %q, want cat", cases[0].Utility)
	}
}

func TestLoadCompareTestCases_NoFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadCompareTestCases(dir)
	if err == nil {
		t.Error("expected error when no test suite files found")
	}
}

func TestLoadCompareTestCases_MalformedYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test-bad.yaml"), []byte("not: [valid: yaml"), 0o644)
	_, err := LoadCompareTestCases(dir)
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}
