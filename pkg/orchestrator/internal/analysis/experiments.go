// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

// Experiments constitution checks. These enforce the mechanical articles of
// docs/constitutions/experiments.yaml: E2 gate-before-numbers, E5
// negative-results-recorded, and E6 manifest-as-truth. Every check is a no-op
// unless the experiments constitution is scaffolded into the project and its
// evidence manifest exists, mirroring how DetectConstitutionDrift gates on the
// constitution files being present.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const experimentsConstitutionPath = "docs/constitutions/experiments.yaml"

// experimentsRegistry mirrors the project_registry block of experiments.yaml.
type experimentsRegistry struct {
	ManifestPath string   `yaml:"manifest_path"`
	GateFields   []string `yaml:"gate_fields"`
	MemoDir      string   `yaml:"memo_dir"`
}

type experimentsConstitution struct {
	ProjectRegistry experimentsRegistry `yaml:"project_registry"`
}

// experimentsManifest is the evidence manifest. Each experiment is a free-form
// map so the configured gate_fields drive the E2 check rather than a fixed
// struct. Recognized keys are id, status, memo, and the gate fields.
type experimentsManifest struct {
	Experiments []map[string]any `yaml:"experiments"`
}

// ExperimentChecks holds the results of the experiments constitution checks.
type ExperimentChecks struct {
	GateViolations []string // E2: experiment missing a declared gate field (error)
	MissingMemos   []string // E5: failed experiment with no decision memo (warning)
	ManifestErrors []string // E6: manifest integrity and unresolved id references (error)
}

// experimentRefGlobs are the document sets scanned for experiment id references
// (E6). A reference is the token exp:<id>. The manifest itself is excluded.
var experimentRefGlobs = []string{"docs/**/*.md", "docs/**/*.yaml"}

// experimentRefRe matches an experiment id reference: exp:<id>.
var experimentRefRe = regexp.MustCompile(`\bexp:([A-Za-z0-9_.\-]+)`)

// RunExperimentChecks loads the experiments constitution and its manifest, then
// runs the mechanical checks. It returns an empty result when the constitution
// or manifest is absent, so projects without experiments pay nothing.
func RunExperimentChecks(log Logger) ExperimentChecks {
	reg := loadExperimentsRegistry(log)
	if reg == nil {
		return ExperimentChecks{}
	}
	manifest := loadExperimentsManifest(reg.ManifestPath, log)
	if manifest == nil {
		return ExperimentChecks{}
	}
	return ExperimentChecks{
		GateViolations: reg.checkGates(manifest),
		MissingMemos:   reg.checkMemos(manifest),
		ManifestErrors: reg.checkManifestTruth(manifest),
	}
}

// loadExperimentsRegistry reads the experiments constitution. It returns nil
// when the file is absent, gating every check off.
func loadExperimentsRegistry(log Logger) *experimentsRegistry {
	data, err := os.ReadFile(experimentsConstitutionPath)
	if err != nil {
		return nil // constitution not scaffolded — checks are no-ops
	}
	var c experimentsConstitution
	if err := yaml.Unmarshal(data, &c); err != nil {
		log("experiments: cannot parse %s: %v", experimentsConstitutionPath, err)
		return nil
	}
	return &c.ProjectRegistry
}

// loadExperimentsManifest reads the evidence manifest. It returns nil when the
// manifest is absent so a constitution without a manifest yet is a no-op.
func loadExperimentsManifest(path string, log Logger) *experimentsManifest {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m experimentsManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		log("experiments: cannot parse manifest %s: %v", path, err)
		return nil
	}
	return &m
}

// checkGates implements E2. Every experiment declares each configured gate field
// with a non-empty value before any run.
func (r *experimentsRegistry) checkGates(m *experimentsManifest) []string {
	if len(r.GateFields) == 0 {
		return nil
	}
	var out []string
	for _, exp := range m.Experiments {
		id := experimentID(exp)
		for _, field := range r.GateFields {
			if isEmptyValue(exp[field]) {
				out = append(out, fmt.Sprintf("experiment %s: missing gate field %q before results exist (E2)", id, field))
			}
		}
	}
	sort.Strings(out)
	return out
}

// checkMemos implements E5. A failed experiment must have a decision memo, named
// by its memo field or living in the memo directory under its id.
func (r *experimentsRegistry) checkMemos(m *experimentsManifest) []string {
	var out []string
	for _, exp := range m.Experiments {
		if !strings.EqualFold(fmt.Sprint(exp["status"]), "failed") {
			continue
		}
		if memoResolves(exp, r.MemoDir) {
			continue
		}
		out = append(out, fmt.Sprintf("experiment %s: failed gate has no decision memo in %q (E5)", experimentID(exp), r.MemoDir))
	}
	sort.Strings(out)
	return out
}

// memoResolves reports whether a failed experiment has a decision memo, either
// through its memo field or a file in the memo directory bearing its id.
func memoResolves(exp map[string]any, memoDir string) bool {
	if memo := strings.TrimSpace(fmt.Sprint(exp["memo"])); memo != "" && memo != "<nil>" {
		if _, err := os.Stat(memo); err == nil {
			return true
		}
	}
	id := experimentID(exp)
	if memoDir == "" || id == "" {
		return false
	}
	entries, err := os.ReadDir(memoDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), id) {
			return true
		}
	}
	return false
}

// checkManifestTruth implements E6. The manifest is the source of truth: every
// experiment has a unique non-empty id, and every exp:<id> reference in the
// documents resolves to a manifest entry.
func (r *experimentsRegistry) checkManifestTruth(m *experimentsManifest) []string {
	ids, dupErrs := manifestIDs(m)
	out := dupErrs
	for _, path := range referenceFiles(r.ManifestPath) {
		out = append(out, unresolvedRefs(path, ids)...)
	}
	sort.Strings(out)
	return dedupe(out)
}

// manifestIDs collects the experiment id set and reports missing or duplicate
// ids.
func manifestIDs(m *experimentsManifest) (map[string]bool, []string) {
	ids := make(map[string]bool)
	var errs []string
	for i, exp := range m.Experiments {
		id := experimentID(exp)
		if id == "" {
			errs = append(errs, fmt.Sprintf("manifest experiment[%d]: missing id (E6)", i))
			continue
		}
		if ids[id] {
			errs = append(errs, fmt.Sprintf("manifest experiment %s: duplicate id (E6)", id))
		}
		ids[id] = true
	}
	return ids, errs
}

// referenceFiles lists the documents scanned for experiment id references,
// excluding the manifest itself.
func referenceFiles(manifestPath string) []string {
	var out []string
	for _, p := range gatherProse(experimentRefGlobs) {
		if filepath.Clean(p) != filepath.Clean(manifestPath) {
			out = append(out, p)
		}
	}
	return out
}

// unresolvedRefs reports exp:<id> references in one file absent from the
// manifest id set.
func unresolvedRefs(path string, ids map[string]bool) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, mtch := range experimentRefRe.FindAllStringSubmatch(string(data), -1) {
		if !ids[mtch[1]] {
			out = append(out, fmt.Sprintf("%s: reference exp:%s does not resolve to a manifest entry (E6)", path, mtch[1]))
		}
	}
	return out
}

// experimentID returns the id of an experiment entry as a trimmed string.
func experimentID(exp map[string]any) string {
	id := strings.TrimSpace(fmt.Sprint(exp["id"]))
	if id == "<nil>" {
		return ""
	}
	return id
}

// isEmptyValue reports whether a YAML value counts as absent for the gate check:
// nil, blank string, or an empty list or map.
func isEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}
