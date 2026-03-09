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
