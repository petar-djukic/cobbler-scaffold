// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// compare.go implements cross-generation differential comparison.
// prd: prd004-differential-comparison R1, R2, R3

package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// BinaryResolver resolves a utility name to a binary path. The returned
// cleanup function removes any temporary resources (worktrees, build
// directories). It is idempotent and safe to call multiple times.
type BinaryResolver interface {
	Resolve(utility string) (path string, cleanup func(), err error)
	ListUtilities() ([]string, error)
}

// --- GitTagResolver ---

// GitTagResolver builds binaries from a git tag. It creates a temporary
// worktree, discovers cmd/ packages, and builds each with go build. The
// cleanup function removes the worktree and temporary build directory.
type GitTagResolver struct {
	Tag      string
	buildDir string
	wtDir    string
}

func (r *GitTagResolver) Resolve(utility string) (string, func(), error) {
	if err := r.ensureBuild(); err != nil {
		return "", noop, fmt.Errorf("git tag resolver: %w", err)
	}
	binPath := filepath.Join(r.buildDir, utility)
	if _, err := os.Stat(binPath); err != nil {
		return "", noop, fmt.Errorf("git tag resolver: binary %q not found in %s: %w", utility, r.Tag, err)
	}
	return binPath, r.cleanup, nil
}

func (r *GitTagResolver) ListUtilities() ([]string, error) {
	if err := r.ensureBuild(); err != nil {
		return nil, fmt.Errorf("git tag resolver: %w", err)
	}
	entries, err := os.ReadDir(r.buildDir)
	if err != nil {
		return nil, fmt.Errorf("listing build dir: %w", err)
	}
	var utils []string
	for _, e := range entries {
		if !e.IsDir() {
			utils = append(utils, e.Name())
		}
	}
	return utils, nil
}

// ensureBuild creates the worktree and builds binaries once.
func (r *GitTagResolver) ensureBuild() error {
	if r.buildDir != "" {
		return nil // already built
	}

	wtDir, err := os.MkdirTemp("", "compare-wt-*")
	if err != nil {
		return fmt.Errorf("creating worktree dir: %w", err)
	}
	// git worktree add requires a non-existing target directory.
	os.Remove(wtDir)

	cmd := exec.Command(binGit, "worktree", "add", wtDir, r.Tag)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating worktree for %s: %w", r.Tag, err)
	}
	r.wtDir = wtDir

	buildDir, err := os.MkdirTemp("", "compare-bin-*")
	if err != nil {
		r.cleanup()
		return fmt.Errorf("creating build dir: %w", err)
	}

	// Discover cmd/ packages in the worktree.
	cmdDir := filepath.Join(wtDir, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		r.cleanup()
		os.RemoveAll(buildDir)
		return fmt.Errorf("reading cmd/ in worktree: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		pkgPath := filepath.Join(cmdDir, name)
		outPath := filepath.Join(buildDir, name)
		logf("compare: building %s from %s", name, r.Tag)
		build := exec.Command(binGo, "build", "-o", outPath, pkgPath)
		build.Dir = wtDir
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			r.cleanup()
			os.RemoveAll(buildDir)
			return fmt.Errorf("building %s from %s: %w", name, r.Tag, err)
		}
	}

	r.buildDir = buildDir
	return nil
}

func (r *GitTagResolver) cleanup() {
	if r.wtDir != "" {
		if err := gitWorktreeRemove(r.wtDir); err != nil {
			logf("compare: warning: removing worktree %s: %v", r.wtDir, err)
		}
		r.wtDir = ""
	}
	if r.buildDir != "" {
		if err := os.RemoveAll(r.buildDir); err != nil {
			logf("compare: warning: removing build dir %s: %v", r.buildDir, err)
		}
		r.buildDir = ""
	}
}

// --- GNUResolver ---

// GNUResolver resolves Homebrew reference binaries via exec.LookPath.
// Coreutils use g-prefixed names (cat -> gcat). Moreutils use unprefixed
// names (ts -> ts).
type GNUResolver struct{}

// moreutils lists utilities that do not use the g-prefix convention.
var moreutils = map[string]bool{
	"chronic": true, "combine": true, "errno": true, "ifdata": true,
	"ifne": true, "isutf8": true, "lckdo": true, "mispipe": true,
	"parallel": true, "pee": true, "sponge": true, "ts": true,
	"vidir": true, "vipe": true, "zrun": true,
}

func (r GNUResolver) binaryName(utility string) string {
	if moreutils[utility] {
		return utility
	}
	return "g" + utility
}

func (r GNUResolver) Resolve(utility string) (string, func(), error) {
	name := r.binaryName(utility)
	path, err := exec.LookPath(name)
	if err != nil {
		return "", noop, fmt.Errorf("gnu resolver: %s (%s) not found on PATH: %w", utility, name, err)
	}
	return path, noop, nil
}

