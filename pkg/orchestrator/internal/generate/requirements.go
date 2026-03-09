// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"fmt"
	"os"
	"path/filepath"
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

// GenerateRequirementsFile scans all PRD YAML files in the given directory
// for R-items and writes a requirements state file where every item starts
// with status "ready". Returns the path written, or an error.
func GenerateRequirementsFile(prdDir, cobblerDir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(prdDir, "prd*.yaml"))
	if err != nil {
		return "", fmt.Errorf("globbing PRDs in %s: %w", prdDir, err)
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
	for _, g := range allReqs {
		total += len(g)
	}
	Log("generateRequirementsFile: wrote %d R-items from %d PRDs to %s", total, len(allReqs), outPath)
	return outPath, nil
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
