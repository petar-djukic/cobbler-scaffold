// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// UCCodeStatus holds the code implementation status for a single use case.
type UCCodeStatus struct {
	ID         string
	SpecStatus string // from road-map.yaml (e.g. "done", "not started")
	CodeStatus string // "implemented" or "not started"
	TestDir    string // path to test directory, empty if none
	TestFiles  int    // number of _test.go files found
}

// ReleaseCodeStatus holds the code implementation status for a release.
type ReleaseCodeStatus struct {
	Version       string
	Name          string
	SpecStatus    string // from road-map.yaml
	CodeReadiness string // "all implemented", "partial", "none"
	UseCases      []UCCodeStatus
}

// CodeStatusReport holds the full spec-vs-code comparison report.
type CodeStatusReport struct {
	Releases []ReleaseCodeStatus
	Gaps     []string
}

// ucIDRe extracts release version and UC number from a use case ID.
var ucIDRe = regexp.MustCompile(`^rel(\d+\.\d+)-uc(\d+)`)

// UCPrefixFromID extracts the structured prefix from a use case ID.
func UCPrefixFromID(ucID string) string {
	return ucIDRe.FindString(ucID)
}

// TestDirForUC returns the expected test directory path for a use case ID.
func TestDirForUC(ucID string) string {
	m := ucIDRe.FindStringSubmatch(ucID)
	if len(m) < 3 {
		return ""
	}
	return filepath.Join("tests", "rel"+m[1], "uc"+m[2])
}

// CountTestFiles counts _test.go files in a directory.
func CountTestFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			count++
		}
	}
	return count
}

// ScanTestDirectories walks the tests root and returns a map from UC
// prefix (e.g. "rel01.0-uc001") to the number of _test.go files found.
func ScanTestDirectories(testsRoot string) map[string]int {
	result := make(map[string]int)
	relDirs, err := os.ReadDir(testsRoot)
	if err != nil {
		return result
	}
	for _, relEntry := range relDirs {
		if !relEntry.IsDir() || !strings.HasPrefix(relEntry.Name(), "rel") {
			continue
		}
		relPath := filepath.Join(testsRoot, relEntry.Name())
		ucDirs, err := os.ReadDir(relPath)
		if err != nil {
			continue
		}
		for _, ucEntry := range ucDirs {
			if !ucEntry.IsDir() || !strings.HasPrefix(ucEntry.Name(), "uc") {
				continue
			}
			ucPath := filepath.Join(relPath, ucEntry.Name())
			prefix := relEntry.Name() + "-" + ucEntry.Name()
			testCount := CountTestFiles(ucPath)
			if testCount > 0 {
				result[prefix] = testCount
			}
		}
	}
	return result
}

// isRequirementComplete returns true if the status represents a completed
// or skipped R-item.
func isRequirementComplete(status string) bool {
	return status == "complete" || status == "complete_with_failures" || status == "skip"
}

// findSRDRequirements looks up the requirement map for a SRD stem, trying
// exact match first, then dash-delimited prefix match (e.g. "srd001" matches
// "srd001-core" but not "srd0011-other").
func findSRDRequirements(reqs map[string]map[string]RequirementState, stem string) map[string]RequirementState {
	if r, ok := reqs[stem]; ok {
		return r
	}
	var bestKey string
	var bestReqs map[string]RequirementState
	for key, r := range reqs {
		if strings.HasPrefix(key, stem+"-") {
			if bestKey == "" || len(key) > len(bestKey) {
				bestKey = key
				bestReqs = r
			}
		}
	}
	return bestReqs
}

// ComputeReqCompletion loads .cobbler/requirements.yaml and use case files,
// then determines which use cases have all their cited R-items complete.
// Returns a map from UC prefix (e.g. "rel01.0-uc001") to completion status.
// Returns nil when requirements.yaml is missing or empty (GH-1948).
func ComputeReqCompletion(cobblerDir string) map[string]bool {
	reqFile := loadYAML[RequirementsFile](filepath.Join(cobblerDir, "requirements.yaml"))
	if reqFile == nil || len(reqFile.Requirements) == 0 {
		return nil
	}

	ucFiles, err := filepath.Glob("docs/specs/use-cases/rel*.yaml")
	if err != nil || len(ucFiles) == 0 {
		return nil
	}

	result := make(map[string]bool)
	for _, path := range ucFiles {
		uc, err := LoadUseCase(path)
		if err != nil || len(uc.Touchpoints) == 0 {
			continue
		}
		prefix := UCPrefixFromID(uc.ID)
		if prefix == "" {
			continue
		}

		citations := ExtractCitationsFromTouchpoints(uc.Touchpoints)
		if len(citations) == 0 {
			continue
		}

		allComplete := true
		for _, cite := range citations {
			srdReqs := findSRDRequirements(reqFile.Requirements, cite.SRDID)
			if srdReqs == nil {
				allComplete = false
				break
			}
			for _, group := range cite.Groups {
				groupPrefix := group + "."
				found := false
				for key, st := range srdReqs {
					if strings.HasPrefix(key, groupPrefix) {
						found = true
						if !isRequirementComplete(st.Status) {
							allComplete = false
							break
						}
					}
				}
				if !found || !allComplete {
					allComplete = false
					break
				}
			}
			if !allComplete {
				break
			}
		}
		result[prefix] = allComplete
	}
	return result
}

