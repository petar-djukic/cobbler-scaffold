// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// ProjectContext: top-level container
// ---------------------------------------------------------------------------

// ProjectContext assembles all project documentation into a single
// structured document for injection into the measure prompt.
type ProjectContext struct {
	Vision         *VisionDoc         `yaml:"vision,omitempty"`
	Architecture   *ArchitectureDoc   `yaml:"architecture,omitempty"`
	Specifications *SpecificationsDoc `yaml:"specifications,omitempty"`
	Roadmap        *RoadmapDoc        `yaml:"roadmap,omitempty"`
	Specs          *SpecsCollection   `yaml:"specs,omitempty"`
	Engineering    []*EngineeringDoc  `yaml:"engineering,omitempty"`
	Constitutions  *ConstitutionsDoc  `yaml:"constitutions,omitempty"`
	SourceCode     []SourceFile       `yaml:"source_code,omitempty"`
	Issues         []ContextIssue     `yaml:"issues,omitempty"`
	Extra          []*NamedDoc        `yaml:"extra,omitempty"`
}

// SourceFile holds the path and content of a single source file
// for inclusion in the project context.
type SourceFile struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
}

// ---------------------------------------------------------------------------
// Vision
// ---------------------------------------------------------------------------

// VisionDoc corresponds to docs/VISION.yaml.
type VisionDoc struct {
	ID                   string            `yaml:"id"`
	Title                string            `yaml:"title"`
	ExecutiveSummary     string            `yaml:"executive_summary"`
	Problem              string            `yaml:"problem"`
	WhatThisDoes         string            `yaml:"what_this_does"`
	WhyWeBuildThis       string            `yaml:"why_we_build_this"`
	RelatedProjects      []RelatedProject  `yaml:"related_projects,omitempty"`
	SuccessCriteria      map[string]string `yaml:"success_criteria"`
	ImplementationPhases []Phase           `yaml:"implementation_phases"`
	Risks                []Risk            `yaml:"risks"`
	Not                  []string          `yaml:"not"`
}

// RelatedProject describes a project related to this one.
type RelatedProject struct {
	Project string `yaml:"project"`
	Role    string `yaml:"role"`
}

// ---------------------------------------------------------------------------
// Architecture
// ---------------------------------------------------------------------------

// ArchitectureDoc corresponds to docs/ARCHITECTURE.yaml.
type ArchitectureDoc struct {
	ID                   string          `yaml:"id"`
	Title                string          `yaml:"title"`
	Overview             ArchOverview    `yaml:"overview"`
	Interfaces           []ArchInterface `yaml:"interfaces"`
	Components           []ArchComponent `yaml:"components"`
	DesignDecisions      []ArchDecision  `yaml:"design_decisions"`
	TechnologyChoices    []ArchTech      `yaml:"technology_choices"`
	ProjectStructure     []ArchPathRole  `yaml:"project_structure"`
	ImplementationStatus ArchImplStatus  `yaml:"implementation_status"`
	RelatedDocuments     []ArchReference `yaml:"related_documents"`
	Figures              []ArchFigure    `yaml:"figures,omitempty"`
}

type ArchOverview struct {
	Summary             string `yaml:"summary"`
	Lifecycle           string `yaml:"lifecycle"`
	CoordinationPattern string `yaml:"coordination_pattern"`
}

// ArchInterface describes a named interface grouping with its
// data structures and operations.
type ArchInterface struct {
	Name           string   `yaml:"name"`
	Summary        string   `yaml:"summary"`
	DataStructures []string `yaml:"data_structures,omitempty"`
	Operations     []string `yaml:"operations,omitempty"`
}

type ArchComponent struct {
	Name           string   `yaml:"name"`
	Responsibility string   `yaml:"responsibility"`
	Capabilities   []string `yaml:"capabilities"`
	References     []string `yaml:"references,omitempty"`
}

type ArchDecision struct {
	ID                   int      `yaml:"id"`
	Title                string   `yaml:"title"`
	Decision             string   `yaml:"decision"`
	Benefits             []string `yaml:"benefits"`
	AlternativesRejected []string `yaml:"alternatives_rejected"`
}

type ArchTech struct {
	Component  string `yaml:"component"`
	Technology string `yaml:"technology"`
	Purpose    string `yaml:"purpose"`
}

type ArchPathRole struct {
	Path string `yaml:"path"`
	Role string `yaml:"role"`
}

