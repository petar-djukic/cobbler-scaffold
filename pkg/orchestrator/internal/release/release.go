// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package release implements release lifecycle operations: updating roadmap
// use-case statuses, mutating project release lists in configuration files,
// and version tagging. The parent orchestrator package provides thin
// receiver-method wrappers around these functions.
package release

import (
	"fmt"
	"os"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Injected dependencies
// ---------------------------------------------------------------------------

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// Package-level variables set by the parent package at init time.
var (
	Log Logger = func(string, ...any) {}

	// GitReader provides read-only repository access (CurrentBranch).
	GitReader gitops.RepoReader
	// GitTags provides tag operations (Tag, ListTags).
	GitTags gitops.TagManager
	// GitCommitter provides staging and commit operations (StageAll, Commit).
	GitCommitter gitops.CommitWriter
)

// ---------------------------------------------------------------------------
// Minimal local types for YAML validation (avoid importing parent types)
// ---------------------------------------------------------------------------

type roadmapDoc struct {
	Releases []roadmapRelease `yaml:"releases"`
}

type roadmapRelease struct {
	Version  string           `yaml:"version"`
	Status   string           `yaml:"status"`
	UseCases []roadmapUseCase `yaml:"use_cases"`
}

type roadmapUseCase struct {
	ID     string `yaml:"id"`
	Status string `yaml:"status"`
}

type configProjectReleases struct {
	Project struct {
		Releases []string `yaml:"releases"`
	} `yaml:"project"`
}

// ---------------------------------------------------------------------------
// ReleaseUpdate / ReleaseClear
// ---------------------------------------------------------------------------

// ReleaseUpdate marks a release as complete. It sets all use-case statuses
// to "implemented" in docs/road-map.yaml and removes the release version
// from project.releases in the config file.
func ReleaseUpdate(configFile, version string) error {
	if err := UpdateRoadmapUCStatuses(version, "implemented"); err != nil {
		return err
	}
	if err := RemoveReleaseFromConfig(configFile, version); err != nil {
		return err
	}
	Log("release:update %s: done", version)
	return nil
}

// ReleaseClear reverses ReleaseUpdate. It resets all use-case statuses to
// "spec_complete" and re-appends the release version to project.releases.
func ReleaseClear(configFile, version string) error {
	if err := UpdateRoadmapUCStatuses(version, "spec_complete"); err != nil {
		return err
	}
	if err := AddReleaseToConfig(configFile, version); err != nil {
		return err
	}
	Log("release:clear %s: done", version)
	return nil
}

// ---------------------------------------------------------------------------
// Roadmap use-case status manipulation
// ---------------------------------------------------------------------------

// UpdateRoadmapUCStatuses loads docs/road-map.yaml via yaml.v3 node API,
// finds the release matching version, sets all its use_cases[*].status
// values to newStatus, and writes the file back.
func UpdateRoadmapUCStatuses(version, newStatus string) error {
	const path = "docs/road-map.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("release update: read %s: %w", path, err)
	}

	// Validate structure via typed unmarshal before mutation.
	var doc roadmapDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("release update: parse %s: %w", path, err)
	}
	found := false
	for _, rel := range doc.Releases {
		if rel.Version == version {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("release update: version %q not found in %s", version, path)
	}

	// Node round-trip to preserve comments and structure.
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("release update: node parse %s: %w", path, err)
	}

	if err := SetRoadmapUCStatuses(&root, version, newStatus); err != nil {
		return fmt.Errorf("release update: mutate %s: %w", path, err)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("release update: marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("release update: write %s: %w", path, err)
	}
	Log("release update: set use-case statuses to %q for release %s in %s", newStatus, version, path)
	return nil
}

// SetRoadmapUCStatuses mutates the yaml.Node tree of road-map.yaml, finding
// the release with the given version and setting both the release-level status
// and all its use_cases[*].status scalar values to newStatus.
func SetRoadmapUCStatuses(root *yaml.Node, version, newStatus string) error {
	// Unwrap document node.
	doc := root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		doc = doc.Content[0]
	}
	// doc should be a mapping node for the roadmap document.
	releases := MappingValue(doc, "releases")
	if releases == nil || releases.Kind != yaml.SequenceNode {
		return fmt.Errorf("releases key not found or not a sequence")
	}
	for _, relNode := range releases.Content {
		versionNode := MappingValue(relNode, "version")
		if versionNode == nil || versionNode.Value != version {
			continue
		}
		// Reset the release-level status itself (GH-1189).
		if relStatus := MappingValue(relNode, "status"); relStatus != nil {
			relStatus.Value = newStatus
		}
		ucSeq := MappingValue(relNode, "use_cases")
		if ucSeq == nil || ucSeq.Kind != yaml.SequenceNode {
			continue
		}
		for _, ucNode := range ucSeq.Content {
			statusNode := MappingValue(ucNode, "status")
			if statusNode != nil {
				statusNode.Value = newStatus
			}
		}
		return nil
	}
	return fmt.Errorf("version %q not found in releases node tree", version)
}

