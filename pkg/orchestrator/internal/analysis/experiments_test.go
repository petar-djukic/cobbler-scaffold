// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

const defaultExpRegistry = `  manifest_path: docs/experiments/manifest.yaml
  gate_fields:
    - question
    - claims
    - apparatus
    - interpret_pass
    - interpret_fail
  memo_dir: docs/experiments/decisions
`

// writeExperimentsProject builds a temp project with an experiments constitution
// carrying defaultExpRegistry, the given manifest body, and extra files, then
// chdirs into it.
func writeExperimentsProject(t *testing.T, manifest string, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	consts := filepath.Join(dir, "docs", "constitutions")
	if err := os.MkdirAll(consts, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "articles:\n  - id: E2\n    title: Gate before numbers\n    rule: x\nproject_registry:\n" + defaultExpRegistry
	if err := os.WriteFile(filepath.Join(consts, "experiments.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	all := map[string]string{}
	for k, v := range files {
		all[k] = v
	}
	if manifest != "" {
		all["docs/experiments/manifest.yaml"] = manifest
	}
	for name, content := range all {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestRunExperimentChecks_NoConstitutionIsNoop(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	got := RunExperimentChecks(noopLog)
	if len(got.GateViolations)+len(got.MissingMemos)+len(got.ManifestErrors) != 0 {
		t.Errorf("expected no findings without experiments.yaml, got %+v", got)
	}
}

func TestRunExperimentChecks_NoManifestIsNoop(t *testing.T) {
	writeExperimentsProject(t, "", nil)
	got := RunExperimentChecks(noopLog)
	if len(got.GateViolations)+len(got.MissingMemos)+len(got.ManifestErrors) != 0 {
		t.Errorf("expected no findings without a manifest, got %+v", got)
	}
}

func TestExperimentGates_E2(t *testing.T) {
	complete := `experiments:
  - id: exp001
    question: does it work
    claims: yes
    apparatus: rig
    interpret_pass: passes
    interpret_fail: fails
`
	missing := `experiments:
  - id: exp001
    question: does it work
    claims: yes
`
	t.Run("complete gate passes", func(t *testing.T) {
		writeExperimentsProject(t, complete, nil)
		if got := RunExperimentChecks(noopLog).GateViolations; len(got) != 0 {
			t.Errorf("GateViolations = %v, want 0", got)
		}
	})
	t.Run("missing gate fields flagged", func(t *testing.T) {
		writeExperimentsProject(t, missing, nil)
		// apparatus, interpret_pass, interpret_fail absent -> 3 violations
		if got := RunExperimentChecks(noopLog).GateViolations; len(got) != 3 {
			t.Errorf("GateViolations = %v, want 3", got)
		}
	})
}

func TestExperimentMemos_E5(t *testing.T) {
	failed := `experiments:
  - id: exp007
    question: q
    claims: c
    apparatus: a
    interpret_pass: p
    interpret_fail: f
    status: failed
`
	t.Run("failed without memo flagged", func(t *testing.T) {
		writeExperimentsProject(t, failed, nil)
		if got := RunExperimentChecks(noopLog).MissingMemos; len(got) != 1 {
			t.Errorf("MissingMemos = %v, want 1", got)
		}
	})
	t.Run("failed with memo in memo_dir passes", func(t *testing.T) {
		writeExperimentsProject(t, failed, map[string]string{
			"docs/experiments/decisions/exp007-memo.md": "why it failed\n",
		})
		if got := RunExperimentChecks(noopLog).MissingMemos; len(got) != 0 {
			t.Errorf("MissingMemos = %v, want 0", got)
		}
	})
}

func TestExperimentManifest_E6(t *testing.T) {
	manifest := `experiments:
  - id: exp001
    question: q
    claims: c
    apparatus: a
    interpret_pass: p
    interpret_fail: f
`
	t.Run("resolved reference passes", func(t *testing.T) {
		writeExperimentsProject(t, manifest, map[string]string{
			"docs/paper.md": "See exp:exp001 for details.\n",
		})
		if got := RunExperimentChecks(noopLog).ManifestErrors; len(got) != 0 {
			t.Errorf("ManifestErrors = %v, want 0", got)
		}
	})
	t.Run("unresolved reference flagged", func(t *testing.T) {
		writeExperimentsProject(t, manifest, map[string]string{
			"docs/paper.md": "See exp:exp999 which does not exist.\n",
		})
		if got := RunExperimentChecks(noopLog).ManifestErrors; len(got) != 1 {
			t.Errorf("ManifestErrors = %v, want 1", got)
		}
	})
	t.Run("duplicate id flagged", func(t *testing.T) {
		dup := manifest + `  - id: exp001
    question: q
    claims: c
    apparatus: a
    interpret_pass: p
    interpret_fail: f
`
		writeExperimentsProject(t, dup, nil)
		if got := RunExperimentChecks(noopLog).ManifestErrors; len(got) != 1 {
			t.Errorf("ManifestErrors = %v, want 1 (duplicate id)", got)
		}
	})
}

func TestIsEmptyValue(t *testing.T) {
	cases := []struct {
		v    any
		want bool
	}{
		{nil, true},
		{"", true},
		{"  ", true},
		{"x", false},
		{[]any{}, true},
		{[]any{1}, false},
		{map[string]any{}, true},
		{42, false},
	}
	for _, tc := range cases {
		if got := isEmptyValue(tc.v); got != tc.want {
			t.Errorf("isEmptyValue(%v) = %v, want %v", tc.v, got, tc.want)
		}
	}
}
