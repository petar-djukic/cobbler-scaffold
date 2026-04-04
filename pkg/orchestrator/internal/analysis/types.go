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
// Minimal YAML document types for loading roadmap, SRD, and architecture
// files. These duplicate only the fields needed by analysis; the canonical
// types live in the parent orchestrator package (context.go).
// ---------------------------------------------------------------------------

// RoadmapDoc corresponds to docs/road-map.yaml (analysis-relevant fields).
type RoadmapDoc struct {
	Releases []RoadmapRelease `yaml:"releases"`
}

// RoadmapRelease is a single release entry in the roadmap.
type RoadmapRelease struct {
	Version   string           `yaml:"version"`
	Name      string           `yaml:"name"`
	Status    string           `yaml:"status"`
	DependsOn []string         `yaml:"depends_on,omitempty"`
	UseCases  []RoadmapUseCase `yaml:"use_cases"`
}

// RoadmapUseCase is a use case entry within a roadmap release.
type RoadmapUseCase struct {
	ID     string `yaml:"id"`
	Status string `yaml:"status"`
}

// SRDDoc corresponds to docs/specs/software-requirements/srd*.yaml
// (analysis-relevant fields).
type SRDDoc struct {
	Requirements       map[string]SRDRequirementGroup `yaml:"requirements"`
	AcceptanceCriteria []AcceptanceCriterion          `yaml:"acceptance_criteria"`
	PackageContract    *SRDPackageContract            `yaml:"package_contract,omitempty"`
	DependsOn          []SRDDependsOn                 `yaml:"depends_on,omitempty"`
	StructRefs         []SRDStructRef                 `yaml:"struct_refs,omitempty"`
	ImplementedBy      []string                       `yaml:"implemented_by,omitempty"`
	UsedBy             []string                       `yaml:"used_by,omitempty"`
}

// AcceptanceCriterion is a structured acceptance criterion with an ID,
// description, and traceability links to requirement items.
type AcceptanceCriterion struct {
	ID        string   `yaml:"id"`
	Criterion string   `yaml:"criterion"`
	Traces    []string `yaml:"traces"`
}

// SuccessCriterion is a structured success criterion from a use case,
// with an ID, description, and traceability links to SRD ACs.
type SuccessCriterion struct {
	ID        string   `yaml:"id"`
	Criterion string   `yaml:"criterion"`
	Traces    []string `yaml:"traces"`
}

// SRDRequirementGroup is a requirement section within a SRD.
// Items uses []any to accept both plain string values ("R1.1: text") and
// weighted values ("R1.1: {text: ..., weight: N}") (GH-1832).
type SRDRequirementGroup struct {
	Title string `yaml:"title"`
	Items []any  `yaml:"items"`
}

// SRDPackageContract describes the public API surface of a pkg/ package.
type SRDPackageContract struct {
	Exports []SRDExport `yaml:"exports,omitempty"`
}

// SRDExport is a single exported symbol with its signature.
type SRDExport struct {
	Name string `yaml:"name"`
}

// SRDDependsOn declares that a cmd/ SRD depends on a pkg/ SRD.
type SRDDependsOn struct {
	SRDID       string   `yaml:"srd_id"`
	SymbolsUsed []string `yaml:"symbols_used,omitempty"`
}

// SRDStructRef cross-references a type definition in another SRD.
type SRDStructRef struct {
	SRDID       string `yaml:"srd_id"`
	Requirement string `yaml:"requirement"`
}

// ArchitectureDoc corresponds to docs/ARCHITECTURE.yaml
// (analysis-relevant fields).
type ArchitectureDoc struct {
	Interfaces            []ArchInterface           `yaml:"interfaces,omitempty"`
	ComponentDependencies []ArchComponentDependency `yaml:"component_dependencies,omitempty"`
	DependencyRules       []ArchDependencyRule      `yaml:"dependency_rules,omitempty"`
}

// ArchInterface is an interface entry from ARCHITECTURE.yaml.
type ArchInterface struct {
	Name     string `yaml:"name"`
	SpecFile string `yaml:"spec_file,omitempty"`
}

// ArchComponentDependency is a single dependency edge in the architecture.
type ArchComponentDependency struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// InterfaceSpecDoc holds the fields parsed from docs/interfaces/ifc-*.yaml
// that are needed for cross-artifact consistency checks (GH-1990).
type InterfaceSpecDoc struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// ArchDependencyRule is a constraint on component dependencies.
type ArchDependencyRule struct {
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Allowed     bool   `yaml:"allowed"`
	Description string `yaml:"description"`
}

// RequirementState holds the status of a single R-item from
// .cobbler/requirements.yaml.
type RequirementState struct {
	Status string `yaml:"status"`
}

// RequirementsFile is the top-level structure of .cobbler/requirements.yaml.
type RequirementsFile struct {
	Requirements map[string]map[string]RequirementState `yaml:"requirements"`
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
