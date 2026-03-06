// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	ConstitutionDrift         []string // Files in docs/constitutions/ that differ from embedded copies
	BrokenCitations                []string // Touchpoints citing non-existent requirement groups in PRDs
	InvalidReleases                []string // Configured releases not found in road-map.yaml
	PRDsSpanningMultipleReleases   []string // PRDs referenced by use cases from more than one release
	DependsOnViolations            []string // depends_on symbols not in referenced package_contract, or prd_id missing
	DependencyRuleViolations       []string // component_dependencies violating an allowed=false dependency_rule
	BrokenStructRefs               []string // struct_refs pointing to non-existent PRD or requirement group
	ComponentDepViolations         []string // depends_on entries not reflected in component_dependencies
	SemanticModelErrors            []string // semantic model validation errors (SM1, SM3, SM7)
}

// analyzeCounts holds the artifact counts discovered during analysis.
type analyzeCounts struct {
	PRDs           int
	UseCases       int
	TestSuites     int
	SemanticModels int
}

// collectAnalyzeResult performs all cross-artifact consistency checks and
// returns the structured result without printing. This is the data-gathering
// core shared by Analyze() (interactive) and RunPreCycleAnalysis() (automated).
func (o *Orchestrator) collectAnalyzeResult() (AnalyzeResult, analyzeCounts, error) {
	logf("analyze: starting cross-artifact consistency checks")

	result := AnalyzeResult{}

	// 1. Load all PRDs
	prdFiles, err := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	if err != nil {
		return result, analyzeCounts{}, fmt.Errorf("globbing PRDs: %w", err)
	}
	prdIDs := make(map[string]bool)
	prdReqGroups := make(map[string]map[string]bool)    // PRD ID -> set of requirement group keys
	prdExports := make(map[string]map[string]bool)       // PRD ID -> set of exported symbol names
	prdDependsOn := make(map[string][]PRDDependsOn)      // PRD ID -> depends_on entries
	prdStructRefs := make(map[string][]PRDStructRef)     // PRD ID -> struct_refs entries
	for _, path := range prdFiles {
		id := extractID(path)
		if id != "" {
			prdIDs[id] = true
		}
		if prd := loadYAML[PRDDoc](path); prd != nil {
			groups := make(map[string]bool)
			for groupKey := range prd.Requirements {
				groups[groupKey] = true
			}
			prdReqGroups[id] = groups
			if prd.PackageContract != nil {
				exports := make(map[string]bool)
				for _, e := range prd.PackageContract.Exports {
					exports[e.Name] = true
				}
				prdExports[id] = exports
			}
			if len(prd.DependsOn) > 0 {
				prdDependsOn[id] = prd.DependsOn
			}
			if len(prd.StructRefs) > 0 {
				prdStructRefs[id] = prd.StructRefs
			}
		}
	}
	logf("analyze: found %d PRDs", len(prdIDs))

	// 1b. Load ARCHITECTURE.yaml for OOD fields.
	var archDoc *ArchitectureDoc
	if data, err := os.ReadFile("docs/ARCHITECTURE.yaml"); err == nil {
		var doc ArchitectureDoc
		if err := yaml.Unmarshal(data, &doc); err == nil {
			archDoc = &doc
		}
	}

	// 2. Load all use cases
	ucFiles, err := filepath.Glob("docs/specs/use-cases/rel*.yaml")
	if err != nil {
		return result, analyzeCounts{}, fmt.Errorf("globbing use cases: %w", err)
	}
	ucIDs := make(map[string]bool)
	ucToPRDs := make(map[string][]string)      // use case ID -> PRD IDs from touchpoints
	ucTouchpoints := make(map[string][]string) // use case ID -> raw touchpoint strings
	prdToReleases := make(map[string]map[string]bool) // PRD ID -> set of releases that reference it
	for _, path := range ucFiles {
		uc, err := loadUseCase(path)
		if err != nil {
			logf("analyze: skipping %s: %v", path, err)
			continue
		}
		ucIDs[uc.ID] = true
		ucToPRDs[uc.ID] = extractPRDsFromTouchpoints(uc.Touchpoints)
		ucTouchpoints[uc.ID] = uc.Touchpoints
		release := extractFileRelease(path)
		if release != "" {
			for _, prdID := range ucToPRDs[uc.ID] {
				if prdToReleases[prdID] == nil {
					prdToReleases[prdID] = make(map[string]bool)
				}
				prdToReleases[prdID][release] = true
			}
		}
	}
	logf("analyze: found %d use cases", len(ucIDs))

	// 3. Load all test suites (per-release YAML specs)
	testFiles, err := filepath.Glob("docs/specs/test-suites/test-rel*.yaml")
	if err != nil {
		return result, analyzeCounts{}, fmt.Errorf("globbing test suites: %w", err)
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

	// Check 0: Configured releases exist in road-map.yaml
	for _, r := range o.cfg.Project.Releases {
		if !roadmapReleaseIDs[r] {
			result.InvalidReleases = append(result.InvalidReleases,
				fmt.Sprintf("configured release %q not found in road-map.yaml", r))
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

	// Check 6: Broken citations (touchpoint cites requirement group not in PRD)
	for ucID, tps := range ucTouchpoints {
		for _, cite := range extractCitationsFromTouchpoints(tps) {
			groups, ok := prdReqGroups[cite.PRDID]
			if !ok {
				continue // PRD missing or unparsable — handled by BrokenTouchpoints
			}
			for _, group := range cite.Groups {
				if !groups[group] {
					result.BrokenCitations = append(result.BrokenCitations,
						fmt.Sprintf("%s: cites %s %s (requirement group not found)", ucID, cite.PRDID, group))
				}
			}
		}
	}
	logf("analyze: broken citations found %d", len(result.BrokenCitations))

	// Check 9: PRDs spanning multiple releases
	for prdID, releases := range prdToReleases {
		if len(releases) > 1 {
			sorted := make([]string, 0, len(releases))
			for r := range releases {
				sorted = append(sorted, r)
			}
			sort.Strings(sorted)
			result.PRDsSpanningMultipleReleases = append(result.PRDsSpanningMultipleReleases,
				fmt.Sprintf("%s: referenced by releases %s", prdID, strings.Join(sorted, ", ")))
		}
	}
	sort.Strings(result.PRDsSpanningMultipleReleases)
	logf("analyze: PRDs spanning multiple releases found %d", len(result.PRDsSpanningMultipleReleases))

	// Check 10: depends_on — referenced prd_id must exist; symbols_used must be
	// in the referenced PRD's package_contract.exports (if a contract is declared).
	for prdID, deps := range prdDependsOn {
		for _, dep := range deps {
			if !prdIDs[dep.PRDID] {
				result.DependsOnViolations = append(result.DependsOnViolations,
					fmt.Sprintf("%s: depends_on references non-existent PRD %s", prdID, dep.PRDID))
				continue
			}
			exports, hasContract := prdExports[dep.PRDID]
			if !hasContract {
				continue // referenced PRD has no package_contract; skip symbol check
			}
			for _, sym := range dep.SymbolsUsed {
				if !exports[sym] {
					result.DependsOnViolations = append(result.DependsOnViolations,
						fmt.Sprintf("%s: depends_on %s symbol %q not in package_contract", prdID, dep.PRDID, sym))
				}
			}
		}
	}
	sort.Strings(result.DependsOnViolations)
	logf("analyze: depends_on violations found %d", len(result.DependsOnViolations))

	// Check 11: dependency_rules — component_dependencies entries must not
	// violate rules with allowed=false. A violation occurs when both from and
	// to match the rule's From and To prefix patterns.
	if archDoc != nil {
		for _, compDep := range archDoc.ComponentDependencies {
			for _, rule := range archDoc.DependencyRules {
				if rule.Allowed {
					continue
				}
				if strings.HasPrefix(compDep.From, rule.From) && strings.HasPrefix(compDep.To, rule.To) {
					result.DependencyRuleViolations = append(result.DependencyRuleViolations,
						fmt.Sprintf("%s -> %s: violates rule %q (%s)", compDep.From, compDep.To, rule.Description, rule.From+" must not import "+rule.To))
				}
			}
		}
	}
	sort.Strings(result.DependencyRuleViolations)
	logf("analyze: dependency rule violations found %d", len(result.DependencyRuleViolations))

	// Check 12: struct_refs — prd_id must exist and requirement must be a key
	// in that PRD's requirement groups.
	for prdID, refs := range prdStructRefs {
		for _, ref := range refs {
			if !prdIDs[ref.PRDID] {
				result.BrokenStructRefs = append(result.BrokenStructRefs,
					fmt.Sprintf("%s: struct_ref prd_id %s not found", prdID, ref.PRDID))
				continue
			}
			groups := prdReqGroups[ref.PRDID]
			if ref.Requirement != "" && !groups[ref.Requirement] {
				result.BrokenStructRefs = append(result.BrokenStructRefs,
					fmt.Sprintf("%s: struct_ref %s#%s requirement group not found", prdID, ref.PRDID, ref.Requirement))
			}
		}
	}
	sort.Strings(result.BrokenStructRefs)
	logf("analyze: broken struct_refs found %d", len(result.BrokenStructRefs))

	// Check 13: component_dependencies — if the architecture declares
	// component_dependencies, every PRD ID referenced in any depends_on entry
	// must appear as a substring in at least one component_dependency endpoint.
	// This catches depends_on declarations that were never added to the arch graph.
	if archDoc != nil && len(archDoc.ComponentDependencies) > 0 {
		compDepEndpoints := make(map[string]bool)
		for _, cd := range archDoc.ComponentDependencies {
			compDepEndpoints[cd.From] = true
			compDepEndpoints[cd.To] = true
		}
		for prdID, deps := range prdDependsOn {
			for _, dep := range deps {
				found := false
				for endpoint := range compDepEndpoints {
					if strings.Contains(endpoint, dep.PRDID) || strings.Contains(dep.PRDID, endpoint) {
						found = true
						break
					}
				}
				if !found {
					result.ComponentDepViolations = append(result.ComponentDepViolations,
						fmt.Sprintf("%s: depends_on %s not reflected in component_dependencies", prdID, dep.PRDID))
				}
			}
		}
	}
	sort.Strings(result.ComponentDepViolations)
	logf("analyze: component_dep violations found %d", len(result.ComponentDepViolations))

	// Check 7: YAML schema validation — load all docs into typed structs
	// with strict field checking. Unknown YAML fields indicate a schema
	// mismatch that will cause data loss during measure prompt assembly.
	result.SchemaErrors = o.validateDocSchemas()
	logf("analyze: schema validation found %d error(s)", len(result.SchemaErrors))

	// Check 8: Constitution drift — compare docs/constitutions/ with
	// embedded copies in pkg/orchestrator/constitutions/.
	result.ConstitutionDrift = detectConstitutionDrift()
	logf("analyze: constitution drift found %d file(s)", len(result.ConstitutionDrift))

	// Check 14: Semantic model validation — validate standalone files,
	// PRD shorthand models, and prompt-embedded full models.
	smErrs, smCount := validateSemanticModels(prdFiles)
	result.SemanticModelErrors = smErrs
	logf("analyze: semantic model validation found %d error(s), %d standalone file(s)", len(smErrs), smCount)

	counts := analyzeCounts{
		PRDs:           len(prdIDs),
		UseCases:       len(ucIDs),
		TestSuites:     len(testSuiteIDs),
		SemanticModels: smCount,
	}
	return result, counts, nil
}

// Analyze performs cross-artifact consistency checks.
// Returns nil error if all checks pass, or an error with detailed report if issues found.
func (o *Orchestrator) Analyze() error {
	result, counts, err := o.collectAnalyzeResult()
	if err != nil {
		return err
	}
	return result.printReport(counts.PRDs, counts.UseCases, counts.TestSuites, counts.SemanticModels)
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
func (r AnalyzeResult) printReport(prdCount, ucCount, tsCount, smCount int) error {
	hasIssues := false
	hasIssues = printSection("Orphaned PRDs (no use case references them)", r.OrphanedPRDs) || hasIssues
	hasIssues = printSection("Releases without test suites (no docs/specs/test-suites/test-<release>.yaml)", r.ReleasesWithoutTestSuites) || hasIssues
	hasIssues = printSection("Orphaned test suites (traces don't reference any known use case)", r.OrphanedTestSuites) || hasIssues
	hasIssues = printSection("Broken touchpoints (use case references non-existent PRD)", r.BrokenTouchpoints) || hasIssues
	hasIssues = printSection("Use cases not in roadmap", r.UseCasesNotInRoadmap) || hasIssues
	hasIssues = printSection("YAML schema errors (fields not matching typed structs — data will be lost in measure prompt)", r.SchemaErrors) || hasIssues
	hasIssues = printSection("Constitution drift (docs/constitutions/ differs from embedded pkg/orchestrator/constitutions/)", r.ConstitutionDrift) || hasIssues
	hasIssues = printSection("Broken citations (touchpoint cites non-existent requirement group)", r.BrokenCitations) || hasIssues
	hasIssues = printSection("Invalid configured releases (not found in road-map.yaml)", r.InvalidReleases) || hasIssues
	hasIssues = printSection("PRDs spanning multiple releases (each PRD must belong to exactly one release)", r.PRDsSpanningMultipleReleases) || hasIssues
	hasIssues = printSection("depends_on violations (symbol not in package_contract or prd_id missing)", r.DependsOnViolations) || hasIssues
	hasIssues = printSection("Dependency rule violations (component_dependency violates allowed=false rule)", r.DependencyRuleViolations) || hasIssues
	hasIssues = printSection("Broken struct_refs (prd_id or requirement group not found)", r.BrokenStructRefs) || hasIssues
	hasIssues = printSection("component_dependencies gaps (depends_on entries missing from component_dependencies)", r.ComponentDepViolations) || hasIssues
	hasIssues = printSection("Semantic model errors (SM1 sections, SM3 traceability, SM7 naming)", r.SemanticModelErrors) || hasIssues

	if !hasIssues {
		fmt.Printf("\n✅ All consistency checks passed\n")
		fmt.Printf("   - %d PRDs\n", prdCount)
		fmt.Printf("   - %d use cases\n", ucCount)
		fmt.Printf("   - %d test suites\n", tsCount)
		fmt.Printf("   - %d semantic models\n", smCount)
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

// prdCitation represents a reference to a PRD with specific requirement
// groups extracted from a use case touchpoint.
type prdCitation struct {
	PRDID  string
	Groups []string // requirement group IDs like "R1", "R2"
}

// reqGroupRe matches requirement group references like "R1", "R2", "R9".
var reqGroupRe = regexp.MustCompile(`^R\d+`)

// extractReqGroup extracts the requirement group prefix from a reference.
// "R1" returns "R1"; "R2.1" returns "R2"; "R9.1-R9.4" returns "R9".
func extractReqGroup(s string) string {
	return reqGroupRe.FindString(s)
}

// extractCitationsFromTouchpoints parses touchpoint strings to extract
// PRD IDs and their associated requirement group references.
// Touchpoint format: "T1: Component: prd001-name R1, R2, prd002-name R3"
func extractCitationsFromTouchpoints(touchpoints []string) []prdCitation {
	var citations []prdCitation
	for _, tp := range touchpoints {
		var current *prdCitation
		for _, part := range strings.Fields(tp) {
			// Strip parentheses and trailing commas/periods.
			cleaned := strings.TrimLeft(part, "(")
			cleaned = strings.TrimRight(cleaned, "),.")

			if strings.HasPrefix(cleaned, "prd") {
				if current != nil {
					citations = append(citations, *current)
				}
				current = &prdCitation{PRDID: cleaned}
				continue
			}
			if current == nil {
				continue
			}
			group := extractReqGroup(cleaned)
			if group == "" {
				continue
			}
			// Deduplicate within this citation.
			dup := false
			for _, g := range current.Groups {
				if g == group {
					dup = true
					break
				}
			}
			if !dup {
				current.Groups = append(current.Groups, group)
			}
		}
		if current != nil {
			citations = append(citations, *current)
		}
	}
	return citations
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
// validation beyond unknown-field checking. validateYAMLStrict calls Validate()
// after a successful strict decode and prepends the file path to each error.
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

// detectConstitutionDrift compares each constitution file in
// docs/constitutions/ with its embedded copy in
// pkg/orchestrator/constitutions/. Returns a list of filenames
// that differ between the two directories.
func detectConstitutionDrift() []string {
	const (
		docsDir     = "docs/constitutions"
		embeddedDir = "pkg/orchestrator/constitutions"
	)

	entries, err := os.ReadDir(docsDir)
	if err != nil {
		logf("detectConstitutionDrift: cannot read %s: %v", docsDir, err)
		return nil
	}

	var drifted []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		docsPath := filepath.Join(docsDir, entry.Name())
		embeddedPath := filepath.Join(embeddedDir, entry.Name())

		docsData, err := os.ReadFile(docsPath)
		if err != nil {
			continue
		}
		embeddedData, err := os.ReadFile(embeddedPath)
		if err != nil {
			continue // file only in docs/ is not drift
		}
		if !bytes.Equal(docsData, embeddedData) {
			drifted = append(drifted, entry.Name())
		}
	}
	return drifted
}

// ---------------------------------------------------------------------------
// Semantic model validation (R1–R7, SM1, SM3, SM7)
// ---------------------------------------------------------------------------

// smNameRe matches a valid semantic model name: lowercase words separated by hyphens, ≥2 parts.
var smNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)+$`)

// smSemverRe matches a valid semver version string: MAJOR.MINOR.PATCH.
var smSemverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// smIdentRe extracts dot-separated identifier chains from a source expression.
var smIdentRe = regexp.MustCompile(`[a-z_][a-z0-9_]*(?:\.[a-z_][a-z0-9_]*)*`)

// validateSemanticModels runs all semantic model checks (R1–R6).
// prdFiles is the already-globbed list of PRD paths. Returns errors and
// the count of standalone semantic model files found.
func validateSemanticModels(prdFiles []string) ([]string, int) {
	var errs []string

	// R1: Count standalone files in docs/specs/semantic-models/.
	smFiles, _ := filepath.Glob("docs/specs/semantic-models/*.yaml")

	// R4 + R5 + R6: Validate each standalone file.
	for _, path := range smFiles {
		errs = append(errs, validateStandaloneSemanticModel(path)...)
	}

	// R2: Scan PRDs for inline shorthand semantic_model:.
	for _, path := range prdFiles {
		errs = append(errs, validatePRDSemanticModel(path)...)
	}

	// R3: Scan docs/prompts/*.yaml for inline full semantic_model:.
	promptFiles, _ := filepath.Glob("docs/prompts/*.yaml")
	for _, path := range promptFiles {
		errs = append(errs, validatePromptSemanticModel(path)...)
	}

	return errs, len(smFiles)
}

// validateStandaloneSemanticModel validates a standalone semantic model file
// (docs/specs/semantic-models/*.yaml). Each top-level key names a behavior;
// its value must have a semantic_model sub-key with all four sections.
func validateStandaloneSemanticModel(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]struct {
		SemanticModel map[string]interface{} `yaml:"semantic_model"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return []string{fmt.Sprintf("%s: invalid YAML: %v", path, err)}
	}
	var errs []string
	for behavior, entry := range raw {
		prefix := fmt.Sprintf("%s [%s].semantic_model", path, behavior)
		sm := entry.SemanticModel
		if sm == nil {
			errs = append(errs, fmt.Sprintf("%s: behavior %q missing semantic_model sub-key", path, behavior))
			continue
		}
		errs = append(errs, smValidateSections(prefix, sm)...)
		errs = append(errs, smValidateSM7(prefix, sm)...)
		errs = append(errs, smValidateSM3(prefix, sm)...)
	}
	return errs
}

// validatePRDSemanticModel checks a PRD file for an optional semantic_model:
// field. When present, it must use the shorthand format (observe, reason, produce).
func validatePRDSemanticModel(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw struct {
		SemanticModel map[string]interface{} `yaml:"semantic_model"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil // YAML errors reported by schema validation
	}
	if raw.SemanticModel == nil {
		return nil // no semantic model in this PRD
	}
	var errs []string
	for _, key := range []string{"observe", "reason", "produce"} {
		if _, ok := raw.SemanticModel[key]; !ok {
			errs = append(errs, fmt.Sprintf("%s: semantic_model missing shorthand key %q (observe/reason/produce required)", path, key))
		}
	}
	return errs
}

// validatePromptSemanticModel checks a prompt template file for an optional
// semantic_model: field. When present, it must use the full format (SM1).
func validatePromptSemanticModel(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw struct {
		SemanticModel map[string]interface{} `yaml:"semantic_model"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	if raw.SemanticModel == nil {
		return nil
	}
	prefix := path + ".semantic_model"
	return smValidateSections(prefix, raw.SemanticModel)
}

// smValidateSections checks that a semantic model map has all four required
// sections: data_sources, features, algorithm, output_format (SM1).
func smValidateSections(prefix string, sm map[string]interface{}) []string {
	var errs []string
	for _, section := range []string{"data_sources", "features", "algorithm", "output_format"} {
		if _, ok := sm[section]; !ok {
			errs = append(errs, fmt.Sprintf("%s: missing required section %q (SM1)", prefix, section))
		}
	}
	return errs
}

// smValidateSM7 checks that a semantic model's name matches the
// {word}-{word}+ pattern and its version is valid semver (SM7).
func smValidateSM7(prefix string, sm map[string]interface{}) []string {
	var errs []string
	if name, _ := sm["name"].(string); name != "" {
		if !smNameRe.MatchString(name) {
			errs = append(errs, fmt.Sprintf("%s: name %q does not match {word}-{word}+ pattern (SM7)", prefix, name))
		}
	}
	if version, _ := sm["version"].(string); version != "" {
		if !smSemverRe.MatchString(version) {
			errs = append(errs, fmt.Sprintf("%s: version %q is not valid semver MAJOR.MINOR.PATCH (SM7)", prefix, version))
		}
	}
	return errs
}

// smValidateSM3 checks that each feature's source references a declared
// data_sources[*].id or another feature name (SM3).
func smValidateSM3(prefix string, sm map[string]interface{}) []string {
	// Collect declared data source IDs.
	dataSourceIDs := make(map[string]bool)
	if dsRaw, ok := sm["data_sources"]; ok {
		if dsList, ok := dsRaw.([]interface{}); ok {
			for _, ds := range dsList {
				if dsMap, ok := ds.(map[string]interface{}); ok {
					if id, ok := dsMap["id"].(string); ok && id != "" {
						dataSourceIDs[id] = true
					}
				}
			}
		}
	}

	// Collect declared feature names.
	featureNames := make(map[string]bool)
	type featureEntry struct{ name, source string }
	var features []featureEntry
	if featRaw, ok := sm["features"]; ok {
		if featList, ok := featRaw.([]interface{}); ok {
			for _, f := range featList {
				if fm, ok := f.(map[string]interface{}); ok {
					name, _ := fm["name"].(string)
					source, _ := fm["source"].(string)
					if name != "" {
						featureNames[name] = true
					}
					features = append(features, featureEntry{name, source})
				}
			}
		}
	}

	// Check each feature's source references.
	var errs []string
	for _, feat := range features {
		for _, ref := range smSourceRefs(feat.source) {
			if !dataSourceIDs[ref] && !featureNames[ref] {
				errs = append(errs, fmt.Sprintf("%s: feature %q source references undeclared id %q (SM3)", prefix, feat.name, ref))
			}
		}
	}
	return errs
}

// smSourceRefs extracts the base identifiers from a source expression.
// "network_state.site_capacity - project_state.implemented" -> ["network_state", "project_state"]
// "implementation_gap" -> ["implementation_gap"]
func smSourceRefs(source string) []string {
	tokens := smIdentRe.FindAllString(strings.ToLower(source), -1)
	seen := make(map[string]bool)
	var bases []string
	for _, tok := range tokens {
		base := strings.SplitN(tok, ".", 2)[0]
		if !seen[base] {
			seen[base] = true
			bases = append(bases, base)
		}
	}
	return bases
}
