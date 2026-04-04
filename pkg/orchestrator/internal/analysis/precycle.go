// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AnalysisFileName is the default file name for the analysis document.
const AnalysisFileName = "analysis.yaml"

// AnalysisDoc holds the combined results of cross-artifact consistency
// checks and code implementation status.
type AnalysisDoc struct {
	ConsistencyErrors  int      `yaml:"consistency_errors"`
	ConsistencyDetails []string `yaml:"consistency_details,omitempty"`
	Defects            []string `yaml:"defects,omitempty"`
	CodeStatus         *CodeStatusReport `yaml:"code_status,omitempty"`
}

// TotalIssues returns the total count of consistency errors and code gaps.
func (a *AnalysisDoc) TotalIssues() int {
	n := a.ConsistencyErrors
	if a.CodeStatus != nil {
		n += len(a.CodeStatus.Gaps)
	}
	return n
}

// PreCycleDeps holds dependencies for pre-cycle analysis.
type PreCycleDeps struct {
	Log        Logger
	CobblerDir string // scratch directory path
	AnalyzeDeps
}

// CollectConsistencyDetails flattens an AnalyzeResult into a single list
// of human-readable issue strings.
func CollectConsistencyDetails(r *AnalyzeResult) []string {
	var details []string
	for _, v := range r.OrphanedSRDs {
		details = append(details, "orphaned SRD: "+v)
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
	for _, v := range r.BrokenCitations {
		details = append(details, "broken citation: "+v)
	}
	for _, v := range r.InvalidReleases {
		details = append(details, "invalid release: "+v)
	}
	return details
}

// CollectDefects extracts schema errors and constitution drift from an
// AnalyzeResult.
func CollectDefects(r *AnalyzeResult) []string {
	var defects []string
	for _, v := range r.SchemaErrors {
		defects = append(defects, "schema error: "+v)
	}
	for _, v := range r.ConstitutionDrift {
		defects = append(defects, "constitution drift: "+v)
	}
	return defects
}

// RunPreCycleAnalysis performs cross-artifact consistency checks and code
// status detection, writes the combined result to {ScratchDir}/analysis.yaml,
// and logs a summary.
func RunPreCycleAnalysis(deps PreCycleDeps) {
	deps.Log("precycle: running pre-cycle analysis")

	doc := AnalysisDoc{}

	// Cross-artifact consistency checks.
	result, _, err := CollectAnalyzeResult(deps.AnalyzeDeps)
	if err != nil {
		deps.Log("precycle: consistency check error: %v", err)
	} else {
		details := CollectConsistencyDetails(&result)
		doc.ConsistencyErrors = len(details)
		doc.ConsistencyDetails = details
		defects := CollectDefects(&result)
		doc.Defects = defects
		if len(defects) > 0 {
			deps.Log("precycle: %d defect(s) routed to target repo (excluded from measure prompt)", len(defects))
		}
	}

	// Code implementation status.
	roadmap := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if roadmap != nil {
		testScan := ScanTestDirectories("tests")
		reqComplete := ComputeReqCompletion(deps.CobblerDir)
		report := ComputeCodeStatus(roadmap, testScan, reqComplete)
		report.Gaps = DetectSpecCodeGaps(&report)
		doc.CodeStatus = &report
	} else {
		deps.Log("precycle: cannot load road-map.yaml, skipping code status")
	}

	// Write to scratch directory.
	outPath := filepath.Join(deps.CobblerDir, AnalysisFileName)
	if err := WriteAnalysisDoc(&doc, outPath); err != nil {
		deps.Log("precycle: failed to write %s: %v", outPath, err)
		return
	}

	deps.Log("precycle: wrote %s (total_issues=%d)", outPath, doc.TotalIssues())
}

// WriteAnalysisDoc marshals an AnalysisDoc to YAML and writes it to path.
func WriteAnalysisDoc(doc *AnalysisDoc, path string) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadAnalysisDoc loads an AnalysisDoc from {cobblerDir}/analysis.yaml.
// Returns nil if the file does not exist or cannot be parsed.
func LoadAnalysisDoc(cobblerDir string) *AnalysisDoc {
	return loadYAML[AnalysisDoc](filepath.Join(cobblerDir, AnalysisFileName))
}
