// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PromptSection is a named section of a rendered prompt. The Name field
// becomes a markdown heading (e.g., "task" → "# TASK"). When the value
// in the YAML is a scalar string, it becomes the Text directly. When it
// is a mapping, it may contain text, append, format, and heading fields.
//
// Append names a key in the data map whose value is appended after Text.
// When Append is set and the corresponding data value is empty, the
// entire section is omitted. Format controls how appended data is
// wrapped: "yaml" wraps in ```yaml code fences; any other value (or
// empty) appends with a leading newline. Heading overrides the
// auto-generated heading derived from Name.
type PromptSection struct {
	Name    string // section identifier (YAML map key)
	Text    string // body text, may contain {key} placeholders
	Append  string // data key whose value is appended after Text
	Format  string // "yaml" wraps in ```yaml fences
	Heading string // custom heading; default: "# NAME"
}

// promptSectionDetail is the struct used when a section value is a
// mapping rather than a scalar.
type promptSectionDetail struct {
	Text    string `yaml:"text"`
	Append  string `yaml:"append"`
	Format  string `yaml:"format"`
	Heading string `yaml:"heading"`
}

// PromptDef is an ordered list of named prompt sections. Each element in
// the YAML sequence is a single-key mapping where the key is the section
// name and the value is either a scalar (the text) or a mapping with
// text/append/format/heading fields.
type PromptDef []PromptSection

// UnmarshalYAML implements custom unmarshaling for PromptDef. It expects
// a YAML sequence of single-key mappings.
func (pd *PromptDef) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("prompt definition must be a YAML sequence, got %v", value.Kind)
	}
	sections := make(PromptDef, 0, len(value.Content))
	for i, item := range value.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("section %d: expected a mapping, got %v", i, item.Kind)
		}
		if len(item.Content) < 2 {
			return fmt.Errorf("section %d: mapping must have at least one key", i)
		}
		// Use only the first key-value pair (single-key map).
		keyNode := item.Content[0]
		valNode := item.Content[1]

		sec := PromptSection{Name: keyNode.Value}

		switch valNode.Kind {
		case yaml.ScalarNode:
			sec.Text = valNode.Value
		case yaml.MappingNode:
			var detail promptSectionDetail
			if err := valNode.Decode(&detail); err != nil {
				return fmt.Errorf("section %q: %w", sec.Name, err)
			}
			sec.Text = detail.Text
			sec.Append = detail.Append
			sec.Format = detail.Format
			sec.Heading = detail.Heading
		default:
			return fmt.Errorf("section %q: unexpected YAML node kind %v", sec.Name, valNode.Kind)
		}

		sections = append(sections, sec)
	}
	*pd = sections
	return nil
}

// parsePromptDef parses a YAML document into a PromptDef.
func parsePromptDef(yamlContent string) (PromptDef, error) {
	var def PromptDef
	if err := yaml.Unmarshal([]byte(yamlContent), &def); err != nil {
		return nil, err
	}
	return def, nil
}

// validatePromptDef reads a YAML file and parses it as a PromptDef.
// Returns a list of errors if the file is malformed. Returns nil if the
// file doesn't exist (missing files are not schema errors).
func validatePromptDef(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // missing file is not a schema error
	}
	if _, err := parsePromptDef(string(data)); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}

// sectionHeading returns the markdown heading for a section. If the
// section has a custom Heading, it is returned directly. Otherwise the
// Name is uppercased with underscores replaced by spaces and prefixed
// with "# ".
func sectionHeading(sec PromptSection) string {
	if sec.Heading != "" {
		return sec.Heading
	}
	return "# " + strings.ToUpper(strings.ReplaceAll(sec.Name, "_", " "))
}

// renderPrompt assembles a prompt string from a PromptDef and a data
// map. Each section produces a markdown heading (derived from its Name)
// followed by its Text. Placeholders in Text use {key} syntax and are
// replaced with the corresponding value from data. Substitution applies
// only to Text, not to appended values, preventing cross-substitution
// when appended data contains brace patterns.
func renderPrompt(def PromptDef, data map[string]string) string {
	var buf strings.Builder
	first := true
	for _, sec := range def {
		// Skip sections whose append data is empty.
		if sec.Append != "" && data[sec.Append] == "" {
			continue
		}

		if !first {
			buf.WriteString("\n")
		}
		first = false

		// Write heading.
		buf.WriteString(sectionHeading(sec))
		buf.WriteString("\n\n")

		// Substitute placeholders in text.
		if sec.Text != "" {
			text := sec.Text
			for k, v := range data {
				text = strings.ReplaceAll(text, "{"+k+"}", v)
			}
			buf.WriteString(text)
		}

		// Append dynamic data.
		if sec.Append != "" {
			val := data[sec.Append]
			switch sec.Format {
			case "yaml":
				buf.WriteString("\n```yaml\n")
				buf.WriteString(val)
				if !strings.HasSuffix(val, "\n") {
					buf.WriteByte('\n')
				}
				buf.WriteString("```\n")
			default:
				buf.WriteString("\n")
				buf.WriteString(val)
			}
		}
	}
	return buf.String()
}
