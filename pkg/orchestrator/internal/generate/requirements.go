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
	var rf RequirementsFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil
	}
	return rf.Requirements
}

// GenerateRequirementsFile scans all PRD YAML files in the given directory
// for R-items and writes a requirements state file. When preserveExisting is
// false, all items start with status "ready" (full regeneration). When true,
// existing requirement states are preserved for items still present in PRDs,
// and only new items default to "ready" (incremental generation).
func GenerateRequirementsFile(prdDir, cobblerDir string, preserveExisting bool) (string, error) {
	paths, err := filepath.Glob(filepath.Join(prdDir, "prd*.yaml"))
	if err != nil {
		return "", fmt.Errorf("globbing PRDs in %s: %w", prdDir, err)
	}

	// Load existing states when preserving.
	var existing map[string]map[string]RequirementState
	if preserveExisting {
		existing = LoadRequirementStates(cobblerDir)
	}

	allReqs := make(map[string]map[string]RequirementState)

	for _, path := range paths {
		slug := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		items := extractRItems(path)
		if len(items) == 0 {
			continue
		}
		group := make(map[string]RequirementState, len(items))
		for _, id := range items {
			if existing != nil {
				if prev, ok := existing[slug]; ok {
					if st, ok := prev[id]; ok {
						group[id] = st
						continue
					}
				}
			}
			group[id] = RequirementState{Status: "ready"}
		}
		allReqs[slug] = group
	}

	if len(allReqs) == 0 {
		Log("generateRequirementsFile: no R-items found in %s", prdDir)
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
		Log("generateRequirementsFile: wrote %d R-items (%d preserved) from %d PRDs to %s", total, preserved, len(allReqs), outPath)
	} else {
		Log("generateRequirementsFile: wrote %d R-items from %d PRDs to %s", total, len(allReqs), outPath)
	}
	return outPath, nil
}

