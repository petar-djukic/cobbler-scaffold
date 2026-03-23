// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- UCPrefixFromID ---

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
		if got := UCPrefixFromID(tc.input); got != tc.want {
			t.Errorf("UCPrefixFromID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- TestDirForUC ---

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
		if got := TestDirForUC(tc.input); got != tc.want {
			t.Errorf("TestDirForUC(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- CountTestFiles ---

func TestCountTestFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "init_test.go"), []byte("package x"), 0o644)
	os.WriteFile(filepath.Join(dir, "bench_test.go"), []byte("package x"), 0o644)
	os.WriteFile(filepath.Join(dir, "helper.go"), []byte("package x"), 0o644)

	if got := CountTestFiles(dir); got != 2 {
		t.Errorf("CountTestFiles = %d, want 2", got)
	}
}

func TestCountTestFiles_Empty(t *testing.T) {
	dir := t.TempDir()
	if got := CountTestFiles(dir); got != 0 {
		t.Errorf("CountTestFiles = %d, want 0", got)
	}
}

func TestCountTestFiles_NoDir(t *testing.T) {
	if got := CountTestFiles("/nonexistent/path"); got != 0 {
		t.Errorf("CountTestFiles = %d, want 0", got)
	}
}

// --- ScanTestDirectories ---

func TestScanTestDirectories(t *testing.T) {
	root := t.TempDir()
	uc001 := filepath.Join(root, "rel01.0", "uc001")
	os.MkdirAll(uc001, 0o755)
	os.WriteFile(filepath.Join(uc001, "init_test.go"), []byte("package x"), 0o644)

	uc002 := filepath.Join(root, "rel01.0", "uc002")
	os.MkdirAll(uc002, 0o755)
	os.WriteFile(filepath.Join(uc002, "life_test.go"), []byte("package x"), 0o644)
	os.WriteFile(filepath.Join(uc002, "bench_test.go"), []byte("package x"), 0o644)

	uc201 := filepath.Join(root, "rel02.0", "uc001")
	os.MkdirAll(uc201, 0o755)
	os.WriteFile(filepath.Join(uc201, "helper.go"), []byte("package x"), 0o644)

	got := ScanTestDirectories(root)
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
	got := ScanTestDirectories(root)
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestScanTestDirectories_NoDir(t *testing.T) {
	got := ScanTestDirectories("/nonexistent/tests")
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestScanTestDirectories_SkipsNonRelDirs(t *testing.T) {
	root := t.TempDir()
	internal := filepath.Join(root, "internal", "testutil")
	os.MkdirAll(internal, 0o755)
	os.WriteFile(filepath.Join(internal, "helper_test.go"), []byte("package x"), 0o644)

	got := ScanTestDirectories(root)
	if len(got) != 0 {
		t.Errorf("got %v, want empty (internal/ should be skipped)", got)
	}
}

// --- ComputeCodeStatus ---

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
	report := ComputeCodeStatus(roadmap, scan, nil)

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
	}
	report := ComputeCodeStatus(roadmap, scan, nil)

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
	report := ComputeCodeStatus(roadmap, scan, nil)

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
	report := ComputeCodeStatus(roadmap, scan, nil)

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
	report := ComputeCodeStatus(roadmap, scan, nil)

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

// --- DetectSpecCodeGaps ---

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
	gaps := DetectSpecCodeGaps(report)
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
	gaps := DetectSpecCodeGaps(report)
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
	gaps := DetectSpecCodeGaps(report)
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
	gaps := DetectSpecCodeGaps(report)
	if len(gaps) != 0 {
		t.Errorf("got %v, want no gaps", gaps)
	}
}

// --- StatusIcon ---

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
		if got := StatusIcon(tc.input); got != tc.want {
			t.Errorf("StatusIcon(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// --- PrintCodeStatusReport ---

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
	PrintCodeStatusReport(report)
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
	PrintCodeStatusReport(report)
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

// --- PrintCodeStatus (integration) ---

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

func TestPrintCodeStatus_NoGaps(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.yaml"), []byte(roadmapYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	testDir := filepath.Join(dir, "tests", "rel01.0", "uc001")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "init_test.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := PrintCodeStatus(); err != nil {
		t.Errorf("PrintCodeStatus() returned error: %v", err)
	}
}

func TestPrintCodeStatus_WithGap(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.yaml"), []byte(roadmapYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	err = PrintCodeStatus()
	if err == nil {
		t.Fatal("PrintCodeStatus() expected error for spec-vs-code gap, got nil")
	}
	if !strings.Contains(err.Error(), "gap") {
		t.Errorf("error should mention 'gap', got: %v", err)
	}
}

// --- ComputeCodeStatus with reqComplete (GH-1948) ---

func TestComputeCodeStatus_ReqCompleteMarksImplemented(t *testing.T) {
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
	scan := map[string]int{} // no test files
	reqComplete := map[string]bool{
		"rel01.0-uc001": true,
		"rel01.0-uc002": true,
	}
	report := ComputeCodeStatus(roadmap, scan, reqComplete)

	if report.Releases[0].CodeReadiness != "all implemented" {
		t.Errorf("CodeReadiness: got %q, want %q", report.Releases[0].CodeReadiness, "all implemented")
	}
	for i, uc := range report.Releases[0].UseCases {
		if uc.CodeStatus != "implemented" {
			t.Errorf("UC[%d] CodeStatus: got %q, want %q", i, uc.CodeStatus, "implemented")
		}
		if uc.TestFiles != 0 {
			t.Errorf("UC[%d] TestFiles: got %d, want 0 (implemented via reqs, not tests)", i, uc.TestFiles)
		}
	}
}

func TestComputeCodeStatus_ReqCompletePartial(t *testing.T) {
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
	scan := map[string]int{}
	reqComplete := map[string]bool{
		"rel01.0-uc001": true,
		"rel01.0-uc002": false, // not all reqs complete
	}
	report := ComputeCodeStatus(roadmap, scan, reqComplete)

	if report.Releases[0].CodeReadiness != "partial" {
		t.Errorf("CodeReadiness: got %q, want %q", report.Releases[0].CodeReadiness, "partial")
	}
}

func TestComputeCodeStatus_TestFilesTakePrecedence(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{{
			Version: "01.0",
			Name:    "Core",
			Status:  "done",
			UseCases: []RoadmapUseCase{
				{ID: "rel01.0-uc001-init", Status: "done"},
			},
		}},
	}
	scan := map[string]int{"rel01.0-uc001": 3}
	reqComplete := map[string]bool{"rel01.0-uc001": true}
	report := ComputeCodeStatus(roadmap, scan, reqComplete)

	uc := report.Releases[0].UseCases[0]
	if uc.CodeStatus != "implemented" {
		t.Errorf("CodeStatus: got %q, want %q", uc.CodeStatus, "implemented")
	}
	if uc.TestFiles != 3 {
		t.Errorf("TestFiles: got %d, want 3 (test files should be counted when present)", uc.TestFiles)
	}
	if uc.TestDir == "" {
		t.Error("TestDir should be set when test files exist")
	}
}

func TestComputeCodeStatus_NilReqCompleteBackwardCompat(t *testing.T) {
	roadmap := &RoadmapDoc{
		Releases: []RoadmapRelease{{
			Version: "01.0",
			Name:    "Core",
			Status:  "done",
			UseCases: []RoadmapUseCase{
				{ID: "rel01.0-uc001-init", Status: "done"},
			},
		}},
	}
	scan := map[string]int{}
	report := ComputeCodeStatus(roadmap, scan, nil)

	if report.Releases[0].UseCases[0].CodeStatus != "not started" {
		t.Errorf("CodeStatus: got %q, want %q (nil reqComplete should not change behavior)",
			report.Releases[0].UseCases[0].CodeStatus, "not started")
	}
}

// --- ComputeReqCompletion (GH-1948) ---

func TestComputeReqCompletion_AllComplete(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)

	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(`requirements:
  prd001-core:
    R1.1:
      status: complete
    R1.2:
      status: complete
`), 0o644)

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(`id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
`), 0o644)

	result := ComputeReqCompletion(cobblerDir)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result["rel01.0-uc001"] {
		t.Error("rel01.0-uc001 should be complete")
	}
}

