// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Binary names.
const (
	binGit    = "git"
	binBd     = "bd"
	binClaude = "claude"
	binGo     = "go"
	binLint   = "golangci-lint"
	binMage   = "mage"
	binPodman = "podman"
)

// defaultClaudeArgs are the CLI arguments for automated Claude execution.
// Used by Config.applyDefaults when ClaudeArgs is empty.
var defaultClaudeArgs = []string{
	"--dangerously-skip-permissions",
	"-p",
	"--verbose",
	"--output-format", "stream-json",
}

// Git helpers.

func gitCheckout(branch string) error {
	return exec.Command(binGit, "checkout", branch).Run()
}

func gitCheckoutNew(branch string) error {
	return exec.Command(binGit, "checkout", "-b", branch).Run()
}

func gitCreateBranch(name string) error {
	return exec.Command(binGit, "branch", name).Run()
}

func gitDeleteBranch(name string) error {
	return exec.Command(binGit, "branch", "-d", name).Run()
}

func gitForceDeleteBranch(name string) error {
	return exec.Command(binGit, "branch", "-D", name).Run()
}

func gitBranchExists(name string) bool {
	return exec.Command(binGit, "show-ref", "--verify", "--quiet", "refs/heads/"+name).Run() == nil
}

func gitListBranches(pattern string) []string {
	out, _ := exec.Command(binGit, "branch", "--list", pattern).Output()
	return parseBranchList(string(out))
}

func gitTag(name string) error {
	return exec.Command(binGit, "tag", name).Run()
}

func gitDeleteTag(name string) error {
	return exec.Command(binGit, "tag", "-d", name).Run()
}

// gitTagAt creates a tag pointing at the given ref (commit, tag, or branch).
func gitTagAt(name, ref string) error {
	return exec.Command(binGit, "tag", name, ref).Run()
}

// gitRenameTag creates newName at the same commit as oldName, then
// deletes oldName. Returns an error if the new tag cannot be created.
func gitRenameTag(oldName, newName string) error {
	if err := exec.Command(binGit, "tag", newName, oldName).Run(); err != nil {
		return err
	}
	return gitDeleteTag(oldName)
}

func gitListTags(pattern string) []string {
	out, _ := exec.Command(binGit, "tag", "--list", pattern).Output()
	return parseBranchList(string(out))
}

func gitStageAll() error {
	return exec.Command(binGit, "add", "-A").Run()
}

func gitUnstageAll() error {
	return exec.Command(binGit, "reset", "HEAD").Run()
}

func gitStageDir(dir string) error {
	return exec.Command(binGit, "add", dir).Run()
}

func gitCommit(msg string) error {
	return exec.Command(binGit, "commit", "-m", msg).Run()
}

func gitCommitAllowEmpty(msg string) error {
	return exec.Command(binGit, "commit", "-m", msg, "--allow-empty").Run()
}

