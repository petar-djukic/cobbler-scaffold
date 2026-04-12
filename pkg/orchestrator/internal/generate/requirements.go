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

// RequirementState represents the status of a single R-item.
type RequirementState struct {
	Status string `yaml:"status"`
	Issue  int    `yaml:"issue,omitempty"`
	Weight int    `yaml:"weight,omitempty"` // from SRD; default 1 (GH-1832)
}

// RequirementsFile is the top-level structure of .cobbler/requirements.yaml.
type RequirementsFile struct {
	Requirements map[string]map[string]RequirementState `yaml:"requirements"`
}

// RequirementsFileName is the basename of the requirements state file inside
// the cobbler directory.
const RequirementsFileName = "requirements.yaml"

// LoadRequirementStates reads requirements.yaml and returns the state map.
// Returns nil if the file does not exist or cannot be parsed.
func LoadRequirementStates(cobblerDir string) map[string]map[string]RequirementState {
	data, err := os.ReadFile(filepath.Join(cobblerDir, RequirementsFileName))
	if err != nil {
		return nil
	}
	return ParseRequirementStates(data)
}

// ParseRequirementStates parses requirements.yaml content from raw bytes.
// Returns nil if the data cannot be parsed.
func ParseRequirementStates(data []byte) map[string]map[string]RequirementState {
	var rf RequirementsFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil
	}
	return rf.Requirements
}

// GenerateRequirementsFile scans all SRD YAML files in the given directory
// for R-items and writes a requirements state file. When preserveExisting is
// false, all items start with status "ready" (full regeneration). When true,
// existing requirement states are preserved for items still present in SRDs,
// and only new items default to "ready" (incremental generation).
func GenerateRequirementsFile(srdDir, cobblerDir string, preserveExisting bool) (string, error) {
	paths, err := filepath.Glob(filepath.Join(srdDir, "srd*.yaml"))
	if err != nil {
		return "", fmt.Errorf("globbing SRDs in %s: %w", srdDir, err)
	}

	// Always load existing requirements.yaml to preserve weights.
	// Weights are the sole authority in requirements.yaml (GH-2080) and
	// must survive regeneration regardless of preserveExisting (GH-2117).
	existing := LoadRequirementStates(cobblerDir)

	allReqs := make(map[string]map[string]RequirementState)

	for _, path := range paths {
		slug := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		items := extractRItemsFromSRD(path)
		if len(items) == 0 {
			continue
		}
		group := make(map[string]RequirementState, len(items))
		for _, item := range items {
			if prev, ok := existing[slug]; ok {
				if st, ok := prev[item.ID]; ok {
					if preserveExisting {
						// Preserve both status and weight.
						group[item.ID] = st
					} else {
						// Reset status to "ready" but preserve weight (GH-2117).
						group[item.ID] = RequirementState{Status: "ready", Weight: st.Weight}
					}
					continue
				}
			}
			// New items default to weight 1. Weights are managed in
			// requirements.yaml, not in SRDs (GH-2080).
			group[item.ID] = RequirementState{Status: "ready", Weight: 1}
		}
		allReqs[slug] = group
	}

	if len(allReqs) == 0 {
		Log("generateRequirementsFile: no R-items found in %s", srdDir)
	}

	rf := RequirementsFile{Requirements: allReqs}
	out, err := yaml.Marshal(rf)
	if err != nil {
		return "", fmt.Errorf("marshalling requirements: %w", err)
	}

	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", cobblerDir, err)
	}
	outPath := filepath.Join(cobblerDir, RequirementsFileName)
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", outPath, err)
	}

	// Count total R-items for logging.
	total := 0
	preserved := 0
	for slug, g := range allReqs {
		total += len(g)
		if existing != nil {
			if prev, ok := existing[slug]; ok {
				for id := range g {
					if _, ok := prev[id]; ok {
						preserved++
					}
				}
			}
		}
	}
	if preserveExisting && preserved > 0 {
		Log("generateRequirementsFile: wrote %d R-items (%d preserved) from %d SRDs to %s", total, preserved, len(allReqs), outPath)
	} else {
		Log("generateRequirementsFile: wrote %d R-items from %d SRDs to %s", total, len(allReqs), outPath)
	}
	return outPath, nil
}

