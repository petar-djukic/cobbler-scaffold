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
	SRDs         int
	SRDsComplete int
	SRDsStarted  int
	SRDsNoReqs   int // SRDs with zero requirements (not counted as untouched)
	Reqs         int
	ReqsDone     int
}

// ---------------------------------------------------------------------------
// Minimal YAML document types for loading roadmap, SRD, and use case files.
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

// SRDDoc corresponds to docs/specs/software-requirements/srd*.yaml
// (stats-relevant fields only).
type SRDDoc struct {
	Requirements map[string]SRDRequirementGroup `yaml:"requirements"`
}

// SRDRequirementGroup is a requirement section within a SRD.
// Items uses []any to accept both plain string values and weighted values (GH-1832).
type SRDRequirementGroup struct {
	Title string `yaml:"title"`
	Items []any  `yaml:"items"`
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

// PrintReleaseStats prints a table of roadmap releases with per-release SRD
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
	fmt.Fprintln(w, "Release\tName\tStatus\tSRDs\tComplete\tStarted\tUntouched\tReqs\tDone")
	for _, r := range rows {
		untouched := r.SRDs - r.SRDsComplete - r.SRDsStarted - r.SRDsNoReqs
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n",
			r.Version, r.Name, r.Status,
			r.SRDs, r.SRDsComplete, r.SRDsStarted, untouched,
			r.Reqs, r.ReqsDone)
	}
	return w.Flush()
}

// BuildReleaseRows loads the roadmap, SRD files, and use case touchpoints to
// produce one row per release with SRD and requirement metrics.
func BuildReleaseRows() ([]ReleaseRow, error) {
	roadmap := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if roadmap == nil {
		return nil, nil
	}

	// Map SRD short names to release versions via use case touchpoints.
	prdRel := BuildSRDReleaseMap()

	// Load requirement counts per SRD.
	_, reqsBySRD := CountTotalSRDRequirements()

	// Group SRDs by release.
	type prdInfo struct {
		short string
		reqs  int
	}
	relSRDs := make(map[string][]prdInfo)
	for stem, rel := range prdRel {
		relSRDs[rel] = append(relSRDs[rel], prdInfo{short: stem, reqs: reqsBySRD[stem]})
	}

	// Determine which SRDs are "complete" by checking if all use cases in that
	// release referencing the SRD are done.
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

		prds := relSRDs[rel.Version]
		sort.Slice(prds, func(i, j int) bool { return prds[i].short < prds[j].short })
		r.SRDs = len(prds)

		// Count total requirements and determine SRD completion.
		allDone := ReleaseAllUCsDone(ucStatuses[rel.Version])
		anyDone := ReleaseAnyUCDone(ucStatuses[rel.Version])

		for _, p := range prds {
			r.Reqs += p.reqs
			if p.reqs == 0 {
				r.SRDsNoReqs++
				continue
			}
			if allDone {
				r.SRDsComplete++
				r.ReqsDone += p.reqs
			} else if anyDone {
				r.SRDsStarted++
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

// CountTotalSRDRequirements loads all SRD files and counts the total number of
// requirement items across all groups. Returns the total count and a map from
// SRD short name (e.g. "srd-003") to its item count.
func CountTotalSRDRequirements() (int, map[string]int) {
	paths, _ := filepath.Glob("docs/specs/software-requirements/srd*.yaml")
	bySRD := make(map[string]int, len(paths))
	total := 0
	for _, path := range paths {
		srd := loadYAML[SRDDoc](path)
		if srd == nil {
			continue
		}
		count := 0
		for _, group := range srd.Requirements {
			count += len(group.Items)
		}
		total += count
		// Store under the full stem (e.g. "srd006-cat") which is what
		// ExtractSRDRefs now produces for srdNNN-name format references.
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		bySRD[stem] = count
	}
	return total, bySRD
}

// BuildSRDReleaseMap loads use case files and maps SRD short names (e.g.
// "srd-003") to their roadmap release version by parsing touchpoint references.
func BuildSRDReleaseMap() map[string]string {
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
		// Touchpoints reference SRDs like "srd003-cobbler-workflows R1, R2".
		for _, tp := range uc.Touchpoints {
			for _, v := range tp {
				for _, word := range strings.Fields(v) {
					w := strings.ToLower(strings.Trim(word, ".,;:()[]`\"'"))
					if !strings.HasPrefix(w, "srd") || len(w) < 6 {
						continue
					}
					if w[3] >= '0' && w[3] <= '9' {
						// Strip trailing requirement refs (e.g. "R1," suffix
						// is already trimmed by Trim above). Keep full stem
						// like "srd003-cobbler-workflows".
						// Also strip anything that is just the "srdNNN" prefix
						// without a hyphen-separated name (e.g. if the
						// touchpoint uses a bare "srd003 R1" token, skip it).
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
