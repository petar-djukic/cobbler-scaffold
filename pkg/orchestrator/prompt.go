// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	ctx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Type aliases — re-export prompt types from internal/context.
// ---------------------------------------------------------------------------

type MeasurePromptDoc = ctx.MeasurePromptDoc
type StitchPromptDoc = ctx.StitchPromptDoc
type promptTemplate = ctx.PromptTemplate

// ---------------------------------------------------------------------------
// Function delegates.
// ---------------------------------------------------------------------------

func parsePromptTemplate(yamlContent string) (promptTemplate, error) {
	return ctx.ParsePromptTemplate(yamlContent)
}

func validatePromptTemplate(path string) []string {
	return ctx.ValidatePromptTemplate(path)
}

func parseYAMLNode(content string) *yaml.Node {
	return ctx.ParseYAMLNode(content)
}

func substitutePlaceholders(text string, data map[string]string) string {
	return ctx.SubstitutePlaceholders(text, data)
}