// UpdateRequirementsFile reads the requirements state file, extracts SRD
// requirement references from the task description YAML, and transitions
// matching entries to their completion state with the given issue number.
//
// When zeroLOC is true, requirements are marked "uncertain" (stitch completed
// but produced no file changes). Otherwise they transition to "complete" or
// "complete_with_failures" based on testsPassed. Only requirements in
// "ready", "proposed", or "in_progress" states are transitioned (GH-2123).
//
// If the file does not exist, the function returns nil (backward compat).
func UpdateRequirementsFile(cobblerDir, description string, issueNumber int, testsPassed, zeroLOC bool) error {
	reqPath := filepath.Join(cobblerDir, RequirementsFileName)
	data, err := os.ReadFile(reqPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", reqPath, err)
	}

	var rf RequirementsFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("parsing %s: %w", reqPath, err)
	}
	if rf.Requirements == nil {
		return nil
	}

	refs := extractSRDRefsFromDescription(description)
	if len(refs) == 0 {
		return nil
	}

	status := "complete"
	if zeroLOC {
		status = "uncertain"
	} else if !testsPassed {
		status = "complete_with_failures"
	}

	updated := 0
	for _, ref := range refs {
		srdReqs := findSRDRequirements(rf.Requirements, ref.SRDStem)
		if srdReqs == nil {
			continue
		}
		if ref.SubItem != "" {
			// Specific sub-item reference (e.g. R1.2).
			key := fmt.Sprintf("R%s.%s", ref.Group, ref.SubItem)
			if st, ok := srdReqs[key]; ok && isRequirementTransitionable(st.Status) {
				srdReqs[key] = RequirementState{Status: status, Issue: issueNumber, Weight: st.Weight}
				updated++
			}
		} else {
			// Group reference (e.g. R1) — mark all sub-items.
			prefix := fmt.Sprintf("R%s.", ref.Group)
			for key, st := range srdReqs {
				if strings.HasPrefix(key, prefix) && isRequirementTransitionable(st.Status) {
					srdReqs[key] = RequirementState{Status: status, Issue: issueNumber, Weight: st.Weight}
					updated++
				}
			}
		}
	}

	if updated == 0 {
		return nil
	}

	out, err := yaml.Marshal(rf)
	if err != nil {
		return fmt.Errorf("marshalling updated requirements: %w", err)
	}
	if err := os.WriteFile(reqPath, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", reqPath, err)
	}
	Log("updateRequirementsFile: marked %d R-items as %s for issue #%d", updated, status, issueNumber)
	return nil
}

// MarkRequirementsProposed transitions R-items cited in a task description
// from "ready" to "proposed" before the GitHub issue is created. This
// prevents duplicate task proposals across measure cycles (GH-2123).
func MarkRequirementsProposed(cobblerDir, description string) error {
	return transitionRequirements(cobblerDir, description, "proposed", "ready")
}

// MarkRequirementsInProgress transitions R-items cited in a task description
// from "proposed" (or "ready" for backward compat) to "in_progress" when
// stitch claims the task (GH-2123).
func MarkRequirementsInProgress(cobblerDir, description string) error {
	return transitionRequirements(cobblerDir, description, "in_progress", "ready", "proposed")
}

