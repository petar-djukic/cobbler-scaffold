// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// PhaseContext: per-phase context overrides (prd003 R9)
// ---------------------------------------------------------------------------

// PhaseContext holds per-phase context specification that overrides
// ProjectConfig fields when loaded from measure_context.yaml or
// stitch_context.yaml. See eng08-phase-context-files for design.
type PhaseContext struct {
	Include string `yaml:"include"`
	Exclude string `yaml:"exclude"`
	Sources string `yaml:"sources"`
	Release string `yaml:"release"`
}

// loadPhaseContext reads a phase context YAML file. Returns (nil, nil)
// when the file does not exist (caller falls back to Config). Returns
// an error when the file exists but is malformed.
func loadPhaseContext(path string) (*PhaseContext, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading phase context %s: %w", path, err)
	}
	var pc PhaseContext
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("parsing phase context %s: %w", path, err)
	}
	return &pc, nil
}

// defaultMeasureContext is the scaffold template for measure_context.yaml.
const defaultMeasureContext = `# Phase context for the measure workflow.
# Fields override the corresponding configuration.yaml settings when present.
# Remove or leave fields empty to use configuration.yaml defaults.
# See docs/engineering/eng08-phase-context-files.yaml for schema details.

# include: |
#   docs/VISION.yaml
#   docs/ARCHITECTURE.yaml
#   docs/specs/**/*.yaml

# exclude: |
#   docs/engineering/*

# sources: |
#   docs/constitutions/*.yaml

# release: "01.0"
`

// defaultStitchContext is the scaffold template for stitch_context.yaml.
const defaultStitchContext = `# Phase context for the stitch workflow.
# Fields override the corresponding configuration.yaml settings when present.
# Remove or leave fields empty to use configuration.yaml defaults.
# See docs/engineering/eng08-phase-context-files.yaml for schema details.

# include: |
#   docs/ARCHITECTURE.yaml
#   docs/specs/product-requirements/prd*.yaml

# exclude: |
#   docs/engineering/*

# sources: |
#   docs/constitutions/*.yaml

# release: "01.0"
`

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
	Analysis       *AnalysisDoc       `yaml:"analysis,omitempty"`
	SourceCode     []SourceFile       `yaml:"source_code,omitempty"`
	Issues         []ContextIssue     `yaml:"issues,omitempty"`
	CompletedWork  []string           `yaml:"completed_work,omitempty"`
	Extra          []*NamedDoc        `yaml:"extra,omitempty"`
}

// SourceFile holds a source file for inclusion in the project context.
// Lines are formatted as "{number} | {content}", with blank lines omitted.
type SourceFile struct {
	File  string `yaml:"file"`
	Lines string `yaml:"lines"`
}

// ---------------------------------------------------------------------------
// Vision
// ---------------------------------------------------------------------------

// VisionDoc corresponds to docs/VISION.yaml.
type VisionDoc struct {
	File                 string            `yaml:"file,omitempty"`
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
	File                 string          `yaml:"file,omitempty"`
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
	File                 string          `yaml:"file,omitempty"`
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
	File           string           `yaml:"file,omitempty"`
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
	File               string                        `yaml:"file,omitempty"`
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
	File            string              `yaml:"file,omitempty"`
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
	File          string     `yaml:"file,omitempty"`
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
	GoTest        string    `yaml:"go_test,omitempty"`
	CoveredBy     string    `yaml:"covered_by,omitempty"`
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
	File         string       `yaml:"file,omitempty"`
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
// Go style (used by analyze.go validateYAMLStrict)
// ---------------------------------------------------------------------------

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
// Design constitution (docs/constitutions/design.yaml)
// ---------------------------------------------------------------------------

// ConstitutionArticle is a single article entry shared by all constitution
// files that use the articles list pattern.
type ConstitutionArticle struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	Rule  string `yaml:"rule"`
}

// DesignDoc corresponds to docs/constitutions/design.yaml.
type DesignDoc struct {
	Articles               []ConstitutionArticle    `yaml:"articles"`
	DocumentationStandards DesignStandards          `yaml:"documentation_standards"`
	DocumentTypes          map[string]DesignDocType `yaml:"document_types"`
	NamingConventions      map[string]string        `yaml:"naming_conventions"`
	WritingGuidelines      map[string]string        `yaml:"writing_guidelines"`
	TraceabilityModel      map[string]string        `yaml:"traceability_model"`
	CompletenessChecklists map[string][]string      `yaml:"completeness_checklists"`
}

