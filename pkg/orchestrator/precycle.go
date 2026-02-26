// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const analysisFileName = "analysis.yaml"

// AnalysisDoc holds the combined results of cross-artifact consistency
// checks and code implementation status. It is written to the cobbler
// scratch directory before each measure/stitch cycle and loaded into
// ProjectContext so Claude sees the current project state.
type AnalysisDoc struct {
	// ConsistencyErrors is the total count of cross-artifact issues found.
	ConsistencyErrors int `yaml:"consistency_errors"`

	// ConsistencyDetails lists individual consistency issues (orphaned PRDs,
	// broken touchpoints, schema errors, etc.).
	ConsistencyDetails []string `yaml:"consistency_details,omitempty"`

	// CodeStatus holds per-release and per-use-case implementation status.
	CodeStatus *CodeStatusReport `yaml:"code_status,omitempty"`
}

// totalIssues returns the total count of consistency errors and code gaps.
func (a *AnalysisDoc) totalIssues() int {
	n := a.ConsistencyErrors
	if a.CodeStatus != nil {
		n += len(a.CodeStatus.Gaps)
	}
	return n
}

// collectConsistencyDetails flattens an AnalyzeResult into a single list
// of human-readable issue strings.
func collectConsistencyDetails(r *AnalyzeResult) []string {
	var details []string
	for _, v := range r.OrphanedPRDs {
		details = append(details, "orphaned PRD: "+v)
	}
	for _, v := range r.ReleasesWithoutTestSuites {
		details = append(details, "release without test suite: "+v)
	}
	for _, v := range r.OrphanedTestSuites {
		details = append(details, "orphaned test suite: "+v)
	}
	for _, v := range r.BrokenTouchpoints {
		details = append(details, "broken touchpoint: "+v)
	}
	for _, v := range r.UseCasesNotInRoadmap {
		details = append(details, "use case not in roadmap: "+v)
	}
	for _, v := range r.SchemaErrors {
		details = append(details, "schema error: "+v)
	}
	for _, v := range r.ConstitutionDrift {
		details = append(details, "constitution drift: "+v)
	}
	for _, v := range r.BrokenCitations {
		details = append(details, "broken citation: "+v)
	}
	for _, v := range r.InvalidReleases {
		details = append(details, "invalid release: "+v)
	}
	return details
}

// RunPreCycleAnalysis performs cross-artifact consistency checks and code
// status detection, writes the combined result to {ScratchDir}/analysis.yaml,
// and logs a summary. Errors are logged but do not fail the caller — the
// analysis is advisory, not blocking.
func (o *Orchestrator) RunPreCycleAnalysis() {
	logf("precycle: running pre-cycle analysis")

	doc := AnalysisDoc{}

	// Cross-artifact consistency checks.
	result, _, err := o.collectAnalyzeResult()
	if err != nil {
		logf("precycle: consistency check error: %v", err)
	} else {
		details := collectConsistencyDetails(&result)
		doc.ConsistencyErrors = len(details)
		doc.ConsistencyDetails = details
	}

	// Code implementation status.
	roadmap := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if roadmap != nil {
		testScan := scanTestDirectories("tests")
		report := computeCodeStatus(roadmap, testScan)
		report.Gaps = detectSpecCodeGaps(&report)
		doc.CodeStatus = &report
	} else {
		logf("precycle: cannot load road-map.yaml, skipping code status")
	}

	// Write to scratch directory.
	outPath := filepath.Join(o.cfg.Cobbler.Dir, analysisFileName)
	if err := writeAnalysisDoc(&doc, outPath); err != nil {
		logf("precycle: failed to write %s: %v", outPath, err)
		return
	}

	logf("precycle: wrote %s (total_issues=%d)", outPath, doc.totalIssues())
}

// writeAnalysisDoc marshals an AnalysisDoc to YAML and writes it to path.
func writeAnalysisDoc(doc *AnalysisDoc, path string) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// loadAnalysisDoc loads an AnalysisDoc from {cobblerDir}/analysis.yaml.
// Returns nil if the file does not exist or cannot be parsed.
func loadAnalysisDoc(cobblerDir string) *AnalysisDoc {
	return loadYAML[AnalysisDoc](filepath.Join(cobblerDir, analysisFileName))
}
