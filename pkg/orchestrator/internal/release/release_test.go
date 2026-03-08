// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package release

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeRoadmapFile(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	path := filepath.Join(dir, "docs", "road-map.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write road-map.yaml: %v", err)
	}
	return path
}

func writeConfigFile(t *testing.T, path string, releases []string) {
	t.Helper()
	relItems := ""
	for _, r := range releases {
		relItems += "\n  - " + r
	}
	content := "project:\n  releases:" + relItems + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

const sampleRoadmap = `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: in_progress
    use_cases:
      - id: rel00.0-uc001-format
        status: spec_complete
        summary: Format output
      - id: rel00.0-uc002-build
        status: spec_complete
        summary: Build pipeline
  - version: "01.0"
    name: Release 1
    status: pending
    use_cases:
      - id: rel01.0-uc001-ext
        status: spec_complete
        summary: Extension
`

// stringsContains reports whether slice contains s.
func stringsContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// cdTemp changes to dir and returns a cleanup func that restores the original
// working directory. Tests that call os.Chdir must not use t.Parallel().
func cdTemp(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	return func() { os.Chdir(orig) } //nolint:errcheck
}

// ---------------------------------------------------------------------------
// UpdateRoadmapUCStatuses
// ---------------------------------------------------------------------------

func TestUpdateRoadmapUCStatuses_SetImplemented(t *testing.T) {
	dir := t.TempDir()
	path := writeRoadmapFile(t, dir, sampleRoadmap)
	defer cdTemp(t, dir)()

	if err := UpdateRoadmapUCStatuses("00.0", "implemented"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses, err := RoadmapUCStatuses(path, "00.0")
	if err != nil {
		t.Fatalf("read statuses: %v", err)
	}
	for id, status := range statuses {
		if status != "implemented" {
			t.Errorf("UC %s: want implemented, got %q", id, status)
		}
	}

	// Release 01.0 must be untouched.
	other, err := RoadmapUCStatuses(path, "01.0")
	if err != nil {
		t.Fatalf("read other release: %v", err)
	}
	for id, status := range other {
		if status != "spec_complete" {
			t.Errorf("UC %s in 01.0 should be untouched, got %q", id, status)
		}
	}
}

func TestUpdateRoadmapUCStatuses_SetSpecComplete(t *testing.T) {
	dir := t.TempDir()
	path := writeRoadmapFile(t, dir, sampleRoadmap)
	defer cdTemp(t, dir)()

	if err := UpdateRoadmapUCStatuses("00.0", "implemented"); err != nil {
		t.Fatalf("set implemented: %v", err)
	}
	if err := UpdateRoadmapUCStatuses("00.0", "spec_complete"); err != nil {
		t.Fatalf("set spec_complete: %v", err)
	}

	statuses, err := RoadmapUCStatuses(path, "00.0")
	if err != nil {
		t.Fatalf("read statuses: %v", err)
	}
	for id, status := range statuses {
		if status != "spec_complete" {
			t.Errorf("UC %s: want spec_complete, got %q", id, status)
		}
	}
}

func TestUpdateRoadmapUCStatuses_VersionNotFound(t *testing.T) {
	dir := t.TempDir()
	writeRoadmapFile(t, dir, sampleRoadmap)
	defer cdTemp(t, dir)()

	if err := UpdateRoadmapUCStatuses("99.9", "implemented"); err == nil {
		t.Error("expected error for missing version, got nil")
	}
}

func TestUpdateRoadmapUCStatuses_MissingFile(t *testing.T) {
	dir := t.TempDir()
	defer cdTemp(t, dir)()

	if err := UpdateRoadmapUCStatuses("00.0", "implemented"); err == nil {
		t.Error("expected error for missing road-map.yaml, got nil")
	}
}

// ---------------------------------------------------------------------------
// RemoveReleaseFromConfig / AddReleaseToConfig
// ---------------------------------------------------------------------------

func TestRemoveReleaseFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	writeConfigFile(t, cfgPath, []string{"00.0", "01.0", "02.0"})

	if err := RemoveReleaseFromConfig(cfgPath, "01.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	versions, err := ReleaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if stringsContains(versions, "01.0") {
		t.Errorf("01.0 should have been removed, versions: %v", versions)
	}
	if !stringsContains(versions, "00.0") || !stringsContains(versions, "02.0") {
		t.Errorf("other versions should remain, got: %v", versions)
	}
}

func TestAddReleaseToConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	writeConfigFile(t, cfgPath, []string{"00.0", "02.0"})

	if err := AddReleaseToConfig(cfgPath, "01.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	versions, err := ReleaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !stringsContains(versions, "01.0") {
		t.Errorf("01.0 should have been added, versions: %v", versions)
	}
}

func TestAddReleaseToConfig_Idempotent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	writeConfigFile(t, cfgPath, []string{"00.0", "01.0"})

	if err := AddReleaseToConfig(cfgPath, "01.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	versions, err := ReleaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	count := 0
	for _, v := range versions {
		if v == "01.0" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of 01.0, got %d in %v", count, versions)
	}
}