// DesignStandards holds the documentation_standards section.
type DesignStandards struct {
	Style              string           `yaml:"style"`
	Voice              string           `yaml:"voice"`
	ForbiddenTerms     []string         `yaml:"forbidden_terms"`
	Formatting         DesignFormatting `yaml:"formatting"`
	FiguresAndDiagrams string           `yaml:"figures_and_diagrams"`
}

// DesignFormatting holds paragraph, list, and abbreviation rules.
type DesignFormatting struct {
	Paragraphs     string `yaml:"paragraphs"`
	ListsAndTables string `yaml:"lists_and_tables"`
	Abbreviations  string `yaml:"abbreviations"`
}

// DesignDocType describes one entry in the document_types map.
type DesignDocType struct {
	Location       string            `yaml:"location,omitempty"`
	FormatRule     string            `yaml:"format_rule,omitempty"`
	RequiredFields []string          `yaml:"required_fields,omitempty"`
	OptionalFields []string          `yaml:"optional_fields,omitempty"`
	Numbering      map[string]string `yaml:"numbering,omitempty"`
	Purpose        string            `yaml:"purpose,omitempty"`
}

// ---------------------------------------------------------------------------
// Execution constitution (docs/constitutions/execution.yaml)
// ---------------------------------------------------------------------------

// ExecutionDoc corresponds to docs/constitutions/execution.yaml.
type ExecutionDoc struct {
	Articles          []ConstitutionArticle `yaml:"articles"`
	CodingStandards   ExecCodingStandards   `yaml:"coding_standards"`
	Traceability      ExecTraceability      `yaml:"traceability"`
	SessionCompletion ExecSessionCompletion `yaml:"session_completion"`
	Technology        ExecTechnology        `yaml:"technology"`
	GitConventions    ExecGitConventions    `yaml:"git_conventions"`
}

// ExecCodingStandards holds the coding_standards section.
type ExecCodingStandards struct {
	CopyrightHeader         string               `yaml:"copyright_header"`
	NeverDuplicateCode      string               `yaml:"never_duplicate_code"`
	DesignPatterns          []map[string]string   `yaml:"design_patterns"`
	Interfaces              string               `yaml:"interfaces"`
	StructAndFunctionDesign string               `yaml:"struct_and_function_design"`
	ErrorHandling           string               `yaml:"error_handling"`
	NoMagicStrings          string               `yaml:"no_magic_strings"`
	ProjectStructure        string               `yaml:"project_structure"`
	StandardPackages        []string             `yaml:"standard_packages"`
	NamingConventions       ExecNamingConventions `yaml:"naming_conventions"`
	Concurrency             string               `yaml:"concurrency"`
	Testing                 string               `yaml:"testing"`
}

// ExecNamingConventions holds the naming convention entries.
type ExecNamingConventions struct {
	Exported        string `yaml:"exported"`
	Unexported      string `yaml:"unexported"`
	CLIFlags        string `yaml:"cli_flags"`
	BinaryConstants string `yaml:"binary_constants"`
	Factories       string `yaml:"factories"`
	Interfaces      string `yaml:"interfaces"`
}

// ExecTraceability holds the traceability section.
type ExecTraceability struct {
	BeforeImplementing []string `yaml:"before_implementing"`
	CommitMessage      string   `yaml:"commit_message"`
	CodeComments       string   `yaml:"code_comments"`
	ReferencePaths     []string `yaml:"reference_paths"`
}

// ExecSessionCompletion holds the session_completion section.
type ExecSessionCompletion struct {
	GitManagedExternally bool     `yaml:"git_managed_externally"`
	TokenTracking        bool     `yaml:"token_tracking"`
	Workflow             []string `yaml:"workflow"`
	CriticalRules        []string `yaml:"critical_rules"`
}

// ExecTechnology holds the technology section.
type ExecTechnology struct {
	PrimaryLanguage string `yaml:"primary_language"`
	PythonManager   string `yaml:"python_manager"`
	CLIFramework    string `yaml:"cli_framework"`
	BuildSystem     string `yaml:"build_system"`
	YAMLLibrary     string `yaml:"yaml_library"`
	IssueTracker    string `yaml:"issue_tracker"`
}

