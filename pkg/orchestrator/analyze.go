// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AnalyzeResult holds the results of the Analyze operation.
type AnalyzeResult struct {
	OrphanedPRDs         []string // PRDs with no use cases referencing them
	UseCasesWithoutTests []string // Use cases missing test suites
	OrphanedTestSuites   []string // Test suites not referenced by any use case
	BrokenTouchpoints    []string // Use case touchpoints referencing non-existent PRDs
	UseCasesNotInRoadmap []string // Use cases not listed in road-map.yaml
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
	ucToPRDs := make(map[string][]string)  // use case ID -> PRD IDs from touchpoints
	ucToTestSuite := make(map[string]string) // use case ID -> test suite ID
	for _, path := range ucFiles {
		uc, err := loadUseCase(path)
		if err != nil {
			logf("analyze: skipping %s: %v", path, err)
			continue
		}
		ucIDs[uc.ID] = true
		ucToPRDs[uc.ID] = extractPRDsFromTouchpoints(uc.Touchpoints)
		ucToTestSuite[uc.ID] = uc.TestSuite
	}
	logf("analyze: found %d use cases", len(ucIDs))

	// 3. Load all test suites
	testFiles, err := filepath.Glob("docs/specs/test-suites/test*.yaml")
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

	// 4. Load road-map.yaml
	roadmapUCs := make(map[string]bool)
	if data, err := os.ReadFile("docs/road-map.yaml"); err == nil {
		var roadmap struct {
			Releases []struct {
				UseCases []struct {
					ID string `yaml:"id"`
				} `yaml:"use_cases"`
			} `yaml:"releases"`
		}
		if err := yaml.Unmarshal(data, &roadmap); err == nil {
			for _, release := range roadmap.Releases {
				for _, uc := range release.UseCases {
					roadmapUCs[uc.ID] = true
				}
			}
			logf("analyze: found %d use cases in roadmap", len(roadmapUCs))
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

	// Check 2: Use cases without test suites
	for ucID, testSuiteID := range ucToTestSuite {
		if testSuiteID == "" || !testSuiteIDs[testSuiteID] {
			result.UseCasesWithoutTests = append(result.UseCasesWithoutTests, ucID)
		}
	}

	// Check 3: Orphaned test suites (no use case references them)
	testSuiteReferencedByUC := make(map[string]bool)
	for _, testSuiteID := range ucToTestSuite {
		if testSuiteID != "" {
			testSuiteReferencedByUC[testSuiteID] = true
		}
	}
	for testSuiteID := range testSuiteIDs {
		if !testSuiteReferencedByUC[testSuiteID] {
			// Also check if test suite traces back to a valid use case
			traces := testSuiteToUCs[testSuiteID]
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

	// Print report
	hasIssues := false
	if len(result.OrphanedPRDs) > 0 {
		hasIssues = true
		fmt.Printf("\n⚠️  Orphaned PRDs (no use case references them):\n")
		for _, prd := range result.OrphanedPRDs {
			fmt.Printf("  - %s\n", prd)
		}
	}
	if len(result.UseCasesWithoutTests) > 0 {
		hasIssues = true
		fmt.Printf("\n⚠️  Use cases without test suites:\n")
		for _, uc := range result.UseCasesWithoutTests {
			fmt.Printf("  - %s\n", uc)
		}
	}
	if len(result.OrphanedTestSuites) > 0 {
		hasIssues = true
		fmt.Printf("\n⚠️  Orphaned test suites (not referenced by any use case):\n")
		for _, ts := range result.OrphanedTestSuites {
			fmt.Printf("  - %s\n", ts)
		}
	}
	if len(result.BrokenTouchpoints) > 0 {
		hasIssues = true
		fmt.Printf("\n⚠️  Broken touchpoints (use case references non-existent PRD):\n")
		for _, tp := range result.BrokenTouchpoints {
			fmt.Printf("  - %s\n", tp)
		}
	}
	if len(result.UseCasesNotInRoadmap) > 0 {
		hasIssues = true
		fmt.Printf("\n⚠️  Use cases not in roadmap:\n")
		for _, uc := range result.UseCasesNotInRoadmap {
			fmt.Printf("  - %s\n", uc)
		}
	}

	if !hasIssues {
		fmt.Printf("\n✅ All consistency checks passed\n")
		fmt.Printf("   - %d PRDs\n", len(prdIDs))
		fmt.Printf("   - %d use cases\n", len(ucIDs))
		fmt.Printf("   - %d test suites\n", len(testSuiteIDs))
		return nil
	}

	return fmt.Errorf("found consistency issues (see above)")
}

// extractID extracts the ID from a file path like "docs/specs/product-requirements/prd001-feature.yaml" -> "prd001-feature"
func extractID(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// loadUseCase loads a use case YAML file and extracts key fields.
func loadUseCase(path string) (*struct {
	ID          string
	Touchpoints []string
	TestSuite   string
}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse the full YAML structure
	var raw struct {
		ID          string                   `yaml:"id"`
		Touchpoints []map[string]string      `yaml:"touchpoints"`
		TestSuite   string                   `yaml:"test_suite"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Convert touchpoints from list of maps to list of strings
	// Format: [{T1: "description"}] -> ["T1: description"]
	var touchpointStrings []string
	for _, tp := range raw.Touchpoints {
		for key, val := range tp {
			touchpointStrings = append(touchpointStrings, key+": "+val)
		}
	}

	return &struct {
		ID          string
		Touchpoints []string
		TestSuite   string
	}{
		ID:          raw.ID,
		Touchpoints: touchpointStrings,
		TestSuite:   raw.TestSuite,
	}, nil
}

// loadTestSuite loads a test suite YAML file and extracts key fields.
func loadTestSuite(path string) (*struct {
	ID     string   `yaml:"id"`
	Traces []string `yaml:"traces"`
}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ts struct {
		ID     string   `yaml:"id"`
		Traces []string `yaml:"traces"`
	}
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
