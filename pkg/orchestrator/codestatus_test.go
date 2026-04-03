// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
)

// --- ucPrefixFromID ---

func TestUCPrefixFromID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"rel01.0-uc001-orchestrator-initialization", "rel01.0-uc001"},
		{"rel02.0-uc006-specification-browser", "rel02.0-uc006"},
		{"rel03.0-uc001-cross-generation-comparison", "rel03.0-uc001"},
		{"rel12.3-uc999-long-name", "rel12.3-uc999"},
		{"not-a-use-case", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := an.UCPrefixFromID(tc.input); got != tc.want {
			t.Errorf("an.UCPrefixFromID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- testDirForUC ---

func TestTestDirForUC(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"rel01.0-uc001-orchestrator-initialization", filepath.Join("tests", "rel01.0", "uc001")},
		{"rel02.0-uc006-specification-browser", filepath.Join("tests", "rel02.0", "uc006")},
		{"not-a-use-case", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := an.TestDirForUC(tc.input); got != tc.want {
			t.Errorf("an.TestDirForUC(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- countTestFiles ---

func TestCountTestFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "init_test.go"), []byte("package x"), 0o644)
	os.WriteFile(filepath.Join(dir, "bench_test.go"), []byte("package x"), 0o644)
	os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package x"), 0o644)

	if got := an.CountTestFiles(dir); got != 2 {
		t.Errorf("countTestFiles = %d, want 2", got)
	}
}

func TestCountTestFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	if got := an.CountTestFiles(dir); got != 0 {
		t.Errorf("countTestFiles = %d, want 0", got)
	}
}

func TestCountTestFiles_NoDir(t *testing.T) {
	if got := an.CountTestFiles("/nonexistent/path"); got != 0 {
		t.Errorf("countTestFiles = %d, want 0", got)
	}
}

// --- scanTestDirectories ---

func TestScanTestDirectories(t *testing.T) {
	root := t.TempDir()
	// Create tests/rel01.0/uc001/ with a test file.
	uc001 := filepath.Join(root, "rel01.0", "uc001")
	os.MkdirAll(uc001, 0o755)
	os.WriteFile(filepath.Join(uc001, "init_test.go"), []byte("package x"), 0o644)

	// Create tests/rel01.0/uc002/ with two test files.
	uc002 := filepath.Join(root, "rel01.0", "uc002")
	os.MkdirAll(uc002, 0o755)
	os.WriteFile(filepath.Join(uc002, "life_test.go"), []byte("package x"), 0o644)
	os.WriteFile(filepath.Join(uc002, "bench_test.go"), []byte("package x"), 0o644)

	// Create tests/rel02.0/uc001/ with no test files.
	uc201 := filepath.Join(root, "rel02.0", "uc001")
	os.MkdirAll(uc201, 0o755)
	os.WriteFile(filepath.Join(uc201, "helper.go"), []byte("package x"), 0o644)

	got := an.ScanTestDirectories(root)
	if got["rel01.0-uc001"] != 1 {
		t.Errorf("rel01.0-uc001: got %d, want 1", got["rel01.0-uc001"])
	}
	if got["rel01.0-uc002"] != 2 {
		t.Errorf("rel01.0-uc002: got %d, want 2", got["rel01.0-uc002"])
	}
	if got["rel02.0-uc001"] != 0 {
		t.Errorf("rel02.0-uc001: got %d, want 0 (no test files)", got["rel02.0-uc001"])
	}
}

func TestScanTestDirectories_Empty(t *testing.T) {
	root := t.TempDir()
	got := an.ScanTestDirectories(root)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestScanTestDirectories_NoDir(t *testing.T) {
	got := an.ScanTestDirectories("/nonexistent/tests")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestScanTestDirectories_SkipsNonRelDirs(t *testing.T) {
	root := t.TempDir()
	// Create an "internal" directory that should be skipped.
	internal := filepath.Join(root, "internal", "testutil")
	os.MkdirAll(internal, 0o755)
	os.WriteFile(filepath.Join(internal, "helper_test.go"), []byte("package x"), 0o644)

	got := an.ScanTestDirectories(root)
	if len(got) != 0 {
		t.Errorf("got %v, want empty (internal/ should be skipped)", got)
	}
}

// --- computeCodeStatus ---

func TestComputeCodeStatus_AllImplemented(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{{
			Version: "01.0",
			Name:    "Core",
			Status:  "done",
			UseCases: []RoadmapUseCase{
				{ID: "rel01.0-uc001-init", Status: "done"},
				{ID: "rel01.0-uc002-lifecycle", Status: "done"},
			},
		}},
	}
	scan := map[string]int{
		"rel01.0-uc001": 1,
		"rel01.0-uc002": 3,
	}
	report := computeCodeStatus(roadmap, scan)

	if len(report.Releases) != 1 {
		t.Fatalf("got %d releases, want 1", len(report.Releases))
	}
	if report.Releases[0].CodeReadiness != "all implemented" {
		t.Errorf("CodeReadiness: got %q, want %q", report.Releases[0].CodeReadiness, "all implemented")
	}
	if report.Releases[0].UseCases[0].CodeStatus != "implemented" {
		t.Errorf("UC[0] CodeStatus: got %q, want %q", report.Releases[0].UseCases[0].CodeStatus, "implemented")
	}
	if report.Releases[0].UseCases[0].TestFiles != 1 {
		t.Errorf("UC[0] TestFiles: got %d, want 1", report.Releases[0].UseCases[0].TestFiles)
	}
}

func TestComputeCodeStatus_Partial(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{{
			Version: "01.0",
			Name:    "Core",
			Status:  "done",
			UseCases: []RoadmapUseCase{
				{ID: "rel01.0-uc001-init", Status: "done"},
				{ID: "rel01.0-uc002-lifecycle", Status: "done"},
			},
		}},
	}
	scan := map[string]int{
		"rel01.0-uc001": 1,
		// uc002 missing from scan
	}
	report := computeCodeStatus(roadmap, scan)

	if report.Releases[0].CodeReadiness != "partial" {
		t.Errorf("CodeReadiness: got %q, want %q", report.Releases[0].CodeReadiness, "partial")
	}
	if report.Releases[0].UseCases[1].CodeStatus != "not started" {
		t.Errorf("UC[1] CodeStatus: got %q, want %q", report.Releases[0].UseCases[1].CodeStatus, "not started")
	}
}

func TestComputeCodeStatus_None(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{{
			Version: "02.0",
			Name:    "Extension",
			Status:  "done",
			UseCases: []RoadmapUseCase{
				{ID: "rel02.0-uc001-lifecycle", Status: "done"},
			},
		}},
	}
	scan := map[string]int{}
	report := computeCodeStatus(roadmap, scan)

	if report.Releases[0].CodeReadiness != "none" {
		t.Errorf("CodeReadiness: got %q, want %q", report.Releases[0].CodeReadiness, "none")
	}
}

func TestComputeCodeStatus_SkipsEmptyReleases(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{
			{Version: "01.0", Name: "Core", Status: "done", UseCases: []RoadmapUseCase{
				{ID: "rel01.0-uc001-init", Status: "done"},
			}},
			{Version: "99.0", Name: "Unscheduled", Status: "not started", UseCases: nil},
		},
	}
	scan := map[string]int{"rel01.0-uc001": 1}
	report := computeCodeStatus(roadmap, scan)

	if len(report.Releases) != 1 {
		t.Errorf("got %d releases, want 1 (empty release should be skipped)", len(report.Releases))
	}
}

