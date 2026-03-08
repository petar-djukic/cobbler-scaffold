// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package generate implements the generation lifecycle: start, run, resume,
// stop, switch, reset, and list operations. The parent orchestrator package
// provides thin receiver-method wrappers around these functions.
package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Injected dependencies
// ---------------------------------------------------------------------------

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// Package-level variables set by the parent package at init time.
var (
	Log Logger = func(string, ...any) {}

	BinGit string
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// BaseBranchFile is the name of the file that records which branch a
// generation was started from, stored inside the cobbler directory.
const BaseBranchFile = "base-branch"

// TagSuffixes lists the lifecycle tag suffixes in order.
var TagSuffixes = []string{"-start", "-finished", "-merged", "-abandoned"}

// ---------------------------------------------------------------------------
// Pure functions
// ---------------------------------------------------------------------------

// ResolveStopTarget returns the branch that generator:stop should merge into.
// callerBranch is the branch checked out when generator:stop was invoked.
// genBranch is the generation branch being stopped. recordedBase is the branch
// written by generator:start. When the caller is on a branch other than the
// generation branch and other than the recorded base, that caller branch is
// returned so that generator:stop merges into an explicit feature branch rather
// than always forcing a merge into the recorded base (GH-523).
func ResolveStopTarget(callerBranch, genBranch, recordedBase string) string {
	if callerBranch != genBranch && callerBranch != recordedBase {
		return callerBranch
	}
	return recordedBase
}

// GenerationName strips the lifecycle suffix from a tag to recover
// the generation name.
func GenerationName(tag string) string {
	for _, suffix := range TagSuffixes {
		if cut, ok := strings.CutSuffix(tag, suffix); ok {
			return cut
		}
	}
	return tag
}

// ---------------------------------------------------------------------------
// Git branch helpers
// ---------------------------------------------------------------------------

// SaveAndSwitchBranch commits or stashes uncommitted changes on the
// current branch, then checks out the target branch. It tries a WIP
// commit first; if that fails and the tree is still dirty, it stashes
// changes so the checkout can succeed.
//
// The caller must provide git helper functions via the deps struct.
func SaveAndSwitchBranch(target string, deps GitDeps) error {
	current, err := deps.CurrentBranch(".")
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}

	if err := deps.StageAll("."); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	msg := fmt.Sprintf("WIP: save state before switching to %s", target)
	if err := deps.Commit(msg, "."); err != nil {
		// Commit failed (e.g. nothing to commit). Unstage and fall
		// back to stash if the tree is still dirty.
		_ = deps.UnstageAll(".") // best-effort; unstage before stash fallback
		if deps.HasChanges(".") {
			Log("saveAndSwitchBranch: commit failed, stashing dirty tree")
			_ = deps.Stash(msg, ".") // best-effort; switching branch is the priority
		}
	}

	Log("saveAndSwitchBranch: %s -> %s", current, target)
	return deps.Checkout(target, ".")
}

// EnsureOnBranch switches to the given branch if not already on it.
func EnsureOnBranch(branch string, deps GitDeps) error {
	current, err := deps.CurrentBranch(".")
	if err != nil {
		return err
	}
	if current == branch {
		return nil
	}
	Log("ensureOnBranch: switching from %s to %s", current, branch)
	return deps.Checkout(branch, ".")
}

// ---------------------------------------------------------------------------
// File system helpers
// ---------------------------------------------------------------------------

// RemoveEmptyDirs removes empty directories under the given root.
func RemoveEmptyDirs(root string) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return
	}
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dirs[i]) // best-effort empty dir cleanup
		}
	}
}

// AppendToGitignore adds entry to the .gitignore file in dir if not already
// present. Creates the file if it does not exist. Used by GeneratorStart to
// ensure build artifacts (bin/) are not committed to the generation branch
// regardless of where they are produced (GH-469).
func AppendToGitignore(dir, entry string) error {
	path := filepath.Join(dir, ".gitignore")

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil // already present
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()

	prefix := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(f, "%s%s\n", prefix, entry)
	return err
}

// ---------------------------------------------------------------------------
// GitDeps provides git operations needed by the generate package.
// ---------------------------------------------------------------------------

// GitDeps holds the git helper functions injected by the parent package.
type GitDeps struct {
	Checkout      func(branch, dir string) error
	CurrentBranch func(dir string) (string, error)
	StageAll      func(dir string) error
	UnstageAll    func(dir string) error
	Commit        func(msg, dir string) error
	HasChanges    func(dir string) bool
	Stash         func(msg, dir string) error
}
