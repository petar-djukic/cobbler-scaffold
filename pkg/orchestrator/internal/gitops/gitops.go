// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package gitops defines a GitOps interface for git operations and provides
// a ShellGitOps implementation that shells out to the git binary.
package gitops

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// DiffStat holds parsed output from git diff --shortstat.
type DiffStat struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

// FileChange holds per-file diff information.
type FileChange struct {
	Path       string `yaml:"path"`
	Status     string `yaml:"status"`
	Insertions int    `yaml:"insertions"`
	Deletions  int    `yaml:"deletions"`
}

// ---------------------------------------------------------------------------
// Interface
// ---------------------------------------------------------------------------

// GitOps defines the git operations used by the orchestrator.
type GitOps interface {
	Checkout(branch, dir string) error
	CheckoutNew(branch, dir string) error
	CreateBranch(name, dir string) error
	DeleteBranch(name, dir string) error
	ForceDeleteBranch(name, dir string) error
	BranchExists(name, dir string) bool
	ListBranches(pattern, dir string) []string
	CurrentBranch(dir string) (string, error)
	Tag(name, dir string) error
	DeleteTag(name, dir string) error
	TagAt(name, ref, dir string) error
	RenameTag(oldName, newName, dir string) error
	ListTags(pattern, dir string) []string
	LsFiles(dir string) []string
	StageAll(dir string) error
	StageDir(path, dir string) error
	UnstageAll(dir string) error
	HasChanges(dir string) bool
	Stash(msg, dir string) error
	Commit(msg, dir string) error
	CommitAllowEmpty(msg, dir string) error
	RevParseHEAD(dir string) (string, error)
	ResetSoft(ref, dir string) error
	MergeCmd(branch, dir string) *exec.Cmd
	WorktreePrune(dir string) error
	WorktreeAdd(worktreeDir, branch, dir string) *exec.Cmd
	WorktreeRemove(worktreeDir, dir string) error
	DiffShortstat(ref, dir string) (DiffStat, error)
	DiffNameStatus(ref, dir string) ([]FileChange, error)
	LsTreeFiles(ref, dir string) ([]string, error)
	ShowFileContent(ref, path, dir string) ([]byte, error)
}

// ---------------------------------------------------------------------------
// ShellGitOps implementation
// ---------------------------------------------------------------------------

// ShellGitOps implements GitOps by shelling out to the git binary.
type ShellGitOps struct {
	// GitBin is the git binary name or path. Defaults to "git" when empty.
	GitBin string
}

func (g *ShellGitOps) gitBin() string {
	if g.GitBin != "" {
		return g.GitBin
	}
	return "git"
}

