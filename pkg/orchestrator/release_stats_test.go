// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildReleaseRows(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()

	// Create roadmap.
	roadmap := `id: test-roadmap
title: Test Roadmap
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        summary: Init
        status: done
      - id: rel01.0-uc002-run
        summary: Run
        status: done
  - version: "02.0"
    name: Extension
    status: in progress
    use_cases:
      - id: rel02.0-uc001-ui
        summary: UI
        status: done
      - id: rel02.0-uc002-dash
        summary: Dashboard
        status: spec_complete
`
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0o755)
	os.WriteFile(filepath.Join(docsDir, "road-map.yaml"), []byte(roadmap), 0o644)

	// Create PRD files.
	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)

	prd001 := `name: orchestrator-core
requirements:
  r1:
    title: Config
    items:
      - R1.1: "config loading"
      - R1.2: "config defaults"
  r2:
    title: Init
    items:
      - R2.1: "initialization"
`
	os.WriteFile(filepath.Join(prdDir, "prd001-orchestrator-core.yaml"), []byte(prd001), 0o644)

	prd006 := `name: vscode-extension
requirements:
  r1:
    title: Lifecycle
    items:
      - R1.1: "start command"
      - R1.2: "stop command"
`
	os.WriteFile(filepath.Join(prdDir, "prd006-vscode-extension.yaml"), []byte(prd006), 0o644)

	// Create use case files with touchpoints referencing PRDs.
	ucDir := filepath.Join(dir, "docs", "specs", "use-cases")
	os.MkdirAll(ucDir, 0o755)

	uc1 := `id: rel01.0-uc001-init
title: Init
summary: Init
actor: Dev
trigger: mage init
flow:
  - F1: "step"
touchpoints:
  - T1: "Config: prd001-orchestrator-core R1"
success_criteria:
  - SC1: "works"
out_of_scope: []
`
	os.WriteFile(filepath.Join(ucDir, "rel01.0-uc001-init.yaml"), []byte(uc1), 0o644)

	uc2 := `id: rel02.0-uc001-ui
title: UI
summary: UI
actor: Dev
trigger: command
flow:
  - F1: "step"
touchpoints:
  - T1: "Extension: prd006-vscode-extension R1"
success_criteria:
  - SC1: "works"
out_of_scope: []
`
	os.WriteFile(filepath.Join(ucDir, "rel02.0-uc001-ui.yaml"), []byte(uc2), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	rows, err := buildReleaseRows()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}

	// Release 01.0: all UCs done → PRD complete.
	r1 := rows[0]
	if r1.Version != "01.0" {
		t.Errorf("row[0].Version = %q, want %q", r1.Version, "01.0")
	}
	if r1.PRDs != 1 {
		t.Errorf("row[0].PRDs = %d, want 1", r1.PRDs)
	}
	if r1.PRDsComplete != 1 {
		t.Errorf("row[0].PRDsComplete = %d, want 1", r1.PRDsComplete)
	}
	if r1.Reqs != 3 {
		t.Errorf("row[0].Reqs = %d, want 3", r1.Reqs)
	}
	if r1.ReqsDone != 3 {
		t.Errorf("row[0].ReqsDone = %d, want 3", r1.ReqsDone)
	}

	// Release 02.0: mixed UC statuses → PRD started.
	r2 := rows[1]
	if r2.Version != "02.0" {
		t.Errorf("row[1].Version = %q, want %q", r2.Version, "02.0")
	}
	if r2.PRDs != 1 {
		t.Errorf("row[1].PRDs = %d, want 1", r2.PRDs)
	}
	if r2.PRDsStarted != 1 {
		t.Errorf("row[1].PRDsStarted = %d, want 1", r2.PRDsStarted)
	}
	if r2.ReqsDone != 0 {
		t.Errorf("row[1].ReqsDone = %d, want 0", r2.ReqsDone)
	}
}

func TestBuildReleaseRows_NoRoadmap(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	rows, err := buildReleaseRows()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rows != nil {
		t.Errorf("expected nil rows, got %v", rows)
	}
}

func TestReleaseAllUCsDone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		statuses []string
		want     bool
	}{
		{nil, false},
		{[]string{"done", "done"}, true},
		{[]string{"done", "implemented"}, true},
		{[]string{"done", "spec_complete"}, false},
		{[]string{"implemented"}, true},
	}
	for _, tc := range tests {
		if got := releaseAllUCsDone(tc.statuses); got != tc.want {
			t.Errorf("releaseAllUCsDone(%v) = %v, want %v", tc.statuses, got, tc.want)
		}
	}
}

func TestReleaseAnyUCDone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		statuses []string
		want     bool
	}{
		{nil, false},
		{[]string{"spec_complete"}, false},
		{[]string{"done", "spec_complete"}, true},
		{[]string{"implemented"}, true},
	}
	for _, tc := range tests {
		if got := releaseAnyUCDone(tc.statuses); got != tc.want {
			t.Errorf("releaseAnyUCDone(%v) = %v, want %v", tc.statuses, got, tc.want)
		}
	}
}

func TestReleaseStats_NoRoadmap(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	o := &Orchestrator{}
	if err := o.ReleaseStats(); err != nil {
		t.Errorf("ReleaseStats() returned error: %v", err)
	}
}

func TestReleaseStats_WithRoadmap(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0o755)
	roadmap := `id: test
title: Test
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        summary: Init
        status: done
`
	os.WriteFile(filepath.Join(docsDir, "road-map.yaml"), []byte(roadmap), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	o := &Orchestrator{}
	if err := o.ReleaseStats(); err != nil {
		t.Errorf("ReleaseStats() returned error: %v", err)
	}
}

func TestBuildReleaseRows_PRDWithZeroRequirements(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(filepath.Join(docsDir, "specs", "product-requirements"), 0o755)
	os.MkdirAll(filepath.Join(docsDir, "specs", "use-cases"), 0o755)

	roadmap := `id: test
title: Test
releases:
  - version: "01.0"
    name: Core
    status: done
    use_cases:
      - id: rel01.0-uc001-init
        summary: Init
        status: done
`
	os.WriteFile(filepath.Join(docsDir, "road-map.yaml"), []byte(roadmap), 0o644)

	// PRD with no requirements section.
	prd := `name: empty-prd
requirements: {}
`
	os.WriteFile(filepath.Join(docsDir, "specs", "product-requirements", "prd001-empty.yaml"), []byte(prd), 0o644)

	uc := `id: rel01.0-uc001-init
title: Init
summary: Init
actor: Dev
trigger: mage init
flow:
  - F1: "step"
touchpoints:
  - T1: "Config: prd001-empty R1"
success_criteria:
  - SC1: "works"
out_of_scope: []
`
	os.WriteFile(filepath.Join(docsDir, "specs", "use-cases", "rel01.0-uc001-init.yaml"), []byte(uc), 0o644)

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	rows, err := buildReleaseRows()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].PRDsNoReqs != 1 {
		t.Errorf("PRDsNoReqs = %d, want 1", rows[0].PRDsNoReqs)
	}
	if rows[0].PRDsComplete != 0 {
		t.Errorf("PRDsComplete = %d, want 0 (no reqs means not counted as complete)", rows[0].PRDsComplete)
	}
}