// UpdateRoadmapSingleUCStatus loads docs/road-map.yaml, finds the release
// matching version, sets the status of the single use case identified by ucID
// to newStatus, and writes the file back. Does not modify the release-level
// status or other use cases.
func UpdateRoadmapSingleUCStatus(version, ucID, newStatus string) error {
	const path = "docs/road-map.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("release update single UC: read %s: %w", path, err)
	}

	var doc roadmapDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("release update single UC: parse %s: %w", path, err)
	}
	found := false
	for _, rel := range doc.Releases {
		if rel.Version == version {
			for _, uc := range rel.UseCases {
				if uc.ID == ucID {
					found = true
					break
				}
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("release update single UC: version %q UC %q not found in %s", version, ucID, path)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("release update single UC: node parse %s: %w", path, err)
	}

	if err := SetRoadmapSingleUCStatus(&root, version, ucID, newStatus); err != nil {
		return fmt.Errorf("release update single UC: mutate %s: %w", path, err)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("release update single UC: marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("release update single UC: write %s: %w", path, err)
	}
	Log("release update: set UC %s status to %q for release %s in %s", ucID, newStatus, version, path)
	return nil
}

// SetRoadmapSingleUCStatus mutates the yaml.Node tree of road-map.yaml,
// finding the release with the given version and setting the status of the
// use case matching ucID to newStatus. Does not modify the release-level
// status or other use cases.
func SetRoadmapSingleUCStatus(root *yaml.Node, version, ucID, newStatus string) error {
	doc := root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		doc = doc.Content[0]
	}
	releases := MappingValue(doc, "releases")
	if releases == nil || releases.Kind != yaml.SequenceNode {
		return fmt.Errorf("releases key not found or not a sequence")
	}
	for _, relNode := range releases.Content {
		versionNode := MappingValue(relNode, "version")
		if versionNode == nil || versionNode.Value != version {
			continue
		}
		ucSeq := MappingValue(relNode, "use_cases")
		if ucSeq == nil || ucSeq.Kind != yaml.SequenceNode {
			return fmt.Errorf("use_cases not found for release %s", version)
		}
		for _, ucNode := range ucSeq.Content {
			idNode := MappingValue(ucNode, "id")
			if idNode != nil && idNode.Value == ucID {
				statusNode := MappingValue(ucNode, "status")
				if statusNode != nil {
					statusNode.Value = newStatus
				}
				return nil
			}
		}
		return fmt.Errorf("UC %q not found in release %s", ucID, version)
	}
	return fmt.Errorf("version %q not found in releases node tree", version)
}

// ---------------------------------------------------------------------------
// Config release list manipulation
// ---------------------------------------------------------------------------

// RemoveReleaseFromConfig loads configPath, removes version from
// project.releases, and writes it back via node round-trip.
func RemoveReleaseFromConfig(configPath, version string) error {
	return MutateConfigReleases(configPath, version, false)
}

// AddReleaseToConfig loads configPath, appends version to project.releases
// if not already present, and writes it back via node round-trip.
func AddReleaseToConfig(configPath, version string) error {
	return MutateConfigReleases(configPath, version, true)
}

// MutateConfigReleases is the shared implementation for RemoveReleaseFromConfig
// and AddReleaseToConfig. When add is true, version is appended (if absent);
// when add is false, version is removed.
func MutateConfigReleases(configPath, version string, add bool) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("release config: read %s: %w", configPath, err)
	}

	// Validate via typed unmarshal.
	var cfg any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("release config: parse %s: %w", configPath, err)
	}

	// Node round-trip.
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("release config: node parse %s: %w", configPath, err)
	}

	if err := MutateProjectReleasesNode(&root, version, add); err != nil {
		// project.releases is optional; log and skip rather than hard-fail.
		Log("release config: project.releases not found in %s, skipping: %v", configPath, err)
		return nil
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("release config: marshal %s: %w", configPath, err)
	}
	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return fmt.Errorf("release config: write %s: %w", configPath, err)
	}
	action := "removed"
	if add {
		action = "added"
	}
	Log("release config: %s %q in project.releases in %s", action, version, configPath)
	return nil
}

// MutateProjectReleasesNode finds project.releases in the node tree and either
// removes or appends version. Returns an error if project.releases is absent.
func MutateProjectReleasesNode(root *yaml.Node, version string, add bool) error {
	doc := root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		doc = doc.Content[0]
	}
	projectNode := MappingValue(doc, "project")
	if projectNode == nil {
		return fmt.Errorf("project key not found")
	}
	releasesNode := MappingValue(projectNode, "releases")
	if releasesNode == nil {
		return fmt.Errorf("project.releases key not found")
	}
	if releasesNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("project.releases is not a sequence")
	}

	if add {
		// Append only if not already present.
		for _, child := range releasesNode.Content {
			if child.Value == version {
				return nil // already present
			}
		}
		releasesNode.Content = append(releasesNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: version,
		})
	} else {
		// Remove all occurrences of version.
		filtered := releasesNode.Content[:0]
		for _, child := range releasesNode.Content {
			if child.Value != version {
				filtered = append(filtered, child)
			}
		}
		releasesNode.Content = filtered
	}
	return nil
}

// MappingValue returns the value node for the given key in a yaml mapping
// node, or nil if not found.
func MappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// ReleaseVersionsFromConfig returns the list of releases in project.releases
// from the given config file.
func ReleaseVersionsFromConfig(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg configProjectReleases
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.Project.Releases, nil
}

// RoadmapUCStatuses returns a map of UC ID to status for the given release
// version.
func RoadmapUCStatuses(roadmapPath, version string) (map[string]string, error) {
	data, err := os.ReadFile(roadmapPath)
	if err != nil {
		return nil, err
	}
	var doc roadmapDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	for _, rel := range doc.Releases {
		if rel.Version == version {
			out := make(map[string]string, len(rel.UseCases))
			for _, uc := range rel.UseCases {
				out[uc.ID] = uc.Status
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("version %q not found", version)
}

// RoadmapReleaseStatus returns the release-level status for the given version.
func RoadmapReleaseStatus(roadmapPath, version string) (string, error) {
	data, err := os.ReadFile(roadmapPath)
	if err != nil {
		return "", err
	}
	var doc roadmapDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	for _, rel := range doc.Releases {
		if rel.Version == version {
			return rel.Status, nil
		}
	}
	return "", fmt.Errorf("version %q not found", version)
}
