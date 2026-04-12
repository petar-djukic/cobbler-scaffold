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
	t.Run("scans SRDs and produces requirements.yaml", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)

		srd := `requirements:
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
		os.WriteFile(filepath.Join(srdDir, "srd001-core.yaml"), []byte(srd), 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
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

		srdReqs, ok := rf.Requirements["srd001-core"]
		if !ok {
			t.Fatal("expected srd001-core in requirements")
		}
		if len(srdReqs) != 3 {
			t.Fatalf("expected 3 R-items, got %d", len(srdReqs))
		}
		for _, id := range []string{"R1.1", "R1.2", "R2.1"} {
			state, ok := srdReqs[id]
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

	t.Run("empty SRD directory", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
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
			t.Errorf("expected 0 SRDs, got %d", len(rf.Requirements))
		}
	})

	t.Run("SRD with no items", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)

		srd := `requirements:
  R1:
    title: "Empty group"
    items: []
`
		os.WriteFile(filepath.Join(srdDir, "srd002-empty.yaml"), []byte(srd), 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
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

		// SRD with empty items should not appear.
		if len(rf.Requirements) != 0 {
			t.Errorf("expected 0 SRDs, got %d", len(rf.Requirements))
		}
	})

	t.Run("multiple SRDs", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)

		srd1 := `requirements:
  R1:
    title: "First"
    items:
      - R1.1: "item A"
`
		srd2 := `requirements:
  R1:
    title: "Second"
    items:
      - R1.1: "item B"
      - R1.2: "item C"
`
		os.WriteFile(filepath.Join(srdDir, "srd001-alpha.yaml"), []byte(srd1), 0o644)
		os.WriteFile(filepath.Join(srdDir, "srd002-beta.yaml"), []byte(srd2), 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
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
			t.Fatalf("expected 2 SRDs, got %d", len(rf.Requirements))
		}
		if len(rf.Requirements["srd001-alpha"]) != 1 {
			t.Errorf("srd001-alpha: expected 1 item, got %d", len(rf.Requirements["srd001-alpha"]))
		}
		if len(rf.Requirements["srd002-beta"]) != 2 {
			t.Errorf("srd002-beta: expected 2 items, got %d", len(rf.Requirements["srd002-beta"]))
		}
	})
}

func TestGenerateRequirementsFile_PreserveExisting(t *testing.T) {
	t.Run("preserves completed states from previous run", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)
		os.MkdirAll(cobblerDir, 0o755)

		srd := `requirements:
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
		os.WriteFile(filepath.Join(srdDir, "srd001-core.yaml"), []byte(srd), 0o644)

		// Seed existing requirements with R1.1 complete.
		existing := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "complete", Issue: 42},
					"R1.2": {Status: "ready"},
					"R2.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(existing)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		// R1.1 should retain its complete state.
		assertReqState(t, result, "srd001-core", "R1.1", "complete", 42)
		// R1.2 and R2.1 should remain ready.
		assertReqState(t, result, "srd001-core", "R1.2", "ready", 0)
		assertReqState(t, result, "srd001-core", "R2.1", "ready", 0)
	})

	t.Run("new R-items default to ready", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)
		os.MkdirAll(cobblerDir, 0o755)

		srd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "existing"
      - R1.2: "new item"
`
		os.WriteFile(filepath.Join(srdDir, "srd001-core.yaml"), []byte(srd), 0o644)

		// Existing file only has R1.1.
		existing := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
				},
			},
		}
		data, _ := yaml.Marshal(existing)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		assertReqState(t, result, "srd001-core", "R1.1", "complete", 10)
		assertReqState(t, result, "srd001-core", "R1.2", "ready", 0)
	})

	t.Run("removed R-items are dropped", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)
		os.MkdirAll(cobblerDir, 0o755)

		// SRD now only has R1.1.
		srd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "kept"
`
		os.WriteFile(filepath.Join(srdDir, "srd001-core.yaml"), []byte(srd), 0o644)

		// Existing file has R1.1 and R1.2 (R1.2 was removed from SRD).
		existing := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "complete", Issue: 5},
					"R1.2": {Status: "complete", Issue: 6},
				},
			},
		}
		data, _ := yaml.Marshal(existing)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		assertReqState(t, result, "srd001-core", "R1.1", "complete", 5)
		if _, ok := result.Requirements["srd001-core"]["R1.2"]; ok {
			t.Error("R1.2 should have been dropped (removed from SRD)")
		}
	})

	t.Run("no existing file behaves like fresh generation", func(t *testing.T) {
		tmp := t.TempDir()
		srdDir := filepath.Join(tmp, "srds")
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(srdDir, 0o755)

		srd := `requirements:
  R1:
    title: "Config"
    items:
      - R1.1: "item"
`
		os.WriteFile(filepath.Join(srdDir, "srd001-core.yaml"), []byte(srd), 0o644)

		path, err := GenerateRequirementsFile(srdDir, cobblerDir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, path)
		assertReqState(t, result, "srd001-core", "R1.1", "ready", 0)
	})
}