// ExecGitConventions holds the git_conventions section.
type ExecGitConventions struct {
	Note string `yaml:"note"`
}

// ---------------------------------------------------------------------------
// Planning constitution (docs/constitutions/planning.yaml)
// ---------------------------------------------------------------------------

// PlanningDoc corresponds to docs/constitutions/planning.yaml.
type PlanningDoc struct {
	Articles                  []ConstitutionArticle  `yaml:"articles"`
	IssueStructure            PlanningIssueStructure `yaml:"issue_structure"`
	ExampleDocumentationIssue string                 `yaml:"example_documentation_issue"`
	ExampleCodeIssue          string                 `yaml:"example_code_issue"`
}

// PlanningIssueStructure holds the issue_structure section.
type PlanningIssueStructure struct {
	CommonFields        map[string]PlanningFieldDef `yaml:"common_fields"`
	DocumentationIssues PlanningDocIssues           `yaml:"documentation_issues"`
	YAMLQuality         []string                   `yaml:"yaml_quality"`
	CodeIssues          PlanningCodeIssues          `yaml:"code_issues"`
}

// PlanningFieldDef describes a single field in the issue schema.
type PlanningFieldDef struct {
	Required    bool     `yaml:"required"`
	Values      []string `yaml:"values,omitempty"`
	Description string   `yaml:"description"`
}

// PlanningDocIssues holds the documentation_issues sub-section.
type PlanningDocIssues struct {
	AdditionalFields map[string]PlanningFieldDef `yaml:"additional_fields"`
	DeliverableTypes []PlanningDeliverableType   `yaml:"deliverable_types"`
}

// PlanningDeliverableType describes a documentation deliverable type.
type PlanningDeliverableType struct {
	Type       string `yaml:"type"`
	Location   string `yaml:"location"`
	FormatRule string `yaml:"format_rule"`
	When       string `yaml:"when"`
}

// PlanningCodeIssues holds the code_issues sub-section.
type PlanningCodeIssues struct {
	Rules []string `yaml:"rules"`
}

// ---------------------------------------------------------------------------
// Testing constitution (docs/constitutions/testing.yaml)
// ---------------------------------------------------------------------------

// TestingDoc corresponds to docs/constitutions/testing.yaml.
type TestingDoc struct {
	Articles []ConstitutionArticle `yaml:"articles"`
}

// ---------------------------------------------------------------------------
// Issue format constitution (pkg/orchestrator/constitutions/issue-format.yaml)
// ---------------------------------------------------------------------------

// IssueFormatDoc corresponds to pkg/orchestrator/constitutions/issue-format.yaml.
type IssueFormatDoc struct {
	Schema     IssueFormatSchema           `yaml:"schema"`
	YAMLRules  []IssueFormatRule           `yaml:"yaml_rules"`
	FieldSpecs map[string]IssueFormatField `yaml:"field_specs"`
	Examples   map[string]string           `yaml:"examples"`
}

// IssueFormatSchema holds the schema section.
type IssueFormatSchema struct {
	Description    string   `yaml:"description"`
	RequiredFields []string `yaml:"required_fields"`
	OptionalFields []string `yaml:"optional_fields"`
}

// IssueFormatRule holds a single YAML formatting rule with examples.
type IssueFormatRule struct {
	Rule        string `yaml:"rule"`
	ExampleBad  string `yaml:"example_bad"`
	ExampleGood string `yaml:"example_good"`
}

// IssueFormatField describes a single field in the issue format spec.
type IssueFormatField struct {
	Type        string                      `yaml:"type"`
	Values      []string                    `yaml:"values,omitempty"`
	Required    bool                        `yaml:"required"`
	Description string                      `yaml:"description"`
	SubFields   map[string]IssueFormatField `yaml:"sub_fields,omitempty"`
}

// ---------------------------------------------------------------------------
// Shared field types
// ---------------------------------------------------------------------------

// Phase represents an implementation phase in the vision document.
// Phase ID is a string (e.g. "1") and Deliverables is a scalar string.
type Phase struct {
	Phase        string `yaml:"phase"`
	Focus        string `yaml:"focus"`
	Deliverables string `yaml:"deliverables"`
}

