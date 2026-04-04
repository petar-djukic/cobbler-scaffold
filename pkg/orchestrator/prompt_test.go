// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
	"gopkg.in/yaml.v3"
)

// --- parsePromptTemplate ---

func TestParsePromptTemplate_MeasureFields(t *testing.T) {
	tmpl, err := ictx.ParsePromptTemplate(defaultMeasurePrompt)
	if err != nil {
		t.Fatalf("parsePromptTemplate: %v", err)
	}
	if tmpl.Role == "" {
		t.Error("role field is empty")
	}
	if tmpl.Task == "" {
		t.Error("task field is empty")
	}
	if tmpl.Constraints == "" {
		t.Error("constraints field is empty")
	}
	if tmpl.OutputFormat == "" {
		t.Error("output_format field is empty")
	}
}

func TestParsePromptTemplate_StitchFields(t *testing.T) {
	tmpl, err := ictx.ParsePromptTemplate(defaultStitchPrompt)
	if err != nil {
		t.Fatalf("parsePromptTemplate: %v", err)
	}
	if tmpl.Role == "" {
		t.Error("role field is empty")
	}
	if tmpl.Task == "" {
		t.Error("task field is empty")
	}
	if tmpl.Constraints == "" {
		t.Error("constraints field is empty")
	}
}

func TestParsePromptTemplate_InvalidYAML(t *testing.T) {
	_, err := ictx.ParsePromptTemplate("not: [valid: yaml")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// --- substitutePlaceholders ---

func TestSubstitutePlaceholders(t *testing.T) {
	text := "Output to {output_path}, max {limit} tasks of {lines_min}-{lines_max} lines."
	data := map[string]string{
		"output_path": "/tmp/out.yaml",
		"limit":       "5",
		"lines_min":   "250",
		"lines_max":   "350",
	}
	got := ictx.SubstitutePlaceholders(text, data)
	want := "Output to /tmp/out.yaml, max 5 tasks of 250-350 lines."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- validatePromptTemplate ---

func TestValidatePromptTemplate_MissingFile(t *testing.T) {
	errs := ictx.ValidatePromptTemplate("/nonexistent/path/prompt.yaml")
	if errs != nil {
		t.Errorf("expected nil for missing file, got %v", errs)
	}
}

func TestValidatePromptTemplate_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/prompt.yaml"
	content := "role: assistant\ntask: do something\nconstraints: be good\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	errs := ictx.ValidatePromptTemplate(path)
	if errs != nil {
		t.Errorf("expected nil for valid file, got %v", errs)
	}
}

func TestValidatePromptTemplate_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/prompt.yaml"
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	errs := ictx.ValidatePromptTemplate(path)
	if len(errs) == 0 {
		t.Error("expected errors for invalid YAML, got none")
	}
}

// --- parseYAMLNode ---

func TestParseYAMLNode_ValidYAML(t *testing.T) {
	node := ictx.ParseYAMLNode("articles:\n  - id: P1\n    title: Test")
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	if node.Kind != yaml.MappingNode {
		t.Errorf("expected MappingNode, got %v", node.Kind)
	}
}

func TestParseYAMLNode_Empty(t *testing.T) {
	node := ictx.ParseYAMLNode("")
	if node != nil {
		t.Error("expected nil for empty input")
	}
}

func TestParseYAMLNode_Invalid(t *testing.T) {
	node := ictx.ParseYAMLNode("not: [valid: yaml")
	if node != nil {
		t.Error("expected nil for invalid YAML")
	}
}

// --- Integration tests for prompt builders ---

func TestMeasurePromptIsValidYAML(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 5)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	var doc MeasurePromptDoc
	if err := yaml.Unmarshal([]byte(prompt), &doc); err != nil {
		t.Fatalf("prompt is not valid YAML: %v", err)
	}
}