func TestUpdateRequirementsFile(t *testing.T) {
	t.Run("marks matching sub-items as complete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
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
    text: "srd001 R1.2 — implement config loading"
  - id: R2
    text: "srd001 R2.1 — implement other thing"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 42, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))

		// R1.2 and R2.1 should be complete with issue 42.
		assertReqState(t, result, "srd001-core", "R1.2", "complete", 42)
		assertReqState(t, result, "srd001-core", "R2.1", "complete", 42)
		// R1.1 should remain ready.
		assertReqState(t, result, "srd001-core", "R1.1", "ready", 0)
	})

	t.Run("group reference marks all sub-items", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd002-lifecycle": {
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
    text: "srd002 R1 — implement entire group"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 99, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "srd002-lifecycle", "R1.1", "complete", 99)
		assertReqState(t, result, "srd002-lifecycle", "R1.2", "complete", 99)
		assertReqState(t, result, "srd002-lifecycle", "R2.1", "ready", 0)
	})

	t.Run("missing file returns nil", func(t *testing.T) {
		tmp := t.TempDir()
		err := UpdateRequirementsFile(tmp, "requirements:\n  - id: R1\n    text: srd001 R1.1", 1, true, false)
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
				"srd001-core": {
					"R1.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd999 R5.3 — nonexistent SRD"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 10, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "srd001-core", "R1.1", "ready", 0)
	})

	t.Run("never regresses complete to ready", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1 — redo whole group"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 20, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		// R1.1 should still be complete with original issue 10.
		assertReqState(t, result, "srd001-core", "R1.1", "complete", 10)
		// R1.2 should now be complete with issue 20.
		assertReqState(t, result, "srd001-core", "R1.2", "complete", 20)
	})
}

func TestUpdateRequirementsFile_TestsFailed(t *testing.T) {
	t.Run("marks as complete_with_failures when tests fail", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "ready"},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1.1 — implement config"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 50, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "srd001-core", "R1.1", "complete_with_failures", 50)
		// R1.2 should remain ready.
		assertReqState(t, result, "srd001-core", "R1.2", "ready", 0)
	})
}

