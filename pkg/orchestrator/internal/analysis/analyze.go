// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

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
	UncoveredRItems                []string // R-items not covered by any acceptance criterion
	UncoveredACs                   []string // ACs not covered by any test case (warning)
	UntracedSuccessCriteria        []string // S-items with no AC traces (warning)
}

// AnalyzeCounts holds the artifact counts discovered during analysis.
type AnalyzeCounts struct {
	PRDs           int
	UseCases       int
	TestSuites     int
	SemanticModels int
}

// AnalyzeDeps holds dependencies for analysis operations.
type AnalyzeDeps struct {
	Log                    Logger
	Releases               []string             // configured release versions
	ValidateDocSchemas     func() []string       // schema validation (uses context types)
	ValidatePromptTemplate func(string) []string // prompt template validation
}

// CollectAnalyzeResult performs all cross-artifact consistency checks and
// returns the structured result without printing.
func CollectAnalyzeResult(deps AnalyzeDeps) (AnalyzeResult, AnalyzeCounts, error) {
	deps.Log("analyze: starting cross-artifact consistency checks")

	result := AnalyzeResult{}

	// 1. Load all PRDs
	prdFiles, err := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	if err != nil {
		return result, AnalyzeCounts{}, fmt.Errorf("globbing PRDs: %w", err)
	}
	prdIDs := make(map[string]bool)
	prdReqGroups := make(map[string]map[string]bool)    // PRD ID -> set of requirement group keys
	prdExports := make(map[string]map[string]bool)       // PRD ID -> set of exported symbol names
	prdDependsOn := make(map[string][]PRDDependsOn)      // PRD ID -> depends_on entries
	prdStructRefs := make(map[string][]PRDStructRef)     // PRD ID -> struct_refs entries
	prdACs := make(map[string][]AcceptanceCriterion)     // PRD ID -> acceptance criteria
	prdRItems := make(map[string][]string)               // PRD ID -> all R-item IDs (R1.1, R1.2, etc.)
	for _, path := range prdFiles {
		id := ExtractID(path)
		if id != "" {
			prdIDs[id] = true
		}
		if prd := loadYAML[PRDDoc](path); prd != nil {
			groups := make(map[string]bool)
			for groupKey, group := range prd.Requirements {
				groups[groupKey] = true
				for _, item := range group.Items {
					for itemKey := range item {
						prdRItems[id] = append(prdRItems[id], itemKey)
					}
				}
			}
			prdReqGroups[id] = groups
			if len(prd.AcceptanceCriteria) > 0 {
				prdACs[id] = prd.AcceptanceCriteria
			}
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
	deps.Log("analyze: found %d PRDs", len(prdIDs))

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
		return result, AnalyzeCounts{}, fmt.Errorf("globbing use cases: %w", err)
	}
	ucIDs := make(map[string]bool)
	ucToPRDs := make(map[string][]string)      // use case ID -> PRD IDs from touchpoints
	ucTouchpoints := make(map[string][]string) // use case ID -> raw touchpoint strings
	ucSuccessCriteria := make(map[string][]SuccessCriterion) // use case ID -> success criteria
	prdToReleases := make(map[string]map[string]bool) // PRD ID -> set of releases that reference it
	for _, path := range ucFiles {
		uc, err := LoadUseCase(path)
		if err != nil {
			deps.Log("analyze: skipping %s: %v", path, err)
			continue
		}
		ucIDs[uc.ID] = true
		ucToPRDs[uc.ID] = ExtractPRDsFromTouchpoints(uc.Touchpoints)
		ucTouchpoints[uc.ID] = uc.Touchpoints
		if len(uc.SuccessCriteria) > 0 {
			ucSuccessCriteria[uc.ID] = uc.SuccessCriteria
		}
		release := ExtractFileRelease(path)
		if release != "" {
			for _, prdID := range ucToPRDs[uc.ID] {
				if prdToReleases[prdID] == nil {
					prdToReleases[prdID] = make(map[string]bool)
				}
				prdToReleases[prdID][release] = true
			}
		}
	}
	deps.Log("analyze: found %d use cases", len(ucIDs))

	// 3. Load all test suites (per-release YAML specs)
	testFiles, err := filepath.Glob("docs/specs/test-suites/test-rel*.yaml")
	if err != nil {
		return result, AnalyzeCounts{}, fmt.Errorf("globbing test suites: %w", err)
	}
	testSuiteIDs := make(map[string]bool)
	testSuiteToUCs := make(map[string][]string) // test suite ID -> use case IDs from traces
	allTestCaseTraces := make(map[string]bool)  // set of all test case trace strings (e.g. "prd001-core AC1")
	for _, path := range testFiles {
		ts, err := LoadTestSuite(path)
		if err != nil {
			deps.Log("analyze: skipping %s: %v", path, err)
			continue
		}
		testSuiteIDs[ts.ID] = true
		testSuiteToUCs[ts.ID] = ExtractUseCaseIDsFromTraces(ts.Traces)
		for _, tc := range ts.TestCases {
			for _, trace := range tc.Traces {
				allTestCaseTraces[trace] = true
			}
		}
	}
	deps.Log("analyze: found %d test suites", len(testSuiteIDs))

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
				if len(release.UseCases) > 0 {
					roadmapReleaseIDs[release.ID] = true
				}
				for _, uc := range release.UseCases {
					roadmapUCs[uc.ID] = true
				}
			}
			deps.Log("analyze: found %d releases, %d use cases in roadmap", len(roadmapReleaseIDs), len(roadmapUCs))
		}
	}

	// Check 0: Configured releases exist in road-map.yaml
	for _, r := range deps.Releases {
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
		for _, cite := range ExtractCitationsFromTouchpoints(tps) {
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
	deps.Log("analyze: broken citations found %d", len(result.BrokenCitations))

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
	deps.Log("analyze: PRDs spanning multiple releases found %d", len(result.PRDsSpanningMultipleReleases))

	// Check 10: depends_on — referenced prd_id must exist; symbols_used must be
	// in the referenced PRD's package_contract.exports (if a contract is declared).
	for prdID, ddeps := range prdDependsOn {
		for _, dep := range ddeps {
			if !prdIDs[dep.PRDID] {
				result.DependsOnViolations = append(result.DependsOnViolations,
					fmt.Sprintf("%s: depends_on references non-existent PRD %s", prdID, dep.PRDID))
				continue
			}
			exports, hasContract := prdExports[dep.PRDID]
			if !hasContract {
				continue
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
	deps.Log("analyze: depends_on violations found %d", len(result.DependsOnViolations))

	// Check 11: dependency_rules — component_dependencies entries must not
	// violate rules with allowed=false.
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
	deps.Log("analyze: dependency rule violations found %d", len(result.DependencyRuleViolations))

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
	deps.Log("analyze: broken struct_refs found %d", len(result.BrokenStructRefs))

	// Check 13: component_dependencies gaps.
	if archDoc != nil && len(archDoc.ComponentDependencies) > 0 {
		compDepEndpoints := make(map[string]bool)
		for _, cd := range archDoc.ComponentDependencies {
			compDepEndpoints[cd.From] = true
			compDepEndpoints[cd.To] = true
		}
		for prdID, ddeps := range prdDependsOn {
			for _, dep := range ddeps {
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
	deps.Log("analyze: component_dep violations found %d", len(result.ComponentDepViolations))

	// Check 15: R-item coverage by acceptance criteria.
	// Every R-item in a PRD should appear in at least one AC's traces list.
	for prdID, rItems := range prdRItems {
		acs := prdACs[prdID]
		acTraced := make(map[string]bool)
		for _, ac := range acs {
			for _, tr := range ac.Traces {
				acTraced[tr] = true
			}
		}
		for _, rItem := range rItems {
			if !acTraced[rItem] {
				result.UncoveredRItems = append(result.UncoveredRItems,
					fmt.Sprintf("%s: R-item %s not covered by any acceptance criterion", prdID, rItem))
			}
		}
	}
	sort.Strings(result.UncoveredRItems)
	deps.Log("analyze: uncovered R-items found %d", len(result.UncoveredRItems))

	// Check 16: AC coverage by test cases (warning).
	// Every PRD AC should be traced by at least one test case.
	for prdID, acs := range prdACs {
		for _, ac := range acs {
			traceKey := fmt.Sprintf("%s %s", prdID, ac.ID)
			if !allTestCaseTraces[traceKey] {
				result.UncoveredACs = append(result.UncoveredACs,
					fmt.Sprintf("%s %s: not covered by any test case", prdID, ac.ID))
			}
		}
	}
	sort.Strings(result.UncoveredACs)
	deps.Log("analyze: uncovered ACs found %d (warning)", len(result.UncoveredACs))

	// Check 17: S-item traces to AC (warning).
	// Every UC success criterion should trace to at least one valid PRD AC.
	for ucID, scs := range ucSuccessCriteria {
		for _, sc := range scs {
			hasACTrace := false
			for _, tr := range sc.Traces {
				// Trace format: "prdNNN-name ACN"
				parts := strings.Fields(tr)
				if len(parts) >= 2 && strings.HasPrefix(parts[0], "prd") && strings.HasPrefix(parts[1], "AC") {
					hasACTrace = true
					break
				}
			}
			if !hasACTrace {
				result.UntracedSuccessCriteria = append(result.UntracedSuccessCriteria,
					fmt.Sprintf("%s %s: no AC trace found", ucID, sc.ID))
			}
		}
	}
	sort.Strings(result.UntracedSuccessCriteria)
	deps.Log("analyze: untraced success criteria found %d (warning)", len(result.UntracedSuccessCriteria))

	// Check 7: YAML schema validation.
	result.SchemaErrors = deps.ValidateDocSchemas()
	deps.Log("analyze: schema validation found %d error(s)", len(result.SchemaErrors))

	// Check 8: Constitution drift.
	result.ConstitutionDrift = DetectConstitutionDrift(deps.Log)
	deps.Log("analyze: constitution drift found %d file(s)", len(result.ConstitutionDrift))

	// Check 14: Semantic model validation.
	smErrs, smCount := ValidateSemanticModels(prdFiles)
	result.SemanticModelErrors = smErrs
	deps.Log("analyze: semantic model validation found %d error(s), %d standalone file(s)", len(smErrs), smCount)

	counts := AnalyzeCounts{
		PRDs:           len(prdIDs),
		UseCases:       len(ucIDs),
		TestSuites:     len(testSuiteIDs),
		SemanticModels: smCount,
	}
	return result, counts, nil
}

// Analyze performs cross-artifact consistency checks and prints a report.
func Analyze(deps AnalyzeDeps) error {
	result, counts, err := CollectAnalyzeResult(deps)
	if err != nil {
		return err
	}
	return result.PrintReport(counts.PRDs, counts.UseCases, counts.TestSuites, counts.SemanticModels)
}

// PrintSection prints a labeled list if items is non-empty, returning true.
func PrintSection(label string, items []string) bool {
	if len(items) == 0 {
		return false
	}
	fmt.Printf("\n⚠️  %s:\n", label)
	for _, item := range items {
		fmt.Printf("  - %s\n", item)
	}
	return true
}

// PrintReport formats the analysis results to stdout.
func (r AnalyzeResult) PrintReport(prdCount, ucCount, tsCount, smCount int) error {
	hasIssues := false
	hasIssues = PrintSection("Orphaned PRDs (no use case references them)", r.OrphanedPRDs) || hasIssues
	hasIssues = PrintSection("Releases without test suites (no docs/specs/test-suites/test-<release>.yaml)", r.ReleasesWithoutTestSuites) || hasIssues
	hasIssues = PrintSection("Orphaned test suites (traces don't reference any known use case)", r.OrphanedTestSuites) || hasIssues
	hasIssues = PrintSection("Broken touchpoints (use case references non-existent PRD)", r.BrokenTouchpoints) || hasIssues
	hasIssues = PrintSection("Use cases not in roadmap", r.UseCasesNotInRoadmap) || hasIssues
	hasIssues = PrintSection("YAML schema errors (fields not matching typed structs — data will be lost in measure prompt)", r.SchemaErrors) || hasIssues
	hasIssues = PrintSection("Constitution drift (docs/constitutions/ differs from embedded pkg/orchestrator/constitutions/)", r.ConstitutionDrift) || hasIssues
	hasIssues = PrintSection("Broken citations (touchpoint cites non-existent requirement group)", r.BrokenCitations) || hasIssues
	hasIssues = PrintSection("Invalid configured releases (not found in road-map.yaml)", r.InvalidReleases) || hasIssues
	hasIssues = PrintSection("PRDs spanning multiple releases (each PRD must belong to exactly one release)", r.PRDsSpanningMultipleReleases) || hasIssues
	hasIssues = PrintSection("depends_on violations (symbol not in package_contract or prd_id missing)", r.DependsOnViolations) || hasIssues
	hasIssues = PrintSection("Dependency rule violations (component_dependency violates allowed=false rule)", r.DependencyRuleViolations) || hasIssues
	hasIssues = PrintSection("Broken struct_refs (prd_id or requirement group not found)", r.BrokenStructRefs) || hasIssues
	hasIssues = PrintSection("component_dependencies gaps (depends_on entries missing from component_dependencies)", r.ComponentDepViolations) || hasIssues
	hasIssues = PrintSection("Semantic model errors (SM1 sections, SM3 traceability, SM7 naming)", r.SemanticModelErrors) || hasIssues
	hasIssues = PrintSection("Uncovered R-items (R-item not traced by any acceptance criterion)", r.UncoveredRItems) || hasIssues
	PrintSection("Uncovered ACs (AC not covered by any test case — warning)", r.UncoveredACs)
	PrintSection("Untraced success criteria (S-item with no AC trace — warning)", r.UntracedSuccessCriteria)

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

// AnalyzeUseCase holds the fields extracted from a use case file
// that are needed for cross-artifact consistency checks.
type AnalyzeUseCase struct {
	ID              string
	Touchpoints     []string
	SuccessCriteria []SuccessCriterion
}

// AnalyzeTestSuite holds the fields extracted from a test suite file
// that are needed for cross-artifact consistency checks.
type AnalyzeTestSuite struct {
	ID        string              `yaml:"id"`
	Traces    []string            `yaml:"traces"`
	TestCases []AnalyzeTestCase   `yaml:"test_cases"`
}

// AnalyzeTestCase holds the fields of a single test case within a
// test suite that are relevant for traceability checks.
type AnalyzeTestCase struct {
	UseCase string   `yaml:"use_case"`
	Name    string   `yaml:"name"`
	Traces  []string `yaml:"traces"`
}

// ExtractID extracts the ID from a file path like
// "docs/specs/product-requirements/prd001-feature.yaml" -> "prd001-feature".
func ExtractID(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// LoadUseCase loads a use case YAML file and extracts key fields.
func LoadUseCase(path string) (*AnalyzeUseCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw struct {
		ID              string              `yaml:"id"`
		Touchpoints     []map[string]string `yaml:"touchpoints"`
		SuccessCriteria []SuccessCriterion  `yaml:"success_criteria"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var touchpointStrings []string
	for _, tp := range raw.Touchpoints {
		for key, val := range tp {
			touchpointStrings = append(touchpointStrings, key+": "+val)
		}
	}

	return &AnalyzeUseCase{
		ID:              raw.ID,
		Touchpoints:     touchpointStrings,
		SuccessCriteria: raw.SuccessCriteria,
	}, nil
}

// LoadTestSuite loads a test suite YAML file and extracts key fields.
func LoadTestSuite(path string) (*AnalyzeTestSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ts AnalyzeTestSuite
	if err := yaml.Unmarshal(data, &ts); err != nil {
		return nil, err
	}
	return &ts, nil
}

// ExtractPRDsFromTouchpoints parses touchpoint strings to extract PRD IDs.
func ExtractPRDsFromTouchpoints(touchpoints []string) []string {
	var prds []string
	for _, tp := range touchpoints {
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

// ExtractUseCaseIDsFromTraces parses trace strings to extract use case IDs.
func ExtractUseCaseIDsFromTraces(traces []string) []string {
	var ucs []string
	for _, trace := range traces {
		parts := strings.Fields(trace)
		for _, part := range parts {
			if strings.HasPrefix(part, "rel") && strings.Contains(part, "-uc") {
				ucs = append(ucs, part)
			}
		}
	}
	return ucs
}

// PRDCitation represents a reference to a PRD with specific requirement
// groups extracted from a use case touchpoint.
type PRDCitation struct {
	PRDID  string
	Groups []string
}

// reqGroupRe matches requirement group references like "R1", "R2", "R9".
var reqGroupRe = regexp.MustCompile(`^R\d+`)

// ExtractReqGroup extracts the requirement group prefix from a reference.
func ExtractReqGroup(s string) string {
	return reqGroupRe.FindString(s)
}

// ExtractCitationsFromTouchpoints parses touchpoint strings to extract
// PRD IDs and their associated requirement group references.
func ExtractCitationsFromTouchpoints(touchpoints []string) []PRDCitation {
	var citations []PRDCitation
	for _, tp := range touchpoints {
		var current *PRDCitation
		for _, part := range strings.Fields(tp) {
			cleaned := strings.TrimLeft(part, "(")
			cleaned = strings.TrimRight(cleaned, "),.")

			if strings.HasPrefix(cleaned, "prd") {
				if current != nil {
					citations = append(citations, *current)
				}
				current = &PRDCitation{PRDID: cleaned}
				continue
			}
			if current == nil {
				continue
			}
			group := ExtractReqGroup(cleaned)
			if group == "" {
				continue
			}
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

// ExtractFileRelease extracts the release version from a use case filename.
// "docs/specs/use-cases/rel01.0-uc003-measure-workflow.yaml" -> "01.0"
func ExtractFileRelease(path string) string {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "rel") {
		return ""
	}
	if dash := strings.Index(base, "-uc"); dash > 3 {
		return base[3:dash]
	}
	return ""
}

// DetectConstitutionDrift compares each constitution file in
// docs/constitutions/ with its embedded copy in
// pkg/orchestrator/constitutions/.
func DetectConstitutionDrift(log Logger) []string {
	const (
		docsDir     = "docs/constitutions"
		embeddedDir = "pkg/orchestrator/constitutions"
	)

	entries, err := os.ReadDir(docsDir)
	if err != nil {
		log("detectConstitutionDrift: cannot read %s: %v", docsDir, err)
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
			continue
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

// ValidateSemanticModels runs all semantic model checks.
func ValidateSemanticModels(prdFiles []string) ([]string, int) {
	var errs []string

	smFiles, _ := filepath.Glob("docs/specs/semantic-models/*.yaml")

	for _, path := range smFiles {
		errs = append(errs, ValidateStandaloneSemanticModel(path)...)
	}

	for _, path := range prdFiles {
		errs = append(errs, ValidatePRDSemanticModel(path)...)
	}

	promptFiles, _ := filepath.Glob("docs/prompts/*.yaml")
	for _, path := range promptFiles {
		errs = append(errs, ValidatePromptSemanticModel(path)...)
	}

	return errs, len(smFiles)
}

func ValidateStandaloneSemanticModel(path string) []string {
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
		errs = append(errs, SmValidateSections(prefix, sm)...)
		errs = append(errs, SmValidateSM7(prefix, sm)...)
		errs = append(errs, SmValidateSM3(prefix, sm)...)
	}
	return errs
}

func ValidatePRDSemanticModel(path string) []string {
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
	var errs []string
	for _, key := range []string{"observe", "reason", "produce"} {
		if _, ok := raw.SemanticModel[key]; !ok {
			errs = append(errs, fmt.Sprintf("%s: semantic_model missing shorthand key %q (observe/reason/produce required)", path, key))
		}
	}
	return errs
}

func ValidatePromptSemanticModel(path string) []string {
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
	return SmValidateSections(prefix, raw.SemanticModel)
}

func SmValidateSections(prefix string, sm map[string]interface{}) []string {
	var errs []string
	for _, section := range []string{"data_sources", "features", "algorithm", "output_format"} {
		if _, ok := sm[section]; !ok {
			errs = append(errs, fmt.Sprintf("%s: missing required section %q (SM1)", prefix, section))
		}
	}
	return errs
}

func SmValidateSM7(prefix string, sm map[string]interface{}) []string {
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

func SmValidateSM3(prefix string, sm map[string]interface{}) []string {
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

	var errs []string
	for _, feat := range features {
		for _, ref := range SmSourceRefs(feat.source) {
			if !dataSourceIDs[ref] && !featureNames[ref] {
				errs = append(errs, fmt.Sprintf("%s: feature %q source references undeclared id %q (SM3)", prefix, feat.name, ref))
			}
		}
	}
	return errs
}

func SmSourceRefs(source string) []string {
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