// cmdGit returns an exec.Cmd for git with cmd.Dir set to dir when non-empty.
func (g *ShellGitOps) cmdGit(dir string, arg ...string) *exec.Cmd {
	cmd := exec.Command(g.gitBin(), arg...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

func (g *ShellGitOps) Checkout(branch, dir string) error {
	return g.cmdGit(dir, "checkout", branch).Run()
}

func (g *ShellGitOps) CheckoutNew(branch, dir string) error {
	return g.cmdGit(dir, "checkout", "-b", branch).Run()
}

func (g *ShellGitOps) CreateBranch(name, dir string) error {
	return g.cmdGit(dir, "branch", name).Run()
}

func (g *ShellGitOps) DeleteBranch(name, dir string) error {
	return g.cmdGit(dir, "branch", "-d", name).Run()
}

func (g *ShellGitOps) ForceDeleteBranch(name, dir string) error {
	return g.cmdGit(dir, "branch", "-D", name).Run()
}

func (g *ShellGitOps) BranchExists(name, dir string) bool {
	return g.cmdGit(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name).Run() == nil
}

func (g *ShellGitOps) ListBranches(pattern, dir string) []string {
	out, _ := g.cmdGit(dir, "branch", "--list", pattern).Output()
	return ParseBranchList(string(out))
}

func (g *ShellGitOps) CurrentBranch(dir string) (string, error) {
	out, err := g.cmdGit(dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *ShellGitOps) Tag(name, dir string) error {
	return g.cmdGit(dir, "tag", name).Run()
}

func (g *ShellGitOps) DeleteTag(name, dir string) error {
	return g.cmdGit(dir, "tag", "-d", name).Run()
}

func (g *ShellGitOps) TagAt(name, ref, dir string) error {
	return g.cmdGit(dir, "tag", name, ref).Run()
}

func (g *ShellGitOps) RenameTag(oldName, newName, dir string) error {
	if err := g.cmdGit(dir, "tag", newName, oldName).Run(); err != nil {
		return err
	}
	return g.DeleteTag(oldName, dir)
}

func (g *ShellGitOps) ListTags(pattern, dir string) []string {
	out, _ := g.cmdGit(dir, "tag", "--list", pattern).Output()
	return ParseBranchList(string(out))
}

func (g *ShellGitOps) LsFiles(dir string) []string {
	if dir == "" {
		return nil
	}
	out, err := g.cmdGit(dir, "ls-files").Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	return ParseBranchList(string(out))
}

func (g *ShellGitOps) StageAll(dir string) error {
	return g.cmdGit(dir, "add", "-A").Run()
}

func (g *ShellGitOps) StageDir(path, dir string) error {
	return g.cmdGit(dir, "add", path).Run()
}

func (g *ShellGitOps) UnstageAll(dir string) error {
	return g.cmdGit(dir, "reset", "HEAD").Run()
}

func (g *ShellGitOps) HasChanges(dir string) bool {
	return g.cmdGit(dir, "diff", "--quiet", "HEAD").Run() != nil
}

func (g *ShellGitOps) Stash(msg, dir string) error {
	return g.cmdGit(dir, "stash", "push", "-m", msg).Run()
}

func (g *ShellGitOps) Commit(msg, dir string) error {
	return g.cmdGit(dir, "commit", "--no-verify", "-m", msg).Run()
}

func (g *ShellGitOps) CommitAllowEmpty(msg, dir string) error {
	return g.cmdGit(dir, "commit", "--no-verify", "-m", msg, "--allow-empty").Run()
}

func (g *ShellGitOps) RevParseHEAD(dir string) (string, error) {
	out, err := g.cmdGit(dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *ShellGitOps) ResetSoft(ref, dir string) error {
	return g.cmdGit(dir, "reset", "--soft", ref).Run()
}

func (g *ShellGitOps) MergeCmd(branch, dir string) *exec.Cmd {
	return g.cmdGit(dir, "merge", branch, "--no-edit")
}

func (g *ShellGitOps) WorktreePrune(dir string) error {
	return g.cmdGit(dir, "worktree", "prune").Run()
}

func (g *ShellGitOps) WorktreeAdd(worktreeDir, branch, dir string) *exec.Cmd {
	return g.cmdGit(dir, "worktree", "add", worktreeDir, branch)
}

func (g *ShellGitOps) WorktreeRemove(worktreeDir, dir string) error {
	return g.cmdGit(dir, "worktree", "remove", worktreeDir, "--force").Run()
}

func (g *ShellGitOps) DiffShortstat(ref, dir string) (DiffStat, error) {
	out, err := g.cmdGit(dir, "diff", "--shortstat", ref).Output()
	if err != nil {
		return DiffStat{}, err
	}
	return ParseDiffShortstat(string(out)), nil
}

func (g *ShellGitOps) DiffNameStatus(ref, dir string) ([]FileChange, error) {
	nsOut, err := g.cmdGit(dir, "diff", "--name-status", ref).Output()
	if err != nil {
		return nil, err
	}

	numOut, _ := g.cmdGit(dir, "diff", "--numstat", ref).Output()
	numMap := ParseNumstat(string(numOut))

	return ParseNameStatus(string(nsOut), numMap), nil
}

func (g *ShellGitOps) LsTreeFiles(ref, dir string) ([]string, error) {
	out, err := g.cmdGit(dir, "ls-tree", "-r", "--name-only", ref).Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func (g *ShellGitOps) ShowFileContent(ref, path, dir string) ([]byte, error) {
	return g.cmdGit(dir, "show", ref+":"+path).Output()
}

// ---------------------------------------------------------------------------
// Parse helpers (exported for use by the orchestrator package)
// ---------------------------------------------------------------------------

// ParseBranchList parses the output of git branch --list or git tag --list.
func ParseBranchList(output string) []string {
	var branches []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "*+ ")
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// ParseDiffShortstat extracts file/insertion/deletion counts from
// git diff --shortstat output.
func ParseDiffShortstat(s string) DiffStat {
	var ds DiffStat
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		var n int
		if _, err := fmt.Sscanf(part, "%d file", &n); err == nil {
			ds.FilesChanged = n
		} else if _, err := fmt.Sscanf(part, "%d insertion", &n); err == nil {
			ds.Insertions = n
		} else if _, err := fmt.Sscanf(part, "%d deletion", &n); err == nil {
			ds.Deletions = n
		}
	}
	return ds
}

// NumstatEntry holds parsed numstat data for a single file.
type NumstatEntry struct {
	Ins int
	Del int
}

// ParseNumstat parses git diff --numstat output into a map keyed by file path.
// Binary files show "-\t-\tpath" and are recorded with zero counts.
func ParseNumstat(output string) map[string]NumstatEntry {
	m := make(map[string]NumstatEntry)
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		path := parts[len(parts)-1]
		m[path] = NumstatEntry{Ins: ins, Del: del}
	}
	return m
}

// ParseNameStatus parses git diff --name-status output and merges it with
// numstat data to produce FileChange entries.
func ParseNameStatus(output string, numMap map[string]NumstatEntry) []FileChange {
	var files []FileChange
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		path := parts[1]

		// Renames show as R### with old\tnew paths.
		if strings.HasPrefix(status, "R") && len(parts) >= 3 {
			path = parts[2]
			status = "R"
		}
		// Copies show as C### with old\tnew paths.
		if strings.HasPrefix(status, "C") && len(parts) >= 3 {
			path = parts[2]
			status = "C"
		}

		fc := FileChange{Path: path, Status: status}
		if ns, ok := numMap[path]; ok {
			fc.Insertions = ns.Ins
			fc.Deletions = ns.Del
		}
		files = append(files, fc)
	}
	return files
}