// transitionRequirements is a helper that reads requirements.yaml, extracts
// SRD refs from the description, and transitions matching R-items from any
// of the fromStatuses to toStatus.
func transitionRequirements(cobblerDir, description, toStatus string, fromStatuses ...string) error {
	reqPath := filepath.Join(cobblerDir, RequirementsFileName)
	data, err := os.ReadFile(reqPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", reqPath, err)
	}

	var rf RequirementsFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("parsing %s: %w", reqPath, err)
	}
	if rf.Requirements == nil {
		return nil
	}

	refs := extractSRDRefsFromDescription(description)
	if len(refs) == 0 {
		return nil
	}

	fromSet := make(map[string]bool, len(fromStatuses))
	for _, s := range fromStatuses {
		fromSet[s] = true
	}

	updated := 0
	for _, ref := range refs {
		srdReqs := findSRDRequirements(rf.Requirements, ref.SRDStem)
		if srdReqs == nil {
			continue
		}
		if ref.SubItem != "" {
			key := fmt.Sprintf("R%s.%s", ref.Group, ref.SubItem)
			if st, ok := srdReqs[key]; ok && fromSet[st.Status] {
				srdReqs[key] = RequirementState{Status: toStatus, Issue: st.Issue, Weight: st.Weight}
				updated++
			}
		} else {
			prefix := fmt.Sprintf("R%s.", ref.Group)
			for key, st := range srdReqs {
				if strings.HasPrefix(key, prefix) && fromSet[st.Status] {
					srdReqs[key] = RequirementState{Status: toStatus, Issue: st.Issue, Weight: st.Weight}
					updated++
				}
			}
		}
	}

	if updated == 0 {
		return nil
	}

	out, err := yaml.Marshal(rf)
	if err != nil {
		return fmt.Errorf("marshalling requirements: %w", err)
	}
	if err := os.WriteFile(reqPath, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", reqPath, err)
	}
	Log("transitionRequirements: marked %d R-items as %s", updated, toStatus)
	return nil
}

// isRequirementComplete returns true if the status represents a completed
// or skipped R-item. Skipped items are requirements that cannot be fulfilled
// by the generator (e.g. manual Magefile authoring) and are treated as
// complete for UC validation and measure filtering (GH-1451).
func isRequirementComplete(status string) bool {
	return status == "complete" || status == "complete_with_failures" || status == "skip"
}

// IsRequirementTerminal returns true if the status represents a terminal
// state — the requirement will not be worked on further. Terminal states
// are: complete, complete_with_failures, failed, uncertain, and skip.
// The generator stops when all requirements reach a terminal state (GH-2123).
func IsRequirementTerminal(status string) bool {
	return isRequirementComplete(status) || status == "failed" || status == "uncertain"
}

// isRequirementTransitionable returns true if the status allows transition
// to a completion state. Requirements in ready, proposed, or in_progress
// can be transitioned by UpdateRequirementsFile (GH-2123).
func isRequirementTransitionable(status string) bool {
	return status == "ready" || status == "proposed" || status == "in_progress"
}

// IsRequirementClaimed returns true if the requirement is already claimed
// by a task (proposed or in_progress) or completed. Used by measure
// validation to reject proposals targeting non-ready requirements (GH-2123).
func IsRequirementClaimed(status string) bool {
	return status != "ready"
}

// AllRefsAlreadyComplete checks whether every SRD requirement reference in
// a task description is already marked complete in the given requirement
// states. Returns true only when at least one reference is found and all
// are complete. Used by stitch to skip tasks whose R-items were completed
// by an earlier task in the same measure batch (GH-1434).
func AllRefsAlreadyComplete(description string, reqStates map[string]map[string]RequirementState) bool {
	refs := extractSRDRefsFromDescription(description)
	if len(refs) == 0 || len(reqStates) == 0 {
		return false
	}
	for _, ref := range refs {
		srdReqs := findSRDRequirements(reqStates, ref.SRDStem)
		if srdReqs == nil {
			return false // unknown SRD — cannot verify
		}
		if ref.SubItem != "" {
			key := fmt.Sprintf("R%s.%s", ref.Group, ref.SubItem)
			st, ok := srdReqs[key]
			if !ok || !isRequirementComplete(st.Status) {
				return false
			}
		} else {
			// Group reference — all sub-items must be complete.
			prefix := fmt.Sprintf("R%s.", ref.Group)
			found := false
			for k, st := range srdReqs {
				if strings.HasPrefix(k, prefix) {
					found = true
					if !isRequirementComplete(st.Status) {
						return false
					}
				}
			}
			if !found {
				return false // group has no sub-items
			}
		}
	}
	return true
}

// srdRef holds a parsed SRD requirement reference.
type srdRef struct {
	SRDStem string // e.g. "srd001" or "srd001-orchestrator-core"
	Group   string // e.g. "1" from R1
	SubItem string // e.g. "2" from R1.2; empty for group refs
}