type Risk struct {
	ID         string `yaml:"id,omitempty"`
	Risk       string `yaml:"risk"`
	Impact     string `yaml:"impact"`
	Likelihood string `yaml:"likelihood"`
	Mitigation string `yaml:"mitigation"`
}

// ContextIssue represents an issue tracker entry in the project context.
// It captures the fields needed for Claude to avoid creating duplicate
// issues during measure.
type ContextIssue struct {
	ID     string `yaml:"id"     json:"id"`
	Title  string `yaml:"title"  json:"title"`
	Status string `yaml:"status" json:"status"`
	Type   string `yaml:"type"   json:"type"`
}

// NamedDoc wraps project-specific YAML files that don't have a fixed
// schema (e.g., utilities.yaml, sources.yaml). Content is stored as a
// yaml.Node to preserve the original structure.
type NamedDoc struct {
	File    string    `yaml:"file,omitempty"`
	Name    string    `yaml:"name"`
	Content yaml.Node `yaml:"content"`
}

// ---------------------------------------------------------------------------
// Source file filtering (selective stitch context, eng05 rec D)
// ---------------------------------------------------------------------------

// stripParenthetical removes a trailing parenthetical note from a path.
// "pkg/types/cupboard.go (CrumbTable interface)" becomes "pkg/types/cupboard.go".
func stripParenthetical(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.LastIndex(s, "("); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

// sourceFileMatchesAny returns true if the source file's path ends with
// any of the given suffixes. Used by both filterSourceFiles and
// applyContextBudget for consistent suffix matching.
func sourceFileMatchesAny(sf SourceFile, suffixes []string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(sf.File, s) {
			return true
		}
	}
	return false
}

// filterSourceFiles returns only the source files whose paths match any of
// the requiredPaths using suffix matching. A required path matches if any
// SourceFile.File ends with it. If requiredPaths is empty, all source files
// are returned (backward-compatible fallback).
func filterSourceFiles(sources []SourceFile, requiredPaths []string) []SourceFile {
	if len(requiredPaths) == 0 {
		return sources
	}

	var filtered []SourceFile
	for _, src := range sources {
		if sourceFileMatchesAny(src, requiredPaths) {
			filtered = append(filtered, src)
		}
	}
	return filtered
}

// applyContextBudget measures the YAML-serialized size of ctx and, if it
// exceeds budget, progressively removes SourceCode entries not in
// requiredPaths until within budget. Files are removed in reverse order
// (last loaded first) to preserve files closer to the top of the directory
// tree. When budget is 0 or negative, this function is a no-op.
func applyContextBudget(ctx *ProjectContext, budget int, requiredPaths []string) {
	if budget <= 0 || ctx == nil {
		return
	}

	data, err := yaml.Marshal(ctx)
	if err != nil {
		logf("applyContextBudget: marshal error: %v", err)
		return
	}
	before := len(data)
	if before <= budget {
		logf("applyContextBudget: context size %d <= budget %d, no truncation needed", before, budget)
		return
	}

	removed := 0
	for len(ctx.SourceCode) > 0 && len(data) > budget {
		// Find the last non-required source file.
		idx := -1
		for i := len(ctx.SourceCode) - 1; i >= 0; i-- {
			if !sourceFileMatchesAny(ctx.SourceCode[i], requiredPaths) {
				idx = i
				break
			}
		}
		if idx < 0 {
			break // all remaining files are required
		}
		ctx.SourceCode = append(ctx.SourceCode[:idx], ctx.SourceCode[idx+1:]...)
		removed++

		data, err = yaml.Marshal(ctx)
		if err != nil {
			logf("applyContextBudget: re-marshal error: %v", err)
			return
		}
	}

	logf("applyContextBudget: context size %d -> %d, removed %d source file(s)", before, len(data), removed)
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

// loadNamedDoc reads a YAML file into a NamedDoc, using the filename
// stem (without extension) as the Name.
func loadNamedDoc(path string) *NamedDoc {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	var content yaml.Node
	if err := yaml.Unmarshal(data, &content); err != nil {
		// Non-YAML files (markdown, text) are loaded as a scalar node
		// containing the raw file content, instead of being silently dropped.
		ext := filepath.Ext(path)
		if ext == ".md" || ext == ".txt" || ext == ".json" {
			return &NamedDoc{
				Name: name,
				Content: yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!!str",
					Value: string(data),
				},
			}
		}
		logf("loadNamedDoc: parse error for %s: %v", path, err)
		return nil
	}
	node := &content
	if content.Kind == yaml.DocumentNode && len(content.Content) > 0 {
		node = content.Content[0]
	}
	return &NamedDoc{Name: name, Content: *node}
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

// numberLines formats source file content as a single string of
// "{number} | {line}" entries joined by newlines. Blank lines are omitted;
// gaps in numbering indicate their positions. yaml.v3 renders the result
// as a block scalar, saving tokens compared to a YAML list.
func numberLines(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			break // trailing newline from Split
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		result = append(result, fmt.Sprintf("%d | %s", i+1, line))
	}
	return strings.Join(result, "\n")
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
				File:  path,
				Lines: numberLines(string(data)),
			})
			return nil
		})
		if err != nil {
			logf("loadSourceFiles: walk error for %s: %v", dir, err)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
	logf("loadSourceFiles: %d file(s) from %d dir(s)", len(files), len(dirs))
	return files
}

