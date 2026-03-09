// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"os"
	"path/filepath"
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

		path, err := GenerateRequirementsFile(prdDir, cobblerDir)
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

		path, err := GenerateRequirementsFile(prdDir, cobblerDir)
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

		path, err := GenerateRequirementsFile(prdDir, cobblerDir)
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

		path, err := GenerateRequirementsFile(prdDir, cobblerDir)
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
		err := UpdateRequirementsFile(cobblerDir, desc, 42)
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
		err := UpdateRequirementsFile(cobblerDir, desc, 99)
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
		err := UpdateRequirementsFile(tmp, "requirements:\n  - id: R1\n    text: prd001 R1.1", 1)
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
		err := UpdateRequirementsFile(cobblerDir, desc, 10)
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
		err := UpdateRequirementsFile(cobblerDir, desc, 20)
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