// ComputeCodeStatus builds the code status report from the roadmap,
// a test directory scan, and optional per-UC requirements completion.
// When reqComplete is non-nil, a use case is considered implemented if
// it has test files OR all its cited R-items are complete in
// requirements.yaml (GH-1948).
func ComputeCodeStatus(roadmap *RoadmapDoc, testDirScan map[string]int, reqComplete map[string]bool) CodeStatusReport {
	var report CodeStatusReport

	for _, release := range roadmap.Releases {
		if len(release.UseCases) == 0 {
			continue
		}

		relStatus := ReleaseCodeStatus{
			Version:    release.Version,
			Name:       release.Name,
			SpecStatus: release.Status,
		}

		implemented := 0
		for _, uc := range release.UseCases {
			prefix := UCPrefixFromID(uc.ID)
			testCount := testDirScan[prefix]

			codeStatus := "not started"
			testDir := ""
			if testCount > 0 {
				codeStatus = "implemented"
				implemented++
				testDir = TestDirForUC(uc.ID)
			} else if reqComplete[prefix] {
				codeStatus = "implemented"
				implemented++
			}

			relStatus.UseCases = append(relStatus.UseCases, UCCodeStatus{
				ID:         uc.ID,
				SpecStatus: uc.Status,
				CodeStatus: codeStatus,
				TestDir:    testDir,
				TestFiles:  testCount,
			})
		}

		switch {
		case implemented == len(release.UseCases):
			relStatus.CodeReadiness = "all implemented"
		case implemented > 0:
			relStatus.CodeReadiness = "partial"
		default:
			relStatus.CodeReadiness = "none"
		}

		report.Releases = append(report.Releases, relStatus)
	}

	return report
}

// DetectSpecCodeGaps identifies discrepancies between specification status
// in road-map.yaml and actual code status based on test file presence.
func DetectSpecCodeGaps(report *CodeStatusReport) []string {
	var gaps []string
	for i := range report.Releases {
		rel := &report.Releases[i]
		if rel.SpecStatus == "done" && rel.CodeReadiness != "all implemented" {
			gaps = append(gaps, fmt.Sprintf(
				"release %s: spec status is %q but code readiness is %q",
				rel.Version, rel.SpecStatus, rel.CodeReadiness))
		}
		for _, uc := range rel.UseCases {
			if uc.SpecStatus == "done" && uc.CodeStatus == "not started" {
				gaps = append(gaps, fmt.Sprintf(
					"%s: spec status is %q but no test files found",
					uc.ID, uc.SpecStatus))
			}
		}
	}
	return gaps
}

// PrintCodeStatus loads the roadmap, scans test directories, and prints
// the code implementation status report.
func PrintCodeStatus() error {
	roadmap := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if roadmap == nil {
		return fmt.Errorf("cannot load docs/road-map.yaml")
	}

	testScan := ScanTestDirectories("tests")

	report := ComputeCodeStatus(roadmap, testScan, nil)
	report.Gaps = DetectSpecCodeGaps(&report)

	PrintCodeStatusReport(&report)

	if len(report.Gaps) > 0 {
		return fmt.Errorf("found %d spec-vs-code gap(s)", len(report.Gaps))
	}
	return nil
}

// StatusIcon returns a visual indicator for a status string.
func StatusIcon(status string) string {
	switch status {
	case "done", "implemented", "all implemented":
		return "[ok]"
	case "partial":
		return "[~~]"
	case "not started", "none":
		return "[  ]"
	default:
		return "[??]"
	}
}

// PrintCodeStatusReport formats the code status report to stdout.
func PrintCodeStatusReport(report *CodeStatusReport) {
	fmt.Println("Code Status Report")
	fmt.Println("==================")

	for _, rel := range report.Releases {
		fmt.Printf("\nRelease %s — %s\n", rel.Version, rel.Name)
		fmt.Printf("  Spec status:    %s\n", rel.SpecStatus)
		fmt.Printf("  Code readiness: %s\n", rel.CodeReadiness)

		for _, uc := range rel.UseCases {
			specTag := StatusIcon(uc.SpecStatus)
			codeTag := StatusIcon(uc.CodeStatus)
			fmt.Printf("    %s spec  %s code  %s", specTag, codeTag, uc.ID)
			if uc.TestFiles > 0 {
				fmt.Printf(" (%d test files)", uc.TestFiles)
			}
			fmt.Println()
		}
	}

	if len(report.Gaps) > 0 {
		fmt.Printf("\nGaps between specification and code:\n")
		for _, gap := range report.Gaps {
			fmt.Printf("  - %s\n", gap)
		}
	} else {
		fmt.Printf("\nNo gaps between specification and code.\n")
	}
}