func TestUCRequirementsComplete_CompleteWithFailures(t *testing.T) {
	t.Run("treats complete_with_failures as complete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "complete_with_failures", Issue: 50},
					"R1.2": {Status: "complete", Issue: 51},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Config struct per srd001-core R1",
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
				"srd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
					"R1.2": {Status: "complete", Issue: 11},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Config struct per srd001-core R1",
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
				"srd001-core": {
					"R1.1": {Status: "complete", Issue: 10},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Config struct per srd001-core R1",
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
		complete, remaining := UCRequirementsComplete("/nonexistent", []string{"T1: srd001 R1"})
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
	t.Run("single SRD single group", func(t *testing.T) {
		tps := []string{"T1: Config struct per srd001-core R1"}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 1 {
			t.Fatalf("expected 1 citation, got %d", len(citations))
		}
		if citations[0].srdID != "srd001-core" {
			t.Errorf("srdID = %q, want srd001-core", citations[0].srdID)
		}
		if len(citations[0].groups) != 1 || citations[0].groups[0] != "R1" {
			t.Errorf("groups = %v, want [R1]", citations[0].groups)
		}
	})

	t.Run("multiple groups", func(t *testing.T) {
		tps := []string{"T1: per srd002-lifecycle R1, R3"}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 1 {
			t.Fatalf("expected 1 citation, got %d", len(citations))
		}
		if len(citations[0].groups) != 2 {
			t.Errorf("groups = %v, want [R1, R3]", citations[0].groups)
		}
	})

	t.Run("multiple SRDs across touchpoints", func(t *testing.T) {
		tps := []string{
			"T1: per srd001-core R1",
			"T2: per srd002-lifecycle R2",
		}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 2 {
			t.Fatalf("expected 2 citations, got %d", len(citations))
		}
	})

	t.Run("no SRD references", func(t *testing.T) {
		tps := []string{"T1: some generic touchpoint"}
		citations := extractTouchpointCitations(tps)
		if len(citations) != 0 {
			t.Errorf("expected 0 citations, got %d", len(citations))
		}
	})
}

func TestFindSRDRequirements(t *testing.T) {
	reqs := map[string]map[string]RequirementState{
		"srd001-core":      {"R1.1": {Status: "ready"}},
		"srd010-ext":       {"R2.1": {Status: "ready"}},
		"srd053-logname":   {"R3.1": {Status: "ready"}},
		"srd053-sort":      {"R3.2": {Status: "ready"}},
	}

	t.Run("exact match", func(t *testing.T) {
		r := findSRDRequirements(reqs, "srd001-core")
		if r == nil || r["R1.1"].Status != "ready" {
			t.Errorf("expected exact match for srd001-core, got %v", r)
		}
	})

	t.Run("dash-prefix match", func(t *testing.T) {
		r := findSRDRequirements(reqs, "srd001")
		if r == nil || r["R1.1"].Status != "ready" {
			t.Errorf("expected srd001 to match srd001-core, got %v", r)
		}
	})

	t.Run("greedy prefix rejected", func(t *testing.T) {
		// "srd01" must NOT match "srd010-ext" — the numeric portions differ.
		r := findSRDRequirements(reqs, "srd01")
		if r != nil {
			t.Errorf("srd01 should not match srd010-ext, got %v", r)
		}
	})

	t.Run("no match returns nil", func(t *testing.T) {
		r := findSRDRequirements(reqs, "srd999")
		if r != nil {
			t.Errorf("expected nil for nonexistent stem, got %v", r)
		}
	})

	t.Run("ambiguous prefix picks longest key", func(t *testing.T) {
		// Both "srd053-logname" and "srd053-sort" match "srd053".
		// Longest key is "srd053-logname" (14 chars vs 10).
		r := findSRDRequirements(reqs, "srd053")
		if r == nil {
			t.Fatal("expected a match for srd053")
		}
		// Should pick srd053-logname (longest key).
		if _, ok := r["R3.1"]; !ok {
			t.Errorf("expected srd053 to match srd053-logname (longest), got %v", r)
		}
	})
}

// ---------------------------------------------------------------------------
// sdd-hello-world fixture: srd003-config with 12 R-items across 4 R-groups.
// Mirrors the structure of sdd-hello-world's rel04.0 release (GH-1394).
// ---------------------------------------------------------------------------

const srd003ConfigFixture = `requirements:
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

// srd001 fixture: 6 R-items across 2 R-groups (mirrors sdd-hello-world srd001).
const srd001CoreFixture = `requirements:
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

// srd002 fixture: 11 R-items across 3 R-groups (mirrors sdd-hello-world srd002).
const srd002LifecycleFixture = `requirements:
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
	srdDir := filepath.Join(tmp, "srds")
	cobblerDir := filepath.Join(tmp, ".cobbler")
	os.MkdirAll(srdDir, 0o755)

	os.WriteFile(filepath.Join(srdDir, "srd001-core.yaml"), []byte(srd001CoreFixture), 0o644)
	os.WriteFile(filepath.Join(srdDir, "srd002-lifecycle.yaml"), []byte(srd002LifecycleFixture), 0o644)
	os.WriteFile(filepath.Join(srdDir, "srd003-config.yaml"), []byte(srd003ConfigFixture), 0o644)

	path, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rf := readReqFile(t, path)

	// Verify all 3 SRDs are present.
	if len(rf.Requirements) != 3 {
		t.Fatalf("expected 3 SRDs, got %d", len(rf.Requirements))
	}

	// Verify item counts: srd001=6, srd002=11, srd003=12, total=29.
	wantCounts := map[string]int{
		"srd001-core":      6,
		"srd002-lifecycle": 11,
		"srd003-config":    12,
	}
	totalItems := 0
	for srd, wantCount := range wantCounts {
		got := len(rf.Requirements[srd])
		if got != wantCount {
			t.Errorf("%s: expected %d R-items, got %d", srd, wantCount, got)
		}
		totalItems += got
	}
	if totalItems != 29 {
		t.Errorf("expected 29 total R-items, got %d", totalItems)
	}

	// Verify all items start as "ready" with no issue.
	for srd, items := range rf.Requirements {
		for rItem, st := range items {
			if st.Status != "ready" {
				t.Errorf("%s %s: expected status ready, got %s", srd, rItem, st.Status)
			}
			if st.Issue != 0 {
				t.Errorf("%s %s: expected issue 0, got %d", srd, rItem, st.Issue)
			}
		}
	}

	// Verify specific srd003-config sub-requirements are extracted.
	for _, id := range []string{"R1.1", "R1.2", "R1.3", "R2.1", "R2.2", "R2.3",
		"R3.1", "R3.2", "R3.3", "R4.1", "R4.2", "R4.3"} {
		if _, ok := rf.Requirements["srd003-config"][id]; !ok {
			t.Errorf("srd003-config missing expected R-item %s", id)
		}
	}
}

func TestPartialCompletionSequence(t *testing.T) {
	// Simulates the srd003-config 3-task sequence from GH-1394:
	// Task 1: srd003-config R1 (3 items) → 3 complete, 9 ready
	// Task 2: srd003-config R2, R3 (6 items) → 9 complete, 3 ready
	// Task 3: srd003-config R4 (3 items) → 12 complete, 0 ready
	tmp := t.TempDir()
	srdDir := filepath.Join(tmp, "srds")
	cobblerDir := filepath.Join(tmp, ".cobbler")
	os.MkdirAll(srdDir, 0o755)

	os.WriteFile(filepath.Join(srdDir, "srd003-config.yaml"), []byte(srd003ConfigFixture), 0o644)

	// Generate initial requirements.yaml.
	reqPath, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
	if err != nil {
		t.Fatalf("GenerateRequirementsFile: %v", err)
	}

	// Verify initial state: 12 items, all ready.
	rf := readReqFile(t, reqPath)
	if len(rf.Requirements["srd003-config"]) != 12 {
		t.Fatalf("expected 12 R-items, got %d", len(rf.Requirements["srd003-config"]))
	}

	// UC touchpoints: uc004 cites R1,R2,R3; uc005 cites R4.
	uc004Touchpoints := []string{
		"T1: Config loading per srd003-config R1, R2, R3",
	}
	uc005Touchpoints := []string{
		"T1: Config hot-reload per srd003-config R4",
	}

	// --- Task 1: Mark R1 complete (3 items) ---
	desc1 := `requirements:
  - id: R1
    text: "srd003-config R1 — implement config file loading"
`
	if err := UpdateRequirementsFile(cobblerDir, desc1, 100, true, false); err != nil {
		t.Fatalf("Task 1 UpdateRequirementsFile: %v", err)
	}

	rf = readReqFile(t, reqPath)
	readyCount, completeCount := countStates(rf.Requirements["srd003-config"])
	if completeCount != 3 {
		t.Errorf("after Task 1: expected 3 complete, got %d", completeCount)
	}
	if readyCount != 9 {
		t.Errorf("after Task 1: expected 9 ready, got %d", readyCount)
	}
	assertReqState(t, rf, "srd003-config", "R1.1", "complete", 100)
	assertReqState(t, rf, "srd003-config", "R1.2", "complete", 100)
	assertReqState(t, rf, "srd003-config", "R1.3", "complete", 100)
	assertReqState(t, rf, "srd003-config", "R2.1", "ready", 0)

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
    text: "srd003-config R2 — implement config validation"
  - id: R2
    text: "srd003-config R3 — implement config defaults"
`
	if err := UpdateRequirementsFile(cobblerDir, desc2, 101, true, false); err != nil {
		t.Fatalf("Task 2 UpdateRequirementsFile: %v", err)
	}

	rf = readReqFile(t, reqPath)
	readyCount, completeCount = countStates(rf.Requirements["srd003-config"])
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
    text: "srd003-config R4 — implement config hot-reload"
`
	if err := UpdateRequirementsFile(cobblerDir, desc3, 102, true, false); err != nil {
		t.Fatalf("Task 3 UpdateRequirementsFile: %v", err)
	}

	rf = readReqFile(t, reqPath)
	readyCount, completeCount = countStates(rf.Requirements["srd003-config"])
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
	if err := UpdateRequirementsFile(cobblerDir, desc1, 999, true, false); err != nil {
		t.Fatalf("re-update should not error: %v", err)
	}
	rf = readReqFile(t, reqPath)
	assertReqState(t, rf, "srd003-config", "R1.1", "complete", 100) // original issue preserved
	assertReqState(t, rf, "srd003-config", "R1.2", "complete", 100)
	assertReqState(t, rf, "srd003-config", "R1.3", "complete", 100)
}

func TestCrossBatchDuplicatePrevention(t *testing.T) {
	// After marking R1-R3 complete, verify that ValidateMeasureOutput
	// rejects proposals targeting completed R-groups and accepts
	// proposals targeting ready R-groups (R4).
	reqStates := map[string]map[string]RequirementState{
		"srd003-config": {
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
	for _, st := range reqStates["srd003-config"] {
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
		desc := makeDesc("srd003-config R1 — re-implement loading")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "R1") && strings.Contains(e, "claimed") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected error for claimed group R1, got errors: %v", result.Errors)
		}
	})

	t.Run("rejects proposal targeting completed sub-item R2.1", func(t *testing.T) {
		desc := makeDesc("srd003-config R2.1 — re-validate fields")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "R2.1") && strings.Contains(e, "already") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected error for completed R2.1, got errors: %v", result.Errors)
		}
	})

	t.Run("accepts proposal targeting ready group R4", func(t *testing.T) {
		desc := makeDesc("srd003-config R4 — implement hot-reload")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
		for _, e := range result.Errors {
			if strings.Contains(e, "R4") && strings.Contains(e, "complete") {
				t.Errorf("R4 is ready, should not be rejected: %s", e)
			}
		}
	})

	t.Run("accepts proposal targeting ready sub-item R4.2", func(t *testing.T) {
		desc := makeDesc("srd003-config R4.2 — re-validate on change")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
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
	// Build requirement states: srd003-config has R1.1, R1.2 complete and R2.1 ready.
	states := map[string]map[string]RequirementState{
		"srd003-config": {
			"R1.1": {Status: "complete", Issue: 100},
			"R1.2": {Status: "complete", Issue: 100},
			"R2.1": {Status: "ready"},
			"R2.2": {Status: "ready"},
		},
		"srd001-core": {
			"R1.1": {Status: "complete", Issue: 101},
			"R1.2": {Status: "complete_with_failures", Issue: 102},
		},
	}

	makeDesc := func(reqText string) string {
		return "requirements:\n  - text: \"" + reqText + "\"\n    source: test\n"
	}

	t.Run("all refs complete returns true", func(t *testing.T) {
		desc := makeDesc("srd003-config R1.1")
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: R1.1 is complete")
		}
	})

	t.Run("partial complete returns false", func(t *testing.T) {
		desc := makeDesc("srd003-config R2.1")
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: R2.1 is ready")
		}
	})

	t.Run("no refs returns false", func(t *testing.T) {
		desc := "requirements:\n  - text: \"no srd refs here\"\n    source: test\n"
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: no SRD refs")
		}
	})

	t.Run("nil states returns false", func(t *testing.T) {
		desc := makeDesc("srd003-config R1.1")
		if AllRefsAlreadyComplete(desc, nil) {
			t.Error("expected false: nil states")
		}
	})

	t.Run("group reference all complete", func(t *testing.T) {
		desc := makeDesc("srd001-core R1")
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: all R1.x in srd001-core are complete")
		}
	})

	t.Run("group reference partial complete", func(t *testing.T) {
		desc := makeDesc("srd003-config R2")
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: R2 group has ready items")
		}
	})

	t.Run("complete_with_failures counts as complete", func(t *testing.T) {
		desc := makeDesc("srd001-core R1.2")
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: complete_with_failures is still complete")
		}
	})

	t.Run("unknown SRD returns false", func(t *testing.T) {
		desc := makeDesc("srd999-unknown R1.1")
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: SRD not in states")
		}
	})

	t.Run("multiple refs all complete", func(t *testing.T) {
		desc := "requirements:\n  - text: \"srd003-config R1.1\"\n    source: test\n  - text: \"srd001-core R1.2\"\n    source: test\n"
		if !AllRefsAlreadyComplete(desc, states) {
			t.Error("expected true: both refs are complete")
		}
	})

	t.Run("multiple refs one incomplete", func(t *testing.T) {
		desc := "requirements:\n  - text: \"srd003-config R1.1\"\n    source: test\n  - text: \"srd003-config R2.1\"\n    source: test\n"
		if AllRefsAlreadyComplete(desc, states) {
			t.Error("expected false: R2.1 is ready")
		}
	})
}

func TestUpdateRequirementsFile_InterveningWord(t *testing.T) {
	// Reproduces the bug from go-unix-utils run 27 (GH-1434): Claude writes
	// "srd002-sys requirement R2.5" instead of "srd002-sys R2.5". The regex
	// must handle intervening words between the SRD stem and R-number.
	dir := t.TempDir()
	reqPath := filepath.Join(dir, RequirementsFileName)
	rf := RequirementsFile{
		Requirements: map[string]map[string]RequirementState{
			"srd002-sys": {
				"R2.5": {Status: "ready"},
				"R2.6": {Status: "ready"},
			},
		},
	}
	data, err := yaml.Marshal(rf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reqPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	description := `deliverable_type: code
requirements:
  - id: R1
    text: "Implement srd002-sys requirement R2.5 as specified in the SRD"
  - id: R2
    text: "Implement srd002-sys requirement R2.6 as specified in the SRD"`

	if err := UpdateRequirementsFile(dir, description, 660, true, false); err != nil {
		t.Fatalf("UpdateRequirementsFile: %v", err)
	}

	updated := readReqFile(t, reqPath)
	assertReqState(t, updated, "srd002-sys", "R2.5", "complete", 660)
	assertReqState(t, updated, "srd002-sys", "R2.6", "complete", 660)
}

func TestCrossBatchDuplicatePrevention_InterveningWord(t *testing.T) {
	// Verifies that ValidateMeasureOutput rejects proposals using the
	// "srd002-sys requirement R2.5" format when R2.5 is already complete.
	reqStates := map[string]map[string]RequirementState{
		"srd002-sys": {
			"R2.5": {Status: "complete", Issue: 660},
			"R2.6": {Status: "complete", Issue: 660},
		},
	}

	desc := `deliverable_type: code
requirements:
  - id: R1
    text: "Implement srd002-sys requirement R2.5 exactly as specified in the SRD"
  - id: R2
    text: "Implement srd002-sys requirement R2.6 exactly as specified in the SRD"
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
  - path: pkg/sys/stat.go`

	issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
	result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
	found := 0
	for _, e := range result.Errors {
		if strings.Contains(e, "already") {
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected 2 'already claimed' errors for R2.5 and R2.6, got %d; errors: %v", found, result.Errors)
	}
}

// --- isRequirementComplete with skip status (GH-1451) ---

func TestIsRequirementComplete_SkipStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   bool
	}{
		{"complete", true},
		{"complete_with_failures", true},
		{"skip", true},
		{"ready", false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := isRequirementComplete(tc.status); got != tc.want {
				t.Errorf("isRequirementComplete(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestUCRequirementsComplete_SkipStatus(t *testing.T) {
	t.Run("skip counts as complete for UC validation", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd011-magefiles": {
					"R1.1": {Status: "skip"},
					"R2.1": {Status: "complete", Issue: 42},
					"R3.1": {Status: "skip"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: Magefiles per srd011-magefiles R1, R2, R3",
		}

		complete, remaining := UCRequirementsComplete(cobblerDir, touchpoints)
		if !complete {
			t.Errorf("expected complete (skip+complete), got remaining: %v", remaining)
		}
	})

	t.Run("skip plus ready is incomplete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd011-magefiles": {
					"R1.1": {Status: "skip"},
					"R2.1": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		touchpoints := []string{
			"T1: per srd011-magefiles R1, R2",
		}

		complete, remaining := UCRequirementsComplete(cobblerDir, touchpoints)
		if complete {
			t.Error("expected incomplete (R2.1 still ready)")
		}
		if len(remaining) != 1 {
			t.Errorf("expected 1 remaining, got %d: %v", len(remaining), remaining)
		}
	})
}

func TestCrossBatchDuplicatePrevention_SkipStatus(t *testing.T) {
	reqStates := map[string]map[string]RequirementState{
		"srd011-magefiles": {
			"R1.1": {Status: "skip"},
			"R2.1": {Status: "ready"},
		},
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
  - path: magefiles/magefile.go`
	}

	t.Run("rejects proposal targeting skipped R-item", func(t *testing.T) {
		desc := makeDesc("srd011-magefiles R1.1 — implement mage target")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "R1.1") && strings.Contains(e, "already") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected rejection for skipped R1.1, got errors: %v", result.Errors)
		}
	})

	t.Run("accepts proposal targeting ready R-item alongside skip", func(t *testing.T) {
		desc := makeDesc("srd011-magefiles R2.1 — implement something")
		issues := []ProposedIssue{{Index: 0, Title: "test", Description: desc}}
		result := ValidateMeasureOutput(issues, 0, 0, nil, reqStates)
		for _, e := range result.Errors {
			if strings.Contains(e, "R2.1") && strings.Contains(e, "claimed") {
				t.Errorf("R2.1 is ready, should not be rejected: %s", e)
			}
		}
	})
}

