// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// constitutionSectionsOnly extracts only the sections field from any
// constitution YAML file without requiring knowledge of the full schema.
type constitutionSectionsOnly struct {
	Sections []ConstitutionSection `yaml:"sections"`
}

// ConstitutionToMarkdown converts a slice of ConstitutionSection values into a
// markdown string. Each section becomes a level-2 heading (## Title), followed
// by a blank line, the section content, and a trailing blank line.
//
// If sections is empty, the function returns an empty string.
func ConstitutionToMarkdown(sections []ConstitutionSection) string {
	var b strings.Builder
	for _, s := range sections {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", s.Title, strings.TrimRight(s.Content, "\n"))
	}
	return b.String()
}

// PreviewConstitutionFile reads the constitution YAML file at path, extracts
// its sections field, and prints the rendered markdown to stdout. It returns
// an error when the file is missing, malformed, or contains no sections.
func PreviewConstitutionFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	var doc constitutionSectionsOnly
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(doc.Sections) == 0 {
		fmt.Fprintf(os.Stderr, "warning: %s has no sections field\n", path)
		return fmt.Errorf("no sections in %s", path)
	}
	fmt.Print(ConstitutionToMarkdown(doc.Sections))
	return nil
}
