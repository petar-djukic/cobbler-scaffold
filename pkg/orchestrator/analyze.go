// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AnalyzeResult holds the results of the Analyze operation.
type AnalyzeResult struct {
	OrphanedPRDs              []string // PRDs with no use cases referencing them
	ReleasesWithoutTestSuites []string // Releases in road-map.yaml with no test-rel*.yaml file
	OrphanedTestSuites        []string // Test suites whose traces don't match any known use case
	BrokenTouchpoints         []string // Use case touchpoints referencing non-existent PRDs
	UseCasesNotInRoadmap      []string // Use cases not listed in road-map.yaml
	SchemaErrors              []string // YAML files with fields not matching typed structs
}

// Analyze performs cross-artifact consistency checks.
// Returns nil error if all checks pass, or an error with detailed report if issues found.
func (o *Orchestrator) Analyze() error {
	logf("analyze: starting cross-artifact consistency checks")

	result := AnalyzeResult{}

	// 1. Load all PRDs
	prdFiles, err := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	if err != nil {
		return fmt.Errorf("globbing PRDs: %w", err)
	}
	prdIDs := make(map[string]bool)
	for _, path := range prdFiles {
		id := extractID(path)
		if id != "" {
			prdIDs[id] = true
		}
	}
	logf("analyze: found %d PRDs", len(prdIDs))

	// 2. Load all use cases
	ucFiles, err := filepath.Glob("docs/specs/use-cases/rel*.yaml")
	if err != nil {
		return fmt.Errorf("globbing use cases: %w", err)
	}
	ucIDs := make(map[string]bool)
	ucToPRDs := make(map[string][]string) // use case ID -> PRD IDs from touchpoints
	for _, path := range ucFiles {
		uc, err := loadUseCase(path)
		if err != nil {
			logf("analyze: skipping %s: %v", path, err)
			continue
		}
		ucIDs[uc.ID] = true
		ucToPRDs[uc.ID] = extractPRDsFromTouchpoints(uc.Touchpoints)
	}
	logf("analyze: found %d use cases", len(ucIDs))

	// 3. Load all test suites (per-release YAML specs)
	testFiles, err := filepath.Glob("docs/specs/test-suites/test-rel*.yaml")
	if err != nil {
		return fmt.Errorf("globbing test suites: %w", err)
	}
	testSuiteIDs := make(map[string]bool)
	testSuiteToUCs := make(map[string][]string) // test suite ID -> use case IDs from traces
	for _, path := range testFiles {
		ts, err := loadTestSuite(path)
		if err != nil {
			logf("analyze: skipping %s: %v", path, err)
			continue
		}
		testSuiteIDs[ts.ID] = true
		testSuiteToUCs[ts.ID] = extractUseCaseIDsFromTraces(ts.Traces)
	}
	logf("analyze: found %d test suites", len(testSuiteIDs))

	// 4. Load road-map.yaml — collect release IDs and use case IDs
	roadmapUCs := make(map[string]bool)
	roadmapReleaseIDs := make(map[string]bool)
	if data, err := os.ReadFile("docs/road-map.yaml"); err == nil {
		var roadmap struct {
			Releases []struct {
				ID       string `yaml:"version"`
				UseCases []struct {
					ID string `yaml:"id"`
				} `yaml:"use_cases"`
			} `yaml:"releases"`
		}
		if err := yaml.Unmarshal(data, &roadmap); err == nil {
			for _, release := range roadmap.Releases {
				// Only track releases that have use cases; empty
				// buckets (e.g. 99.0 Unscheduled) don't need test suites.
				if len(release.UseCases) > 0 {
					roadmapReleaseIDs[release.ID] = true
				}
				for _, uc := range release.UseCases {
					roadmapUCs[uc.ID] = true
				}
			}
			logf("analyze: found %d releases, %d use cases in roadmap", len(roadmapReleaseIDs), len(roadmapUCs))
		}
	}

	// Check 1: Orphaned PRDs (no use case references them)
	prdReferencedByUC := make(map[string]bool)
	for _, prds := range ucToPRDs {
		for _, prd := range prds {
			prdReferencedByUC[prd] = true
		}
	}
	for prdID := range prdIDs {
		if !prdReferencedByUC[prdID] {
			result.OrphanedPRDs = append(result.OrphanedPRDs, prdID)
		}
	}

	// Check 2: Releases in road-map.yaml without a test suite file
	for releaseID := range roadmapReleaseIDs {
		if !testSuiteIDs["test-rel"+releaseID] {
			result.ReleasesWithoutTestSuites = append(result.ReleasesWithoutTestSuites, releaseID)
		}
	}

	// Check 3: Orphaned test suites (traces don't reference any known use case)
	for testSuiteID, traces := range testSuiteToUCs {
		valid := false
		for _, ucID := range traces {
			if ucIDs[ucID] {
				valid = true
				break
			}
		}
		if !valid {
			result.OrphanedTestSuites = append(result.OrphanedTestSuites, testSuiteID)
		}
	}

	// Check 4: Broken touchpoints (use case references non-existent PRD)
	for ucID, prds := range ucToPRDs {
		for _, prd := range prds {
			if !prdIDs[prd] {
				result.BrokenTouchpoints = append(result.BrokenTouchpoints, fmt.Sprintf("%s -> %s (missing)", ucID, prd))
			}
		}
	}

	// Check 5: Use cases not in roadmap
	for ucID := range ucIDs {
		if !roadmapUCs[ucID] {
			result.UseCasesNotInRoadmap = append(result.UseCasesNotInRoadmap, ucID)
		}
	}

	// Check 6: YAML schema validation — load all docs into typed structs
	// with strict field checking. Unknown YAML fields indicate a schema
	// mismatch that will cause data loss during measure prompt assembly.
	result.SchemaErrors = o.validateDocSchemas()
	logf("analyze: schema validation found %d error(s)", len(result.SchemaErrors))

	return result.printReport(len(prdIDs), len(ucIDs), len(testSuiteIDs))
}