func TestGenerateRequirementsFile_PreservesSkipStatus(t *testing.T) {
	tmp := t.TempDir()
	srdDir := filepath.Join(tmp, "srds")
	cobblerDir := filepath.Join(tmp, ".cobbler")
	os.MkdirAll(srdDir, 0o755)
	os.MkdirAll(cobblerDir, 0o755)

	srd := `requirements:
  R1:
    title: "Mage targets"
    items:
      - R1.1: "build target"
      - R1.2: "test target"
`
	os.WriteFile(filepath.Join(srdDir, "srd011-magefiles.yaml"), []byte(srd), 0o644)

	existing := RequirementsFile{
		Requirements: map[string]map[string]RequirementState{
			"srd011-magefiles": {
				"R1.1": {Status: "skip"},
				"R1.2": {Status: "ready"},
			},
		},
	}
	data, _ := yaml.Marshal(existing)
	os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

	path, err := GenerateRequirementsFile(srdDir, cobblerDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := readReqFile(t, path)
	assertReqState(t, result, "srd011-magefiles", "R1.1", "skip", 0)
	assertReqState(t, result, "srd011-magefiles", "R1.2", "ready", 0)
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

func assertReqState(t *testing.T, rf RequirementsFile, srd, rItem, wantStatus string, wantIssue int) {
	t.Helper()
	srdReqs, ok := rf.Requirements[srd]
	if !ok {
		t.Errorf("SRD %s not found", srd)
		return
	}
	st, ok := srdReqs[rItem]
	if !ok {
		t.Errorf("%s %s not found", srd, rItem)
		return
	}
	if st.Status != wantStatus {
		t.Errorf("%s %s: status = %q, want %q", srd, rItem, st.Status, wantStatus)
	}
	if st.Issue != wantIssue {
		t.Errorf("%s %s: issue = %d, want %d", srd, rItem, st.Issue, wantIssue)
	}
}

// ---------------------------------------------------------------------------
// Requirement weights (GH-2080: weights live in requirements.yaml, not SRDs)
// ---------------------------------------------------------------------------

func TestExtractRItemsFromSRD_ExtractsIDsOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	srdPath := filepath.Join(dir, "srd001-test.yaml")
	srd := `id: srd001
title: Test
problem: test
goals:
  - G1: goal
requirements:
  R1:
    title: Basic
    items:
      - R1.1: Simple requirement
      - R1.2:
          text: Complex with SRD weight field
          weight: 5
non_goals: []
acceptance_criteria: []
`
	os.WriteFile(srdPath, []byte(srd), 0o644)

	items := extractRItemsFromSRD(srdPath)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids["R1.1"] || !ids["R1.2"] {
		t.Errorf("expected R1.1 and R1.2, got %v", ids)
	}
}

func TestGenerateRequirementsFile_NewItemsDefaultWeight1(t *testing.T) {
	dir := t.TempDir()
	srdDir := filepath.Join(dir, "docs", "specs", "software-requirements")
	os.MkdirAll(srdDir, 0o755)
	cobblerDir := filepath.Join(dir, ".cobbler")

	// SRD has weight annotations but they should be ignored.
	srd := `id: srd001
title: Test
problem: test
goals:
  - G1: goal
requirements:
  R1:
    title: Mixed
    items:
      - R1.1: Simple
      - R1.2:
          text: Has SRD weight but should be ignored
          weight: 5
non_goals: []
acceptance_criteria: []
`
	os.WriteFile(filepath.Join(srdDir, "srd001-test.yaml"), []byte(srd), 0o644)

	_, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
	if err != nil {
		t.Fatalf("GenerateRequirementsFile: %v", err)
	}

	states := LoadRequirementStates(cobblerDir)
	if states == nil {
		t.Fatal("LoadRequirementStates returned nil")
	}

	srdStates := states["srd001-test"]
	if srdStates == nil {
		t.Fatal("no states for srd001-test")
	}

	// Both items should default to weight 1 regardless of SRD weight field.
	if w := srdStates["R1.1"].Weight; w != 1 {
		t.Errorf("R1.1 weight = %d, want 1", w)
	}
	if w := srdStates["R1.2"].Weight; w != 1 {
		t.Errorf("R1.2 weight = %d, want 1 (SRD weight should be ignored)", w)
	}
}

func TestGenerateRequirementsFile_PreservesExistingWeight(t *testing.T) {
	dir := t.TempDir()
	srdDir := filepath.Join(dir, "docs", "specs", "software-requirements")
	os.MkdirAll(srdDir, 0o755)
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)

	srd := `id: srd001
title: Test
problem: test
goals:
  - G1: goal
requirements:
  R1:
    title: Test
    items:
      - R1.1: Item one
      - R1.2: Item two
non_goals: []
acceptance_criteria: []
`
	os.WriteFile(filepath.Join(srdDir, "srd001-test.yaml"), []byte(srd), 0o644)

	// Pre-populate requirements.yaml with custom weights.
	existing := `requirements:
    srd001-test:
        R1.1:
            status: ready
            weight: 4
        R1.2:
            status: complete
            issue: 42
            weight: 7
`
	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(existing), 0o644)

	// Regenerate with preserveExisting=true.
	_, err := GenerateRequirementsFile(srdDir, cobblerDir, true)
	if err != nil {
		t.Fatalf("GenerateRequirementsFile: %v", err)
	}

	states := LoadRequirementStates(cobblerDir)
	srdStates := states["srd001-test"]

	// Weights from existing requirements.yaml should be preserved.
	if w := srdStates["R1.1"].Weight; w != 4 {
		t.Errorf("R1.1 weight = %d, want 4 (preserved)", w)
	}
	if w := srdStates["R1.2"].Weight; w != 7 {
		t.Errorf("R1.2 weight = %d, want 7 (preserved)", w)
	}
	// Status should also be preserved.
	if s := srdStates["R1.2"].Status; s != "complete" {
		t.Errorf("R1.2 status = %q, want complete", s)
	}
}