// ---------------------------------------------------------------------------
// Context source resolution
// ---------------------------------------------------------------------------

// parseContextSources splits a newline-delimited text block into
// individual glob patterns, trimming whitespace and skipping blanks
// and comment lines (starting with #).
func parseContextSources(text string) []string {
	var patterns []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// resolveContextSources expands glob patterns from ContextSources into
// a deduplicated, sorted list of real file paths. Duplicate files
// (matched by multiple patterns) are logged and removed.
func resolveContextSources(sources string) []string {
	patterns := parseContextSources(sources)
	seen := make(map[string]string) // path -> first pattern that matched
	var files []string

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logf("resolveContextSources: bad glob %q: %v", pattern, err)
			continue
		}
		for _, path := range matches {
			if _, err := os.Stat(path); err != nil {
				continue
			}
			if prev, dup := seen[path]; dup {
				logf("resolveContextSources: duplicate %s (matched by %q and %q)", path, prev, pattern)
				continue
			}
			seen[path] = pattern
			files = append(files, path)
		}
	}

	sort.Strings(files)
	logf("resolveContextSources: %d pattern(s) -> %d file(s)", len(patterns), len(files))
	return files
}

// resolveFileSet expands newline-delimited glob patterns into a set of
// file paths. Directory matches are walked recursively so that excluding
// a directory excludes all files underneath it.
func resolveFileSet(text string) map[string]bool {
	patterns := parseContextSources(text)
	set := make(map[string]bool)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logf("resolveFileSet: bad glob %q: %v", pattern, err)
			continue
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				continue
			}
			if info.IsDir() {
				filepath.WalkDir(m, func(p string, d fs.DirEntry, err error) error {
					if err == nil && !d.IsDir() {
						set[p] = true
					}
					return nil
				})
			} else {
				set[m] = true
			}
		}
	}
	logf("resolveFileSet: %d pattern(s) -> %d file(s)", len(patterns), len(set))
	return set
}

// classifyContextFile determines how a file should be loaded based on
// its path. Returns a category string used by buildProjectContext to
// dispatch to the appropriate typed loader.
func classifyContextFile(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	switch {
	case dir == "docs" && base == "VISION.yaml":
		return "vision"
	case dir == "docs" && base == "ARCHITECTURE.yaml":
		return "architecture"
	case dir == "docs" && base == "SPECIFICATIONS.yaml":
		return "specifications"
	case dir == "docs" && base == "road-map.yaml":
		return "roadmap"
	case dir == filepath.Join("docs", "specs", "product-requirements"):
		return "prd"
	case dir == filepath.Join("docs", "specs", "use-cases"):
		return "use_case"
	case dir == filepath.Join("docs", "specs", "test-suites"):
		return "test_suite"
	case dir == filepath.Join("docs", "specs"):
		return "spec_aux"
	case dir == filepath.Join("docs", "engineering"):
		return "engineering"
	case dir == filepath.Join("docs", "constitutions"):
		return "constitution"
	default:
		return "extra"
	}
}

// ---------------------------------------------------------------------------
// Standard document structure
// ---------------------------------------------------------------------------

