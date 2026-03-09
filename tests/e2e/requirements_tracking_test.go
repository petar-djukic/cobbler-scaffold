//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package e2e_test validates end-to-end requirement tracking through the
// orchestrator's GeneratorStart pipeline (GH-1394).
package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"gopkg.in/yaml.v3"
)

// reqFile mirrors generate.RequirementsFile for YAML parsing without
// importing the internal package.
type reqFile struct {
	Requirements map[string]map[string]struct {
		Status string `yaml:"status"`
		Issue  int    `yaml:"issue,omitempty"`
	} `yaml:"requirements"`
}

// TestRequirementTracking_GeneratorStartProducesRequirements exercises the
// full GeneratorStart pipeline with a minimal repo containing 3 PRDs (29
// R-items). Validates that .cobbler/requirements.yaml is generated with
// all items in "ready" status.
func TestRequirementTracking_GeneratorStartProducesRequirements(t *testing.T) {
	t.Parallel()
	dir := setupReqTrackingRepo(t)

	cfg := orchestrator.Config{
		Project: orchestrator.ProjectConfig{
			ModulePath:   "example.com/reqtest",
			BinaryName:   "reqtest",
			GoSourceDirs: []string{"pkg/"},
		},
		Cobbler: orchestrator.CobblerConfig{
			Dir: ".cobbler/",
		},
		Generation: orchestrator.GenerationConfig{
			Prefix:          "generation-",
			Name:            "test-req",
			PreserveSources: true,
		},
		Claude: orchestrator.ClaudeConfig{
			SecretsDir: "/dev/null/impossible",
		},
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	o := orchestrator.New(cfg)
	if err := o.GeneratorStart(); err != nil {
		t.Fatalf("GeneratorStart: %v", err)
	}

	// Read and verify the generated requirements.yaml.
	reqPath := filepath.Join(dir, ".cobbler", "requirements.yaml")
	data, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("reading requirements.yaml: %v", err)
	}

	var rf reqFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		t.Fatalf("parsing requirements.yaml: %v", err)
	}

	// Verify all 3 PRDs are present.
	if len(rf.Requirements) != 3 {
		t.Fatalf("expected 3 PRDs, got %d: %v", len(rf.Requirements), keys(rf.Requirements))
	}

	// Verify item counts: prd001=6, prd002=11, prd003=12, total=29.
	wantCounts := map[string]int{
		"prd001-core":      6,
		"prd002-lifecycle": 11,
		"prd003-config":    12,
	}
	totalItems := 0
	for prd, wantCount := range wantCounts {
		items, ok := rf.Requirements[prd]
		if !ok {
			t.Errorf("missing PRD %s in requirements.yaml", prd)
			continue
		}
		if len(items) != wantCount {
			t.Errorf("%s: expected %d R-items, got %d", prd, wantCount, len(items))
		}
		totalItems += len(items)
	}
	if totalItems != 29 {
		t.Errorf("expected 29 total R-items, got %d", totalItems)
	}

	// Verify all items have status "ready".
	for prd, items := range rf.Requirements {
		for rItem, st := range items {
			if st.Status != "ready" {
				t.Errorf("%s %s: expected status ready, got %s", prd, rItem, st.Status)
			}
		}
	}

	t.Logf("requirements.yaml: %d PRDs, %d total R-items, all ready", len(rf.Requirements), totalItems)
}

// --- helpers ---

func keys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func setupReqTrackingRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Go module and minimal source.
	writeFile(t, dir, "go.mod", "module example.com/reqtest\n\ngo 1.23\n")
	os.MkdirAll(filepath.Join(dir, "pkg", "core"), 0o755)
	writeFile(t, dir, "pkg/core/core.go",
		"package core\n\nfunc Hello() string { return \"hello\" }\n")

	// Minimal docs.
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	writeFile(t, dir, "docs/VISION.yaml", "id: v1\ntitle: Test Vision\n")

	// PRD fixtures matching sdd-hello-world structure.
	prdDir := filepath.Join(dir, "docs", "specs", "product-requirements")
	os.MkdirAll(prdDir, 0o755)

	writeFile(t, dir, "docs/specs/product-requirements/prd001-core.yaml",
		prd001Fixture)
	writeFile(t, dir, "docs/specs/product-requirements/prd002-lifecycle.yaml",
		prd002Fixture)
	writeFile(t, dir, "docs/specs/product-requirements/prd003-config.yaml",
		prd003Fixture)

	// Configuration file.
	writeFile(t, dir, "configuration.yaml", `project:
  module_path: example.com/reqtest
  binary_name: reqtest
  go_source_dirs:
    - pkg/
cobbler:
  dir: .cobbler/
generation:
  prefix: "generation-"
  preserve_sources: true
`)

	// Git init.
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.local"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "config", "tag.gpgsign", "false"},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args[1:], err, out)
		}
	}

	return dir
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", rel, err)
	}
}

// PRD fixtures mimicking sdd-hello-world's spec structure (GH-1394).
// prd001: 6 R-items, prd002: 11 R-items, prd003: 12 R-items = 29 total.

const prd001Fixture = `requirements:
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

const prd002Fixture = `requirements:
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

const prd003Fixture = `requirements:
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