type ArchImplStatus struct {
	CurrentFocus string              `yaml:"current_focus"`
	Progress     []map[string]string `yaml:"progress"`
}

// ArchReference is a related-document entry in ARCHITECTURE.yaml.
type ArchReference struct {
	Doc     string `yaml:"doc"`
	Purpose string `yaml:"purpose"`
}

// ArchFigure is an architecture diagram reference.
type ArchFigure struct {
	Path    string `yaml:"path"`
	Caption string `yaml:"caption"`
}

// ---------------------------------------------------------------------------
// Specifications
// ---------------------------------------------------------------------------

// SpecificationsDoc corresponds to docs/SPECIFICATIONS.yaml.
type SpecificationsDoc struct {
	ID                   string          `yaml:"id"`
	Title                string          `yaml:"title"`
	Overview             string          `yaml:"overview"`
	RoadmapSummary       []SpecRelease   `yaml:"roadmap_summary"`
	PRDIndex             []SpecIndex     `yaml:"prd_index"`
	UseCaseIndex         []SpecIndex     `yaml:"use_case_index"`
	TestSuiteIndex       []TestSuiteRef  `yaml:"test_suite_index"`
	PRDToUseCaseMapping  []PRDUseCaseMap `yaml:"prd_to_use_case_mapping"`
	TraceabilityDiagram  string          `yaml:"traceability_diagram,omitempty"`
	CoverageGaps         string          `yaml:"coverage_gaps"`
	References           []string        `yaml:"references,omitempty"`
}

type SpecRelease struct {
	Version       string `yaml:"version"`
	Name          string `yaml:"name"`
	UseCasesDone  int    `yaml:"use_cases_done"`
	UseCasesTotal int    `yaml:"use_cases_total"`
	Status        string `yaml:"status"`
}

type SpecIndex struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	Summary   string `yaml:"summary,omitempty"`
	Release   string `yaml:"release,omitempty"`
	Status    string `yaml:"status,omitempty"`
	TestSuite string `yaml:"test_suite,omitempty"`
	Path      string `yaml:"path,omitempty"`
}

type TestSuiteRef struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Release       string   `yaml:"release"`
	Traces        []string `yaml:"traces"`
	TestCaseCount int      `yaml:"test_case_count"`
	Path          string   `yaml:"path"`
}

type PRDUseCaseMap struct {
	UseCase     string `yaml:"use_case"`
	PRD         string `yaml:"prd"`
	WhyRequired string `yaml:"why_required"`
	Coverage    string `yaml:"coverage"`
}

// ---------------------------------------------------------------------------
// Roadmap
// ---------------------------------------------------------------------------

// RoadmapDoc corresponds to docs/road-map.yaml.
type RoadmapDoc struct {
	ID             string           `yaml:"id"`
	Title          string           `yaml:"title"`
	Releases       []RoadmapRelease `yaml:"releases"`
	Prioritization []string         `yaml:"prioritization,omitempty"`
}

type RoadmapRelease struct {
	Version     string           `yaml:"version"`
	Name        string           `yaml:"name"`
	Status      string           `yaml:"status"`
	Description string           `yaml:"description,omitempty"`
	UseCases    []RoadmapUseCase `yaml:"use_cases"`
}

type RoadmapUseCase struct {
	ID      string `yaml:"id"`
	Summary string `yaml:"summary,omitempty"`
	Status  string `yaml:"status"`
}

// ---------------------------------------------------------------------------
// Specs collection
// ---------------------------------------------------------------------------

// SpecsCollection groups product requirements, use cases, test suites,
// and supporting specs from docs/specs/.
type SpecsCollection struct {
	ProductRequirements []*PRDDoc       `yaml:"product_requirements,omitempty"`
	UseCases            []*UseCaseDoc   `yaml:"use_cases,omitempty"`
	TestSuites          []*TestSuiteDoc `yaml:"test_suites,omitempty"`
	DependencyMap       *NamedDoc       `yaml:"dependency_map,omitempty"`
	Sources             *NamedDoc       `yaml:"sources,omitempty"`
}

// ---------------------------------------------------------------------------
// PRD
// ---------------------------------------------------------------------------

