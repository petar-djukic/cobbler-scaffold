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
	Warnings          []string // advisory issues (logged but do not block import)
	Errors            []string // blocking issues not categorized below (e.g., completed R-items)
	WeightErrors      []string // weight budget violations (GH-2070)
	GranularityErrors []string // P9 requirement/AC/DD count range violations (GH-2070)
	FileNamingErrors  []string // P7 file naming convention violations (GH-2070)
}

// HasErrors returns true if the validation found any blocking issues.
func (v ValidationResult) HasErrors() bool {
	return len(v.Errors) > 0 || len(v.WeightErrors) > 0 || len(v.GranularityErrors) > 0 || len(v.FileNamingErrors) > 0
}

// AllErrors returns all error slices concatenated, for backward-compatible
// callers that treat all errors uniformly.
func (v ValidationResult) AllErrors() []string {
	all := make([]string, 0, len(v.Errors)+len(v.WeightErrors)+len(v.GranularityErrors)+len(v.FileNamingErrors))
	all = append(all, v.WeightErrors...)
	all = append(all, v.GranularityErrors...)
	all = append(all, v.FileNamingErrors...)
	all = append(all, v.Errors...)
	return all
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
// SRD reference pattern
// ---------------------------------------------------------------------------

// SRDRefPattern matches SRD requirement references in task requirement text.
// Examples: "srd003 R2", "srd004-ts R1.3", "srd002-sys requirement R2.5".
// Allows up to 2 intervening words between the SRD stem and R-number to handle
// Claude inserting words like "requirement" (e.g., "srd002-sys requirement R2.5").
// Group 1 = SRD stem (e.g., "srd003" or "srd004-ts").
// Group 2 = requirement group number (e.g., "2" from "R2").
// Group 3 = optional sub-item number (e.g., "3" from "R1.3"); empty for groups.
var SRDRefPattern = regexp.MustCompile(`(srd\d+[-\w]*)\s+(?:\w+\s+){0,2}R(\d+)(?:\.(\d+))?`)

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidateMeasureOutput checks proposed issues against P9 granularity ranges
// and P7 file naming conventions. Returns structured warnings and errors.
// All issues are logged regardless of enforcing mode. maxReqs is the
// operator-configured requirement cap (0 = unlimited). subItemCounts maps
// SRD stems to group IDs to sub-item counts; when a task requirement
// references a SRD group, the expanded sub-item count is used instead of 1.
// Expanded-count violations are logged as warnings (best-effort), not errors.
// reqStates, when non-nil, cross-references proposed R-items against
// requirements.yaml and rejects proposals targeting completed R-items (GH-1386).
func ValidateMeasureOutput(issues []ProposedIssue, maxReqs, maxWeight int, subItemCounts map[string]map[string]int, reqStates map[string]map[string]RequirementState) ValidationResult {
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

		// Compute expanded requirement count by resolving SRD group
		// references to their sub-item counts (GH-122).
		expandedCount := ExpandedRequirementCount(desc.Requirements, subItemCounts)

		// When max_weight_per_task is set, enforce weight budget instead
		// of requirement count (GH-1832). Weight takes precedence.
		if maxWeight > 0 {
			expandedWeight := ExpandedRequirementWeight(desc.Requirements, subItemCounts, reqStates)
			if expandedWeight > maxWeight {
				msg := fmt.Sprintf("[%d] %q: total weight is %d, max is %d", issue.Index, issue.Title, expandedWeight, maxWeight)
				Log("validateMeasureOutput: %s", msg)
				result.WeightErrors = append(result.WeightErrors, msg)
			}
		} else if maxReqs > 0 && expandedCount > maxReqs {
			// Fall back to count-based enforcement when weight is not configured.
			msg := fmt.Sprintf("[%d] %q: expanded sub-item count is %d, max is %d", issue.Index, issue.Title, expandedCount, maxReqs)
			Log("validateMeasureOutput: %s", msg)
			result.WeightErrors = append(result.WeightErrors, msg)
		}

		if desc.DeliverableType == "code" {
			if rCount < 5 || rCount > 8 {
				msg := fmt.Sprintf("[%d] %q: requirement count %d outside P9 range 5-8", issue.Index, issue.Title, rCount)
				Log("validateMeasureOutput: %s", msg)
				result.GranularityErrors = append(result.GranularityErrors, msg)
			}
			if acCount < 5 || acCount > 8 {
				msg := fmt.Sprintf("[%d] %q: acceptance criteria count %d outside P9 range 5-8", issue.Index, issue.Title, acCount)
				Log("validateMeasureOutput: %s", msg)
				result.GranularityErrors = append(result.GranularityErrors, msg)
			}
			if dCount < 3 || dCount > 5 {
				msg := fmt.Sprintf("[%d] %q: design decision count %d outside P9 range 3-5", issue.Index, issue.Title, dCount)
				Log("validateMeasureOutput: %s", msg)
				result.GranularityErrors = append(result.GranularityErrors, msg)
			}
		} else if desc.DeliverableType == "documentation" {
			if rCount < 2 || rCount > 4 {
				msg := fmt.Sprintf("[%d] %q: requirement count %d outside P9 doc range 2-4", issue.Index, issue.Title, rCount)
				Log("validateMeasureOutput: %s", msg)
				result.GranularityErrors = append(result.GranularityErrors, msg)
			}
			if acCount < 3 || acCount > 5 {
				msg := fmt.Sprintf("[%d] %q: acceptance criteria count %d outside P9 doc range 3-5", issue.Index, issue.Title, acCount)
				Log("validateMeasureOutput: %s", msg)
				result.GranularityErrors = append(result.GranularityErrors, msg)
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
					result.FileNamingErrors = append(result.FileNamingErrors, msg)
				}
			}
		}

		// Check for completed R-items: proposals must not target R-items
		// already marked complete in requirements.yaml (GH-1386).
		if len(reqStates) > 0 {
			for _, req := range desc.Requirements {
				matches := SRDRefPattern.FindAllStringSubmatch(req.Text, -1)
				for _, m := range matches {
					srdStem := m[1]
					groupNum := m[2]
					subItem := m[3]
					srdReqs := findSRDReqStates(reqStates, srdStem)
					if srdReqs == nil {
						continue
					}
					if subItem != "" {
						key := fmt.Sprintf("R%s.%s", groupNum, subItem)
						if st, ok := srdReqs[key]; ok && isRequirementComplete(st.Status) {
							msg := fmt.Sprintf("[%d] %q: requirement %s %s is already complete (issue #%d)",
								issue.Index, issue.Title, srdStem, key, st.Issue)
							Log("validateMeasureOutput: %s", msg)
							result.Errors = append(result.Errors, msg)
						}
					} else {
						// Group reference — check if ALL sub-items are complete.
						prefix := fmt.Sprintf("R%s.", groupNum)
						allComplete := true
						for k, st := range srdReqs {
							if strings.HasPrefix(k, prefix) && !isRequirementComplete(st.Status) {
								allComplete = false
								break
							}
						}
						if allComplete {
							// Check there are actually sub-items.
							hasItems := false
							for k := range srdReqs {
								if strings.HasPrefix(k, prefix) {
									hasItems = true
									break
								}
							}
							if hasItems {
								msg := fmt.Sprintf("[%d] %q: requirement group %s R%s is already fully complete",
									issue.Index, issue.Title, srdStem, groupNum)
								Log("validateMeasureOutput: %s", msg)
								result.Errors = append(result.Errors, msg)
							}
						}
					}
				}
			}
		}
	}
	return result
}

