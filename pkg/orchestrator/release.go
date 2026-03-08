// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	rel "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/release"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Dependency injection: wire the parent package's logf and git helper
// functions into the internal/release package at init time.
// ---------------------------------------------------------------------------

func init() {
	rel.Log = logf
	rel.GitCurrentBranchFn = gitCurrentBranch
	rel.GitListTagsFn = gitListTags
	rel.GitTagFn = gitTag
	rel.GitStageAllFn = gitStageAll
	rel.GitCommitFn = gitCommit
}

// ReleaseUpdate marks a release as complete in the project files. It sets all
// use-case statuses to "implemented" for the named release in
// docs/road-map.yaml and removes the release version from project.releases in
// configuration.yaml (DefaultConfigFile). Both files are rewritten using
// yaml.v3 node round-trip to preserve document structure and comments.
//
// Returns an error if the release version is not found in road-map.yaml, or
// if either file fails schema validation.
func (o *Orchestrator) ReleaseUpdate(version string) error {
	return rel.ReleaseUpdate(DefaultConfigFile, version)
}

// ReleaseClear reverses ReleaseUpdate. It resets all use-case statuses to
// "spec_complete" for the named release in docs/road-map.yaml and
// re-appends the release version to project.releases in configuration.yaml.
//
// Returns an error if the release version is not found in road-map.yaml, or
// if either file fails schema validation.
func (o *Orchestrator) ReleaseClear(version string) error {
	return rel.ReleaseClear(DefaultConfigFile, version)
}

// updateRoadmapUCStatuses delegates to the internal/release package.
func updateRoadmapUCStatuses(version, newStatus string) error {
	return rel.UpdateRoadmapUCStatuses(version, newStatus)
}

// setRoadmapUCStatuses delegates to the internal/release package.
func setRoadmapUCStatuses(root *yaml.Node, version, newStatus string) error {
	return rel.SetRoadmapUCStatuses(root, version, newStatus)
}

// removeReleaseFromConfig delegates to the internal/release package.
func removeReleaseFromConfig(configPath, version string) error {
	return rel.RemoveReleaseFromConfig(configPath, version)
}

// addReleaseToConfig delegates to the internal/release package.
func addReleaseToConfig(configPath, version string) error {
	return rel.AddReleaseToConfig(configPath, version)
}

// mutateConfigReleases delegates to the internal/release package.
func mutateConfigReleases(configPath, version string, add bool) error {
	return rel.MutateConfigReleases(configPath, version, add)
}

// mutateProjectReleasesNode delegates to the internal/release package.
func mutateProjectReleasesNode(root *yaml.Node, version string, add bool) error {
	return rel.MutateProjectReleasesNode(root, version, add)
}

// mappingValue delegates to the internal/release package.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	return rel.MappingValue(node, key)
}

// releaseVersionsFromConfig delegates to the internal/release package.
func releaseVersionsFromConfig(configPath string) ([]string, error) {
	return rel.ReleaseVersionsFromConfig(configPath)
}

// roadmapUCStatuses delegates to the internal/release package.
func roadmapUCStatuses(roadmapPath, version string) (map[string]string, error) {
	return rel.RoadmapUCStatuses(roadmapPath, version)
}

// roadmapReleaseStatus delegates to the internal/release package.
func roadmapReleaseStatus(roadmapPath, version string) (string, error) {
	return rel.RoadmapReleaseStatus(roadmapPath, version)
}