// UpdateRequirementsFile reads the requirements state file, extracts PRD
// requirement references from the task description YAML, and transitions
// matching entries from "ready" to "complete" (or "complete_with_failures"
// when testsPassed is false) with the given issue number.
// If the file does not exist, the function returns nil (backward compat).
func UpdateRequirementsFile(cobblerDir, description string, issueNumber int, testsPassed bool) error {
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

	refs := extractPRDRefsFromDescription(description)
	if len(refs) == 0 {
		return nil
	}

	status := "complete"
	if !testsPassed {
		status = "complete_with_failures"
	}

	updated := 0
	for _, ref := range refs {
		prdReqs := findPRDRequirements(rf.Requirements, ref.PRDStem)
		if prdReqs == nil {
			continue
		}
		if ref.SubItem != "" {
			// Specific sub-item reference (e.g. R1.2).
			key := fmt.Sprintf("R%s.%s", ref.Group, ref.SubItem)
			if st, ok := prdReqs[key]; ok && st.Status == "ready" {
				prdReqs[key] = RequirementState{Status: status, Issue: issueNumber}
				updated++
			}
		} else {
			// Group reference (e.g. R1) — mark all sub-items.
			prefix := fmt.Sprintf("R%s.", ref.Group)
			for key, st := range prdReqs {
				if strings.HasPrefix(key, prefix) && st.Status == "ready" {
					prdReqs[key] = RequirementState{Status: status, Issue: issueNumber}
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

// isRequirementComplete returns true if the status represents a completed
// R-item, including items completed with test failures.
func isRequirementComplete(status string) bool {
	return status == "complete" || status == "complete_with_failures"
}

// AllRefsAlreadyComplete checks whether every PRD requirement reference in
// a task description is already marked complete in the given requirement
// states. Returns true only when at least one reference is found and all
// are complete. Used by stitch to skip tasks whose R-items were completed
// by an earlier task in the same measure batch (GH-1434).
func AllRefsAlreadyComplete(description string, reqStates map[string]map[string]RequirementState) bool {
	refs := extractPRDRefsFromDescription(description)
	if len(refs) == 0 || len(reqStates) == 0 {
		return false
	}
	for _, ref := range refs {
		prdReqs := findPRDRequirements(reqStates, ref.PRDStem)
		if prdReqs == nil {
			return false // unknown PRD — cannot verify
		}
		if ref.SubItem != "" {
			key := fmt.Sprintf("R%s.%s", ref.Group, ref.SubItem)
			st, ok := prdReqs[key]
			if !ok || !isRequirementComplete(st.Status) {
				return false
			}
		} else {
			// Group reference — all sub-items must be complete.
			prefix := fmt.Sprintf("R%s.", ref.Group)
			found := false
			for k, st := range prdReqs {
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

// prdRef holds a parsed PRD requirement reference.
type prdRef struct {
	PRDStem string // e.g. "prd001" or "prd001-orchestrator-core"
	Group   string // e.g. "1" from R1
	SubItem string // e.g. "2" from R1.2; empty for group refs
}

// extractPRDRefsFromDescription parses the YAML description's requirements
// section and returns all PRD refs found in requirement text fields.
func extractPRDRefsFromDescription(description string) []prdRef {
	var desc IssueDescription
	if err := yaml.Unmarshal([]byte(description), &desc); err != nil {
		return nil
	}
	var refs []prdRef
	for _, req := range desc.Requirements {
		matches := PRDRefPattern.FindAllStringSubmatch(req.Text, -1)
		for _, m := range matches {
			refs = append(refs, prdRef{
				PRDStem: m[1],
				Group:   m[2],
				SubItem: m[3],
			})
		}
	}
	return refs
}

// findPRDRequirements looks up the requirement map for a PRD stem, trying
// exact match first, then dash-delimited prefix match (e.g. "prd001" matches
// "prd001-core" but not "prd0011-other"). When multiple candidates match,
// the longest (most specific) key wins.
func findPRDRequirements(reqs map[string]map[string]RequirementState, stem string) map[string]RequirementState {
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

	// Extract PRD citations from touchpoints (e.g. "prd001-core R1, R2").
	citations := extractTouchpointCitations(touchpoints)
	if len(citations) == 0 {
		return false, nil
	}

	var remaining []string
	for _, cite := range citations {
		prdReqs := findPRDRequirements(rf.Requirements, cite.prdID)
		if prdReqs == nil {
			// PRD not in requirements file — cannot verify.
			remaining = append(remaining, fmt.Sprintf("%s (missing)", cite.prdID))
			continue
		}
		for _, group := range cite.groups {
			prefix := group + "."
			for key, st := range prdReqs {
				if strings.HasPrefix(key, prefix) && !isRequirementComplete(st.Status) {
					remaining = append(remaining, fmt.Sprintf("%s %s", cite.prdID, key))
				}
			}
		}
	}

	sort.Strings(remaining)
	return len(remaining) == 0, remaining
}

// touchpointCitation holds a PRD reference and its cited R-groups from a
// use case touchpoint.
type touchpointCitation struct {
	prdID  string   // e.g. "prd001-orchestrator-core"
	groups []string // e.g. ["R1", "R2"]
}

// touchpointPRDRefRe matches PRD + R-group references in touchpoint text.
var touchpointPRDRefRe = regexp.MustCompile(`(prd\d+[-\w]*)\s+(R\d+(?:\s*,\s*R\d+)*)`)

// extractTouchpointCitations parses touchpoint strings to extract PRD
// citations with their requirement groups.
func extractTouchpointCitations(touchpoints []string) []touchpointCitation {
	var citations []touchpointCitation
	seen := make(map[string]map[string]bool) // prdID → set of groups
	for _, tp := range touchpoints {
		matches := touchpointPRDRefRe.FindAllStringSubmatch(tp, -1)
		for _, m := range matches {
			prdID := m[1]
			groupStr := m[2]
			if seen[prdID] == nil {
				seen[prdID] = make(map[string]bool)
			}
			for _, g := range strings.Split(groupStr, ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					seen[prdID][g] = true
				}
			}
		}
	}
	for prdID, groups := range seen {
		var gs []string
		for g := range groups {
			gs = append(gs, g)
		}
		sort.Strings(gs)
		citations = append(citations, touchpointCitation{prdID: prdID, groups: gs})
	}
	sort.Slice(citations, func(i, j int) bool { return citations[i].prdID < citations[j].prdID })
	return citations
}

// extractRItems reads a PRD YAML file and returns all R-item IDs (e.g.
// R1.1, R1.2, R2.1) in sorted order.
func extractRItems(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var prd PRDDoc
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil
	}

	var items []string
	for _, group := range prd.Requirements {
		for _, item := range group.Items {
			// Each item is a map with a single key like "R1.1".
			// The generate package's PRDDoc uses Items []any,
			// so we need a type assertion.
			switch v := item.(type) {
			case map[string]interface{}:
				for k := range v {
					items = append(items, k)
				}
			}
		}
	}
	sort.Strings(items)
	return items
}