// standardContextPatterns lists the glob patterns for the standard
// documentation structure. These are loaded automatically by
// resolveStandardFiles. Does NOT include docs/constitutions/*.yaml
// (constitutions are injected separately as top-level prompt keys)
// or docs/*.yaml (catchall that pulled in utilities.yaml).
var standardContextPatterns = []string{
	"docs/VISION.yaml",
	"docs/ARCHITECTURE.yaml",
	"docs/SPECIFICATIONS.yaml",
	"docs/road-map.yaml",
	"docs/specs/product-requirements/prd*.yaml",
	"docs/specs/use-cases/rel*.yaml",
	"docs/specs/test-suites/test-rel*.yaml",
	"docs/specs/dependency-map.yaml",
	"docs/specs/sources.yaml",
}

// typedDocPaths lists documents that must always be loaded through their
// dedicated typed parsers (VisionDoc, ArchitectureDoc, etc.), even when
// context_include replaces the standard discovery. If context_include
// patterns don't cover these paths but the files exist on disk, they are
// added to the resolved set so classifyContextFile routes them correctly.
var typedDocPaths = []string{
	"docs/VISION.yaml",
	"docs/ARCHITECTURE.yaml",
	"docs/SPECIFICATIONS.yaml",
	"docs/road-map.yaml",
}

// ensureTypedDocs merges the always-load typed document paths into the
// file list if they exist on disk but are not already present.
func ensureTypedDocs(files []string) []string {
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
	}
	for _, path := range typedDocPaths {
		if present[path] {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
			logf("ensureTypedDocs: added missing typed doc %s", path)
		}
	}
	return files
}

// resolveStandardFiles expands standardContextPatterns into a
// deduplicated, sorted list of real file paths.
func resolveStandardFiles() []string {
	seen := make(map[string]bool)
	var files []string
	for _, pattern := range standardContextPatterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logf("resolveStandardFiles: bad glob %q: %v", pattern, err)
			continue
		}
		for _, path := range matches {
			if _, err := os.Stat(path); err != nil {
				continue
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			files = append(files, path)
		}
	}
	sort.Strings(files)
	logf("resolveStandardFiles: %d pattern(s) -> %d file(s)", len(standardContextPatterns), len(files))
	return files
}

// releaseFilter controls which use-case and test-suite files are included
// based on their release version. When ReleaseSet is non-nil, only files
// whose extracted release is in the set pass. When ReleaseSet is nil but
// MaxRelease is non-empty, the legacy <= comparison is used. When both are
// empty/nil, all files pass (no filtering).
type releaseFilter struct {
	ReleaseSet map[string]bool // explicit set of in-scope releases
	MaxRelease string          // legacy: include files with release <= this value
}

// newReleaseFilter builds a releaseFilter from the effective config fields.
// Releases (list) takes precedence over Release (single string).
func newReleaseFilter(releases []string, release string) releaseFilter {
	if len(releases) > 0 {
		set := make(map[string]bool, len(releases))
		for _, r := range releases {
			set[r] = true
		}
		return releaseFilter{ReleaseSet: set}
	}
	return releaseFilter{MaxRelease: release}
}

// active reports whether any release filtering is in effect.
func (rf releaseFilter) active() bool {
	return len(rf.ReleaseSet) > 0 || rf.MaxRelease != ""
}

// extractFileRelease extracts the release version from a use-case or
// test-suite filename. Returns "" if the release cannot be determined.
func extractFileRelease(path string) string {
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(base, "rel"):
		// rel01.0-uc001-... → extract "01.0"
		rest := strings.TrimPrefix(base, "rel")
		if idx := strings.Index(rest, "-"); idx > 0 {
			return rest[:idx]
		}
	case strings.HasPrefix(base, "test-rel"):
		// test-rel01.0.yaml → extract "01.0"
		rest := strings.TrimPrefix(base, "test-rel")
		return strings.TrimSuffix(rest, ".yaml")
	}
	return ""
}

// fileMatchesRelease returns true if the file's release passes the filter.
// Returns true if no filter is active or if the file's release cannot be
// determined. String comparison works for the zero-padded "NN.N" format.
func fileMatchesRelease(path string, rf releaseFilter) bool {
	if !rf.active() {
		return true
	}
	fileRelease := extractFileRelease(path)
	if fileRelease == "" {
		return true
	}
	if rf.ReleaseSet != nil {
		return rf.ReleaseSet[fileRelease]
	}
	return fileRelease <= rf.MaxRelease
}

