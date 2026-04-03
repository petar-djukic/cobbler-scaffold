// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
)

// Type aliases for backward-compatible re-exports.
type AnalyzeResult = an.AnalyzeResult

// Analyzer provides cross-artifact consistency checks and code status.
type Analyzer struct {
	cfg  Config
	logf func(string, ...any)
}

// NewAnalyzer creates an Analyzer with explicit dependencies.
func NewAnalyzer(cfg Config, logf func(string, ...any)) *Analyzer {
	return &Analyzer{cfg: cfg, logf: logf}
}

// collectAnalyzeResult performs all cross-artifact consistency checks and
// returns the structured result without printing.
func (a *Analyzer) collectAnalyzeResult() (AnalyzeResult, an.AnalyzeCounts, error) {
	return an.CollectAnalyzeResult(a.analyzeDeps())
}

// Analyze performs cross-artifact consistency checks.
func (a *Analyzer) Analyze() error {
	return an.Analyze(a.analyzeDeps())
}

// analyzeDeps builds the AnalyzeDeps struct.
func (a *Analyzer) analyzeDeps() an.AnalyzeDeps {
	return an.AnalyzeDeps{
		Log:                    a.logf,
		Releases:               a.cfg.Project.Releases,
		ValidateDocSchemas:     a.validateDocSchemas,
		ValidatePromptTemplate: ictx.ValidatePromptTemplate,
	}
}

// validateDocSchemas resolves configured context sources and validates
// each file against its typed struct using strict YAML decoding
// (KnownFields). Any YAML key that doesn't map to a struct field is
// reported — these indicate schema drift that causes data loss during
// measure prompt assembly.
func (a *Analyzer) validateDocSchemas() []string {
	var errs []string

	// Validate standard documentation files.
	for _, path := range ictx.ResolveStandardFiles() {
		switch ictx.ClassifyContextFile(path) {
		case "vision":
			errs = append(errs, validateYAMLStrict[VisionDoc](path)...)
		case "architecture":
			errs = append(errs, validateYAMLStrict[ArchitectureDoc](path)...)
		case "specifications":
			errs = append(errs, validateYAMLStrict[SpecificationsDoc](path)...)
		case "roadmap":
			errs = append(errs, validateYAMLStrict[RoadmapDoc](path)...)
		case "prd":
			errs = append(errs, validateYAMLStrict[PRDDoc](path)...)
		case "use_case":
			errs = append(errs, validateYAMLStrict[UseCaseDoc](path)...)
		case "test_suite":
			errs = append(errs, validateYAMLStrict[TestSuiteDoc](path)...)
		case "engineering":
			errs = append(errs, validateYAMLStrict[EngineeringDoc](path)...)
		}
	}

	// Constitutions in docs/constitutions/.
	errs = append(errs, validateYAMLStrict[DesignDoc]("docs/constitutions/design.yaml")...)
	errs = append(errs, validateYAMLStrict[ExecutionDoc]("docs/constitutions/execution.yaml")...)
	errs = append(errs, validateYAMLStrict[GoStyleDoc]("docs/constitutions/go-style.yaml")...)
	errs = append(errs, validateYAMLStrict[PlanningDoc]("docs/constitutions/planning.yaml")...)
	errs = append(errs, validateYAMLStrict[InterfaceDoc]("docs/constitutions/interface.yaml")...)
	errs = append(errs, validateYAMLStrict[SemanticModelDoc]("docs/constitutions/semantic-model.yaml")...)
	errs = append(errs, validateYAMLStrict[TestingDoc]("docs/constitutions/testing.yaml")...)

	// Embedded constitutions in pkg/orchestrator/constitutions/.
	errs = append(errs, validateYAMLStrict[DesignDoc]("pkg/orchestrator/constitutions/design.yaml")...)
	errs = append(errs, validateYAMLStrict[ExecutionDoc]("pkg/orchestrator/constitutions/execution.yaml")...)
	errs = append(errs, validateYAMLStrict[GoStyleDoc]("pkg/orchestrator/constitutions/go-style.yaml")...)
	errs = append(errs, validateYAMLStrict[InterfaceDoc]("pkg/orchestrator/constitutions/interface.yaml")...)
	errs = append(errs, validateYAMLStrict[IssueFormatDoc]("pkg/orchestrator/constitutions/issue-format.yaml")...)
	errs = append(errs, validateYAMLStrict[PlanningDoc]("pkg/orchestrator/constitutions/planning.yaml")...)
	errs = append(errs, validateYAMLStrict[TestingDoc]("pkg/orchestrator/constitutions/testing.yaml")...)

	// Prompts (simple YAML mapping with text fields).
	errs = append(errs, ictx.ValidatePromptTemplate("docs/prompts/measure.yaml")...)
	errs = append(errs, ictx.ValidatePromptTemplate("docs/prompts/stitch.yaml")...)
	errs = append(errs, ictx.ValidatePromptTemplate("pkg/orchestrator/prompts/measure.yaml")...)
	errs = append(errs, ictx.ValidatePromptTemplate("pkg/orchestrator/prompts/stitch.yaml")...)

	return errs
}

// docValidator is implemented by document structs that support required-field
// validation beyond unknown-field checking.
type docValidator interface {
	Validate() []string
}

// validateYAMLStrict reads a YAML file and decodes it into T with
// KnownFields enabled. Any YAML key not present in the struct is
// reported as an error. After a successful decode, if T implements
// docValidator, Validate() is called to check required fields.
// Returns nil if the file doesn't exist.
func validateYAMLStrict[T any](path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // missing file is not a schema error
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var v T
	if err := dec.Decode(&v); err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}
	var errs []string
	if val, ok := any(&v).(docValidator); ok {
		for _, e := range val.Validate() {
			errs = append(errs, fmt.Sprintf("%s: %s", path, e))
		}
	}
	return errs
}

// computeCodeStatus converts parent-package RoadmapDoc to the internal type
// before delegating to an.ComputeCodeStatus. This wrapper exists because the
// parent and internal packages define separate RoadmapDoc structs.
func computeCodeStatus(roadmap *RoadmapDoc, scan map[string]int) CodeStatusReport {
	var internalRoadmap an.RoadmapDoc
	for _, r := range roadmap.Releases {
		var ucs []an.RoadmapUseCase
		for _, uc := range r.UseCases {
			ucs = append(ucs, an.RoadmapUseCase{ID: uc.ID, Status: uc.Status})
		}
		internalRoadmap.Releases = append(internalRoadmap.Releases, an.RoadmapRelease{
			Version:  r.Version,
			Name:     r.Name,
			Status:   r.Status,
			UseCases: ucs,
		})
	}
	return an.ComputeCodeStatus(&internalRoadmap, scan, nil)
}