// findSRDReqStates looks up requirement states for a SRD stem, trying exact
// match and prefix match (e.g. "srd001" matches "srd001-core").
func findSRDReqStates(states map[string]map[string]RequirementState, stem string) map[string]RequirementState {
	if r, ok := states[stem]; ok {
		return r
	}
	for key, r := range states {
		if strings.HasPrefix(key, stem+"-") {
			return r
		}
	}
	return nil
}

// ExpandedRequirementWeight computes the total weight of a task's
// requirements by summing weights from requirements.yaml. A requirement
// referencing "srd003 R1.2" looks up the weight for that R-item. A group
// reference "srd003 R2" sums weights of all sub-items in that group.
// Requirements without a recognized reference default to weight 1. When
// reqStates is nil, falls back to ExpandedRequirementCount (GH-1832).
func ExpandedRequirementWeight(reqs []IssueDescItem, subItemCounts map[string]map[string]int, reqStates map[string]map[string]RequirementState) int {
	if len(reqStates) == 0 {
		return ExpandedRequirementCount(reqs, subItemCounts)
	}
	total := 0
	for _, req := range reqs {
		matches := SRDRefPattern.FindStringSubmatch(req.Text)
		if matches == nil {
			total++
			continue
		}
		srdStem := matches[1]
		groupNum := matches[2]
		subItem := matches[3]

		srdReqs := reqStates[srdStem]
		if srdReqs == nil {
			// Try short prefix (e.g., "srd003" for "srd003-core").
			for k, v := range reqStates {
				if idx := indexByte(k, '-'); idx > 0 && k[:idx] == srdStem {
					srdReqs = v
					break
				}
			}
		}
		if srdReqs == nil {
			total++
			continue
		}

		if subItem != "" {
			// Specific sub-item reference (e.g., R1.3).
			key := "R" + groupNum + "." + subItem
			if st, ok := srdReqs[key]; ok && st.Weight > 0 {
				total += st.Weight
			} else {
				total++
			}
			continue
		}

		// Group reference (e.g., R2). Sum weights of all sub-items in the group.
		prefix := "R" + groupNum + "."
		groupWeight := 0
		found := false
		for id, st := range srdReqs {
			if len(id) > len(prefix) && id[:len(prefix)] == prefix {
				w := st.Weight
				if w <= 0 {
					w = 1
				}
				groupWeight += w
				found = true
			}
		}
		if found {
			total += groupWeight
		} else {
			total++
		}
	}
	return total
}