// extractSRDRefsFromDescription parses the YAML description's requirements
// section and returns all SRD refs found in requirement text fields.
func extractSRDRefsFromDescription(description string) []srdRef {
	var desc IssueDescription
	if err := yaml.Unmarshal([]byte(description), &desc); err != nil {
		return nil
	}
	var refs []srdRef
	for _, req := range desc.Requirements {
		matches := SRDRefPattern.FindAllStringSubmatch(req.Text, -1)
		for _, m := range matches {
			refs = append(refs, srdRef{
				SRDStem: m[1],
				Group:   m[2],
				SubItem: m[3],
			})
		}
	}
	return refs
}

// findSRDRequirements looks up the requirement map for a SRD stem, trying
// exact match first, then dash-delimited prefix match (e.g. "srd001" matches
// "srd001-core" but not "srd0011-other"). When multiple candidates match,
// the longest (most specific) key wins.
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

// UCRequirementsComplete checks whether all R-items cited by a use case's
// touchpoints are marked "complete" in requirements.yaml. Returns true when
// every cited R-group's sub-items are complete, and the list of any remaining
// ready items. If requirements.yaml is missing, returns false with no items.
func UCRequirementsComplete(cobblerDir string, touchpoints []string) (bool, []string) {
	reqPath := filepath.Join(cobblerDir, RequirementsFileName)
	data, err := os.ReadFile(reqPath)
	if err != nil {
		return false, nil
	}

	var rf RequirementsFile
	if err := yaml.Unmarshal(data, &rf); err != nil || len(rf.Requirements) == 0 {
		return false, nil
	}

	// Extract SRD citations from touchpoints (e.g. "srd001-core R1, R2").
	citations := extractTouchpointCitations(touchpoints)
	if len(citations) == 0 {
		return false, nil
	}

	var remaining []string
	for _, cite := range citations {
		srdReqs := findSRDRequirements(rf.Requirements, cite.srdID)
		if srdReqs == nil {
			// SRD not in requirements file — cannot verify.
			remaining = append(remaining, fmt.Sprintf("%s (missing)", cite.srdID))
			continue
		}
		for _, group := range cite.groups {
			prefix := group + "."
			for key, st := range srdReqs {
				if strings.HasPrefix(key, prefix) && !isRequirementComplete(st.Status) {
					remaining = append(remaining, fmt.Sprintf("%s %s", cite.srdID, key))
				}
			}
		}
	}

	sort.Strings(remaining)
	return len(remaining) == 0, remaining
}

// touchpointCitation holds a SRD reference and its cited R-groups from a
// use case touchpoint.
type touchpointCitation struct {
	srdID  string   // e.g. "srd001-orchestrator-core"
	groups []string // e.g. ["R1", "R2"]
}

// TouchpointSRDRefRe matches SRD + R-group references in touchpoint text.
var TouchpointSRDRefRe = regexp.MustCompile(`(srd\d+[-\w]*)\s+(R\d+(?:\s*,\s*R\d+)*)`)

// BareSRDRefRe matches bare SRD stems in touchpoint text, including those
// without R-group references (e.g., "srd096-users" in parentheses). This
// catches touchpoints that omit R-group citations (GH-1960).
var BareSRDRefRe = regexp.MustCompile(`srd\d+[-\w]*`)

// extractTouchpointCitations parses touchpoint strings to extract SRD
// citations with their requirement groups.
func extractTouchpointCitations(touchpoints []string) []touchpointCitation {
	var citations []touchpointCitation
	seen := make(map[string]map[string]bool) // srdID → set of groups
	for _, tp := range touchpoints {
		matches := TouchpointSRDRefRe.FindAllStringSubmatch(tp, -1)
		for _, m := range matches {
			srdID := m[1]
			groupStr := m[2]
			if seen[srdID] == nil {
				seen[srdID] = make(map[string]bool)
			}
			for _, g := range strings.Split(groupStr, ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					seen[srdID][g] = true
				}
			}
		}
	}
	for srdID, groups := range seen {
		var gs []string
		for g := range groups {
			gs = append(gs, g)
		}
		sort.Strings(gs)
		citations = append(citations, touchpointCitation{srdID: srdID, groups: gs})
	}
	sort.Slice(citations, func(i, j int) bool { return citations[i].srdID < citations[j].srdID })
	return citations
}

