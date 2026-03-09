// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateRequirementsFile(t *testing.T) {
	t.Run("scans PRDs and produces requirements.yaml", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)

		prd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "first item"
      - R1.2: "second item"
  R2:
    title: "Other"
    items:
      - R2.1: "third item"
`
		os.WriteFile(filepath.Join(prdDir, "prd001-core.yaml"), []byte(prd), 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read output: %v", err)
		}

		var rf RequirementsFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			t.Fatalf("cannot parse output: %v", err)
		}

		prdReqs, ok := rf.Requirements["prd001-core"]
		if !ok {
			t.Fatal("expected prd001-core in requirements")
		}
		if len(prdReqs) != 3 {
			t.Fatalf("expected 3 R-items, got %d", len(prdReqs))
		}
		for _, id := range []string{"R1.1", "R1.2", "R2.1"} {
			state, ok := prdReqs[id]
			if !ok {
				t.Errorf("missing R-item %s", id)
				continue
			}
			if state.Status != "ready" {
				t.Errorf("R-item %s: expected status ready, got %s", id, state.Status)
			}
			if state.Issue != 0 {
				t.Errorf("R-item %s: expected issue 0, got %d", id, state.Issue)
			}
		}
	})

	t.Run("empty PRD directory", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read output: %v", err)
		}

		var rf RequirementsFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			t.Fatalf("cannot parse output: %v", err)
		}

		if len(rf.Requirements) != 0 {
			t.Errorf("expected 0 PRDs, got %d", len(rf.Requirements))
		}
	})

	t.Run("PRD with no items", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)

		prd := `requirements:
  R1:
    title: "Empty group"
    items: []
`
		os.WriteFile(filepath.Join(prdDir, "prd002-empty.yaml"), []byte(prd), 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read output: %v", err)
		}

		var rf RequirementsFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			t.Fatalf("cannot parse output: %v", err)
		}

		// PRD with empty items should not appear.
		if len(rf.Requirements) != 0 {
			t.Errorf("expected 0 PRDs, got %d", len(rf.Requirements))
		}
	})

	t.Run("multiple PRDs", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)

		prd1 := `requirements:
  R1:
    title: "First"
    items:
      - R1.1: "item A"
`
		prd2 := `requirements:
  R1:
    title: "Second"
    items:
      - R1.1: "item B"
      - R1.2: "item C"
`
		os.WriteFile(filepath.Join(prdDir, "prd001-alpha.yaml"), []byte(prd1), 0o644)
		os.WriteFile(filepath.Join(prdDir, "prd002-beta.yaml"), []byte(prd2), 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read output: %v", err)
		}

		var rf RequirementsFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			t.Fatalf("cannot parse output: %v", err)
		}

		if len(rf.Requirements) != 2 {
			t.Fatalf("expected 2 PRDs, got %d", len(rf.Requirements))
		}
		if len(rf.Requirements["prd001-alpha"]) != 1 {
			t.Errorf("prd001-alpha: expected 1 item, got %d", len(rf.Requirements["prd001-alpha"]))
		}
		if len(rf.Requirements["prd002-beta"]) != 2 {
			t.Errorf("prd002-beta: expected 2 items, got %d", len(rf.Requirements["prd002-beta"]))
		}
	})
}

func TestGenerateRequirementsFile_PreserveExisting(t *testing.T) {
	t.Run("preserves completed states from previous run", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)
		os.MkdirAll(cobblerDir, 0o755)

		prd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "first item"
      - R1.2: "second item"
  R2:
    title: "Other"
    items:
      - R2.1: "third item"
`
		os.WriteFile(filepath.Join(prdDir, "prd001-core.yaml"), []byte(prd), 0o644)

		// Seed existing requirements with R1.1 complete.
		existing := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete", Issue: 42},
					"R1.2": {Status: "ready"},
					"R2.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(existing)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		// R1.1 should retain its complete state.
		assertReqState(t, result, "prd001-core", "R1.1", "complete", 42)
		// R1.2 and R2.1 should remain ready.
		assertReqState(t, result, "prd001-core", "R1.2", "ready", 0)
		assertReqState(t, result, "prd001-core", "R2.1", "ready", 0)
	})

	t.Run("new R-items default to ready", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)
		os.MkdirAll(cobblerDir, 0o755)

		prd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "existing"
      - R1.2: "new item"