// PRDDoc corresponds to docs/specs/product-requirements/prd*.yaml.
// Goals use "- G1: text" format (list of single-key maps).
// Requirements use a map keyed by group ID (R1, R2, ...).
// AcceptanceCriteria are plain strings.
type PRDDoc struct {
	ID                 string                        `yaml:"id"`
	Title              string                        `yaml:"title"`
	Problem            string                        `yaml:"problem"`
	Goals              []map[string]string           `yaml:"goals"`
	Requirements       map[string]PRDRequirementGroup `yaml:"requirements"`
	NonGoals           []string                      `yaml:"non_goals"`
	AcceptanceCriteria []string                      `yaml:"acceptance_criteria"`
	References         []string                      `yaml:"references,omitempty"`
}

// PRDRequirementGroup is a requirement section within a PRD.
// Items use "- R1.1: text" format (list of single-key maps).
type PRDRequirementGroup struct {
	Title string              `yaml:"title"`
	Items []map[string]string `yaml:"items"`
}

// ---------------------------------------------------------------------------
// Use case
// ---------------------------------------------------------------------------

// UseCaseDoc corresponds to docs/specs/use-cases/rel*.yaml.
// Flow, touchpoints, and success_criteria use "- KEY: text" format.
type UseCaseDoc struct {
	ID              string              `yaml:"id"`
	Title           string              `yaml:"title"`
	Summary         string              `yaml:"summary"`
	Actor           string              `yaml:"actor"`
	Trigger         string              `yaml:"trigger"`
	Flow            []map[string]string `yaml:"flow"`
	Touchpoints     []map[string]string `yaml:"touchpoints"`
	SuccessCriteria []map[string]string `yaml:"success_criteria"`
	Dependencies    []map[string]string `yaml:"dependencies,omitempty"`
	Risks           []map[string]string `yaml:"risks,omitempty"`
	OutOfScope      []string            `yaml:"out_of_scope"`
	TestSuite       string              `yaml:"test_suite,omitempty"`
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

// TestSuiteDoc corresponds to docs/specs/test-suites/test-rel*.yaml.
type TestSuiteDoc struct {
	ID            string     `yaml:"id"`
	Title         string     `yaml:"title"`
	Release       string     `yaml:"release"`
	Traces        []string   `yaml:"traces"`
	Tags          []string   `yaml:"tags,omitempty"`
	Preconditions []string   `yaml:"preconditions"`
	TestCases     []TestCase `yaml:"test_cases"`
	Cleanup       []string   `yaml:"cleanup,omitempty"`
}

// TestCase uses yaml.Node for Inputs and Expected because test suites
// across releases have different field schemas (CLI tests vs UI tests).
type TestCase struct {
	UseCase       string    `yaml:"use_case,omitempty"`
	Name          string    `yaml:"name"`
	Description   string    `yaml:"description,omitempty"`
	Inputs        yaml.Node `yaml:"inputs"`
	Normalization string    `yaml:"normalization,omitempty"`
	Expected      yaml.Node `yaml:"expected"`
	Traces        []string  `yaml:"traces,omitempty"`
}

// ---------------------------------------------------------------------------
// Engineering
// ---------------------------------------------------------------------------

// EngineeringDoc corresponds to docs/engineering/eng*.yaml.
// References are plain strings (file paths or document IDs).
type EngineeringDoc struct {
	ID           string       `yaml:"id"`
	Title        string       `yaml:"title"`
	Introduction string       `yaml:"introduction"`
	Sections     []DocSection `yaml:"sections"`
	References   []string     `yaml:"references,omitempty"`
}

type DocSection struct {
	Title   string `yaml:"title"`
	Content string `yaml:"content"`
}

// ---------------------------------------------------------------------------
// Constitutions
// ---------------------------------------------------------------------------

// ConstitutionsDoc holds the project's constitution files. Each
// constitution is stored as a yaml.Node to preserve its full schema
// without enumerating every field across different constitution types.
type ConstitutionsDoc struct {
	Design    *yaml.Node `yaml:"design,omitempty"`
	Planning  *yaml.Node `yaml:"planning,omitempty"`
	Execution *yaml.Node `yaml:"execution,omitempty"`
	GoStyle   *yaml.Node `yaml:"go_style,omitempty"`
}

// GoStyleDoc corresponds to docs/constitutions/go-style.yaml.
// It provides typed schema enforcement for the Go coding standards
// constitution via validateYAMLStrict.
type GoStyleDoc struct {
	CopyrightHeader         string           `yaml:"copyright_header"`
	Duplication             string           `yaml:"duplication"`
	DesignPatterns          []GoStylePattern `yaml:"design_patterns"`
	Interfaces              string           `yaml:"interfaces"`
	StructAndFunctionDesign string           `yaml:"struct_and_function_design"`
	ErrorHandling           string           `yaml:"error_handling"`
	NoMagicStrings          string           `yaml:"no_magic_strings"`
	ProjectStructure        string           `yaml:"project_structure"`
	StandardPackages        []string         `yaml:"standard_packages"`
	StructEmbedding         string           `yaml:"struct_embedding"`
	NamingConventions       []string         `yaml:"naming_conventions"`
	Concurrency             string           `yaml:"concurrency"`
	Testing                 string           `yaml:"testing"`
	CodeReviewChecklist     []string         `yaml:"code_review_checklist"`
}

// GoStylePattern represents a single design pattern entry in the Go style
// constitution. Symptoms is optional (not all patterns list symptoms).
type GoStylePattern struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Symptoms    string `yaml:"symptoms,omitempty"`
}