func TestGenerateRequirementsFile_PreserveFalseRetainsWeights(t *testing.T) {
	dir := t.TempDir()
	srdDir := filepath.Join(dir, "docs", "specs", "software-requirements")
	os.MkdirAll(srdDir, 0o755)
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)

	srd := `id: srd001
title: Test
problem: test
goals:
  - G1: goal
requirements:
  R1:
    title: Test
    items:
      - R1.1: Item one
      - R1.2: Item two
non_goals: []
acceptance_criteria: []
`
	os.WriteFile(filepath.Join(srdDir, "srd001-test.yaml"), []byte(srd), 0o644)

	// Pre-populate requirements.yaml with custom weights and statuses.
	existing := `requirements:
    srd001-test:
        R1.1:
            status: complete
            issue: 42
            weight: 4
        R1.2:
            status: complete
            issue: 43
            weight: 7
`
	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(existing), 0o644)

	// Regenerate with preserveExisting=false (full reset).
	_, err := GenerateRequirementsFile(srdDir, cobblerDir, false)
	if err != nil {
		t.Fatalf("GenerateRequirementsFile: %v", err)
	}

	states := LoadRequirementStates(cobblerDir)
	srdStates := states["srd001-test"]

	// Weights must be preserved even with preserveExisting=false (GH-2117).
	if w := srdStates["R1.1"].Weight; w != 4 {
		t.Errorf("R1.1 weight = %d, want 4 (preserved despite preserveExisting=false)", w)
	}
	if w := srdStates["R1.2"].Weight; w != 7 {
		t.Errorf("R1.2 weight = %d, want 7 (preserved despite preserveExisting=false)", w)
	}
	// Statuses should be reset to "ready" since preserveExisting=false.
	if s := srdStates["R1.1"].Status; s != "ready" {
		t.Errorf("R1.1 status = %q, want ready (reset)", s)
	}
	if s := srdStates["R1.2"].Status; s != "ready" {
		t.Errorf("R1.2 status = %q, want ready (reset)", s)
	}
}

