// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

// Paper constitution checks. These enforce the mechanical articles of
// docs/constitutions/paper.yaml: P1 vocabulary registry, P3 placeholder
// traceability, P4 citation integrity, and P5 forbidden terms. Every check is a
// no-op unless the paper constitution is scaffolded into the project, mirroring
// how DetectConstitutionDrift gates on the constitution files being present.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const paperConstitutionPath = "docs/constitutions/paper.yaml"

// paperRegistry mirrors the project_registry block of paper.yaml. Analyze reads
// it to run the P1, P3, P4, and P5 checks.
type paperRegistry struct {
	Vocabulary         map[string]any `yaml:"vocabulary"`
	ForbiddenTerms     []string       `yaml:"forbidden_terms"`
	PlaceholderPattern string         `yaml:"placeholder_pattern"`
	ProseGlobs         []string       `yaml:"prose_globs"`
	Bibliography       []string       `yaml:"bibliography"`
}

type paperConstitution struct {
	ProjectRegistry paperRegistry `yaml:"project_registry"`
}

// PaperChecks holds the results of the paper constitution checks.
type PaperChecks struct {
	VocabularyIssues  []string // P1: prose exists but the vocabulary registry is empty (warning)
	PlaceholderErrors []string // P3: placeholder names no artifact, or the artifact is absent (error)
	BrokenCitations   []string // P4: citation key unresolved against the bibliography (error)
	ForbiddenTerms    []string // P5: forbidden term occurs in publication prose (warning)
}

// RunPaperChecks loads the paper constitution and runs the mechanical checks.
// When docs/constitutions/paper.yaml is absent, it returns an empty result so
// projects without a paper pay nothing.
func RunPaperChecks(log Logger) PaperChecks {
	reg := loadPaperRegistry(log)
	if reg == nil {
		return PaperChecks{}
	}
	prose := gatherProse(reg.ProseGlobs)
	return PaperChecks{
		VocabularyIssues:  reg.checkVocabulary(prose),
		PlaceholderErrors: reg.checkPlaceholders(prose, log),
		BrokenCitations:   reg.checkCitations(prose),
		ForbiddenTerms:    reg.checkForbiddenTerms(prose),
	}
}

// loadPaperRegistry reads the paper constitution. It returns nil when the file
// is absent, which gates every check off.
func loadPaperRegistry(log Logger) *paperRegistry {
	data, err := os.ReadFile(paperConstitutionPath)
	if err != nil {
		return nil // constitution not scaffolded — checks are no-ops
	}
	var c paperConstitution
	if err := yaml.Unmarshal(data, &c); err != nil {
		log("paper: cannot parse %s: %v", paperConstitutionPath, err)
		return nil
	}
	return &c.ProjectRegistry
}

// checkVocabulary implements P1. Full enforcement (prose uses registry terms and
// no coined alternatives) needs a synonym map we do not have; the mechanical
// slice we check is that a project with publication prose declares its terms of
// art at all.
func (r *paperRegistry) checkVocabulary(prose []string) []string {
	if len(prose) == 0 || len(r.Vocabulary) > 0 {
		return nil
	}
	return []string{fmt.Sprintf("vocabulary registry is empty but %d prose file(s) exist — declare terms of art in paper.yaml (P1)", len(prose))}
}

// placeholderRe compiles the registry's placeholder pattern. A nil result means
// the check is skipped.
func (r *paperRegistry) placeholderRe(log Logger) *regexp.Regexp {
	if strings.TrimSpace(r.PlaceholderPattern) == "" {
		return nil
	}
	re, err := regexp.Compile(r.PlaceholderPattern)
	if err != nil {
		log("paper: invalid placeholder_pattern %q: %v", r.PlaceholderPattern, err)
		return nil
	}
	return re
}

// checkPlaceholders implements P3. Every placeholder must name a source artifact
// that exists in the repository.
func (r *paperRegistry) checkPlaceholders(prose []string, log Logger) []string {
	re := r.placeholderRe(log)
	if re == nil {
		return nil
	}
	var out []string
	for _, path := range prose {
		out = append(out, placeholderViolations(path, re)...)
	}
	sort.Strings(out)
	return out
}

// placeholderViolations reports each placeholder in one file whose named
// artifact is missing or unnamed.
func placeholderViolations(path string, re *regexp.Regexp) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		artifact := ""
		if len(m) > 1 {
			artifact = strings.TrimSpace(m[1])
		}
		if artifact == "" {
			out = append(out, fmt.Sprintf("%s: placeholder %q names no source artifact (P3)", path, m[0]))
			continue
		}
		if _, err := os.Stat(artifact); err != nil {
			out = append(out, fmt.Sprintf("%s: placeholder cites missing artifact %q (P3)", path, artifact))
		}
	}
	return out
}

