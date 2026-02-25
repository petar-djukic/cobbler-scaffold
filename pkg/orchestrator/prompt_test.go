// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- parsePromptTemplate ---

func TestParsePromptTemplate_MeasureFields(t *testing.T) {
	tmpl, err := parsePromptTemplate(defaultMeasurePrompt)
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
	tmpl, err := parsePromptTemplate(defaultStitchPrompt)
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
	_, err := parsePromptTemplate("not: [valid: yaml")
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
	got := substitutePlaceholders(text, data)
	want := "Output to /tmp/out.yaml, max 5 tasks of 250-350 lines."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- parseYAMLNode ---

func TestParseYAMLNode_ValidYAML(t *testing.T) {
	node := parseYAMLNode("articles:\n  - id: P1\n    title: Test")
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	if node.Kind != yaml.MappingNode {
		t.Errorf("expected MappingNode, got %v", node.Kind)
	}
}

func TestParseYAMLNode_Empty(t *testing.T) {
	node := parseYAMLNode("")
	if node != nil {
		t.Error("expected nil for empty input")
	}
}

func TestParseYAMLNode_Invalid(t *testing.T) {
	node := parseYAMLNode("not: [valid: yaml")
	if node != nil {
		t.Error("expected nil for invalid YAML")
	}
}

// --- Integration tests for prompt builders ---

func TestMeasurePromptIsValidYAML(t *testing.T) {
	o := New(Config{})
	prompt, err := o.buildMeasurePrompt("", "[]", 5)
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
	prompt, err := o.buildMeasurePrompt("", "[]", 5)
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
	prompt, err := o.buildMeasurePrompt("", "[]", 5)
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
	prompt, err := o.buildMeasurePrompt("", "[]", 5)
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
	prompt, err := o.buildMeasurePrompt("", "[]", 3)
	if err != nil {
		t.Fatalf("buildMeasurePrompt: %v", err)
	}

	if strings.Contains(prompt, "{limit}") {
		t.Error("measure prompt has unsubstituted {limit} placeholder")
	}
	if strings.Contains(prompt, "{lines_min}") {
		t.Error("measure prompt has unsubstituted {lines_min} placeholder")
	}
	if strings.Contains(prompt, "{output_path}") {
		t.Error("measure prompt still references removed {output_path} placeholder")
	}
}

func TestMeasurePromptNoWriteToolReferences(t *testing.T) {
	o := New(Config{})
	prompt, err := o.buildMeasurePrompt("", "[]", 1)
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

func TestStitchPromptIsValidYAML(t *testing.T) {
	o := New(Config{})
	task := stitchTask{
		id:          "test-001",
		title:       "Test task",
		issueType:   "task",
		description: "A test description.",
		worktreeDir: "/tmp",
	}

	prompt, err := o.buildStitchPrompt(task)
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
		id:          "test-001",
		title:       "Test task",
		issueType:   "task",
		description: "A test description.",
		worktreeDir: "/tmp",
	}

	prompt, err := o.buildStitchPrompt(task)
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
		id:          "task-123",
		title:       "Implement feature X",
		issueType:   "task",
		description: "Detailed requirements here.",
		worktreeDir: "/tmp",
	}

	prompt, err := o.buildStitchPrompt(task)
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
		id:          "test-001",
		title:       "Test task",
		issueType:   "task",
		description: "A test description.",
		worktreeDir: "/tmp",
	}

	prompt, err := o.buildStitchPrompt(task)
	if err != nil {
		t.Fatalf("buildStitchPrompt: %v", err)
	}

	if !strings.Contains(prompt, "go_style_constitution:") {
		t.Error("stitch prompt missing go_style_constitution YAML key")
	}
}