// --- GH-2123: Requirement state machine tests ---

func TestMarkRequirementsProposed(t *testing.T) {
	t.Run("marks ready requirements as proposed", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "ready", Weight: 3},
					"R1.2": {Status: "ready", Weight: 1},
					"R2.1": {Status: "complete", Issue: 10, Weight: 2},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1.1 — implement config"
`
		err := MarkRequirementsProposed(cobblerDir, desc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "srd001-core", "R1.1", "proposed", 0)
		// Weight must be preserved.
		if w := result.Requirements["srd001-core"]["R1.1"].Weight; w != 3 {
			t.Errorf("R1.1 weight = %d, want 3", w)
		}
		// R1.2 and R2.1 should be unchanged.
		assertReqState(t, result, "srd001-core", "R1.2", "ready", 0)
		assertReqState(t, result, "srd001-core", "R2.1", "complete", 10)
	})

	t.Run("skips already proposed requirements", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "proposed"},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1 — implement entire group"
`
		err := MarkRequirementsProposed(cobblerDir, desc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		// R1.1 stays proposed (not re-proposed).
		assertReqState(t, result, "srd001-core", "R1.1", "proposed", 0)
		// R1.2 transitions from ready to proposed.
		assertReqState(t, result, "srd001-core", "R1.2", "proposed", 0)
	})
}

