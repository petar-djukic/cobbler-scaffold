// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// IssueDescription is the subset of fields parsed from an issue description
// YAML for advisory validation.
type IssueDescription struct {
	DeliverableType    string          `yaml:"deliverable_type"`
	Files              []IssueDescFile `yaml:"files"`
	Requirements       []IssueDescItem `yaml:"requirements"`
	AcceptanceCriteria []IssueDescItem `yaml:"acceptance_criteria"`
	DesignDecisions    []IssueDescItem `yaml:"design_decisions"`
}

// IssueDescFile holds a file path from an issue description.
type IssueDescFile struct {
	Path string `yaml:"path"`
}

// IssueDescItem holds an ID+text pair from an issue description.
type IssueDescItem struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// ValidationResult holds the outcome of measure output validation.
type ValidationResult struct {
	Warnings []string // advisory issues (logged but do not block import)
	Errors   []string // blocking issues (cause rejection in enforcing mode)
}

// HasErrors returns true if the validation found blocking issues.
func (v ValidationResult) HasErrors() bool {
	return len(v.Errors) > 0
}

// ProposedIssue is the minimal interface needed by validation and logging.
// The parent package aliases this to the internal/github ProposedIssue.
type ProposedIssue struct {
	Index       int    `yaml:"index" json:"index"`
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	Dependency  int    `yaml:"dependency" json:"dependency"`
}

// ---------------------------------------------------------------------------
// Pure functions
// ---------------------------------------------------------------------------

// TruncateSHA returns the first 8 characters of a SHA, or the full
// string if shorter.
func TruncateSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// MeasureReleasesConstraint returns a hard constraint string to append to the
// measure prompt when a release scope is configured. Returns "" when no scope
// is set. Releases (list) takes precedence over Release (single string).
func MeasureReleasesConstraint(releases []string, release string) string {
	if len(releases) > 0 {
		return fmt.Sprintf(
			"\n\nRelease scope: You MUST only propose tasks for use cases in releases [%s]. Do not propose tasks for any other release.",
			strings.Join(releases, ", "),
		)
	}
	if release != "" {
		return fmt.Sprintf(
			"\n\nRelease scope: You MUST only propose tasks for use cases in release %q or earlier. Do not propose tasks for later releases.",
			release,
		)
	}
	return ""
}

// ---------------------------------------------------------------------------
// PRD reference pattern
// ---------------------------------------------------------------------------

