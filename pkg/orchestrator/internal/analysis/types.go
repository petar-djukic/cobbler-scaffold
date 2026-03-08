// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// ---------------------------------------------------------------------------
// Minimal YAML document types for loading roadmap, PRD, and architecture
// files. These duplicate only the fields needed by analysis; the canonical
// types live in the parent orchestrator package (context.go).
// ---------------------------------------------------------------------------

// RoadmapDoc corresponds to docs/road-map.yaml (analysis-relevant fields).
type RoadmapDoc struct {
	Releases []RoadmapRelease `yaml:"releases"`
}

// RoadmapRelease is a single release entry in the roadmap.
type RoadmapRelease struct {
	Version  string           `yaml:"version"`
	Name     string           `yaml:"name"`
	Status   string           `yaml:"status"`
	UseCases []RoadmapUseCase `yaml:"use_cases"`
}

// RoadmapUseCase is a use case entry within a roadmap release.
type RoadmapUseCase struct {
	ID     string `yaml:"id"`
	Status string `yaml:"status"`
}

// PRDDoc corresponds to docs/specs/product-requirements/prd*.yaml
// (analysis-relevant fields).
type PRDDoc struct {
	Requirements       map[string]PRDRequirementGroup `yaml:"requirements"`
	AcceptanceCriteria []AcceptanceCriterion          `yaml:"acceptance_criteria"`
	PackageContract    *PRDPackageContract            `yaml:"package_contract,omitempty"`
	DependsOn          []PRDDependsOn                 `yaml:"depends_on,omitempty"`
	StructRefs         []PRDStructRef                 `yaml:"struct_refs,omitempty"`
}

// AcceptanceCriterion is a structured acceptance criterion with an ID,
// description, and traceability links to requirement items.
type AcceptanceCriterion struct {
	ID        string   `yaml:"id"`
	Criterion string   `yaml:"criterion"`
	Traces    []string `yaml:"traces"`
}

// SuccessCriterion is a structured success criterion from a use case,
// with an ID, description, and traceability links to PRD ACs.
type SuccessCriterion struct {
	ID        string   `yaml:"id"`
	Criterion string   `yaml:"criterion"`
	Traces    []string `yaml:"traces"`
}

// PRDRequirementGroup is a requirement section within a PRD.
type PRDRequirementGroup struct {
	Title string              `yaml:"title"`
	Items []map[string]string `yaml:"items"`
}

// PRDPackageContract describes the public API surface of a pkg/ package.
type PRDPackageContract struct {
	Exports []PRDExport `yaml:"exports,omitempty"`
}

// PRDExport is a single exported symbol with its signature.
type PRDExport struct {
	Name string `yaml:"name"`
}

// PRDDependsOn declares that a cmd/ PRD depends on a pkg/ PRD.
type PRDDependsOn struct {
	PRDID       string   `yaml:"prd_id"`
	SymbolsUsed []string `yaml:"symbols_used,omitempty"`
}

// PRDStructRef cross-references a type definition in another PRD.
type PRDStructRef struct {
	PRDID       string `yaml:"prd_id"`
	Requirement string `yaml:"requirement"`
}

// ArchitectureDoc corresponds to docs/ARCHITECTURE.yaml
// (analysis-relevant fields).
type ArchitectureDoc struct {
	ComponentDependencies []ArchComponentDependency `yaml:"component_dependencies,omitempty"`
	DependencyRules       []ArchDependencyRule      `yaml:"dependency_rules,omitempty"`
}

// ArchComponentDependency is a single dependency edge in the architecture.
type ArchComponentDependency struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// ArchDependencyRule is a constraint on component dependencies.
type ArchDependencyRule struct {
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Allowed     bool   `yaml:"allowed"`
	Description string `yaml:"description"`
}

// loadYAML reads a YAML file and unmarshals it into T.
// Returns nil if the file does not exist or cannot be parsed.
func loadYAML[T any](path string) *T {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil
	}
	return &v
}