func TestMarkRequirementsInProgress(t *testing.T) {
	t.Run("transitions proposed to in_progress", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "proposed", Weight: 5},
					"R1.2": {Status: "ready", Weight: 1},
					"R2.1": {Status: "complete", Issue: 10},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1 — implement group"
`
		err := MarkRequirementsInProgress(cobblerDir, desc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		// Both ready and proposed transition to in_progress.
		assertReqState(t, result, "srd001-core", "R1.1", "in_progress", 0)
		assertReqState(t, result, "srd001-core", "R1.2", "in_progress", 0)
		// Weight must be preserved.
		if w := result.Requirements["srd001-core"]["R1.1"].Weight; w != 5 {
			t.Errorf("R1.1 weight = %d, want 5", w)
		}
		// R2.1 stays complete.
		assertReqState(t, result, "srd001-core", "R2.1", "complete", 10)
	})
}

func TestUpdateRequirementsFile_ZeroLOC(t *testing.T) {
	t.Run("marks as uncertain when zeroLOC is true", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "in_progress", Weight: 2},
					"R1.2": {Status: "ready"},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1.1 — implement config"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 77, true, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "srd001-core", "R1.1", "uncertain", 77)
		// Weight must be preserved.
		if w := result.Requirements["srd001-core"]["R1.1"].Weight; w != 2 {
			t.Errorf("R1.1 weight = %d, want 2", w)
		}
		// R1.2 should remain ready (not referenced in desc).
		assertReqState(t, result, "srd001-core", "R1.2", "ready", 0)
	})

	t.Run("uncertain requirements do not block new transitions", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		// Simulate: R1.1 was marked uncertain, now another task completes it.
		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "uncertain", Issue: 77},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1.1 — implement config"
`
		// uncertain is not transitionable, so this should be a no-op.
		err := UpdateRequirementsFile(cobblerDir, desc, 88, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		// uncertain is terminal — should not be overwritten.
		assertReqState(t, result, "srd001-core", "R1.1", "uncertain", 77)
	})
}

func TestUpdateRequirementsFile_TransitionsFromProposedAndInProgress(t *testing.T) {
	t.Run("transitions proposed and in_progress to complete", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd001-core": {
					"R1.1": {Status: "proposed", Weight: 3},
					"R1.2": {Status: "in_progress", Weight: 1},
					"R1.3": {Status: "complete", Issue: 5},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		desc := `requirements:
  - id: R1
    text: "srd001 R1 — implement group"
`
		err := UpdateRequirementsFile(cobblerDir, desc, 60, true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		result := readReqFile(t, filepath.Join(cobblerDir, RequirementsFileName))
		assertReqState(t, result, "srd001-core", "R1.1", "complete", 60)
		assertReqState(t, result, "srd001-core", "R1.2", "complete", 60)
		// R1.3 was already complete — should not be overwritten.
		assertReqState(t, result, "srd001-core", "R1.3", "complete", 5)
		// Weights must be preserved.
		if w := result.Requirements["srd001-core"]["R1.1"].Weight; w != 3 {
			t.Errorf("R1.1 weight = %d, want 3", w)
		}
	})
}