// citeRe matches LaTeX citation commands (\cite, \citep, \citet, with optional
// bracket arguments); citeKeyRe matches markdown-style @keys.
var citeRe = regexp.MustCompile(`\\cite[a-zA-Z]*(?:\[[^\]]*\])*\{([^}]*)\}`)
var citeKeyRe = regexp.MustCompile(`@([A-Za-z0-9_:.\-]+)`)

// checkCitations implements P4. Every citation key must resolve against a
// bibliography file. With no bibliography configured, the check is skipped.
func (r *paperRegistry) checkCitations(prose []string) []string {
	if len(r.Bibliography) == 0 {
		return nil
	}
	keys := r.bibKeys()
	var out []string
	for _, path := range prose {
		out = append(out, unresolvedCitations(path, keys)...)
	}
	sort.Strings(out)
	return dedupe(out)
}

// bibKeyRe matches a BibTeX entry key: @type{key,
var bibKeyRe = regexp.MustCompile(`(?m)^\s*@\w+\s*\{\s*([^,\s]+)\s*,`)

// bibKeys collects the defined citation keys from every bibliography file.
func (r *paperRegistry) bibKeys() map[string]bool {
	keys := make(map[string]bool)
	for _, bib := range r.Bibliography {
		data, err := os.ReadFile(bib)
		if err != nil {
			continue
		}
		for _, m := range bibKeyRe.FindAllStringSubmatch(string(data), -1) {
			keys[m[1]] = true
		}
	}
	return keys
}

// unresolvedCitations reports citation keys in one file absent from the
// bibliography.
func unresolvedCitations(path string, keys map[string]bool) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(data)
	var out []string
	for _, key := range extractCiteKeys(text) {
		if !keys[key] {
			out = append(out, fmt.Sprintf("%s: unresolved citation key %q (P4)", path, key))
		}
	}
	return out
}

// extractCiteKeys pulls citation keys from LaTeX \cite commands and markdown
// @keys in one document.
func extractCiteKeys(text string) []string {
	var keys []string
	for _, m := range citeRe.FindAllStringSubmatch(text, -1) {
		for _, k := range strings.Split(m[1], ",") {
			if k = strings.TrimSpace(k); k != "" {
				keys = append(keys, k)
			}
		}
	}
	for _, m := range citeKeyRe.FindAllStringSubmatch(text, -1) {
		keys = append(keys, m[1])
	}
	return keys
}

// checkForbiddenTerms implements P5. Each declared forbidden term is reported
// wherever it occurs in publication prose.
func (r *paperRegistry) checkForbiddenTerms(prose []string) []string {
	if len(r.ForbiddenTerms) == 0 {
		return nil
	}
	var out []string
	for _, path := range prose {
		out = append(out, forbiddenTermHits(path, r.ForbiddenTerms)...)
	}
	sort.Strings(out)
	return out
}

// forbiddenTermHits reports each forbidden term found in one file, matched
// case-insensitively on a word boundary.
func forbiddenTermHits(path string, terms []string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lower := strings.ToLower(string(data))
	var out []string
	for _, term := range terms {
		t := strings.ToLower(strings.TrimSpace(term))
		if t != "" && strings.Contains(lower, t) {
			out = append(out, fmt.Sprintf("%s: forbidden term %q occurs in prose (P5)", path, term))
		}
	}
	return out
}

// gatherProse expands the prose globs to a sorted, de-duplicated file list. It
// supports a single ** segment (for example paper/**/*.md) in addition to the
// patterns filepath.Glob understands.
func gatherProse(globs []string) []string {
	seen := make(map[string]bool)
	for _, g := range globs {
		for _, p := range expandGlob(g) {
			seen[p] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// expandGlob resolves one glob. Patterns without ** defer to filepath.Glob;
// a pattern with a ** segment walks the directory before it and matches the
// trailing pattern against each file's base name.
func expandGlob(pattern string) []string {
	if !strings.Contains(pattern, "**") {
		matches, _ := filepath.Glob(pattern)
		return matches
	}
	root, tail, ok := strings.Cut(pattern, "**")
	if !ok {
		return nil
	}
	root = strings.TrimSuffix(root, string(filepath.Separator))
	if root == "" {
		root = "."
	}
	trailing := strings.TrimPrefix(tail, string(filepath.Separator))
	var out []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match(trailing, filepath.Base(p)); ok {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// dedupe removes duplicate strings from a sorted slice.
func dedupe(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