`
		os.WriteFile(filepath.Join(prdDir, "prd001-core.yaml"), []byte(prd), 0o644)

		// Existing file only has R1.1.
		existing := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
				},
			},
		}
		data, _ := yaml.Marshal(existing)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		assertReqState(t, result, "prd001-core", "R1.1", "complete", 10)
		assertReqState(t, result, "prd001-core", "R1.2", "ready", 0)
	})

	t.Run("removed R-items are dropped", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)
		os.MkdirAll(cobblerDir, 0o755)

		// PRD now only has R1.1.
		prd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "kept"
`
		os.WriteFile(filepath.Join(prdDir, "prd001-core.yaml"), []byte(prd), 0o644)

		// Existing file has R1.1 and R1.2 (R1.2 was removed from PRD).
		existing := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete", Issue: 5},
					"R1.2": {Status: "complete", Issue: 6},
				},
			},
		}
		data, _ := yaml.Marshal(existing)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		assertReqState(t, result, "prd001-core", "R1.1", "complete", 5)
		if _, ok := result.Requirements["prd001-core"]["R1.2"]; ok {
			t.Error("R1.2 should have been dropped (removed from PRD)")
		}
	})

	t.Run("no existing file behaves like fresh generation", func(t *testing.T) {
		tmp := t.TempDir()
		prdDir := filepath.Join(tmp, "prds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(prdDir, 0o755)

		prd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "item"
`
		os.WriteFile(filepath.Join(prdDir, "prd001-core.yaml"), []byte(prd), 0o644)

		path, err := GenerateRequirementsFile(prdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		assertReqState(t, result, "prd001-core", "R1.1", "ready", 0)
	})
}

func TestUpdateRequirementsFile(t *testing.T) {
	t.Run("marks matching sub-items as complete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "ready"},
					"R1.2": {Status: "ready"},
					"R2.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "prd001 R1.2 — implement config loading"
  - id: R2
    text: "prd001 R2.1 — implement other thing"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 42, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))

		// R1.2 and R2.1 should be complete with issue 42.
		assertReqState(t, result, "prd001-core", "R1.2", "complete", 42)
		assertReqState(t, result, "prd001-core", "R2.1", "complete", 42)
		// R1.1 should remain ready.
		assertReqState(t, result, "prd001-core", "R1.1", "ready", 0)
	})

	t.Run("group reference marks all sub-items", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd002-lifecycle": {
					"R1.1": {Status: "ready"},
					"R1.2": {Status: "ready"},
					"R2.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "prd002 R1 — implement entire group"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 99, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "prd002-lifecycle", "R1.1", "complete", 99)
		assertReqState(t, result, "prd002-lifecycle", "R1.2", "complete", 99)
		assertReqState(t, result, "prd002-lifecycle", "R2.1", "ready", 0)
	})

	t.Run("missing file returns nil", func(t *testing.T) {
		tmp := t.TempDir()
		err := UpdateRequirementsFile(tmp, "requirements:\n  - id: R1\n    text: prd001 R1.1", 1, true)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("no matching requirements is no-op", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "prd999 R5.3 — nonexistent PRD"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 10, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "prd001-core", "R1.1", "ready", 0)
	})

	t.Run("never regresses complete to ready", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "prd001 R1 — redo whole group"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 20, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		// R1.1 should still be complete with original issue 10.
		assertReqState(t, result, "prd001-core", "R1.1", "complete", 10)
		// R1.2 should now be complete with issue 20.
		assertReqState(t, result, "prd001-core", "R1.2", "complete", 20)
	})
}

func TestUpdateRequirementsFile_TestsFailed(t *testing.T) {
	t.Run("marks as complete_with_failures when tests fail", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "ready"},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "prd001 R1.1 — implement config"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 50, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "prd001-core", "R1.1", "complete_with_failures", 50)
		// R1.2 should remain ready.
		assertReqState(t, result, "prd001-core", "R1.2", "ready", 0)
	})
}

func TestUCRequirementsComplete_CompleteWithFailures(t *testing.T) {
	t.Run("treats complete_with_failures as complete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete_with_failures", Issue: 50},
					"R1.2": {Status: "complete", Issue: 51},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Config struct per prd001-core R1",
		}

		complete, remaining := UCRequirementsComplete(cobblerDir, touchpoints)
		if !complete {
			t.Errorf("expected complete (complete_with_failures counts), got remaining: %v", remaining)
		}
	})
}

func TestUCRequirementsComplete(t *testing.T) {
	t.Run("all requirements complete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
					"R1.2": {Status: "complete", Issue: 11},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Config struct per prd001-core R1",
		}

		complete, remaining := UCRequirementsComplete(cobblerDir, touchpoints)
		if !complete {
			t.Errorf("expected complete, got remaining: %v", remaining)
		}
	})

	t.Run("partial completion", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"prd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Config struct per prd001-core R1",
		}

		complete, remaining := UCRequirementsComplete(cobblerDir, touchpoints)
		if complete {
			t.Error("expected incomplete")
		}
		if len(remaining) != 1 {
			t.Errorf("expected 1 remaining, got %d: %v", len(remaining), remaining)
		}
	})

	t.Run("missing requirements file", func(t *testing.T) {
		complete, remaining := UCRequirementsComplete("/nonexistent", []string{"T1: prd001 R1"})
		if complete {
			t.Error("expected incomplete for missing file")
		}
		if len(remaining) != 0 {
			t.Errorf("expected no remaining items, got %v", remaining)
		}
	})

	t.Run("no touchpoints", func(t *testing.T) {
		tmp := t.TempDir()
		complete, _ := UCRequirementsComplete(tmp, nil)
		if complete {
			t.Error("expected incomplete for no touchpoints")
		}
	})
}

func TestExtractTouchpointCitations(t *testing.T) {
	t.Run("single PRD single group", func(t *testing.T) {
		tps := []string{"T1: Config struct per prd001-core R1"}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 1 {
			t.Fatalf("expected 1 citation, got %d", len(citations))
		}
		if citations[0].prdID != "prd001-core" {
			t.Errorf("prdID = %q, want prd001-core", citations[0].prdID)
		}
		if len(citations[0].groups) != 1 || citations[0].groups[0] != "R1" {
			t.Errorf("groups = %v, want [R1]", citations[0].groups)
		}
	})

	t.Run("multiple groups", func(t *testing.T) {
		tps := []string{"T1: per prd002-lifecycle R1, R3"}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 1 {
			t.Fatalf("expected 1 citation, got %d", len(citations))
		}
		if len(citations[0].groups) != 2 {
			t.Errorf("groups = %v, want [R1, R3]", citations[0].groups)
		}
	})

	t.Run("multiple PRDs across touchpoints", func(t *testing.T) {
		tps := []string{
			"T1: per prd001-core R1",
			"T2: per prd002-lifecycle R2",
		}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 2 {
			t.Fatalf("expected 2 citations, got %d", len(citations))
		}
	})

	t.Run("no PRD references", func(t *testing.T) {
		tps := []string{"T1: some generic touchpoint"}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 0 {
			t.Errorf("expected 0 citations, got %d", len(citations))
		}
	})
}

func TestFindPRDRequirements(t *testing.T) {
	reqs := map[string]map[string]RequirementState{
		"prd001-core":      {"R1.1": {Status: "ready"}},
		"prd010-ext":       {"R2.1": {Status: "ready"}},
		"prd053-logname":   {"R3.1": {Status: "ready"}},
		"prd053-sort":      {"R3.2": {Status: "ready"}},
	}

	t.Run("exact match", func(t *testing.T) {
		r := findPRDRequirements(reqs, "prd001-core")
		if r == nil || r["R1.1"].Status != "ready" {
			t.Errorf("expected exact match for prd001-core, got %v", r)
		}
	})

	t.Run("dash-prefix match", func(t *testing.T) {
		r := findPRDRequirements(reqs, "prd001")
		if r == nil || r["R1.1"].Status != "ready" {
			t.Errorf("expected prd001 to match prd001-core, got %v", r)
		}
	})

	t.Run("greedy prefix rejected", func(t *testing.T) {
		// "prd01" must NOT match "prd010-ext" — the numeric portions differ.
		r := findPRDRequirements(reqs, "prd01")
		if r != nil {
			t.Errorf("prd01 should not match prd010-ext, got %v", r)
		}
	})

	t.Run("no match returns nil", func(t *testing.T) {
		r := findPRDRequirements(reqs, "prd999")
		if r != nil {
			t.Errorf("expected nil for nonexistent stem, got %v", r)
		}
	})

	t.Run("ambiguous prefix picks longest key", func(t *testing.T) {
		// Both "prd053-logname" and "prd053-sort" match "prd053".
		// Longest key is "prd053-logname" (14 chars vs 10).
		r := findPRDRequirements(reqs, "prd053")
		if r == nil {
			t.Fatal("expected a match for prd053")
		}
		// Should pick prd053-logname (longest key).
		if _, ok := r["R3.1"]; !ok {
			t.Errorf("expected prd053 to match prd053-logname (longest), got %v", r)
		}
	})
}

// ---------------------------------------------------------------------------
// sdd-hello-world fixture: prd003-config with 12 R-items across 4 R-groups.
// Mirrors the structure of sdd-hello-world's rel04.0 release (GH-1394).
// ---------------------------------------------------------------------------

const prd003ConfigFixture = `requirements:
  R1:
    title: "Config file loading"
    items:
      - R1.1: "Load YAML configuration from default path"
      - R1.2: "Support environment variable override for config path"
      - R1.3: "Return typed config struct"
  R2:
    title: "Config validation"
    items:
      - R2.1: "Validate required fields are present"
      - R2.2: "Validate field types and ranges"
      - R2.3: "Return structured validation errors"
  R3:
    title: "Config defaults"
    items:
      - R3.1: "Apply default values for optional fields"
      - R3.2: "Merge defaults with user-provided values"
      - R3.3: "Document default values in config template"
  R4:
    title: "Config hot-reload"
    items:
      - R4.1: "Watch config file for changes"
      - R4.2: "Re-validate on change"
      - R4.3: "Notify subscribers of config updates"
`

// prd001 fixture: 6 R-items across 2 R-groups (mirrors sdd-hello-world prd001).
const prd001CoreFixture = `requirements:
  R1:
    title: "Core operations"
    items:
      - R1.1: "Operation A"
      - R1.2: "Operation B"
      - R1.3: "Operation C"
  R2:
    title: "Output formatting"
    items:
      - R2.1: "Format stdout"
      - R2.2: "Format stderr"
      - R2.3: "Exit codes"
`

// prd002 fixture: 11 R-items across 3 R-groups (mirrors sdd-hello-world prd002).
const prd002LifecycleFixture = `requirements:
  R1:
    title: "Lifecycle init"
    items:
      - R1.1: "Initialize state"
      - R1.2: "Validate preconditions"
      - R1.3: "Create workspace"
      - R1.4: "Register signal handlers"
  R2:
    title: "Lifecycle run"
    items:
      - R2.1: "Main loop"
      - R2.2: "Error recovery"
      - R2.3: "Progress reporting"
  R3:
    title: "Lifecycle shutdown"
    items:
      - R3.1: "Graceful shutdown"
      - R3.2: "Cleanup resources"
      - R3.3: "Final status report"
      - R3.4: "Exit with appropriate code"
`

func TestGenerateRequirementsFile_SDDHelloWorldFixture(t *testing.T) {
	tmp := t.TempDir()
	prdDir := filepath.Join(tmp, "prds")
	cobblerDir := filepath.Join(tmp, ".cobbler")
	os.MkdirAll(prdDir, 0o755)

	os.WriteFile(filepath.Join(prdDir, "prd001-core.yaml"), []byte(prd001CoreFixture), 0o644)
	os.WriteFile(filepath.Join(prdDir, "prd002-lifecycle.yaml"), []byte(prd002LifecycleFixture), 0o644)
	os.WriteFile(filepath.Join(prdDir, "prd003-config.yaml"), []byte(prd003ConfigFixture), 0o644)

	path, err := GenerateRequirementsFile(prdDir, cobblerDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rf := readReqFile(t, path)

	// Verify all 3 PRDs are present.
	if len(rf.Requirements) != 3 {
		t.Fatalf("expected 3 PRDs, got %d", len(rf.Requirements))
	}

	// Verify item counts: prd001=6, prd002=11, prd003=12, total=29.
	wantCounts := map[string]int{
		"prd001-core":      6,
		"prd002-lifecycle": 11,
		"prd003-config":    12,
	}
	totalItems := 0
	for prd, wantCount := range wantCounts {
		got := len(rf.Requirements[prd])
		if got != wantCount {
			t.Errorf("%s: expected %d R-items, got %d", prd, wantCount, got)
		}
		totalItems += got
	}
	if totalItems != 29 {
		t.Errorf("expected 29 total R-items, got %d", totalItems)
	}

	// Verify all items start as "ready" with no issue.
	for prd, items := range rf.Requirements {
		for rItem, st := range items {
			if st.Status != "ready" {
				t.Errorf("%s %s: expected status ready, got %s", prd, rItem, st.Status)
			}
			if st.Issue != 0 {
				t.Errorf("%s %s: expected issue 0, got %d", prd, rItem, st.Issue)
			}
		}
	}

	// Verify specific prd003-config sub-requirements are extracted.
	for _, id := range []string{"R1.1", "R1.2", "R1.3", "R2.1", "R2.2", "R2.3",
		"R3.1", "R3.2", "R3.3", "R4.1", "R4.2", "R4.3"} {
		if _, ok := rf.Requirements["prd003-config"][id]; !ok {
			t.Errorf("prd003-config missing expected R-item %s", id)
		}
	}
}

func TestPartialCompletionSequence(t *testing.T) {
	// Simulates the prd003-config 3-task sequence from GH-1394:
	// Task 1: prd003-config R1 (3 items) → 3 complete, 9 ready
	// Task 2: prd003-config R2, R3 (6 items) → 9 complete, 3 ready
	// Task 3: prd003-config R4 (3 items) → 12 complete, 0 ready
	tmp := t.TempDir()
	prdDir := filepath.Join(tmp, "prds")
	cobblerDir := filepath.Join(tmp, ".cobbler")
	os.MkdirAll(prdDir, 0o755)

	os.WriteFile(filepath.Join(prdDir, "prd003-config.yaml"), []byte(prd003ConfigFixture), 0o644)

	// Generate initial requirements.yaml.
	reqPath, err := GenerateRequirementsFile(prdDir, cobblerDir, false)
	if err != nil {
		t.Fatalf("GenerateRequirementsFile: %v", err)
	}

	// Verify initial state: 12 items, all ready.
	rf := readReqFile(t, reqPath)
	if len(rf.Requirements["prd003-config"]) != 12 {
		t.Fatalf("expected 12 R-items, got %d", len(rf.Requirements["prd003-config"]))
	}

	// UC touchpoints: uc004 cites R1,R2,R3; uc005 cites R4.
	uc004Touchpoints := []string{
		"T1: Config loading per prd003-config R1, R2, R3",
	}
	uc005Touchpoints := []string{
		"T1: Config hot-reload per prd003-config R4",
	}

	// --- Task 1: Mark R1 complete (3 items) ---
	desc1 := `requirements:
  - id: R1
    text: "prd003-config R1 — implement config file loading"
`
	if err := UpdateRequirementsFile(cobblerDir, desc1, 100, true); err != nil {
		t.Fatalf("Task 1 UpdateRequirementsFile: %v", err)
	}

	rf = readReqFile(t, reqPath)
	readyCount, completeCount := countStates(rf.Requirements["prd003-config"])
	if completeCount != 3 {
		t.Errorf("after Task 1: expected 3 complete, got %d", completeCount)
	}
	if readyCount != 9 {
		t.Errorf("after Task 1: expected 9 ready, got %d", readyCount)
	}
	assertReqState(t, rf, "prd003-config", "R1.1", "complete", 100)
	assertReqState(t, rf, "prd003-config", "R1.2", "complete", 100)
	assertReqState(t, rf, "prd003-config", "R1.3", "complete", 100)
	assertReqState(t, rf, "prd003-config", "R2.1", "ready", 0)

	// UC checks after Task 1: neither complete.
	complete, remaining := UCRequirementsComplete(cobblerDir, uc004Touchpoints)
	if complete {
		t.Error("after Task 1: uc004 should not be complete (R2,R3 still ready)")
	}
	if len(remaining) != 6 {
		t.Errorf("after Task 1: uc004 expected 6 remaining, got %d: %v", len(remaining), remaining)
	}
	complete, _ = UCRequirementsComplete(cobblerDir, uc005Touchpoints)
	if complete {
		t.Error("after Task 1: uc005 should not be complete (R4 still ready)")
	}

	// --- Task 2: Mark R2, R3 complete (6 items) ---
	desc2 := `requirements:
  - id: R1
    text: "prd003-config R2 — implement config validation"
  - id: R2
    text: "prd003-config R3 — implement config defaults"
`
	if err := UpdateRequirementsFile(cobblerDir, desc2, 101, true); err != nil {
		t.Fatalf("Task 2 UpdateRequirementsFile: %v", err)
	}

	rf = readReqFile(t, reqPath)
	readyCount, completeCount = countStates(rf.Requirements["prd003-config"])
	if completeCount != 9 {
		t.Errorf("after Task 2: expected 9 complete, got %d", completeCount)
	}
	if readyCount != 3 {
		t.Errorf("after Task 2: expected 3 ready, got %d", readyCount)
	}

	// uc004 should now be complete (R1,R2,R3 all done).
	complete, remaining = UCRequirementsComplete(cobblerDir, uc004Touchpoints)
	if !complete {
		t.Errorf("after Task 2: uc004 should be complete, remaining: %v", remaining)
	}
	// uc005 should still be incomplete (R4 ready).
	complete, remaining = UCRequirementsComplete(cobblerDir, uc005Touchpoints)
	if complete {
		t.Error("after Task 2: uc005 should not be complete (R4 still ready)")
	}
	if len(remaining) != 3 {
		t.Errorf("after Task 2: uc005 expected 3 remaining, got %d: %v", len(remaining), remaining)
	}

	// --- Task 3: Mark R4 complete (3 items) ---
	desc3 := `requirements:
  - id: R1
    text: "prd003-config R4 — implement config hot-reload"
`
	if err := UpdateRequirementsFile(cobblerDir, desc3, 102, true); err != nil {
		t.Fatalf("Task 3 UpdateRequirementsFile: %v", err)
	}

	rf = readReqFile(t, reqPath)
	readyCount, completeCount = countStates(rf.Requirements["prd003-config"])
	if completeCount != 12 {
		t.Errorf("after Task 3: expected 12 complete, got %d", completeCount)
	}
	if readyCount != 0 {
		t.Errorf("after Task 3: expected 0 ready, got %d", readyCount)
	}

	// Both UCs should now be complete.
	complete, remaining = UCRequirementsComplete(cobblerDir, uc004Touchpoints)
	if !complete {
		t.Errorf("after Task 3: uc004 should be complete, remaining: %v", remaining)
	}
	complete, remaining = UCRequirementsComplete(cobblerDir, uc005Touchpoints)
	if !complete {
		t.Errorf("after Task 3: uc005 should be complete, remaining: %v", remaining)
	}

	// Verify no regression: re-updating already-complete items is a no-op.
	if err := UpdateRequirementsFile(cobblerDir, desc1, 999, true); err != nil {
		t.Fatalf("re-update should not error: %v", err)
	}
	rf = readReqFile(t, reqPath)
	assertReqState(t, rf, "prd003-config", "R1.1", "complete", 100) // original issue preserved
	assertReqState(t, rf, "prd003-config", "R1.2", "complete", 100)
	assertReqState(t, rf, "prd003-config", "R1.3", "complete", 100)
}

func TestCrossBatchDuplicatePrevention(t *testing.T) {
	// After marking R1-R3 complete, verify that ValidateMeasureOutput
	// rejects proposals targeting completed R-groups and accepts
	// proposals targeting ready R-groups (R4).
	reqStates := map[string]map[string]RequirementState{
		"prd003-config": {
			"R1.1": {Status: "complete", Issue: 100},
			"R1.2": {Status: "complete", Issue: 100},
			"R1.3": {Status: "complete", Issue: 100},
			"R2.1": {Status: "complete", Issue: 101},
			"R2.2": {Status: "complete", Issue: 101},
			"R2.3": {Status: "complete", Issue: 101},
			"R3.1": {Status: "complete", Issue: 101},
			"R3.2": {Status: "complete", Issue: 101},
			"R3.3": {Status: "complete", Issue: 101},
			"R4.1": {Status: "ready"},
			"R4.2": {Status: "ready"},
			"R4.3": {Status: "ready"},
		},
	}

	// Verify fixture counts: 9 complete, 3 ready.
	ready, complete := 0, 0
	for _, st := range reqStates["prd003-config"] {
		if st.Status == "ready" {
			ready++
		} else if isRequirementComplete(st.Status) {
			complete++
		}
	}
	if complete != 9 || ready != 3 {
		t.Fatalf("fixture should have 9 complete + 3 ready, got %d + %d", complete, ready)
	}

	makeDesc := func(reqText string) string {
		return `deliverable_type: code
requirements:
  - id: R1
    text: "` + reqText + `"
  - id: R2
    text: "General work"
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
  - path: pkg/config/load.go`
	}

	t.Run("rejects proposal targeting completed group R1", func(t *testing.T) {
		desc := makeDesc("prd003-config R1 — re-implement loading")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, nil, reqStates)
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "R1") && strings.Contains(e, "complete") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected error for completed group R1, got errors: %v", result.Errors)
		}
	})

	t.Run("rejects proposal targeting completed sub-item R2.1", func(t *testing.T) {
		desc := makeDesc("prd003-config R2.1 — re-validate fields")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, nil, reqStates)
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "R2.1") && strings.Contains(e, "already complete") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected error for completed R2.1, got errors: %v", result.Errors)
		}
	})

	t.Run("accepts proposal targeting ready group R4", func(t *testing.T) {
		desc := makeDesc("prd003-config R4 — implement hot-reload")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, nil, reqStates)
		for _, e := range result.Errors {
			if strings.Contains(e, "R4") && strings.Contains(e, "complete") {
				t.Errorf("R4 is ready, should not be rejected: %s", e)
			}
		}
	})

	t.Run("accepts proposal targeting ready sub-item R4.2", func(t *testing.T) {
		desc := makeDesc("prd003-config R4.2 — re-validate on change")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, nil, reqStates)
		for _, e := range result.Errors {
			if strings.Contains(e, "R4.2") && strings.Contains(e, "complete") {
				t.Errorf("R4.2 is ready, should not be rejected: %s", e)
			}
		}
	})
}

// countStates returns the number of ready and complete items in a requirement map.
func countStates(items map[string]RequirementState) (ready, complete int) {
	for _, st := range items {
		switch {
		case st.Status == "ready":
			ready++
		case isRequirementComplete(st.Status):
			complete++
		}
	}
	return
}

func TestAllRefsAlreadyComplete(t *testing.T) {
	// Build requirement states: prd003-config has R1.1, R1.2 complete and R2.1 ready.
	states := map[string]map[string]RequirementState{
		"prd003-config": {
			"R1.1": {Status: "complete", Issue: 100},
			"R1.2": {Status: "complete", Issue: 100},
			"R2.1": {Status: "ready"},
			"R2.2": {Status: "ready"},
		},
		"prd001-core": {
			"R1.1": {Status: "complete", Issue: 101},
			"R1.2": {Status: "complete_with_failures", Issue: 102},
		},
	}

	makeDesc := func(reqText string) string {
		return "requirements:\n  - text: \"" + reqText + "\"\n    source: test\n"
	}

	t.Run("all refs complete returns true", func(t *testing.T) {
		desc := makeDesc("prd003-config R1.1")
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: R1.1 is complete")
		}
	})

	t.Run("partial complete returns false", func(t *testing.T) {
		desc := makeDesc("prd003-config R2.1")
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: R2.1 is ready")
		}
	})

	t.Run("no refs returns false", func(t *testing.T) {
		desc := "requirements:\n  - text: \"no prd refs here\"\n    source: test\n"
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: no PRD refs")
		}
	})

	t.Run("nil states returns false", func(t *testing.T) {
		desc := makeDesc("prd003-config R1.1")
		if AllRefsAlreadyComplete(desc, nil) {
			t.Error("expected false: nil states")
		}
	})

	t.Run("group reference all complete", func(t *testing.T) {
		desc := makeDesc("prd001-core R1")
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: all R1.x in prd001-core are complete")
		}
	})

	t.Run("group reference partial complete", func(t *testing.T) {
		desc := makeDesc("prd003-config R2")
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: R2 group has ready items")
		}
	})

	t.Run("complete_with_failures counts as complete", func(t *testing.T) {
		desc := makeDesc("prd001-core R1.2")
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: complete_with_failures is still complete")
		}
	})

	t.Run("unknown PRD returns false", func(t *testing.T) {
		desc := makeDesc("prd999-unknown R1.1")
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: PRD not in states")
		}
	})

	t.Run("multiple refs all complete", func(t *testing.T) {
		desc := "requirements:\n  - text: \"prd003-config R1.1\"\n    source: test\n  - text: \"prd001-core R1.2\"\n    source: test\n"
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: both refs are complete")
		}
	})

	t.Run("multiple refs one incomplete", func(t *testing.T) {
		desc := "requirements:\n  - text: \"prd003-config R1.1\"\n    source: test\n  - text: \"prd003-config R2.1\"\n    source: test\n"
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: R2.1 is ready")
		}
	})
}

func readReqFile(t *testing.T, path string) RequirementsFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	var rf RequirementsFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		t.Fatalf("cannot parse %s: %v", path, err)
	}
	return rf
}

func assertReqState(t *testing.T, rf RequirementsFile, prd, rItem, wantStatus string, wantIssue int) {
	t.Helper()
	prdReqs, ok := rf.Requirements[prd]
	if !ok {
		t.Errorf("PRD %s not found", prd)
		return
	}
	st, ok := prdReqs[rItem]
	if !ok {
		t.Errorf("%s %s not found", prd, rItem)
		return
	}
	if st.Status != wantStatus {
		t.Errorf("%s %s: status = %q, want %q", prd, rItem, st.Status, wantStatus)
	}
	if st.Issue != wantIssue {
		t.Errorf("%s %s: issue = %d, want %d", prd, rItem, st.Issue, wantIssue)
	}
}
