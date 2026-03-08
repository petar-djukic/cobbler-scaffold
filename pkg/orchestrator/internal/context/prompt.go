// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package context

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
	Role                    string                   `yaml:"role"`
	ProjectContext          *ProjectContext          `yaml:"project_context,omitempty"`
	PlanningConstitution    *yaml.Node              `yaml:"planning_constitution,omitempty"`
	IssueFormatConstitution *yaml.Node              `yaml:"issue_format_constitution,omitempty"`
	Task                    string                   `yaml:"task"`
	Constraints             string                   `yaml:"constraints"`
	OutputFormat            string                   `yaml:"output_format"`
	GoldenExample           string                   `yaml:"golden_example,omitempty"`
	AdditionalContext       string                   `yaml:"additional_context,omitempty"`
	ValidationErrors        []string                 `yaml:"validation_errors,omitempty"`
	PackageContracts        []OODPackageContractRef  `yaml:"package_contracts,omitempty"`
}

// StitchPromptDoc is the complete stitch prompt as a YAML document.
type StitchPromptDoc struct {
	Role                  string                   `yaml:"role"`
	RepositoryFiles       []string                 `yaml:"repository_files,omitempty"`
	ProjectContext        *ProjectContext          `yaml:"project_context,omitempty"`
	Context               string                   `yaml:"context"`
	ExecutionConstitution *yaml.Node              `yaml:"execution_constitution,omitempty"`
	GoStyleConstitution   *yaml.Node              `yaml:"go_style_constitution,omitempty"`
	Task                  string                   `yaml:"task"`
	Constraints           string                   `yaml:"constraints"`
	Description           string                   `yaml:"description"`
	SharedProtocols       []ArchSharedProtocol     `yaml:"shared_protocols,omitempty"`
	PackageContracts      []OODPackageContractRef  `yaml:"package_contracts,omitempty"`
}

// PromptTemplate holds the static text fields parsed from a prompt
// template YAML file. Both measure and stitch templates share this
// structure; measure uses OutputFormat while stitch leaves it empty.
type PromptTemplate struct {
	Role         string `yaml:"role"`
	Task         string `yaml:"task"`
	Constraints  string `yaml:"constraints"`
	OutputFormat string `yaml:"output_format,omitempty"`
}

// ParsePromptTemplate parses a YAML mapping into a PromptTemplate.
func ParsePromptTemplate(yamlContent string) (PromptTemplate, error) {
	var tmpl PromptTemplate
	if err := yaml.Unmarshal([]byte(yamlContent), &tmpl); err != nil {
		return PromptTemplate{}, err
	}
	return tmpl, nil
}

// ValidatePromptTemplate reads a YAML file and parses it as a
// PromptTemplate. Returns a list of errors if the file is malformed.
// Returns nil if the file doesn't exist.
func ValidatePromptTemplate(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // missing file is not a schema error
	}
	if _, err := ParsePromptTemplate(string(data)); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}

// ParseYAMLNode parses a YAML string into a yaml.Node, preserving
// the original structure. Returns nil if the content is empty or
// unparseable.
func ParseYAMLNode(content string) *yaml.Node {
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

// SubstitutePlaceholders replaces {key} patterns in text with values
// from the data map.
func SubstitutePlaceholders(text string, data map[string]string) string {
	for k, v := range data {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}