func TestMeasurePromptIncludesPlanningConstitution(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 5)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if !strings.Contains(prompt, "planning_constitution:") {
		t.Error("measure prompt missing planning_constitution YAML key")
	}
	if !strings.Contains(prompt, "Release-driven priority") {
		t.Error("measure prompt missing planning constitution content (article P1)")
	}
}

func TestMeasurePromptIncludesIssueFormatConstitution(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 5)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if !strings.Contains(prompt, "issue_format_constitution:") {
		t.Error("measure prompt missing issue_format_constitution YAML key")
	}
	if !strings.Contains(prompt, "deliverable_type") {
		t.Error("measure prompt missing issue format constitution content")
	}
}

func TestMeasurePromptIncludesProjectContext(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 5)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if !strings.Contains(prompt, "project_context:") {
		t.Error("measure prompt missing project_context YAML key")
	}
	if !strings.Contains(prompt, "task:") {
		t.Error("measure prompt missing task YAML key")
	}
}

func TestMeasurePromptSubstitutesPlaceholders(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 3)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if strings.Contains(prompt, "{limit}") {
		t.Error("measure prompt has unsubstituted {limit} placeholder")
	}
	if strings.Contains(prompt, "{lines_min}") {
		t.Error("measure prompt has unsubstituted {lines_min} placeholder")
	}
	if strings.Contains(prompt, "{max_requirements}") {
		t.Error("measure prompt has unsubstituted {max_requirements} placeholder")
	}
	if strings.Contains(prompt, "{output_path}") {
		t.Error("measure prompt still references removed {output_path} placeholder")
	}
}

func TestBuildMeasurePrompt_MaxRequirementsPlaceholder(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	cfg.Cobbler.MaxRequirementsPerTask = 7
	o := New(cfg)
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}
	if !strings.Contains(prompt, "7") {
		t.Error("measure prompt does not contain the configured max_requirements value (7)")
	}
}

func TestMeasurePromptNoWriteToolReferences(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if strings.Contains(prompt, "Write tool") {
		t.Error("measure prompt still references 'Write tool'")
	}
	if strings.Contains(prompt, "output_path") {
		t.Error("measure prompt still references 'output_path'")
	}
	if !strings.Contains(prompt, "Do NOT use any tools") {
		t.Error("measure prompt missing single-turn constraint (no tool calls)")
	}
}

func TestMeasurePromptClosedIssueConstraint(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if !strings.Contains(prompt, "COMPLETED work") {
		t.Error("measure prompt missing closed-issue deduplication constraint")
	}
	if !strings.Contains(prompt, "completed_work") {
		t.Error("measure prompt missing completed_work field reference")
	}
}

func TestMeasurePromptSourceCodeOverProseConstraint(t *testing.T) {
	o := New(Config{})
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if !strings.Contains(prompt, "Trust the source code over prose") {
		t.Error("measure prompt missing source-code-over-prose constraint")
	}
}

func TestStitchPromptIsValidYAML(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		ID:          "test-001",
		Title:       "Test task",
		IssueType:   "task",
		Description: "A test description.",
		WorktreeDir: "/tmp",
	}

	prompt, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}

	var doc StitchPromptDoc
	if err := yaml.Unmarshal([]byte(prompt), &doc); err != nil {
		t.Fatalf("prompt is not valid YAML: %v", err)
	}
}

func TestStitchPromptIncludesExecutionConstitution(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		ID:          "test-001",
		Title:       "Test task",
		IssueType:   "task",
		Description: "A test description.",
		WorktreeDir: "/tmp",
	}

	prompt, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}

	if !strings.Contains(prompt, "execution_constitution:") {
		t.Error("stitch prompt missing execution_constitution YAML key")
	}
	if !strings.Contains(prompt, "Specification-first") {
		t.Error("stitch prompt missing execution constitution content (article E1)")
	}
}

