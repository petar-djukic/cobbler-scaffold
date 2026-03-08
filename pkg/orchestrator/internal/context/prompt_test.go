// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- ParsePromptTemplate ---

func TestParsePromptTemplate_InvalidYAML(t *testing.T) {
	_, err := ParsePromptTemplate("not: [valid: yaml")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParsePromptTemplate_ValidFields(t *testing.T) {
	tmpl, err := ParsePromptTemplate("role: assistant\ntask: do something\nconstraints: be good\noutput_format: yaml\n")
	if err != nil {
		t.Fatalf("ParsePromptTemplate: %v", err)
	}
	if tmpl.Role != "assistant" {
		t.Errorf("Role = %q, want %q", tmpl.Role, "assistant")
	}
	if tmpl.Task != "do something" {
		t.Errorf("Task = %q, want %q", tmpl.Task, "do something")
	}
	if tmpl.Constraints != "be good" {
		t.Errorf("Constraints = %q, want %q", tmpl.Constraints, "be good")
	}
	if tmpl.OutputFormat != "yaml" {
		t.Errorf("OutputFormat = %q, want %q", tmpl.OutputFormat, "yaml")
	}
}

// --- SubstitutePlaceholders ---

func TestSubstitutePlaceholders(t *testing.T) {
	text := "Output to {output_path}, max {limit} tasks of {lines_min}-{lines_max} lines."
	data := map[string]string{
		"output_path": "/tmp/out.yaml",
		"limit":       "5",
		"lines_min":   "250",
		"lines_max":   "350",
	}
	got := SubstitutePlaceholders(text, data)
	want := "Output to /tmp/out.yaml, max 5 tasks of 250-350 lines."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- ValidatePromptTemplate ---

func TestValidatePromptTemplate_MissingFile(t *testing.T) {
	errs := ValidatePromptTemplate("/nonexistent/path/prompt.yaml")
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
	errs := ValidatePromptTemplate(path)
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
	errs := ValidatePromptTemplate(path)
	if len(errs) == 0 {
		t.Error("expected errors for invalid YAML, got none")
	}
}

// --- ParseYAMLNode ---

func TestParseYAMLNode_ValidYAML(t *testing.T) {
	node := ParseYAMLNode("articles:\n  - id: P1\n    title: Test")
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	if node.Kind != yaml.MappingNode {
		t.Errorf("expected MappingNode, got %v", node.Kind)
	}
}

func TestParseYAMLNode_Empty(t *testing.T) {
	node := ParseYAMLNode("")
	if node != nil {
		t.Error("expected nil for empty input")
	}
}

func TestParseYAMLNode_Invalid(t *testing.T) {
	node := ParseYAMLNode("not: [valid: yaml")
	if node != nil {
		t.Error("expected nil for invalid YAML")
	}
}
