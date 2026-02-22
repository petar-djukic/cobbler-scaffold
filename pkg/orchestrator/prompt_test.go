// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"strings"
	"testing"
)

// --- renderPrompt ---

func TestRenderPrompt_PlaceholderSubstitution(t *testing.T) {
	def := PromptDef{
		{Name: "greeting", Text: "Hello {name}, your ID is {id}.\n"},
	}
	data := map[string]string{"name": "Alice", "id": "42"}
	got := renderPrompt(def, data)
	if !strings.Contains(got, "Hello Alice, your ID is 42.\n") {
		t.Errorf("missing substituted text in %q", got)
	}
	if !strings.Contains(got, "# GREETING") {
		t.Errorf("missing auto-generated heading in %q", got)
	}
}

func TestRenderPrompt_SkipEmptyAppend(t *testing.T) {
	def := PromptDef{
		{Name: "always", Text: "Always present.\n"},
		{Name: "conditional", Text: "Conditional.\n", Append: "data"},
		{Name: "also_always", Text: "Also always.\n"},
	}
	data := map[string]string{"data": ""}
	got := renderPrompt(def, data)
	if strings.Contains(got, "Conditional") {
		t.Error("section with empty append data should be skipped")
	}
	if !strings.Contains(got, "Always present") || !strings.Contains(got, "Also always") {
		t.Error("non-conditional sections should be present")
	}
}

func TestRenderPrompt_AppendYAMLFormat(t *testing.T) {
	def := PromptDef{
		{Name: "context", Text: "Details:\n", Append: "ctx", Format: "yaml"},
	}
	data := map[string]string{"ctx": "key: value\n"}
	got := renderPrompt(def, data)
	if !strings.Contains(got, "```yaml\nkey: value\n```") {
		t.Errorf("expected yaml code fence, got %q", got)
	}
}

func TestRenderPrompt_AppendYAMLAddsNewlineIfMissing(t *testing.T) {
	def := PromptDef{
		{Name: "header", Text: "", Append: "ctx", Format: "yaml"},
	}
	data := map[string]string{"ctx": "no trailing newline"}
	got := renderPrompt(def, data)
	if !strings.Contains(got, "no trailing newline\n```") {
		t.Errorf("expected newline before closing fence, got %q", got)
	}
}

func TestRenderPrompt_AppendDefaultFormat(t *testing.T) {
	def := PromptDef{
		{Name: "description", Append: "desc"},
	}
	data := map[string]string{"desc": "Task details here."}
	got := renderPrompt(def, data)
	if !strings.Contains(got, "# DESCRIPTION") {
		t.Errorf("missing heading in %q", got)
	}
	if !strings.Contains(got, "\nTask details here.") {
		t.Errorf("missing appended text in %q", got)
	}
}

func TestRenderPrompt_NoSubstitutionInAppendedValue(t *testing.T) {
	def := PromptDef{
		{Name: "info", Text: "ID: {id}\n", Append: "body"},
	}
	data := map[string]string{"id": "42", "body": "Implement {id} field."}
	got := renderPrompt(def, data)
	if !strings.Contains(got, "Implement {id} field.") {
		t.Error("appended value should not have placeholders substituted")
	}
	if !strings.Contains(got, "ID: 42") {
		t.Error("content placeholders should be substituted")
	}
}

func TestRenderPrompt_CustomHeading(t *testing.T) {
	def := PromptDef{
		{Name: "exec_const", Text: "Rules:\n", Heading: "## Execution Constitution"},
	}
	got := renderPrompt(def, nil)
	if !strings.Contains(got, "## Execution Constitution") {
		t.Errorf("expected custom heading, got %q", got)
	}
	if strings.Contains(got, "# EXEC CONST") {
		t.Error("auto-generated heading should not appear when custom heading is set")
	}
}

func TestRenderPrompt_HeadingFromName(t *testing.T) {
	def := PromptDef{
		{Name: "project_context", Text: "Data here.\n"},
	}
	got := renderPrompt(def, nil)
	if !strings.Contains(got, "# PROJECT CONTEXT") {
		t.Errorf("expected auto heading '# PROJECT CONTEXT', got %q", got)
	}
}