// PRDRefPattern matches PRD requirement references in task requirement text.
// Examples: "prd003 R2", "prd004-ts R1.3", "prd001-orchestrator-core R5".
// Group 1 = PRD stem (e.g., "prd003" or "prd004-ts").
// Group 2 = requirement group number (e.g., "2" from "R2").
// Group 3 = optional sub-item number (e.g., "3" from "R1.3"); empty for groups.
var PRDRefPattern = regexp.MustCompile(`(prd\d+[-\w]*)\s+R(\d+)(?:\.(\d+))?`)

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidateMeasureOutput checks proposed issues against P9 granularity ranges
// and P7 file naming conventions. Returns structured warnings and errors.
// All issues are logged regardless of enforcing mode. maxReqs is the
// operator-configured requirement cap (0 = unlimited). subItemCounts maps
// PRD stems to group IDs to sub-item counts; when a task requirement
// references a PRD group, the expanded sub-item count is used instead of 1.
// Expanded-count violations are logged as warnings (best-effort), not errors.
func ValidateMeasureOutput(issues []ProposedIssue, maxReqs int, subItemCounts map[string]map[string]int) ValidationResult {
	var result ValidationResult
	for _, issue := range issues {
		var desc IssueDescription
		if err := yaml.Unmarshal([]byte(issue.Description), &desc); err != nil {
			msg := fmt.Sprintf("[%d] %q: could not parse description: %v", issue.Index, issue.Title, err)
			Log("validateMeasureOutput: %s", msg)
			result.Warnings = append(result.Warnings, msg)
			continue
		}

		rCount := len(desc.Requirements)
		acCount := len(desc.AcceptanceCriteria)
		dCount := len(desc.DesignDecisions)

		// Compute expanded requirement count by resolving PRD group
		// references to their sub-item counts (GH-122).
		expandedCount := ExpandedRequirementCount(desc.Requirements, subItemCounts)

		// Enforce max_requirements_per_task on the expanded sub-item count,
		// not the top-level group count (GH-535). A requirement referencing
		// "prd003 R2" where R2 has 10 sub-items counts as 10, not 1.
		if maxReqs > 0 && expandedCount > maxReqs {
			msg := fmt.Sprintf("[%d] %q: expanded sub-item count is %d, max is %d", issue.Index, issue.Title, expandedCount, maxReqs)
			Log("validateMeasureOutput: %s", msg)
			result.Errors = append(result.Errors, msg)
		}

		if desc.DeliverableType == "code" {
			if rCount < 5 || rCount > 8 {
				msg := fmt.Sprintf("[%d] %q: requirement count %d outside P9 range 5-8", issue.Index, issue.Title, rCount)
				Log("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
			if acCount < 5 || acCount > 8 {
				msg := fmt.Sprintf("[%d] %q: acceptance criteria count %d outside P9 range 5-8", issue.Index, issue.Title, acCount)
				Log("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
			if dCount < 3 || dCount > 5 {
				msg := fmt.Sprintf("[%d] %q: design decision count %d outside P9 range 3-5", issue.Index, issue.Title, dCount)
				Log("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
		} else if desc.DeliverableType == "documentation" {
			if rCount < 2 || rCount > 4 {
				msg := fmt.Sprintf("[%d] %q: requirement count %d outside P9 doc range 2-4", issue.Index, issue.Title, rCount)
				Log("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
			if acCount < 3 || acCount > 5 {
				msg := fmt.Sprintf("[%d] %q: acceptance criteria count %d outside P9 doc range 3-5", issue.Index, issue.Title, acCount)
				Log("validateMeasureOutput: %s", msg)
				result.Errors = append(result.Errors, msg)
			}
		}

		// Check for P7 violation: file named after its package.
		for _, f := range desc.Files {
			parts := strings.Split(f.Path, "/")
			if len(parts) >= 2 {
				dir := parts[len(parts)-2]
				file := parts[len(parts)-1]
				if file == dir+".go" || file == dir+"_test.go" {
					msg := fmt.Sprintf("[%d] %q: file %s matches package name (P7 violation)", issue.Index, issue.Title, f.Path)
					Log("validateMeasureOutput: %s", msg)
					result.Errors = append(result.Errors, msg)
				}
			}
		}
	}
	return result
}

// ExpandedRequirementCount computes the effective requirement count by
// parsing PRD group references from each requirement's text and expanding
// groups to their sub-item counts. A requirement referencing "prd003 R2"
// where R2 has 4 sub-items counts as 4, not 1. Requirements without a
// recognized PRD reference or referencing a specific sub-item (R1.3)
// count as 1.
func ExpandedRequirementCount(reqs []IssueDescItem, subItemCounts map[string]map[string]int) int {
	if len(subItemCounts) == 0 {
		return len(reqs)
	}
	total := 0
	for _, req := range reqs {
		matches := PRDRefPattern.FindStringSubmatch(req.Text)
		if matches == nil {
			total++
			continue
		}
		prdStem := matches[1]
		groupNum := matches[2]
		subItem := matches[3]

		// Specific sub-item reference (e.g., R1.3) counts as 1.
		if subItem != "" {
			total++
			continue
		}

		// Group reference (e.g., R2). Look up sub-item count.
		groupKey := "R" + groupNum
		if groups, ok := subItemCounts[prdStem]; ok {
			if count, found := groups[groupKey]; found {
				total += count
				continue
			}
		}
		// PRD or group not found — count as 1.
		total++
	}
	return total
}

// ---------------------------------------------------------------------------
// PRD loading and warnings
// ---------------------------------------------------------------------------

// PRDDoc is the minimal PRD structure needed for sub-item counting.
type PRDDoc struct {
	Requirements map[string]PRDRequirementGroup `yaml:"requirements"`
}

// PRDRequirementGroup represents a single requirement group with sub-items.
type PRDRequirementGroup struct {
	Items []any `yaml:"items"`
}

// LoadPRDSubItemCounts loads all PRDs from the standard path and returns a
// map of PRD stem -> group key -> sub-item count. A group with no sub-items
// maps to 1. The stem is the filename without path and extension (e.g.,
// "prd003-cobbler-workflows"); an additional entry keyed by the short prefix
// (e.g., "prd003") is added for fuzzy matching.
func LoadPRDSubItemCounts() map[string]map[string]int {
	paths, _ := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	counts := make(map[string]map[string]int, len(paths)*2)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var prd PRDDoc
		if err := yaml.Unmarshal(data, &prd); err != nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		groupCounts := make(map[string]int, len(prd.Requirements))
		for key, group := range prd.Requirements {
			if len(group.Items) > 0 {
				groupCounts[key] = len(group.Items)
			} else {
				groupCounts[key] = 1
			}
		}
		counts[stem] = groupCounts
		// Add short prefix entry (e.g., "prd003") for fuzzy matching.
		if idx := strings.IndexByte(stem, '-'); idx > 0 {
			short := stem[:idx]
			if _, exists := counts[short]; !exists {
				counts[short] = groupCounts
			}
		}
	}
	return counts
}

// WarnOversizedGroups loads PRDs and logs a warning for each requirement
// group whose sub-item count exceeds maxReqs. This is advisory and runs
// before the measure prompt is built so operators can restructure PRDs.
func WarnOversizedGroups(maxReqs int) {
	paths, _ := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var prd PRDDoc
		if err := yaml.Unmarshal(data, &prd); err != nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		keys := make([]string, 0, len(prd.Requirements))
		for k := range prd.Requirements {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			group := prd.Requirements[key]
			if len(group.Items) > maxReqs {
				Log("warning: %s %s has %d sub-items (max_requirements_per_task=%d); consider splitting this requirement group",
					stem, key, len(group.Items), maxReqs)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Measure log persistence
// ---------------------------------------------------------------------------

// AppendMeasureLog merges newIssues into the persistent measure.yaml list.
// measure.yaml is a single growing YAML list of all issues proposed across runs.
func AppendMeasureLog(cobblerDir string, newIssues []ProposedIssue) {
	logPath := filepath.Join(cobblerDir, "measure.yaml")

	var existing []ProposedIssue
	if data, err := os.ReadFile(logPath); err == nil {
		if err := yaml.Unmarshal(data, &existing); err != nil {
			Log("appendMeasureLog: could not parse existing list, starting fresh: %v", err)
			existing = nil
		}
	}

	combined := append(existing, newIssues...)
	out, err := yaml.Marshal(combined)
	if err != nil {
		Log("appendMeasureLog: marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(logPath, out, 0o644); err != nil {
		Log("appendMeasureLog: write failed: %v", err)
		return
	}
	Log("appendMeasureLog: %d total issues in %s", len(combined), logPath)
}

// ---------------------------------------------------------------------------
// Release filtering
// ---------------------------------------------------------------------------

// RoadmapDoc is the minimal road-map structure needed for release filtering.
type RoadmapDoc struct {
	Releases []RoadmapRelease `yaml:"releases"`
}

// RoadmapRelease represents a single release in road-map.yaml.
type RoadmapRelease struct {
	Version string `yaml:"version"`
	Status  string `yaml:"status"`
}

// UCStatusDone returns true when the status string indicates a completed
// use case or release ("implemented", "done", or "closed").
func UCStatusDone(status string) bool {
	s := strings.ToLower(status)
	return s == "implemented" || s == "done" || s == "closed"
}

// FilterImplementedReleases returns a copy of releases with any entry whose
// road-map status is "implemented" or "done" removed. Releases not found in
// road-map.yaml are kept (unknown status is not treated as implemented).
// Returns nil when all releases are filtered out.
func FilterImplementedReleases(releases []string) []string {
	if len(releases) == 0 {
		return releases
	}
	data, err := os.ReadFile("docs/road-map.yaml")
	if err != nil {
		return releases
	}
	var rm RoadmapDoc
	if err := yaml.Unmarshal(data, &rm); err != nil {
		return releases
	}
	status := make(map[string]string, len(rm.Releases))
	for _, rel := range rm.Releases {
		status[rel.Version] = rel.Status
	}
	var out []string
	for _, r := range releases {
		if UCStatusDone(status[r]) {
			Log("filterImplementedReleases: dropping implemented release %s from constraint", r)
			continue
		}
		out = append(out, r)
	}
	return out
}

// FilterImplementedRelease returns the release string unchanged unless the
// road-map marks that release as implemented/done, in which case "" is
// returned so no legacy single-release constraint is emitted.
func FilterImplementedRelease(release string) string {
	if release == "" {
		return ""
	}
	data, err := os.ReadFile("docs/road-map.yaml")
	if err != nil {
		return release
	}
	var rm RoadmapDoc
	if err := yaml.Unmarshal(data, &rm); err != nil {
		return release
	}
	for _, rel := range rm.Releases {
		if rel.Version == release && UCStatusDone(rel.Status) {
			Log("filterImplementedRelease: dropping implemented release %s from constraint", release)
			return ""
		}
	}
	return release
}