// printSection prints a labeled list if items is non-empty, returning true.
func printSection(label string, items []string) bool {
	if len(items) == 0 {
		return false
	}
	fmt.Printf("\n⚠️  %s:\n", label)
	for _, item := range items {
		fmt.Printf("  - %s\n", item)
	}
	return true
}

// printReport formats the analysis results to stdout. Returns nil when
// all checks pass, or an error summarising that issues were found.
func (r AnalyzeResult) printReport(prdCount, ucCount, tsCount int) error {
	hasIssues := false
	hasIssues = printSection("Orphaned PRDs (no use case references them)", r.OrphanedPRDs) || hasIssues
	hasIssues = printSection("Releases without test suites (no docs/specs/test-suites/test-<release>.yaml)", r.ReleasesWithoutTestSuites) || hasIssues
	hasIssues = printSection("Orphaned test suites (traces don't reference any known use case)", r.OrphanedTestSuites) || hasIssues
	hasIssues = printSection("Broken touchpoints (use case references non-existent PRD)", r.BrokenTouchpoints) || hasIssues
	hasIssues = printSection("Use cases not in roadmap", r.UseCasesNotInRoadmap) || hasIssues
	hasIssues = printSection("YAML schema errors (fields not matching typed structs — data will be lost in measure prompt)", r.SchemaErrors) || hasIssues

	if !hasIssues {
		fmt.Printf("\n✅ All consistency checks passed\n")
		fmt.Printf("   - %d PRDs\n", prdCount)
		fmt.Printf("   - %d use cases\n", ucCount)
		fmt.Printf("   - %d test suites\n", tsCount)
		return nil
	}
	return fmt.Errorf("found consistency issues (see above)")
}

// analyzeUseCase holds the fields extracted from a use case file
// that are needed for cross-artifact consistency checks.
type analyzeUseCase struct {
	ID          string
	Touchpoints []string
}

// analyzeTestSuite holds the fields extracted from a test suite file
// that are needed for cross-artifact consistency checks.
type analyzeTestSuite struct {
	ID     string   `yaml:"id"`
	Traces []string `yaml:"traces"`
}

// extractID extracts the ID from a file path like "docs/specs/product-requirements/prd001-feature.yaml" -> "prd001-feature"
func extractID(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// loadUseCase loads a use case YAML file and extracts key fields.
func loadUseCase(path string) (*analyzeUseCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID          string              `yaml:"id"`
		Touchpoints []map[string]string `yaml:"touchpoints"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Convert touchpoints from list of maps to list of strings.
	// Format: [{T1: "description"}] -> ["T1: description"]
	var touchpointStrings []string
	for _, tp := range raw.Touchpoints {
		for key, val := range tp {
			touchpointStrings = append(touchpointStrings, key+": "+val)
		}
	}

	return &analyzeUseCase{
		ID:          raw.ID,
		Touchpoints: touchpointStrings,
	}, nil
}

// loadTestSuite loads a test suite YAML file and extracts key fields.
func loadTestSuite(path string) (*analyzeTestSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ts analyzeTestSuite
	if err := yaml.Unmarshal(data, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

// extractPRDsFromTouchpoints parses touchpoint strings to extract PRD IDs.
// Touchpoint format: "T1: Component (prdXXX-name R1, R2)"
func extractPRDsFromTouchpoints(touchpoints []string) []string {
	var prds []string
	for _, tp := range touchpoints {
		// Look for patterns like "prd001-feature" or "prd-feature-name"
		parts := strings.Fields(tp)
		for _, part := range parts {
			part = strings.TrimSuffix(strings.TrimPrefix(part, "("), ")")
			if strings.HasPrefix(part, "prd") && !strings.Contains(part, " ") {
				prds = append(prds, part)
			}
		}
	}
	return prds
}

// extractUseCaseIDsFromTraces parses trace strings to extract use case IDs.
// Trace format: "rel01.0-uc001-cupboard-lifecycle" or "prd001-cupboard-core R4"
func extractUseCaseIDsFromTraces(traces []string) []string {
	var ucs []string
	for _, trace := range traces {
		// Look for patterns like "rel01.0-uc001-name"
		parts := strings.Fields(trace)
		for _, part := range parts {
			if strings.HasPrefix(part, "rel") && strings.Contains(part, "-uc") {
				ucs = append(ucs, part)
			}
		}
	}
	return ucs
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

	// Go style constitution (not in standard set but has typed schema).
	errs = append(errs, validateYAMLStrict[GoStyleDoc]("docs/constitutions/go-style.yaml")...)

	// Embedded constitutions (pkg/orchestrator/constitutions/).
	errs = append(errs, validateYAMLStrict[GoStyleDoc]("pkg/orchestrator/constitutions/go-style.yaml")...)

	// Prompts (simple YAML mapping with text fields).
	errs = append(errs, validatePromptTemplate("docs/prompts/measure.yaml")...)
	errs = append(errs, validatePromptTemplate("docs/prompts/stitch.yaml")...)
	errs = append(errs, validatePromptTemplate("pkg/orchestrator/prompts/measure.yaml")...)
	errs = append(errs, validatePromptTemplate("pkg/orchestrator/prompts/stitch.yaml")...)

	return errs
}

// validateYAMLStrict reads a YAML file and decodes it into T with
// KnownFields enabled. Any YAML key not present in the struct is
// reported as an error. Returns nil if the file doesn't exist.
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
	return nil
}