func gitRevParseHEAD() (string, error) {
	out, err := exec.Command(binGit, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return string(out[:len(out)-1]), nil
}

func gitResetSoft(ref string) error {
	return exec.Command(binGit, "reset", "--soft", ref).Run()
}

func gitMergeCmd(branch string) *exec.Cmd {
	return exec.Command(binGit, "merge", branch, "--no-edit")
}

func gitWorktreePrune() error {
	return exec.Command(binGit, "worktree", "prune").Run()
}

func gitWorktreeAdd(dir, branch string) *exec.Cmd {
	return exec.Command(binGit, "worktree", "add", dir, branch)
}

func gitWorktreeRemove(dir string) error {
	return exec.Command(binGit, "worktree", "remove", dir, "--force").Run()
}

func gitCurrentBranch() (string, error) {
	out, err := exec.Command(binGit, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return string(out[:len(out)-1]), nil // trim trailing newline
}

// gitCountCommits returns the number of commits reachable from toRef
// but not from fromRef (i.e. commits in fromRef..toRef).
func gitCountCommits(fromRef, toRef string) (int, error) {
	out, err := exec.Command(binGit, "rev-list", "--count", fromRef+".."+toRef).Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// gitWorktreeCount returns the number of linked worktrees (excludes
// the main worktree). Uses git worktree list --porcelain and counts
// "worktree " lines beyond the first (main).
func gitWorktreeCount() int {
	out, _ := exec.Command(binGit, "worktree", "list", "--porcelain").Output()
	count := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			count++
		}
	}
	// The first "worktree" entry is always the main worktree.
	if count > 0 {
		count--
	}
	return count
}

// parseBranchList parses the output of git branch --list or git tag --list.
func parseBranchList(output string) []string {
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

// Beads helpers.

func bdSync() error {
	return exec.Command(binBd, "sync").Run()
}

func (o *Orchestrator) bdAdminReset() error {
	if _, err := os.Stat(o.cfg.BeadsDir); os.IsNotExist(err) {
		return nil // nothing to reset
	}
	// Stop the daemon before destroying the database; otherwise the
	// stale daemon blocks subsequent bd commands.
	_ = exec.Command(binBd, "daemon", "stop", ".").Run()
	return exec.Command(binBd, "admin", "reset", "--force").Run()
}

func bdInit(prefix string) error {
	return exec.Command(binBd, "init", "--prefix", prefix, "--force").Run()
}

func bdClose(id string) error {
	return exec.Command(binBd, "close", id).Run()
}

func bdUpdateStatus(id, status string) error {
	return exec.Command(binBd, "update", id, "--status", status).Run()
}

func bdListJSON() ([]byte, error) {
	return exec.Command(binBd, "list", "--json").Output()
}

func bdListInProgressTasks() ([]byte, error) {
	return exec.Command(binBd, "list", "--json", "--status", "in_progress", "--type", "task").Output()
}

func bdNextReadyTask() ([]byte, error) {
	return exec.Command(binBd, "ready", "-n", "1", "--json", "--type", "task").Output()
}

func bdAddDep(childID, parentID string) error {
	return exec.Command(binBd, "dep", "add", childID, parentID).Run()
}

func bdCreateTask(title, description string) ([]byte, error) {
	return exec.Command(binBd, "create", "--type", "task", "--json", title, "--description", description).Output()
}

func bdListClosedTasks() ([]byte, error) {
	return exec.Command(binBd, "list", "--json", "--status", "closed", "--type", "task").Output()
}

func bdListReadyTasks() ([]byte, error) {
	return exec.Command(binBd, "list", "--json", "--status", "ready", "--type", "task").Output()
}

func bdCommentAdd(id, comment string) error {
	return exec.Command(binBd, "comments", "add", id, comment).Run()
}

func bdShowJSON(id string) ([]byte, error) {
	return exec.Command(binBd, "show", id, "--json").Output()
}

// diffStat holds parsed output from git diff --shortstat.
type diffStat struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

// gitDiffShortstat runs git diff --shortstat against the given ref and
// parses the output (e.g. "5 files changed, 100 insertions(+), 20 deletions(-)").
func gitDiffShortstat(ref string) (diffStat, error) {
	out, err := exec.Command(binGit, "diff", "--shortstat", ref).Output()
	if err != nil {
		return diffStat{}, err
	}
	return parseDiffShortstat(string(out)), nil
}

// parseDiffShortstat extracts file/insertion/deletion counts from
// git diff --shortstat output.
func parseDiffShortstat(s string) diffStat {
	var ds diffStat
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

// Podman helpers.

// podmanBuild builds a container image from a Dockerfile, applying one or
// more image tags. Each tag is a full image reference (e.g., "name:v1").
func podmanBuild(dockerfile string, tags ...string) error {
	args := []string{"build", "-f", dockerfile}
	for _, t := range tags {
		args = append(args, "-t", t)
	}
	args = append(args, ".")
	cmd := exec.Command(binPodman, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Go helpers.

func (o *Orchestrator) goModInit() error {
	return exec.Command(binGo, "mod", "init", o.cfg.ModulePath).Run()
}

func goModEditReplace(old, new string) error {
	return exec.Command(binGo, "mod", "edit", "-replace", old+"="+new).Run()
}

func goModTidy() error {
	return exec.Command(binGo, "mod", "tidy").Run()
}