func TestComputeReqCompletion_PartiallyComplete(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)

	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(`requirements:
  prd001-core:
    R1.1:
      status: complete
    R1.2:
      status: ready
`), 0o644)

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(`id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
`), 0o644)

	result := ComputeReqCompletion(cobblerDir)
	if result["rel01.0-uc001"] {
		t.Error("rel01.0-uc001 should NOT be complete (R1.2 is ready)")
	}
}

func TestComputeReqCompletion_NoRequirementsFile(t *testing.T) {
	dir := t.TempDir()
	result := ComputeReqCompletion(dir)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestComputeReqCompletion_SkipStatusCountsAsComplete(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)

	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(`requirements:
  prd001-core:
    R1.1:
      status: complete
    R1.2:
      status: skip
`), 0o644)

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(`id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
`), 0o644)

	result := ComputeReqCompletion(cobblerDir)
	if !result["rel01.0-uc001"] {
		t.Error("rel01.0-uc001 should be complete (skip counts as complete)")
	}
}

func TestComputeReqCompletion_CompleteWithFailuresCountsAsComplete(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)

	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(`requirements:
  prd001-core:
    R1.1:
      status: complete_with_failures
`), 0o644)

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(`id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
`), 0o644)

	result := ComputeReqCompletion(cobblerDir)
	if !result["rel01.0-uc001"] {
		t.Error("rel01.0-uc001 should be complete (complete_with_failures counts)")
	}
}