func TestStitchPromptIncludesTaskContext(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		ID:          "task-123",
		Title:       "Implement feature X",
		IssueType:   "task",
		Description: "Detailed requirements here.",
		WorktreeDir: "/tmp",
	}

	prompt, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}

	if !strings.Contains(prompt, "task-123") {
		t.Error("stitch prompt missing task ID")
	}
	if !strings.Contains(prompt, "Implement feature X") {
		t.Error("stitch prompt missing task title")
	}
	if !strings.Contains(prompt, "Detailed requirements here.") {
		t.Error("stitch prompt missing task description")
	}
}

func TestStitchPromptIncludesGoStyleConstitution(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		ID:          "test-001",
		Title:       "Test task",
		IssueType:   "task",
		Description: "A test description.",
		WorktreeDir: "/tmp",
	}

	prompt, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}

	if !strings.Contains(prompt, "go_style_constitution:") {
		t.Error("stitch prompt missing go_style_constitution YAML key")
	}
}

// --- loadOODPromptContext ---

func TestLoadOODPromptContext_Empty(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)
	os.MkdirAll("docs/specs/software-requirements", 0o755)

	contracts, protocols := ictx.LoadOODPromptContext()
	if len(contracts) != 0 {
		t.Errorf("expected no contracts with no SRD files, got %d", len(contracts))
	}
	if len(protocols) != 0 {
		t.Errorf("expected no protocols with no ARCHITECTURE.yaml, got %d", len(protocols))
	}
}