// prdIDsFromUseCases extracts PRD IDs referenced by touchpoints in the
// given use cases. Returns a set of PRD filename stems (e.g., "prd001-feature").
func prdIDsFromUseCases(useCases []*UseCaseDoc) map[string]bool {
	if len(useCases) == 0 {
		return nil
	}
	ids := make(map[string]bool)
	for _, uc := range useCases {
		for _, tp := range uc.Touchpoints {
			for _, text := range tp {
				for _, word := range strings.Fields(text) {
					word = strings.TrimSuffix(strings.TrimPrefix(word, "("), ")")
					if strings.HasPrefix(word, "prd") {
						ids[word] = true
					}
				}
			}
		}
	}
	return ids
}

// loadContextFileInto loads a single file into the appropriate field
// of ctx based on its classified category. Applies release filtering
// for use_case and test_suite categories. Does not handle constitution
// or extra categories.
func loadContextFileInto(ctx *ProjectContext, path string, rf releaseFilter) {
	switch classifyContextFile(path) {
	case "vision":
		if v := loadYAML[VisionDoc](path); v != nil {
			v.File = path
			ctx.Vision = v
		}
	case "architecture":
		if v := loadYAML[ArchitectureDoc](path); v != nil {
			v.File = path
			ctx.Architecture = v
		}
	case "specifications":
		if v := loadYAML[SpecificationsDoc](path); v != nil {
			v.File = path
			ctx.Specifications = v
		}
	case "roadmap":
		if v := loadYAML[RoadmapDoc](path); v != nil {
			v.File = path
			ctx.Roadmap = v
		}
	case "use_case":
		if !fileMatchesRelease(path, rf) {
			return
		}
		if v := loadYAML[UseCaseDoc](path); v != nil {
			v.File = path
			ctx.Specs.UseCases = append(ctx.Specs.UseCases, v)
		}
	case "test_suite":
		if !fileMatchesRelease(path, rf) {
			return
		}
		if v := loadYAML[TestSuiteDoc](path); v != nil {
			v.File = path
			ctx.Specs.TestSuites = append(ctx.Specs.TestSuites, v)
		}
	case "spec_aux":
		if v := loadNamedDoc(path); v != nil {
			v.File = path
			switch filepath.Base(path) {
			case "dependency-map.yaml":
				ctx.Specs.DependencyMap = v
			case "sources.yaml":
				ctx.Specs.Sources = v
			default:
				ctx.Extra = append(ctx.Extra, v)
			}
		}
	case "engineering":
		if v := loadYAML[EngineeringDoc](path); v != nil {
			v.File = path
			ctx.Engineering = append(ctx.Engineering, v)
		}
	case "extra":
		if v := loadNamedDoc(path); v != nil {
			v.File = path
			ctx.Extra = append(ctx.Extra, v)
		}
	}
}

// ---------------------------------------------------------------------------
// Assembly
// ---------------------------------------------------------------------------