func TestComputeReqCompletion_MissingPRDInReqs(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)

	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(`requirements:
  prd099-other:
    R1.1:
      status: complete
`), 0o644)

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(`id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001-core R1
`), 0o644)

	result := ComputeReqCompletion(cobblerDir)
	if result["rel01.0-uc001"] {
		t.Error("rel01.0-uc001 should NOT be complete (prd001-core not in requirements)")
	}
}

func TestComputeReqCompletion_PrefixMatch(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	os.MkdirAll("docs/specs/use-cases", 0o755)

	// requirements.yaml has full stem "prd001-core", touchpoint cites "prd001"
	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(`requirements:
  prd001-core:
    R1.1:
      status: complete
`), 0o644)

	os.WriteFile("docs/specs/use-cases/rel01.0-uc001-init.yaml", []byte(`id: rel01.0-uc001-init
title: Init
touchpoints:
  - T1: prd001 R1
`), 0o644)

	result := ComputeReqCompletion(cobblerDir)
	if !result["rel01.0-uc001"] {
		t.Error("rel01.0-uc001 should be complete (prd001 prefix matches prd001-core)")
	}
}

// --- isRequirementComplete ---

func TestIsRequirementComplete(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"complete", true},
		{"complete_with_failures", true},
		{"skip", true},
		{"ready", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isRequirementComplete(tc.status); got != tc.want {
			t.Errorf("isRequirementComplete(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

// --- findPRDRequirements ---

func TestFindPRDRequirements_ExactMatch(t *testing.T) {
	reqs := map[string]map[string]RequirementState{
		"prd001-core": {"R1.1": {Status: "complete"}},
	}
	got := findPRDRequirements(reqs, "prd001-core")
	if got == nil || got["R1.1"].Status != "complete" {
		t.Errorf("expected exact match for prd001-core")
	}
}

func TestFindPRDRequirements_PrefixMatch(t *testing.T) {
	reqs := map[string]map[string]RequirementState{
		"prd001-core": {"R1.1": {Status: "ready"}},
	}
	got := findPRDRequirements(reqs, "prd001")
	if got == nil || got["R1.1"].Status != "ready" {
		t.Errorf("expected prefix match prd001 -> prd001-core")
	}
}

func TestFindPRDRequirements_NoMatch(t *testing.T) {
	reqs := map[string]map[string]RequirementState{
		"prd001-core": {"R1.1": {Status: "ready"}},
	}
	got := findPRDRequirements(reqs, "prd999")
	if got != nil {
		t.Errorf("expected nil for non-matching stem, got %v", got)
	}
}

func TestFindPRDRequirements_LongestPrefixWins(t *testing.T) {
	reqs := map[string]map[string]RequirementState{
		"prd001-core":     {"R1.1": {Status: "ready"}},
		"prd001-core-ext": {"R1.1": {Status: "complete"}},
	}
	got := findPRDRequirements(reqs, "prd001-core")
	// Exact match should win over prefix
	if got == nil || got["R1.1"].Status != "ready" {
		t.Errorf("exact match should take precedence")
	}
}

func TestPrintCodeStatus_MissingRoadmap(t *testing.T) {
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := PrintCodeStatus(); err == nil {
		t.Fatal("PrintCodeStatus() expected error when road-map.yaml missing, got nil")
	}
}