// rItemInfo holds an R-item ID extracted from a SRD. Weight is not read
// from SRDs — weights live only in requirements.yaml (GH-2080).
type rItemInfo struct {
	ID string
}

// extractRItemsFromSRD reads a SRD YAML file and returns all R-item IDs
// (e.g. R1.1, R1.2, R2.1), sorted by ID. Weights are not extracted from
// SRDs — they are managed in requirements.yaml (GH-2080).
func extractRItemsFromSRD(path string) []rItemInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var srd SRDDoc
	if err := yaml.Unmarshal(data, &srd); err != nil {
		return nil
	}

	var items []rItemInfo
	for _, group := range srd.Requirements {
		for _, item := range group.Items {
			switch v := item.(type) {
			case map[string]interface{}:
				for k := range v {
					items = append(items, rItemInfo{ID: k})
				}
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

// ValidateTaskWeights parses an input string of the form
// "srd005-wc R2.5, R2.6, R3.1, R3.2", looks up each R-item's weight
// from requirements.yaml, and returns a formatted report with per-item
// weights, total, max, and PASS/FAIL. Used by the measure agent to
// validate weight budgets before finalizing proposals (GH-2078).
func ValidateTaskWeights(cobblerDir, input string, maxWeight int) string {
	// Parse input: first token is SRD stem, rest are comma-separated R-items.
	input = strings.TrimSpace(input)
	parts := strings.SplitN(input, " ", 2)
	if len(parts) < 2 {
		return fmt.Sprintf("FAIL: invalid input %q — expected 'srd-stem R1.1, R1.2, ...'", input)
	}
	srdStem := parts[0]
	rItemsStr := parts[1]

	// Parse R-item list.
	var rItems []string
	for _, item := range strings.Split(rItemsStr, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			rItems = append(rItems, item)
		}
	}
	if len(rItems) == 0 {
		return fmt.Sprintf("FAIL: no R-items found in %q", input)
	}

	// Load requirements.yaml.
	reqStates := LoadRequirementStates(cobblerDir)

	// Find SRD requirements (exact or prefix match).
	var srdReqs map[string]RequirementState
	if reqStates != nil {
		srdReqs = findSRDRequirements(reqStates, srdStem)
	}

	// Look up weights and build report.
	var lines []string
	total := 0
	for _, rItem := range rItems {
		key := rItem
		// Normalize: ensure it starts with "R".
		if !strings.HasPrefix(key, "R") {
			key = "R" + key
		}

		w := 1 // default weight
		if srdReqs != nil {
			if st, ok := srdReqs[key]; ok && st.Weight > 0 {
				w = st.Weight
			} else if !ok {
				// Check if this is a group reference (e.g. R2 without sub-item).
				// Sum all sub-items in the group.
				prefix := key + "."
				groupWeight := 0
				found := false
				for id, st := range srdReqs {
					if strings.HasPrefix(id, prefix) {
						gw := st.Weight
						if gw <= 0 {
							gw = 1
						}
						groupWeight += gw
						found = true
					}
				}
				if found {
					lines = append(lines, fmt.Sprintf("%s %s: weight %d (group)", srdStem, key, groupWeight))
					total += groupWeight
					continue
				}
				// Not found at all — use default.
				lines = append(lines, fmt.Sprintf("%s %s: weight %d (default, not found)", srdStem, key, w))
				total += w
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s: weight %d", srdStem, key, w))
		total += w
	}

	lines = append(lines, fmt.Sprintf("total: %d", total))
	lines = append(lines, fmt.Sprintf("max: %d", maxWeight))
	if maxWeight > 0 && total > maxWeight {
		lines = append(lines, fmt.Sprintf("FAIL: total weight %d exceeds max %d", total, maxWeight))
	} else {
		lines = append(lines, "PASS")
	}
	return strings.Join(lines, "\n")
}

// extractRItems reads a SRD YAML file and returns all R-item IDs in sorted order.
func extractRItems(path string) []string {
	items := extractRItemsFromSRD(path)
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}
