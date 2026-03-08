// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	ctx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
)

// ConstitutionToMarkdown delegates to the internal/context package.
func ConstitutionToMarkdown(sections []ConstitutionSection) string {
	return ctx.ConstitutionToMarkdown(sections)
}

// ConstitutionPreviewFile reads the constitution YAML file at path, extracts
// its sections field, and prints the rendered markdown to stdout. It returns
// an error when the file is missing, malformed, or contains no sections.
func (o *Orchestrator) ConstitutionPreviewFile(path string) error {
	return ctx.PreviewConstitutionFile(path)
}
