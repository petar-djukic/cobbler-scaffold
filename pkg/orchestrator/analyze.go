// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
)

// Type aliases for backward-compatible re-exports.
type AnalyzeResult = an.AnalyzeResult

// collectAnalyzeResult performs all cross-artifact consistency checks and
// returns the structured result without printing.
func (o *Orchestrator) collectAnalyzeResult() (AnalyzeResult, an.AnalyzeCounts, error) {
	return an.CollectAnalyzeResult(an.AnalyzeDeps{
		Log:                    logf,
		Releases:               o.cfg.Project.Releases,
		ValidateDocSchemas:     o.validateDocSchemas,
		ValidatePromptTemplate: validatePromptTemplate,
	})
}

// Analyze performs cross-artifact consistency checks.
func (o *Orchestrator) Analyze() error {
	return an.Analyze(an.AnalyzeDeps{
		Log:                    logf,
		Releases:               o.cfg.Project.Releases,
		ValidateDocSchemas:     o.validateDocSchemas,
		ValidatePromptTemplate: validatePromptTemplate,
	})
}

// validateDocSchemas resolves configured context sources and validates
// each file against its typed struct using strict YAML decoding
// (KnownFields). Any YAML key that doesn't map to a struct field is
// reported — these indicate schema drift that causes data loss during
// measure prompt assembly.
func (o *Orchestrator) validateDocSchemas() []string {
	var errs []string

	// Validate standard documentation files.
	for _, path := range resolveStandardFiles() {
		switch classifyContextFile(path) {
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
	errs = append(errs, validateYAMLStrict[SemanticModelDoc]("docs/constitutions/semantic-model.yaml")...)
	errs = append(errs, validateYAMLStrict[TestingDoc]("docs/constitutions/testing.yaml")...)

	// Embedded constitutions in pkg/orchestrator/constitutions/.
	errs = append(errs, validateYAMLStrict[DesignDoc]("pkg/orchestrator/constitutions/design.yaml")...)
	errs = append(errs, validateYAMLStrict[ExecutionDoc]("pkg/orchestrator/constitutions/execution.yaml")...)
	errs = append(errs, validateYAMLStrict[GoStyleDoc]("pkg/orchestrator/constitutions/go-style.yaml")...)
	errs = append(errs, validateYAMLStrict[IssueFormatDoc]("pkg/orchestrator/constitutions/issue-format.yaml")...)
	errs = append(errs, validateYAMLStrict[PlanningDoc]("pkg/orchestrator/constitutions/planning.yaml")...)
	errs = append(errs, validateYAMLStrict[TestingDoc]("pkg/orchestrator/constitutions/testing.yaml")...)

	// Prompts (simple YAML mapping with text fields).
	errs = append(errs, validatePromptTemplate("docs/prompts/measure.yaml")...)
	errs = append(errs, validatePromptTemplate("docs/prompts/stitch.yaml")...)
	errs = append(errs, validatePromptTemplate("pkg/orchestrator/prompts/measure.yaml")...)
	errs = append(errs, validatePromptTemplate("pkg/orchestrator/prompts/stitch.yaml")...)

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

// Unexported delegations for parent-level functions that were previously
// defined here and are still referenced by other parent-package code or tests.

func extractID(path string) string                         { return an.ExtractID(path) }
func extractPRDsFromTouchpoints(tps []string) []string     { return an.ExtractPRDsFromTouchpoints(tps) }
func extractUseCaseIDsFromTraces(traces []string) []string  { return an.ExtractUseCaseIDsFromTraces(traces) }
func extractReqGroup(s string) string                       { return an.ExtractReqGroup(s) }
func extractCitationsFromTouchpoints(tps []string) []an.PRDCitation {
	return an.ExtractCitationsFromTouchpoints(tps)
}

func loadUseCase(path string) (*an.AnalyzeUseCase, error) { return an.LoadUseCase(path) }
func loadTestSuite(path string) (*an.AnalyzeTestSuite, error) { return an.LoadTestSuite(path) }

func detectConstitutionDrift() []string { return an.DetectConstitutionDrift(logf) }

func printSection(label string, items []string) bool { return an.PrintSection(label, items) }

func validateSemanticModels(prdFiles []string) ([]string, int) {
	return an.ValidateSemanticModels(prdFiles)
}
func validateStandaloneSemanticModel(path string) []string {
	return an.ValidateStandaloneSemanticModel(path)
}
func validatePRDSemanticModel(path string) []string { return an.ValidatePRDSemanticModel(path) }
func validatePromptSemanticModel(path string) []string {
	return an.ValidatePromptSemanticModel(path)
}
func smValidateSections(prefix string, sm map[string]interface{}) []string {
	return an.SmValidateSections(prefix, sm)
}
func smValidateSM7(prefix string, sm map[string]interface{}) []string {
	return an.SmValidateSM7(prefix, sm)
}
func smValidateSM3(prefix string, sm map[string]interface{}) []string {
	return an.SmValidateSM3(prefix, sm)
}
func smSourceRefs(source string) []string { return an.SmSourceRefs(source) }

// Type aliases for internal types used in parent tests.
type prdCitation = an.PRDCitation
type analyzeUseCase = an.AnalyzeUseCase
type analyzeTestSuite = an.AnalyzeTestSuite
type analyzeCounts = an.AnalyzeCounts

// Function aliases for code status helpers used by other parent code.
func scanTestDirectories(root string) map[string]int { return an.ScanTestDirectories(root) }
func computeCodeStatus(roadmap *RoadmapDoc, scan map[string]int) CodeStatusReport {
	// Convert parent RoadmapDoc to internal RoadmapDoc.
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
	return an.ComputeCodeStatus(&internalRoadmap, scan)
}
func detectSpecCodeGaps(report *CodeStatusReport) []string { return an.DetectSpecCodeGaps(report) }
func printCodeStatusReport(report *CodeStatusReport)       { an.PrintCodeStatusReport(report) }
func statusIcon(status string) string                      { return an.StatusIcon(status) }
func ucPrefixFromID(ucID string) string                    { return an.UCPrefixFromID(ucID) }
func testDirForUC(ucID string) string                      { return an.TestDirForUC(ucID) }
func countTestFiles(dir string) int                        { return an.CountTestFiles(dir) }

// Unexported aliases for precycle functions used by parent code.
func collectConsistencyDetails(r *AnalyzeResult) []string { return an.CollectConsistencyDetails(r) }
func collectDefects(r *AnalyzeResult) []string            { return an.CollectDefects(r) }
func writeAnalysisDoc(doc *AnalysisDoc, path string) error { return an.WriteAnalysisDoc(doc, path) }

// The analysisFileName constant is needed by precycle_test.go.
const analysisFileName = an.AnalysisFileName