func TestParsePromptDef_SemanticKeys(t *testing.T) {
	yamlStr := `
- role: |
    You are an architect.
- project_context:
    text: |
      All docs below.
    append: project_context
    format: yaml
- task: |
    Do the thing.
`
	def, err := parsePromptDef(yamlStr)
	if err != nil {
		t.Fatalf("parsePromptDef: %v", err)
	}
	if len(def) != 3 {
		t.Fatalf("got %d sections, want 3", len(def))
	}
	if def[0].Name != "role" {
		t.Errorf("section 0 name: got %q, want %q", def[0].Name, "role")
	}
	if !strings.Contains(def[0].Text, "architect") {
		t.Errorf("section 0 text missing 'architect': %q", def[0].Text)
	}
	if def[1].Name != "project_context" {
		t.Errorf("section 1 name: got %q, want %q", def[1].Name, "project_context")
	}
	if def[1].Append != "project_context" {
		t.Errorf("section 1 append: got %q, want %q", def[1].Append, "project_context")
	}
	if def[1].Format != "yaml" {
		t.Errorf("section 1 format: got %q, want %q", def[1].Format, "yaml")
	}
	if def[2].Name != "task" {
		t.Errorf("section 2 name: got %q, want %q", def[2].Name, "task")
	}
}

func TestParsePromptDef_CustomHeading(t *testing.T) {
	yamlStr := `
- execution_constitution:
    heading: "## Execution Constitution"
    text: |
      The rules:
    append: execution_constitution
    format: yaml
`
	def, err := parsePromptDef(yamlStr)
	if err != nil {
		t.Fatalf("parsePromptDef: %v", err)
	}
	if len(def) != 1 {
		t.Fatalf("got %d sections, want 1", len(def))
	}
	if def[0].Heading != "## Execution Constitution" {
		t.Errorf("heading: got %q, want %q", def[0].Heading, "## Execution Constitution")
	}
}

func TestParsePromptDef_InvalidYAML(t *testing.T) {
	_, err := parsePromptDef("not: [valid: yaml")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParsePromptDef_NotSequence(t *testing.T) {
	_, err := parsePromptDef("key: value")
	if err == nil {
		t.Error("expected error for non-sequence YAML")
	}
}

// --- Integration tests for prompt builders ---

func TestMeasurePromptIncludesPlanningConstitution(t *testing.T) {
	o := New(Config{})
	prompt := o.buildMeasurePrompt("", "[]", 5, "/tmp/out.yaml")

	if !strings.Contains(prompt, "## Planning Constitution") {
		t.Error("measure prompt missing '## Planning Constitution' section")
	}
	if !strings.Contains(prompt, "```yaml") {
		t.Error("measure prompt missing YAML code fence for constitution")
	}
	// Check for a key planning constitution article
	if !strings.Contains(prompt, "Release-driven priority") {
		t.Error("measure prompt missing planning constitution content (article P1)")
	}
}

func TestMeasurePromptIncludesProjectContext(t *testing.T) {
	o := New(Config{})
	prompt := o.buildMeasurePrompt("", "[]", 5, "/tmp/out.yaml")

	if !strings.Contains(prompt, "# PROJECT CONTEXT") {
		t.Error("measure prompt missing '# PROJECT CONTEXT' section")
	}
	if !strings.Contains(prompt, "# TASK") {
		t.Error("measure prompt missing TASK section")
	}
	if !strings.Contains(prompt, "Do NOT read docs/") {
		t.Error("measure prompt missing instruction to not read docs/ files")
	}
}

func TestStitchPromptIncludesExecutionConstitution(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		id:          "test-001",
		title:       "Test task",
		issueType:   "task",
		description: "A test description.",
		worktreeDir: "/tmp",
	}

	prompt := o.buildStitchPrompt(task)

	if !strings.Contains(prompt, "## Execution Constitution") {
		t.Error("stitch prompt missing '## Execution Constitution' section")
	}
	if !strings.Contains(prompt, "```yaml") {
		t.Error("stitch prompt missing YAML code fence for constitution")
	}
	// Check for a key execution constitution article
	if !strings.Contains(prompt, "Specification-first") {
		t.Error("stitch prompt missing execution constitution content (article E1)")
	}
}

func TestStitchPromptIncludesTaskContext(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		id:          "task-123",
		title:       "Implement feature X",
		issueType:   "task",
		description: "Detailed requirements here.",
		worktreeDir: "/tmp",
	}

	prompt := o.buildStitchPrompt(task)

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
