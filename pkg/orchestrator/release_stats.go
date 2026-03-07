// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

// releaseRow holds per-release metrics for the stats:releases table.
type releaseRow struct {
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

// ReleaseStats prints a table of roadmap releases with per-release PRD and
// requirement counts. PRD-to-release mapping uses use case touchpoints.
// Requirement counts come from PRD YAML files.
func (o *Orchestrator) ReleaseStats() error {
	rows, err := buildReleaseRows()
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

// buildReleaseRows loads the roadmap, PRD files, and use case touchpoints to
// produce one row per release with PRD and requirement metrics.
func buildReleaseRows() ([]releaseRow, error) {
	roadmap := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if roadmap == nil {
		return nil, nil
	}

	// Map PRD short names to release versions via use case touchpoints.
	prdRel := buildPRDReleaseMap()

	// Load requirement counts per PRD.
	_, reqsByPRD := countTotalPRDRequirements()

	// Group PRDs by release.
	type prdInfo struct {
		short string
		reqs  int
	}
	relPRDs := make(map[string][]prdInfo)
	for short, rel := range prdRel {
		// Only use the "prd-NNN" form, skip long stems like "prd003-cobbler-workflows".
		if !strings.HasPrefix(short, "prd-") || len(short) != 7 {
			continue
		}
		relPRDs[rel] = append(relPRDs[rel], prdInfo{short: short, reqs: reqsByPRD[short]})
	}

	// Determine which PRDs are "complete" by checking if all use cases in that
	// release referencing the PRD are done. We approximate: a PRD is complete if
	// the release status is "done", started if the release is in progress.
	// More precise: check roadmap use case statuses for the release.
	ucStatuses := make(map[string][]string) // release → list of use case statuses
	for _, rel := range roadmap.Releases {
		for _, uc := range rel.UseCases {
			ucStatuses[rel.Version] = append(ucStatuses[rel.Version], uc.Status)
		}
	}

	rows := make([]releaseRow, 0, len(roadmap.Releases))
	for _, rel := range roadmap.Releases {
		r := releaseRow{
			Version: rel.Version,
			Name:    rel.Name,
			Status:  rel.Status,
		}

		prds := relPRDs[rel.Version]
		sort.Slice(prds, func(i, j int) bool { return prds[i].short < prds[j].short })
		r.PRDs = len(prds)

		// Count total requirements and determine PRD completion.
		allDone := releaseAllUCsDone(ucStatuses[rel.Version])
		anyDone := releaseAnyUCDone(ucStatuses[rel.Version])

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

// releaseAllUCsDone returns true if every use case status is "done" or
// "implemented".
func releaseAllUCsDone(statuses []string) bool {
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

// releaseAnyUCDone returns true if at least one use case status is "done" or
// "implemented".
func releaseAnyUCDone(statuses []string) bool {
	for _, s := range statuses {
		if s == "done" || s == "implemented" {
			return true
		}
	}
	return false
}