// buildProjectContext resolves the standard document structure and any
// extra context sources, reads all matching files and source code, parses
// existing issues, and assembles them into a ProjectContext struct.
// The project config controls include/exclude filtering and release scoping.
// When phaseCtx is non-nil, its non-empty fields override the corresponding
// ProjectConfig fields (prd003 R9.5-R9.7).
func buildProjectContext(existingIssuesJSON string, project ProjectConfig, phaseCtx *PhaseContext) (*ProjectContext, error) {
	ctx := &ProjectContext{}
	ctx.Specs = &SpecsCollection{}

	// Resolve effective settings: PhaseContext overrides when present.
	ctxInclude := project.ContextInclude
	ctxExclude := project.ContextExclude
	ctxSources := project.ContextSources
	releases := project.Releases
	release := project.Release
	if phaseCtx != nil {
		if phaseCtx.Include != "" {
			ctxInclude = phaseCtx.Include
		}
		if phaseCtx.Exclude != "" {
			ctxExclude = phaseCtx.Exclude
		}
		if phaseCtx.Sources != "" {
			ctxSources = phaseCtx.Sources
		}
		if phaseCtx.Release != "" {
			// PhaseContext single release overrides both Releases and Release.
			releases = nil
			release = phaseCtx.Release
		}
	}
	rf := newReleaseFilter(releases, release)

	// Compute exclude set when configured.
	var excludeSet map[string]bool
	if strings.TrimSpace(ctxExclude) != "" {
		excludeSet = resolveFileSet(ctxExclude)
		logf("buildProjectContext: exclude set has %d file(s)", len(excludeSet))
	}

	// Resolve document files: use ContextInclude when set, otherwise
	// fall back to the standard document discovery.
	var docFiles []string
	if strings.TrimSpace(ctxInclude) != "" {
		docFiles = resolveContextSources(ctxInclude)
		// Ensure core typed documents (Vision, Architecture, Roadmap) are
		// always present so they go through dedicated parsers rather than
		// falling into the generic loadNamedDoc path.
		docFiles = ensureTypedDocs(docFiles)
		logf("buildProjectContext: using context_include (%d file(s))", len(docFiles))
	} else {
		docFiles = resolveStandardFiles()
	}

	// Filter through exclude set.
	if excludeSet != nil {
		var filtered []string
		for _, f := range docFiles {
			if !excludeSet[f] {
				filtered = append(filtered, f)
			}
		}
		docFiles = filtered
	}

	standardSet := make(map[string]bool, len(docFiles))
	var prdPaths []string

	for _, path := range docFiles {
		standardSet[path] = true
		if classifyContextFile(path) == "prd" {
			prdPaths = append(prdPaths, path)
			continue
		}
		loadContextFileInto(ctx, path, rf)
	}

	// Load PRDs filtered by release: when a release filter is active, only
	// include PRDs referenced by the loaded (release-scoped) use cases.
	if !rf.active() {
		for _, path := range prdPaths {
			if v := loadYAML[PRDDoc](path); v != nil {
				v.File = path
				ctx.Specs.ProductRequirements = append(ctx.Specs.ProductRequirements, v)
			}
		}
	} else {
		referencedPRDs := prdIDsFromUseCases(ctx.Specs.UseCases)
		for _, path := range prdPaths {
			stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if referencedPRDs[stem] {
				if v := loadYAML[PRDDoc](path); v != nil {
					v.File = path
					ctx.Specs.ProductRequirements = append(ctx.Specs.ProductRequirements, v)
				}
			}
		}
	}

	// Load extras from contextSources (if non-empty), skipping files
	// already in the standard set and files in the exclude set.
	if ctxSources != "" {
		extras := resolveContextSources(ctxSources)
		for _, path := range extras {
			if standardSet[path] {
				continue
			}
			if excludeSet != nil && excludeSet[path] {
				continue
			}
			if v := loadNamedDoc(path); v != nil {
				v.File = path
				ctx.Extra = append(ctx.Extra, v)
			}
		}
	}

	// Omit empty collections.
	if ctx.Specs.ProductRequirements == nil && ctx.Specs.UseCases == nil &&
		ctx.Specs.TestSuites == nil && ctx.Specs.DependencyMap == nil &&
		ctx.Specs.Sources == nil {
		ctx.Specs = nil
	}

	// Load source code and filter through exclude set.
	ctx.SourceCode = loadSourceFiles(project.GoSourceDirs)
	if excludeSet != nil {
		var filtered []SourceFile
		for _, sf := range ctx.SourceCode {
			if !excludeSet[sf.File] {
				filtered = append(filtered, sf)
			}
		}
		ctx.SourceCode = filtered
	}

	ctx.Issues = parseIssuesJSON(existingIssuesJSON)

	// Load pre-cycle analysis results if present in the scratch directory.
	ctx.Analysis = loadAnalysisDoc(dirCobbler)

	logf("buildProjectContext: vision=%v arch=%v roadmap=%v specs=%v eng=%d analysis=%v issues=%d extra=%d src=%d files=%d",
		ctx.Vision != nil,
		ctx.Architecture != nil,
		ctx.Roadmap != nil,
		ctx.Specs != nil,
		len(ctx.Engineering),
		ctx.Analysis != nil,
		len(ctx.Issues),
		len(ctx.Extra),
		len(ctx.SourceCode),
		len(docFiles),
	)
	return ctx, nil
}