func TestLoadOODPromptContext_PackageContracts(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	// One SRD with package_contract, one without.
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-pkg.yaml"), []byte(`id: srd001-pkg
title: Pkg
package_contract:
  exports:
    - name: FuncA
      signature: "func FuncA() error"
`), 0o644)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd002-cmd.yaml"), []byte(`id: srd002-cmd
title: Cmd
`), 0o644)

	contracts, _ := ictx.LoadOODPromptContext()
	if len(contracts) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(contracts))
	}
	if contracts[0].SRDID != "srd001-pkg" {
		t.Errorf("expected srd001-pkg, got %q", contracts[0].SRDID)
	}
	if len(contracts[0].Contract.Exports) != 1 || contracts[0].Contract.Exports[0].Name != "FuncA" {
		t.Errorf("expected FuncA export, got %v", contracts[0].Contract.Exports)
	}
}

func TestLoadOODPromptContext_SharedProtocols(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.MkdirAll("docs", 0o755)
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte(`id: arch
title: Architecture
overview:
  summary: s
  lifecycle: l
  coordination_pattern: c
shared_protocols:
  - name: SIGPIPE handling
    description: All commands must handle SIGPIPE
    pattern: "signal.Notify(...)"
`), 0o644)

	_, protocols := ictx.LoadOODPromptContext()
	if len(protocols) != 1 {
		t.Fatalf("expected 1 protocol, got %d", len(protocols))
	}
	if protocols[0].Name != "SIGPIPE handling" {
		t.Errorf("expected SIGPIPE handling, got %q", protocols[0].Name)
	}
}

func TestLoadOODPromptContext_EmptyContract_Skipped(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// SRD with package_contract but no exports should be skipped.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-empty.yaml"), []byte(`id: srd001-empty
title: Empty
package_contract:
  exports: []
`), 0o644)

	contracts, _ := ictx.LoadOODPromptContext()
	if len(contracts) != 0 {
		t.Errorf("expected no contracts for empty exports, got %d", len(contracts))
	}
}

// --- OOD injection in buildStitchPrompt ---

func TestBuildStitchPrompt_OODSharedProtocols(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// When ARCHITECTURE.yaml has shared_protocols, the stitch prompt includes them.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.MkdirAll("docs", 0o755)
	os.WriteFile("docs/ARCHITECTURE.yaml", []byte(`id: arch
title: Architecture
overview:
  summary: s
  lifecycle: l
  coordination_pattern: c
shared_protocols:
  - name: error-reporting
    description: Use stderr for all error output
`), 0o644)

	o := New(Config{})
	task := stitchTask{
		ID:        "test-ood",
		Title:     "Implement ls",
		IssueType: "code",
	}
	out, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}
	if !strings.Contains(out, "shared_protocols:") {
		t.Errorf("stitch prompt missing shared_protocols key; output:\n%s", out)
	}
	if !strings.Contains(out, "error-reporting") {
		t.Errorf("stitch prompt missing protocol name 'error-reporting'; output:\n%s", out)
	}
}

func TestBuildStitchPrompt_OODPackageContracts(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-pkg.yaml"), []byte(`id: srd001-pkg
title: Pkg
package_contract:
  exports:
    - name: FuncA
`), 0o644)

	o := New(Config{})
	task := stitchTask{ID: "t1", Title: "impl", IssueType: "code"}
	out, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}
	if !strings.Contains(out, "package_contracts:") {
		t.Errorf("stitch prompt missing package_contracts key; output:\n%s", out)
	}
	if !strings.Contains(out, "FuncA") {
		t.Errorf("stitch prompt missing FuncA export; output:\n%s", out)
	}
}

// --- Semantic model injection in buildStitchPrompt ---

func TestBuildStitchPrompt_SemanticModelPresent(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// When a SRD has a semantic_model, the stitch prompt includes it.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-core.yaml"), []byte(`id: srd001-core
title: Core
semantic_model:
  entities:
    - name: Widget
      attributes:
        - name: id
          type: string
`), 0o644)

	o := New(Config{})
	task := stitchTask{ID: "t-sm-1", Title: "impl widget", IssueType: "code"}
	out, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}
	if !strings.Contains(out, "semantic_model:") {
		t.Errorf("stitch prompt missing semantic_model key; output:\n%s", out)
	}
	if !strings.Contains(out, "Widget") {
		t.Errorf("stitch prompt missing Widget entity from semantic_model; output:\n%s", out)
	}
}

func TestBuildStitchPrompt_SemanticModelAbsent(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// When no SRD has a semantic_model, the field is omitted from the prompt.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-plain.yaml"), []byte(`id: srd001-plain
title: Plain SRD
`), 0o644)

	o := New(Config{})
	task := stitchTask{ID: "t-sm-2", Title: "impl plain", IssueType: "code"}
	out, err := o.Stitch.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}
	if strings.Contains(out, "semantic_model:") {
		t.Errorf("stitch prompt should omit semantic_model when no SRD has one; output:\n%s", out)
	}
}

// --- OOD injection in buildMeasurePrompt ---

func TestBuildMeasurePrompt_OODContractsHeaders(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// When source_mode=headers and a SRD has package_contract, measure prompt includes contracts.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-pkg.yaml"), []byte(`id: srd001-pkg
title: Pkg
package_contract:
  exports:
    - name: ExportedFunc
`), 0o644)
	// Write a stitch context to force source_mode=headers.
	os.MkdirAll(".cobbler", 0o755)
	os.WriteFile(".cobbler/measure_context.yaml", []byte("source_mode: headers\n"), 0o644)

	o := New(Config{Cobbler: CobblerConfig{Dir: ".cobbler"}})
	out, err := o.Measure.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}
	if !strings.Contains(out, "package_contracts:") {
		t.Errorf("measure prompt missing package_contracts with source_mode=headers; output:\n%s", out)
	}
	if !strings.Contains(out, "ExportedFunc") {
		t.Errorf("measure prompt missing ExportedFunc; output:\n%s", out)
	}
}

func TestBuildMeasurePrompt_OODContractsFullMode_Excluded(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// When source_mode is not headers/custom, package_contracts are NOT injected.
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	os.MkdirAll("docs/specs/software-requirements", 0o755)
	os.WriteFile(filepath.Join("docs/specs/software-requirements", "srd001-pkg.yaml"), []byte(`id: srd001-pkg
title: Pkg
package_contract:
  exports:
    - name: ExportedFunc
`), 0o644)

	o := New(Config{})
	out, err := o.Measure.buildMeasurePrompt("", "", 1)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}
	if strings.Contains(out, "package_contracts:") {
		t.Errorf("measure prompt should not include package_contracts with default source_mode; output:\n%s", out)
	}
}