func (r GNUResolver) ListUtilities() ([]string, error) {
	// Discover by scanning Homebrew coreutils and moreutils bin directories.
	var utils []string
	for _, dir := range []string{
		"/opt/homebrew/opt/coreutils/libexec/gnubin",
		"/usr/local/opt/coreutils/libexec/gnubin",
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				utils = append(utils, e.Name())
			}
		}
		break // use first found
	}
	for name := range moreutils {
		if _, err := exec.LookPath(name); err == nil {
			utils = append(utils, name)
		}
	}
	return utils, nil
}

// --- PathResolver ---

// PathResolver resolves pre-built binaries from a directory.
type PathResolver struct {
	Dir string
}

func (r PathResolver) Resolve(utility string) (string, func(), error) {
	path := filepath.Join(r.Dir, utility)
	if _, err := os.Stat(path); err != nil {
		return "", noop, fmt.Errorf("path resolver: %s not found in %s: %w", utility, r.Dir, err)
	}
	return path, noop, nil
}

func (r PathResolver) ListUtilities() ([]string, error) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return nil, fmt.Errorf("listing directory %s: %w", r.Dir, err)
	}
	var utils []string
	for _, e := range entries {
		if !e.IsDir() {
			utils = append(utils, e.Name())
		}
	}
	return utils, nil
}

// --- Factory ---

// ResolverFromArg selects a BinaryResolver based on the argument value.
// "gnu" returns a GNUResolver. A path to an existing directory returns
// a PathResolver. Anything else is treated as a git tag.
func ResolverFromArg(arg string) BinaryResolver {
	if strings.ToLower(arg) == "gnu" {
		return GNUResolver{}
	}
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		return PathResolver{Dir: arg}
	}
	return &GitTagResolver{Tag: arg}
}

// noop is a no-op cleanup function.
func noop() {}

// defaultSpecsDir is the conventional location of test suite YAML files.
const defaultSpecsDir = "docs/specs/test-suites"

// Compare runs differential comparison between two binary sources.
// argA and argB are passed to ResolverFromArg (git tag, "gnu", or directory).
// When utility is non-empty, only that utility is compared; otherwise all
// common utilities between the two sources are compared.
func (o *Orchestrator) Compare(argA, argB, utility string) error {
	resolverA := ResolverFromArg(argA)
	resolverB := ResolverFromArg(argB)

	specsDir := defaultSpecsDir
	cases, err := LoadCompareTestCases(specsDir)
	if err != nil {
		return fmt.Errorf("loading test cases: %w", err)
	}

	utilities, err := commonUtilities(resolverA, resolverB, utility)
	if err != nil {
		return err
	}
	if len(utilities) == 0 {
		return fmt.Errorf("no common utilities found between %s and %s", argA, argB)
	}

	logf("compare: %d utilities to compare between %s and %s", len(utilities), argA, argB)

	var allResults []TestResult
	var cleanups []func()
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	for _, util := range utilities {
		utilCases := FilterByUtility(cases, util)
		if len(utilCases) == 0 {
			logf("compare: skipping %s (no test cases)", util)
			continue
		}

		pathA, cleanupA, err := resolverA.Resolve(util)
		if err != nil {
			logf("compare: skipping %s: resolver A: %v", util, err)
			continue
		}
		cleanups = append(cleanups, cleanupA)

		pathB, cleanupB, err := resolverB.Resolve(util)
		if err != nil {
			logf("compare: skipping %s: resolver B: %v", util, err)
			continue
		}
		cleanups = append(cleanups, cleanupB)

		logf("compare: running %d test cases for %s", len(utilCases), util)
		results := CompareUtility(pathA, pathB, utilCases)
		allResults = append(allResults, results...)
	}

	fmt.Print(FormatResults(allResults))

	for _, r := range allResults {
		if !r.Passed {
			return fmt.Errorf("comparison failed: %d tests did not match", countFailed(allResults))
		}
	}
	return nil
}

// commonUtilities returns the sorted intersection of utilities available
// in both resolvers. When utility is non-empty, returns only that utility
// (validating it exists in both).
func commonUtilities(a, b BinaryResolver, utility string) ([]string, error) {
	if utility != "" {
		return []string{utility}, nil
	}

	utilsA, err := a.ListUtilities()
	if err != nil {
		return nil, fmt.Errorf("listing utilities from A: %w", err)
	}
	utilsB, err := b.ListUtilities()
	if err != nil {
		return nil, fmt.Errorf("listing utilities from B: %w", err)
	}

	setB := make(map[string]bool, len(utilsB))
	for _, u := range utilsB {
		setB[u] = true
	}

	var common []string
	for _, u := range utilsA {
		if setB[u] {
			common = append(common, u)
		}
	}
	sort.Strings(common)
	return common, nil
}

// countFailed returns the number of failed results.
func countFailed(results []TestResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}