func TestIsRequirementTerminal(t *testing.T) {
	terminal := []string{"complete", "complete_with_failures", "failed", "uncertain", "skip"}
	for _, s := range terminal {
		if !IsRequirementTerminal(s) {
			t.Errorf("IsRequirementTerminal(%q) = false, want true", s)
		}
	}
	nonTerminal := []string{"ready", "proposed", "in_progress"}
	for _, s := range nonTerminal {
		if IsRequirementTerminal(s) {
			t.Errorf("IsRequirementTerminal(%q) = true, want false", s)
		}
	}
}

// --- GH-2078: ValidateTaskWeights tests ---

func TestValidateTaskWeights(t *testing.T) {
	t.Run("reports weights and PASS", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd005-wc": {
					"R2.5": {Status: "ready", Weight: 4},
					"R2.6": {Status: "ready", Weight: 1},
					"R3.1": {Status: "ready", Weight: 1},
					"R3.2": {Status: "ready", Weight: 1},
					"R3.3": {Status: "ready", Weight: 1},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		result := ValidateTaskWeights(cobblerDir, "srd005-wc R2.6, R3.1, R3.2, R3.3", 4)
		if !strings.Contains(result, "PASS") {
			t.Errorf("expected PASS, got:\n%s", result)
		}
		if !strings.Contains(result, "total: 4") {
			t.Errorf("expected total: 4, got:\n%s", result)
		}
		if !strings.Contains(result, "max: 4") {
			t.Errorf("expected max: 4, got:\n%s", result)
		}
	})

	t.Run("reports weights and FAIL when exceeding max", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd005-wc": {
					"R2.5": {Status: "ready", Weight: 4},
					"R2.6": {Status: "ready", Weight: 1},
					"R3.1": {Status: "ready", Weight: 1},
					"R3.2": {Status: "ready", Weight: 1},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		result := ValidateTaskWeights(cobblerDir, "srd005-wc R2.5, R2.6, R3.1, R3.2", 4)
		if !strings.Contains(result, "FAIL") {
			t.Errorf("expected FAIL, got:\n%s", result)
		}
		if !strings.Contains(result, "total: 7") {
			t.Errorf("expected total: 7, got:\n%s", result)
		}
		if !strings.Contains(result, "weight 4") {
			t.Errorf("expected R2.5 weight 4, got:\n%s", result)
		}
	})

	t.Run("defaults to weight 1 for missing requirements", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd005-wc": {
					"R1.1": {Status: "ready", Weight: 3},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		result := ValidateTaskWeights(cobblerDir, "srd005-wc R1.1, R9.9", 10)
		if !strings.Contains(result, "PASS") {
			t.Errorf("expected PASS, got:\n%s", result)
		}
		if !strings.Contains(result, "total: 4") {
			t.Errorf("expected total: 4 (3+1 default), got:\n%s", result)
		}
		if !strings.Contains(result, "not found") {
			t.Errorf("expected 'not found' annotation for R9.9, got:\n%s", result)
		}
	})

	t.Run("handles missing requirements.yaml gracefully", func(t *testing.T) {
		tmp := t.TempDir()
		result := ValidateTaskWeights(tmp, "srd005-wc R1.1, R1.2", 4)
		if !strings.Contains(result, "PASS") {
			t.Errorf("expected PASS (defaults to weight 1 each), got:\n%s", result)
		}
		if !strings.Contains(result, "total: 2") {
			t.Errorf("expected total: 2, got:\n%s", result)
		}
	})

	t.Run("handles group references", func(t *testing.T) {
		tmp := t.TempDir()
		cobblerDir := filepath.Join(tmp, ".cobbler")
		os.MkdirAll(cobblerDir, 0o755)

		initial := RequirementsFile{
			Requirements: map[string]map[string]RequirementState{
				"srd005-wc": {
					"R2.1": {Status: "ready", Weight: 2},
					"R2.2": {Status: "ready", Weight: 3},
					"R3.1": {Status: "ready", Weight: 1},
				},
			},
		}
		data, _ := yaml.Marshal(initial)
		os.WriteFile(filepath.Join(cobblerDir, RequirementsFileName), data, 0o644)

		result := ValidateTaskWeights(cobblerDir, "srd005-wc R2, R3.1", 10)
		if !strings.Contains(result, "PASS") {
			t.Errorf("expected PASS, got:\n%s", result)
		}
		// R2 group = 2+3 = 5, R3.1 = 1, total = 6
		if !strings.Contains(result, "total: 6") {
			t.Errorf("expected total: 6, got:\n%s", result)
		}
		if !strings.Contains(result, "group") {
			t.Errorf("expected 'group' annotation for R2, got:\n%s", result)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		result := ValidateTaskWeights(".", "bad-input", 4)
		if !strings.Contains(result, "FAIL") {
			t.Errorf("expected FAIL for bad input, got:\n%s", result)
		}
	})

	t.Run("PASS when maxWeight is 0 (unlimited)", func(t *testing.T) {
		tmp := t.TempDir()
		result := ValidateTaskWeights(tmp, "srd005-wc R1.1, R1.2", 0)
		if !strings.Contains(result, "PASS") {
			t.Errorf("expected PASS when maxWeight=0, got:\n%s", result)
		}
	})
}