func indexByte(s string, b byte) int {
	for i := range s {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// ExpandedRequirementCount computes the effective requirement count by
// parsing SRD group references from each requirement's text and expanding
// groups to their sub-item counts. A requirement referencing "srd003 R2"
// where R2 has 4 sub-items counts as 4, not 1. Requirements without a
// recognized SRD reference or referencing a specific sub-item (R1.3)
// count as 1.
func ExpandedRequirementCount(reqs []IssueDescItem, subItemCounts map[string]map[string]int) int {
	if len(subItemCounts) == 0 {
		return len(reqs)
	}
	total := 0
	for _, req := range reqs {
		matches := SRDRefPattern.FindStringSubmatch(req.Text)
		if matches == nil {
			total++
			continue
		}
		srdStem := matches[1]
		groupNum := matches[2]
		subItem := matches[3]

		// Specific sub-item reference (e.g., R1.3) counts as 1.
		if subItem != "" {
			total++
			continue
		}

		// Group reference (e.g., R2). Look up sub-item count.
		groupKey := "R" + groupNum
		if groups, ok := subItemCounts[srdStem]; ok {
			if count, found := groups[groupKey]; found {
				total += count
				continue
			}
		}
		// SRD or group not found — count as 1.
		total++
	}
	return total
}

// ---------------------------------------------------------------------------
// Overweight task splitting (GH-2072)
// ---------------------------------------------------------------------------

// SingleRequirementWeight returns the weight of a single requirement item
// by looking it up in reqStates. Returns 1 if the requirement has no SRD
// reference, no matching state, or an explicit weight of 0.
func SingleRequirementWeight(req IssueDescItem, subItemCounts map[string]map[string]int, reqStates map[string]map[string]RequirementState) int {
	return ExpandedRequirementWeight([]IssueDescItem{req}, subItemCounts, reqStates)
}

// SplitOverweightTasks examines each proposed issue and splits any whose
// total requirement weight exceeds maxWeight into smaller tasks that fit
// within the budget. Tasks within budget pass through unchanged.
//
// The splitting algorithm walks requirements in order and greedily packs
// them into sub-tasks. A requirement whose individual weight >= maxWeight
// gets its own task. Metadata (files, acceptance criteria, design decisions,
// deliverable type) is copied to each sub-task. Titles receive a
// "(part N/M)" suffix.
//
// When maxWeight <= 0 or reqStates is nil, all tasks pass through unchanged.
func SplitOverweightTasks(issues []ProposedIssue, maxWeight int, subItemCounts map[string]map[string]int, reqStates map[string]map[string]RequirementState) []ProposedIssue {
	if maxWeight <= 0 {
		return issues
	}
	var result []ProposedIssue
	for _, issue := range issues {
		var desc IssueDescription
		if err := yaml.Unmarshal([]byte(issue.Description), &desc); err != nil {
			result = append(result, issue)
			continue
		}

		totalWeight := ExpandedRequirementWeight(desc.Requirements, subItemCounts, reqStates)
		if totalWeight <= maxWeight {
			result = append(result, issue)
			continue
		}

		// Partition requirements into bins that fit the weight budget.
		bins := partitionRequirements(desc.Requirements, maxWeight, subItemCounts, reqStates)
		Log("splitOverweightTasks: %q (weight %d, max %d) split into %d parts", issue.Title, totalWeight, maxWeight, len(bins))

		for i, bin := range bins {
			partDesc := IssueDescription{
				DeliverableType:    desc.DeliverableType,
				Files:              desc.Files,
				Requirements:       bin,
				AcceptanceCriteria: desc.AcceptanceCriteria,
				DesignDecisions:    desc.DesignDecisions,
			}
			descBytes, _ := yaml.Marshal(partDesc)
			partTitle := issue.Title
			if len(bins) > 1 {
				partTitle = fmt.Sprintf("%s (part %d/%d)", issue.Title, i+1, len(bins))
			}
			result = append(result, ProposedIssue{
				Index:       issue.Index,
				Title:       partTitle,
				Description: string(descBytes),
				Dependency:  issue.Dependency,
			})
		}
	}
	return result
}

// partitionRequirements greedily packs requirements into bins where each
// bin's total weight <= maxWeight. Requirements with individual weight
// >= maxWeight get their own bin.
func partitionRequirements(reqs []IssueDescItem, maxWeight int, subItemCounts map[string]map[string]int, reqStates map[string]map[string]RequirementState) [][]IssueDescItem {
	var bins [][]IssueDescItem
	var current []IssueDescItem
	currentWeight := 0

	for _, req := range reqs {
		w := SingleRequirementWeight(req, subItemCounts, reqStates)

		// Requirement alone exceeds budget — give it its own bin.
		if w >= maxWeight {
			if len(current) > 0 {
				bins = append(bins, current)
				current = nil
				currentWeight = 0
			}
			bins = append(bins, []IssueDescItem{req})
			continue
		}

		// Adding this requirement would exceed budget — start new bin.
		if currentWeight+w > maxWeight {
			if len(current) > 0 {
				bins = append(bins, current)
			}
			current = []IssueDescItem{req}
			currentWeight = w
			continue
		}

		current = append(current, req)
		currentWeight += w
	}
	if len(current) > 0 {
		bins = append(bins, current)
	}
	return bins
}

// ---------------------------------------------------------------------------
// SRD loading and warnings
// ---------------------------------------------------------------------------

// SRDDoc is the minimal SRD structure needed for sub-item counting.
type SRDDoc struct {
	Requirements map[string]SRDRequirementGroup `yaml:"requirements"`
}

// SRDRequirementGroup represents a single requirement group with sub-items.
type SRDRequirementGroup struct {
	Items []any `yaml:"items"`
}

// LoadSRDSubItemCounts loads all SRDs from the standard path and returns a
// map of SRD stem -> group key -> sub-item count. A group with no sub-items
// maps to 1. The stem is the filename without path and extension (e.g.,
// "srd003-cobbler-workflows"); an additional entry keyed by the short prefix
// (e.g., "srd003") is added for fuzzy matching.
func LoadSRDSubItemCounts() map[string]map[string]int {
	paths, _ := filepath.Glob("docs/specs/software-requirements/srd*.yaml")
	counts := make(map[string]map[string]int, len(paths)*2)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var srd SRDDoc
		if err := yaml.Unmarshal(data, &srd); err != nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		groupCounts := make(map[string]int, len(srd.Requirements))
		for key, group := range srd.Requirements {
			if len(group.Items) > 0 {
				groupCounts[key] = len(group.Items)
			} else {
				groupCounts[key] = 1
			}
		}
		counts[stem] = groupCounts
		// Add short prefix entry (e.g., "srd003") for fuzzy matching.
		if idx := strings.IndexByte(stem, '-'); idx > 0 {
			short := stem[:idx]
			if _, exists := counts[short]; !exists {
				counts[short] = groupCounts
			}
		}
	}
	return counts
}

// WarnOversizedGroups loads SRDs and logs a warning for each requirement
// group whose sub-item count exceeds maxReqs. This is advisory and runs
// before the measure prompt is built so operators can restructure SRDs.
func WarnOversizedGroups(maxReqs int) {
	paths, _ := filepath.Glob("docs/specs/software-requirements/srd*.yaml")
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var srd SRDDoc
		if err := yaml.Unmarshal(data, &srd); err != nil {
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		keys := make([]string, 0, len(srd.Requirements))
		for k := range srd.Requirements {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			group := srd.Requirements[key]
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
// use case or release. Recognized statuses: "implemented", "done", "closed",
// "code_complete", "released" (GH-1703).
func UCStatusDone(status string) bool {
	s := strings.ToLower(status)
	return s == "implemented" || s == "done" || s == "closed" ||
		s == "code_complete" || s == "released"
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

// reRelease matches release patterns like "rel01.0" or "rel02.1" in text.
var reRelease = regexp.MustCompile(`rel(\d{2}\.\d)`)

// ExtractReleaseFromText returns the release version (e.g. "05.5") from text
// containing a pattern like "rel05.5". Returns "" if no release is found.
func ExtractReleaseFromText(text string) string {
	m := reRelease.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// IsOutOfScopeRelease returns true if the proposed task references a release
// that is not in the active releases list (GH-1703). When activeReleases is
// empty, no filtering is applied. The release is extracted from the task's
// title and description using the "relNN.N" pattern.
func IsOutOfScopeRelease(title, description string, activeReleases []string) bool {
	if len(activeReleases) == 0 {
		return false
	}
	rel := ExtractReleaseFromText(title + " " + description)
	if rel == "" {
		return false // cannot determine release — allow it
	}
	for _, r := range activeReleases {
		if r == rel {
			return false
		}
	}
	return true
}