// ---------------------------------------------------------------------------
// Shared field types
// ---------------------------------------------------------------------------

// Phase represents an implementation phase in the vision document.
// Phase ID is a string (e.g. "01.0") and Deliverables is a scalar string.
type Phase struct {
	Phase        string `yaml:"phase"`
	Focus        string `yaml:"focus"`
	Deliverables string `yaml:"deliverables"`
}

type Risk struct {
	Risk       string `yaml:"risk"`
	Impact     string `yaml:"impact"`
	Likelihood string `yaml:"likelihood"`
	Mitigation string `yaml:"mitigation"`
}

// ContextIssue represents an issue tracker entry in the project context.
// It captures the fields needed for Claude to avoid creating duplicate
// issues during measure.
type ContextIssue struct {
	ID          string `yaml:"id"          json:"id"`
	Title       string `yaml:"title"       json:"title"`
	Status      string `yaml:"status"      json:"status"`
	Type        string `yaml:"type"        json:"type"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// NamedDoc wraps project-specific YAML files that don't have a fixed
// schema (e.g., utilities.yaml, sources.yaml). Content is stored as a
// yaml.Node to preserve the original structure.
type NamedDoc struct {
	Name    string    `yaml:"name"`
	Content yaml.Node `yaml:"content"`
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// loadYAML reads a YAML file and unmarshals it into T.
// Returns nil if the file does not exist or cannot be parsed.
func loadYAML[T any](path string) *T {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		logf("loadYAML: parse error for %s: %v", path, err)
		return nil
	}
	return &v
}

// loadYAMLDir globs for YAML files matching a pattern, loads each into T,
// and returns them sorted by filename.
func loadYAMLDir[T any](glob string) []*T {
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	var result []*T
	for _, path := range matches {
		v := loadYAML[T](path)
		if v != nil {
			result = append(result, v)
		}
	}
	return result
}

// loadYAMLNode reads a YAML file into a yaml.Node, preserving the
// original structure without requiring a typed struct.
func loadYAMLNode(path string) *yaml.Node {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		logf("loadYAMLNode: parse error for %s: %v", path, err)
		return nil
	}
	// yaml.Unmarshal wraps content in a DocumentNode; extract the inner node.
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &doc
}

// loadNamedDoc reads a YAML file into a NamedDoc, using the filename
// stem (without extension) as the Name.
func loadNamedDoc(path string) *NamedDoc {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var content yaml.Node
	if err := yaml.Unmarshal(data, &content); err != nil {
		logf("loadNamedDoc: parse error for %s: %v", path, err)
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	node := &content
	if content.Kind == yaml.DocumentNode && len(content.Content) > 0 {
		node = content.Content[0]
	}
	return &NamedDoc{Name: name, Content: *node}
}

// knownDocFiles lists the docs/ files loaded into typed structs.
// loadExtraDocs skips these to avoid duplication.
var knownDocFiles = map[string]bool{
	"VISION.yaml":         true,
	"ARCHITECTURE.yaml":   true,
	"SPECIFICATIONS.yaml": true,
	"road-map.yaml":       true,
}

// loadExtraDocs loads any YAML files in dir that are not already
// handled by typed struct loading (not in knownDocFiles).
func loadExtraDocs(dir string) []*NamedDoc {
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	var result []*NamedDoc
	for _, path := range matches {
		base := filepath.Base(path)
		if knownDocFiles[base] {
			continue
		}
		doc := loadNamedDoc(path)
		if doc != nil {
			result = append(result, doc)
		}
	}
	return result
}

// parseIssuesJSON converts a JSON array of issue tracker entries into typed
// ContextIssue values for inclusion in the project context.
func parseIssuesJSON(jsonStr string) []ContextIssue {
	if jsonStr == "" || jsonStr == "[]" {
		return nil
	}
	var issues []ContextIssue
	if err := json.Unmarshal([]byte(jsonStr), &issues); err != nil {
		logf("parseIssuesJSON: parse error: %v", err)
		return nil
	}
	return issues
}

// loadSourceFiles walks the given directories and reads all .go files,
// returning them sorted by path for deterministic prompt output.
func loadSourceFiles(dirs []string) []SourceFile {
	var files []SourceFile
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				logf("loadSourceFiles: read error for %s: %v", path, readErr)
				return nil
			}
			files = append(files, SourceFile{
				Path:    path,
				Content: string(data),
			})
			return nil
		})
		if err != nil {
			logf("loadSourceFiles: walk error for %s: %v", dir, err)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	logf("loadSourceFiles: %d file(s) from %d dir(s)", len(files), len(dirs))
	return files
}

// ---------------------------------------------------------------------------
// Assembly
// ---------------------------------------------------------------------------

// buildProjectContext reads all docs/ files and source code, parses
// existing issues, and assembles them into a single YAML document for
// prompt injection.
func buildProjectContext(existingIssuesJSON string, goSourceDirs []string) (string, error) {
	ctx := &ProjectContext{}

	ctx.Vision = loadYAML[VisionDoc]("docs/VISION.yaml")
	ctx.Architecture = loadYAML[ArchitectureDoc]("docs/ARCHITECTURE.yaml")
	ctx.Specifications = loadYAML[SpecificationsDoc]("docs/SPECIFICATIONS.yaml")
	ctx.Roadmap = loadYAML[RoadmapDoc]("docs/road-map.yaml")

	ctx.Specs = &SpecsCollection{
		ProductRequirements: loadYAMLDir[PRDDoc]("docs/specs/product-requirements/prd*.yaml"),
		UseCases:            loadYAMLDir[UseCaseDoc]("docs/specs/use-cases/rel*.yaml"),
		TestSuites:          loadYAMLDir[TestSuiteDoc]("docs/specs/test-suites/test-rel*.yaml"),
		DependencyMap:       loadNamedDoc("docs/specs/dependency-map.yaml"),
		Sources:             loadNamedDoc("docs/specs/sources.yaml"),
	}
	// Omit empty specs collection.
	if ctx.Specs.ProductRequirements == nil && ctx.Specs.UseCases == nil &&
		ctx.Specs.TestSuites == nil && ctx.Specs.DependencyMap == nil &&
		ctx.Specs.Sources == nil {
		ctx.Specs = nil
	}

	ctx.Engineering = loadYAMLDir[EngineeringDoc]("docs/engineering/eng*.yaml")

	ctx.Constitutions = &ConstitutionsDoc{
		Design:    loadYAMLNode("docs/constitutions/design.yaml"),
		Planning:  loadYAMLNode("docs/constitutions/planning.yaml"),
		Execution: loadYAMLNode("docs/constitutions/execution.yaml"),
		GoStyle:   loadYAMLNode("docs/constitutions/go-style.yaml"),
	}
	if ctx.Constitutions.Design == nil && ctx.Constitutions.Planning == nil &&
		ctx.Constitutions.Execution == nil && ctx.Constitutions.GoStyle == nil {
		ctx.Constitutions = nil
	}

	ctx.Extra = loadExtraDocs("docs/")
	ctx.SourceCode = loadSourceFiles(goSourceDirs)
	ctx.Issues = parseIssuesJSON(existingIssuesJSON)

	out, err := yaml.Marshal(ctx)
	if err != nil {
		logf("buildProjectContext: marshal error: %v", err)
		return "", err
	}
	logf("buildProjectContext: %d bytes, vision=%v arch=%v roadmap=%v specs=%v eng=%d const=%v issues=%d extra=%d src=%d",
		len(out),
		ctx.Vision != nil,
		ctx.Architecture != nil,
		ctx.Roadmap != nil,
		ctx.Specs != nil,
		len(ctx.Engineering),
		ctx.Constitutions != nil,
		len(ctx.Issues),
		len(ctx.Extra),
		len(ctx.SourceCode),
	)
	return string(out), nil
}
