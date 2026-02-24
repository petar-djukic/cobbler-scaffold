// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// MeasurePromptDoc is the complete measure prompt as a YAML document.
// Each field maps directly to a top-level YAML key. When marshaled,
// it produces a single syntactically correct YAML document.
type MeasurePromptDoc struct {
	Role                    string          `yaml:"role"`
	ProjectContext          *ProjectContext `yaml:"project_context,omitempty"`
	PlanningConstitution    *yaml.Node     `yaml:"planning_constitution,omitempty"`
	IssueFormatConstitution *yaml.Node     `yaml:"issue_format_constitution,omitempty"`
	Task                    string          `yaml:"task"`
	Constraints             string          `yaml:"constraints"`
	OutputFormat            string          `yaml:"output_format"`
	AdditionalContext       string          `yaml:"additional_context,omitempty"`
}

// StitchPromptDoc is the complete stitch prompt as a YAML document.
type StitchPromptDoc struct {
	Role                  string          `yaml:"role"`
	ProjectContext        *ProjectContext `yaml:"project_context,omitempty"`
	Context               string          `yaml:"context"`
	ExecutionConstitution *yaml.Node     `yaml:"execution_constitution,omitempty"`
	GoStyleConstitution   *yaml.Node     `yaml:"go_style_constitution,omitempty"`
	Task                  string          `yaml:"task"`
	Constraints           string          `yaml:"constraints"`
	Description           string          `yaml:"description"`
}

// promptTemplate holds the static text fields parsed from a prompt
// template YAML file. Both measure and stitch templates share this
// structure; measure uses OutputFormat while stitch leaves it empty.
type promptTemplate struct {
	Role         string `yaml:"role"`
	Task         string `yaml:"task"`
	Constraints  string `yaml:"constraints"`
	OutputFormat string `yaml:"output_format,omitempty"`
}

// parsePromptTemplate parses a YAML mapping into a promptTemplate.
func parsePromptTemplate(yamlContent string) (promptTemplate, error) {
	var tmpl promptTemplate
	if err := yaml.Unmarshal([]byte(yamlContent), &tmpl); err != nil {
		return promptTemplate{}, err
	}
	return tmpl, nil
}

// validatePromptTemplate reads a YAML file and parses it as a
// promptTemplate. Returns a list of errors if the file is malformed.
// Returns nil if the file doesn't exist.
func validatePromptTemplate(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // missing file is not a schema error
	}
	if _, err := parsePromptTemplate(string(data)); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}

// parseYAMLNode parses a YAML string into a yaml.Node, preserving
// the original structure. Returns nil if the content is empty or
// unparseable.
func parseYAMLNode(content string) *yaml.Node {
	if content == "" {
		return nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &doc
}

// substitutePlaceholders replaces {key} patterns in text with values
// from the data map.
func substitutePlaceholders(text string, data map[string]string) string {
	for k, v := range data {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}
