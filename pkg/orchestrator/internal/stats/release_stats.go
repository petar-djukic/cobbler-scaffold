// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// ReleaseRow holds per-release metrics for the stats:releases table.
type ReleaseRow struct {
	Version      string
	Name         string
	Status       string
	PRDs         int
	PRDsComplete int
	PRDsStarted  int
	PRDsNoReqs   int // PRDs with zero requirements (not counted as untouched)
	Reqs         int
	ReqsDone     int
}

// ---------------------------------------------------------------------------
// Minimal YAML document types for loading roadmap, PRD, and use case files.
// These duplicate only the fields needed by stats; the canonical types live
// in the parent orchestrator package (context.go).
// ---------------------------------------------------------------------------

// RoadmapDoc corresponds to docs/road-map.yaml (stats-relevant fields only).
type RoadmapDoc struct {
	Releases []RoadmapRelease `yaml:"releases"`
}

// RoadmapRelease is a single release entry in the roadmap.
type RoadmapRelease struct {
	Version  string           `yaml:"version"`
	Name     string           `yaml:"name"`
	Status   string           `yaml:"status"`
	UseCases []RoadmapUseCase `yaml:"use_cases"`
}

// RoadmapUseCase is a use case entry within a roadmap release.
type RoadmapUseCase struct {
	ID     string `yaml:"id"`
	Status string `yaml:"status"`
}

// PRDDoc corresponds to docs/specs/product-requirements/prd*.yaml
// (stats-relevant fields only).
type PRDDoc struct {
	Requirements map[string]PRDRequirementGroup `yaml:"requirements"`
}

// PRDRequirementGroup is a requirement section within a PRD.
type PRDRequirementGroup struct {
	Title string              `yaml:"title"`
	Items []map[string]string `yaml:"items"`
}

// UseCaseDoc corresponds to docs/specs/use-cases/rel*.yaml
// (stats-relevant fields only).
type UseCaseDoc struct {
	Touchpoints []map[string]string `yaml:"touchpoints"`
}

// loadYAML reads a YAML file and unmarshals it into T.
// Returns nil if the file does not exist or cannot be parsed.
func loadYAML[T any](path string) *T {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil
	}
	return &v
}

// PrintReleaseStats prints a table of roadmap releases with per-release PRD
// and requirement counts.
func PrintReleaseStats() error {
	rows, err := BuildReleaseRows()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("no releases found in road-map.yaml")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Release\tName\tStatus\tPRDs\tComplete\tStarted\tUntouched\tReqs\tDone")
	for _, r := range rows {
		untouched := r.PRDs - r.PRDsComplete - r.PRDsStarted - r.PRDsNoReqs
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n",
			r.Version, r.Name, r.Status,
			r.PRDs, r.PRDsComplete, r.PRDsStarted, untouched,
			r.Reqs, r.ReqsDone)
	}
	return w.Flush()
}

// BuildReleaseRows loads the roadmap, PRD files, and use case touchpoints to
// produce one row per release with PRD and requirement metrics.
func BuildReleaseRows() ([]ReleaseRow, error) {
	roadmap := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if roadmap == nil {
		return nil, nil
	}

	// Map PRD short names to release versions via use case touchpoints.
	prdRel := BuildPRDReleaseMap()

	// Load requirement counts per PRD.
	_, reqsByPRD := CountTotalPRDRequirements()

	// Group PRDs by release.
	type prdInfo struct {
		short string
		reqs  int
	}
	relPRDs := make(map[string][]prdInfo)
	for stem, rel := range prdRel {
		relPRDs[rel] = append(relPRDs[rel], prdInfo{short: stem, reqs: reqsByPRD[stem]})
	}

	// Determine which PRDs are "complete" by checking if all use cases in that
	// release referencing the PRD are done.
	ucStatuses := make(map[string][]string) // release → list of use case statuses
	for _, rel := range roadmap.Releases {
		for _, uc := range rel.UseCases {
			ucStatuses[rel.Version] = append(ucStatuses[rel.Version], uc.Status)
		}
	}

	rows := make([]ReleaseRow, 0, len(roadmap.Releases))
	for _, rel := range roadmap.Releases {
		r := ReleaseRow{
			Version: rel.Version,
			Name:    rel.Name,
			Status:  rel.Status,
		}

		prds := relPRDs[rel.Version]
		sort.Slice(prds, func(i, j int) bool { return prds[i].short < prds[j].short })
		r.PRDs = len(prds)

		// Count total requirements and determine PRD completion.
		allDone := ReleaseAllUCsDone(ucStatuses[rel.Version])
		anyDone := ReleaseAnyUCDone(ucStatuses[rel.Version])

		for _, p := range prds {
			r.Reqs += p.reqs
			if p.reqs == 0 {
				r.PRDsNoReqs++
				continue
			}
			if allDone {
				r.PRDsComplete++
				r.ReqsDone += p.reqs
			} else if anyDone {
				r.PRDsStarted++
			}
		}

		rows = append(rows, r)
	}

	return rows, nil
}