func TestComputeCodeStatus_MultipleReleases(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{
			{Version: "01.0", Name: "Core", Status: "done", UseCases: []RoadmapUseCase{
				{ID: "rel01.0-uc001-init", Status: "done"},
			}},
			{Version: "02.0", Name: "Ext", Status: "done", UseCases: []RoadmapUseCase{
				{ID: "rel02.0-uc001-lifecycle", Status: "done"},
			}},
		},
	}
	scan := map[string]int{"rel01.0-uc001": 2}
	report := computeCodeStatus(roadmap, scan)

	if len(report.Releases) != 2 {
		t.Fatalf("got %d releases, want 2", len(report.Releases))
	}
	if report.Releases[0].CodeReadiness != "all implemented" {
		t.Errorf("rel01.0 CodeReadiness: got %q, want %q", report.Releases[0].CodeReadiness, "all implemented")
	}
	if report.Releases[1].CodeReadiness != "none" {
		t.Errorf("rel02.0 CodeReadiness: got %q, want %q", report.Releases[1].CodeReadiness, "none")
	}
}

// --- detectSpecCodeGaps ---

func TestDetectSpecCodeGaps_NoGaps(t *testing.T) {
	report := &CodeStatusReport{
		Releases: []ReleaseCodeStatus{{
			Version:       "01.0",
			SpecStatus:    "done",
			CodeReadiness: "all implemented",
			UseCases: []UCCodeStatus{
				{ID: "rel01.0-uc001-init", SpecStatus: "done", CodeStatus: "implemented"},
			},
		}},
	}
	gaps := an.DetectSpecCodeGaps(report)
	if len(gaps) != 0 {
		t.Errorf("got %v, want no gaps", gaps)
	}
}

func TestDetectSpecCodeGaps_ReleaseLevelGap(t *testing.T) {
	report := &CodeStatusReport{
		Releases: []ReleaseCodeStatus{{
			Version:       "01.0",
			SpecStatus:    "done",
			CodeReadiness: "partial",
			UseCases: []UCCodeStatus{
				{ID: "rel01.0-uc001-init", SpecStatus: "done", CodeStatus: "implemented"},
				{ID: "rel01.0-uc002-lifecycle", SpecStatus: "done", CodeStatus: "not started"},
			},
		}},
	}
	gaps := an.DetectSpecCodeGaps(report)
	if len(gaps) != 2 {
		t.Fatalf("got %d gaps, want 2", len(gaps))
	}
}