func TestMutateConfigReleases_MissingFile(t *testing.T) {
	t.Parallel()
	err := MutateConfigReleases(filepath.Join(t.TempDir(), "nonexistent.yaml"), "01.0", true)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestMutateConfigReleases_InvalidYAML(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(path, []byte("{{{not yaml"), 0o644)
	err := MutateConfigReleases(path, "01.0", true)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestMutateConfigReleases_NoProjectReleases(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("project:\n  module_path: foo\n"), 0o644)
	// Should log and skip, not error.
	err := MutateConfigReleases(path, "01.0", true)
	if err != nil {
		t.Errorf("expected nil (skip) when project.releases absent, got: %v", err)
	}
}

func TestMutateProjectReleasesNode_NoProjectKey(t *testing.T) {
	t.Parallel()
	var root yaml.Node
	yaml.Unmarshal([]byte("other: value\n"), &root)
	err := MutateProjectReleasesNode(&root, "01.0", true)
	if err == nil {
		t.Error("expected error for missing project key")
	}
}

func TestMutateProjectReleasesNode_ReleasesNotSequence(t *testing.T) {
	t.Parallel()
	var root yaml.Node
	yaml.Unmarshal([]byte("project:\n  releases: not-a-list\n"), &root)
	err := MutateProjectReleasesNode(&root, "01.0", true)
	if err == nil {
		t.Error("expected error when project.releases is not a sequence")
	}
}

func TestReleaseVersionsFromConfig_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := ReleaseVersionsFromConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReleaseVersionsFromConfig_InvalidYAML(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(path, []byte("{{{"), 0o644)
	_, err := ReleaseVersionsFromConfig(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestRoadmapUCStatuses_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := RoadmapUCStatuses(filepath.Join(t.TempDir(), "missing.yaml"), "01.0")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRoadmapUCStatuses_VersionNotFound(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "roadmap.yaml")
	os.WriteFile(path, []byte(sampleRoadmap), 0o644)
	_, err := RoadmapUCStatuses(path, "99.9")
	if err == nil {
		t.Error("expected error for unknown version")
	}
}

func TestRemoveReleaseFromConfig_NotPresent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	writeConfigFile(t, cfgPath, []string{"00.0", "01.0"})

	if err := RemoveReleaseFromConfig(cfgPath, "99.9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	versions, err := ReleaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2 versions unchanged, got %v", versions)
	}
}

// ---------------------------------------------------------------------------
// ReleaseUpdate / ReleaseClear round-trip
// ---------------------------------------------------------------------------

func TestReleaseUpdate_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	rmPath := writeRoadmapFile(t, dir, sampleRoadmap)
	cfgPath := filepath.Join(dir, "configuration.yaml")
	writeConfigFile(t, cfgPath, []string{"00.0", "01.0"})
	defer cdTemp(t, dir)()

	if err := ReleaseUpdate(cfgPath, "00.0"); err != nil {
		t.Fatalf("ReleaseUpdate: %v", err)
	}

	statuses, err := RoadmapUCStatuses(rmPath, "00.0")
	if err != nil {
		t.Fatalf("read statuses: %v", err)
	}
	for id, s := range statuses {
		if s != "implemented" {
			t.Errorf("UC %s: want implemented, got %q", id, s)
		}
	}

	versions, err := ReleaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if stringsContains(versions, "00.0") {
		t.Errorf("00.0 should have been removed from releases, got %v", versions)
	}

	// Clear restores spec_complete and re-adds the version.
	if err := ReleaseClear(cfgPath, "00.0"); err != nil {
		t.Fatalf("ReleaseClear: %v", err)
	}

	statuses, err = RoadmapUCStatuses(rmPath, "00.0")
	if err != nil {
		t.Fatalf("read statuses after clear: %v", err)
	}
	for id, s := range statuses {
		if s != "spec_complete" {
			t.Errorf("UC %s after clear: want spec_complete, got %q", id, s)
		}
	}

	versions, err = ReleaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config after clear: %v", err)
	}
	if !stringsContains(versions, "00.0") {
		t.Errorf("00.0 should have been re-added to releases, got %v", versions)
	}
}

func TestReleaseUpdate_VersionNotFound(t *testing.T) {
	dir := t.TempDir()
	writeRoadmapFile(t, dir, sampleRoadmap)
	cfgPath := filepath.Join(dir, "configuration.yaml")
	writeConfigFile(t, cfgPath, []string{"00.0"})
	defer cdTemp(t, dir)()

	if err := ReleaseUpdate(cfgPath, "99.9"); err == nil {
		t.Error("expected error for missing version, got nil")
	}
}

// ---------------------------------------------------------------------------
// MappingValue
// ---------------------------------------------------------------------------

func TestMappingValue(t *testing.T) {
	t.Parallel()

	raw := `key: value
nested:
  inner: 42
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	doc := root.Content[0] // unwrap document node

	v := MappingValue(doc, "key")
	if v == nil || v.Value != "value" {
		t.Errorf("key: got %v, want scalar 'value'", v)
	}

	n := MappingValue(doc, "nested")
	if n == nil {
		t.Fatal("nested: got nil")
	}
	inner := MappingValue(n, "inner")
	if inner == nil || inner.Value != "42" {
		t.Errorf("inner: got %v, want scalar '42'", inner)
	}

	if MappingValue(doc, "missing") != nil {
		t.Error("missing key: expected nil")
	}
	if MappingValue(nil, "key") != nil {
		t.Error("nil node: expected nil")
	}
}