// ReleaseAllUCsDone returns true if every use case status is "done" or
// "implemented".
func ReleaseAllUCsDone(statuses []string) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, s := range statuses {
		if s != "done" && s != "implemented" {
			return false
		}
	}
	return true
}

// ReleaseAnyUCDone returns true if at least one use case status is "done" or
// "implemented".
func ReleaseAnyUCDone(statuses []string) bool {
	for _, s := range statuses {
		if s == "done" || s == "implemented" {
			return true
		}
	}
	return false
}

// CountTotalPRDRequirements loads all PRD files and counts the total number of
// requirement items across all groups. Returns the total count and a map from
// PRD short name (e.g. "prd-003") to its item count.
func CountTotalPRDRequirements() (int, map[string]int) {
	paths, _ := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	byPRD := make(map[string]int, len(paths))
	total := 0
	for _, path := range paths {
		prd := loadYAML[PRDDoc](path)
		if prd == nil {
			continue
		}
		count := 0
		for _, group := range prd.Requirements {
			count += len(group.Items)
		}
		total += count
		// Store under the full stem (e.g. "prd006-cat") which is what
		// ExtractPRDRefs now produces for prdNNN-name format references.
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		byPRD[stem] = count
	}
	return total, byPRD
}

// BuildPRDReleaseMap loads use case files and maps PRD short names (e.g.
// "prd-003") to their roadmap release version by parsing touchpoint references.
func BuildPRDReleaseMap() map[string]string {
	paths, _ := filepath.Glob("docs/specs/use-cases/rel*.yaml")
	prdRelease := make(map[string]string)
	for _, path := range paths {
		base := filepath.Base(path)
		// Extract release from filename: "rel01.0-uc003-..." → "01.0"
		rel := ""
		if strings.HasPrefix(base, "rel") {
			if dash := strings.Index(base, "-uc"); dash > 3 {
				rel = base[3:dash]
			}
		}
		if rel == "" {
			continue
		}

		uc := loadYAML[UseCaseDoc](path)
		if uc == nil {
			continue
		}
		// Touchpoints reference PRDs like "prd003-cobbler-workflows R1, R2".
		for _, tp := range uc.Touchpoints {
			for _, v := range tp {
				for _, word := range strings.Fields(v) {
					w := strings.ToLower(strings.Trim(word, ".,;:()[]`\"'"))
					if !strings.HasPrefix(w, "prd") || len(w) < 6 {
						continue
					}
					if w[3] >= '0' && w[3] <= '9' {
						// Strip trailing requirement refs (e.g. "R1," suffix
						// is already trimmed by Trim above). Keep full stem
						// like "prd003-cobbler-workflows".
						// Also strip anything that is just the "prdNNN" prefix
						// without a hyphen-separated name (e.g. if the
						// touchpoint uses a bare "prd003 R1" token, skip it).
						if !strings.ContainsRune(w[3:], '-') {
							continue
						}
						if _, exists := prdRelease[w]; !exists {
							prdRelease[w] = rel
						}
					}
				}
			}
		}
	}
	return prdRelease
}