func TestDetectSpecCodeGaps_UCLevelGap(t *testing.T) {
	report := &CodeStatusReport{
		Releases: []ReleaseCodeStatus{{
			Version:       "01.0",
			SpecStatus:    "not started",
			CodeReadiness: "partial",
			UseCases: []UCCodeStatus{
				{ID: "rel01.0-uc001-init", SpecStatus: "done", CodeStatus: "not started"},
				{ID: "rel01.0-uc002-lifecycle", SpecStatus: "not started", CodeStatus: "not started"},
			},
		}},
	}
	gaps := an.DetectSpecCodeGaps(report)
	// Release spec is "not started" so no release-level gap. But UC001 has a gap.
	if len(gaps) != 1 {
		t.Fatalf("got %d gaps, want 1", len(gaps))
	}
}

func TestDetectSpecCodeGaps_SpecNotStarted_NoGap(t *testing.T) {
	report := &CodeStatusReport{
		Releases: []ReleaseCodeStatus{{
			Version:       "99.0",
			SpecStatus:    "not started",
			CodeReadiness: "none",
			UseCases: []UCCodeStatus{
				{ID: "rel99.0-uc001-future", SpecStatus: "not started", CodeStatus: "not started"},
			},
		}},
	}
	gaps := an.DetectSpecCodeGaps(report)
	if len(gaps) != 0 {
		t.Errorf("got %v, want no gaps", gaps)
	}
}

// --- statusIcon ---

func TestStatusIcon(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"done", "[ok]"},
		{"implemented", "[ok]"},
		{"all implemented", "[ok]"},
		{"partial", "[~~]"},
		{"not started", "[  ]"},
		{"none", "[  ]"},
		{"unknown", "[??]"},
	}
	for _, tc := range cases {
		if got := an.StatusIcon(tc.input); got != tc.want {
			t.Errorf("an.StatusIcon(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- printCodeStatusReport ---

func TestPrintCodeStatusReport_ContainsReleaseInfo(t *testing.T) {
	report := &CodeStatusReport{
		Releases: []ReleaseCodeStatus{{
			Version:       "01.0",
			Name:          "Core",
			SpecStatus:    "done",
			CodeReadiness: "all implemented",
			UseCases: []UCCodeStatus{
				{ID: "rel01.0-uc001-init", SpecStatus: "done", CodeStatus: "implemented", TestFiles: 2},
			},
		}},
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	an.PrintCodeStatusReport(report)
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	for _, want := range []string{"01.0", "Core", "done", "all implemented", "rel01.0-uc001-init", "No gaps"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestPrintCodeStatusReport_ShowsGaps(t *testing.T) {
	report := &CodeStatusReport{
		Releases: []ReleaseCodeStatus{{
			Version:       "01.0",
			Name:          "Core",
			SpecStatus:    "done",
			CodeReadiness: "none",
			UseCases: []UCCodeStatus{
				{ID: "rel01.0-uc001-init", SpecStatus: "done", CodeStatus: "not started"},
			},
		}},
		Gaps: []string{"release 01.0: spec status is \"done\" but code readiness is \"none\""},
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	an.PrintCodeStatusReport(report)
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "Gaps") {
		t.Errorf("output missing 'Gaps' section\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "01.0") {
		t.Errorf("output missing release version\nfull output:\n%s", out)
	}
}

// --- CodeStatus (integration) ---
// These tests use os.Chdir because CodeStatus reads docs/road-map.yaml and
// tests/ relative to the working directory.

const roadmapYAML = `id: test-roadmap
title: Test Roadmap
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        status: done
`

func TestCodeStatus_NoGaps(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Write docs/road-map.yaml.
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.yaml"), []byte(roadmapYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create matching test directory with a test file.
	testDir := filepath.Join(dir, "tests", "rel01.0", "uc001")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "init_test.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := New(Config{})
	if err := o.Analyzer.CodeStatus(); err != nil {
		t.Errorf("CodeStatus() returned error: %v", err)
	}
}

func TestCodeStatus_WithGap(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Write docs/road-map.yaml with status=done but no matching test files.
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.yaml"), []byte(roadmapYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	// Do not create tests/ directory — gaps should be detected.

	o := New(Config{})
	err = o.Analyzer.CodeStatus()
	if err == nil {
		t.Fatal("CodeStatus() expected error for spec-vs-code gap, got nil")
	}
	if !strings.Contains(err.Error(), "gap") {
		t.Errorf("error should mention 'gap', got: %v", err)
	}
}

func TestCodeStatus_MissingRoadmap(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// No docs/road-map.yaml present.
	o := New(Config{})
	if err := o.Analyzer.CodeStatus(); err == nil {
		t.Fatal("CodeStatus() expected error when road-map.yaml missing, got nil")
	}
}
