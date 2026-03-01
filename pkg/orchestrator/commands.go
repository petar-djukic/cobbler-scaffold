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
	binGit      = "git"
	binClaude   = "claude"
	binGh       = "gh"
	binGo       = "go"
	binLint     = "golangci-lint"
	binMage     = "mage"
	binPodman   = "podman"
	binSecurity = "security"
)

// Directory and file path constants.
const (
	dirMagefiles = "magefiles"
	dirCobbler   = ".cobbler"
)

// orDefault returns val if non-empty, otherwise fallback.
func orDefault(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// defaultClaudeArgs are the CLI arguments for automated Claude execution.
// Used by Config.applyDefaults when ClaudeArgs is empty.
var defaultClaudeArgs = []string{
	"--dangerously-skip-permissions",
	"-p",
	"--verbose",
	"--output-format", "stream-json",
}

func init() {
	// Ensure GOBIN (or GOPATH/bin) is in PATH so exec.LookPath finds
	// Go-installed binaries like mage and golangci-lint.
	if gobin, err := exec.Command(binGo, "env", "GOBIN").Output(); err == nil {
		if dir := strings.TrimSpace(string(gobin)); dir != "" {
			os.Setenv("PATH", dir+":"+os.Getenv("PATH"))
			return
		}
	}
	if gopath, err := exec.Command(binGo, "env", "GOPATH").Output(); err == nil {
		if dir := strings.TrimSpace(string(gopath)); dir != "" {
			os.Setenv("PATH", dir+"/bin:"+os.Getenv("PATH"))
		}
	}
}

// cmdGit returns an exec.Cmd for git with cmd.Dir set to dir when dir is non-empty.
// Pass an empty string to use the process working directory (backward-compatible default).
func cmdGit(dir string, arg ...string) *exec.Cmd {
	cmd := exec.Command(binGit, arg...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

// Git helpers.
// Each function accepts a dir string parameter; when dir is non-empty it is
// forwarded to exec.Cmd.Dir so the command runs in that directory rather than
// the process-wide working directory. Pass "" to use the existing CWD (the
// original behaviour, preserved for callers that rely on os.Chdir).

func gitCheckout(branch, dir string) error {
	return cmdGit(dir, "checkout", branch).Run()
}

func gitCheckoutNew(branch, dir string) error {
	return cmdGit(dir, "checkout", "-b", branch).Run()
}

func gitCreateBranch(name, dir string) error {
	return cmdGit(dir, "branch", name).Run()
}

func gitDeleteBranch(name, dir string) error {
	return cmdGit(dir, "branch", "-d", name).Run()
}

func gitForceDeleteBranch(name, dir string) error {
	return cmdGit(dir, "branch", "-D", name).Run()
}

func gitBranchExists(name, dir string) bool {
	return cmdGit(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name).Run() == nil
}

func gitListBranches(pattern, dir string) []string {
	out, _ := cmdGit(dir, "branch", "--list", pattern).Output() // empty output on error is acceptable
	return parseBranchList(string(out))
}

func gitTag(name, dir string) error {
	return cmdGit(dir, "tag", name).Run()
}

func gitDeleteTag(name, dir string) error {
	return cmdGit(dir, "tag", "-d", name).Run()
}

// gitTagAt creates a tag pointing at the given ref (commit, tag, or branch).
func gitTagAt(name, ref, dir string) error {
	return cmdGit(dir, "tag", name, ref).Run()
}

// gitRenameTag creates newName at the same commit as oldName, then
// deletes oldName. Returns an error if the new tag cannot be created.
func gitRenameTag(oldName, newName, dir string) error {
	if err := cmdGit(dir, "tag", newName, oldName).Run(); err != nil {
		return err
	}
	return gitDeleteTag(oldName, dir)
}

func gitListTags(pattern, dir string) []string {
	out, _ := cmdGit(dir, "tag", "--list", pattern).Output() // empty output on error is acceptable
	return parseBranchList(string(out))
}

// gitLsFiles returns all git-tracked file paths in dir, relative to dir.
// Returns nil if dir is empty, if git ls-files produces no output, or on error.
func gitLsFiles(dir string) []string {
	if dir == "" {
		return nil
	}
	out, err := cmdGit(dir, "ls-files").Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	return parseBranchList(string(out))
}

func gitStageAll(dir string) error {
	return cmdGit(dir, "add", "-A").Run()
}

func gitUnstageAll(dir string) error {
	return cmdGit(dir, "reset", "HEAD").Run()
}

// gitHasChanges returns true if the working tree has staged or unstaged
// changes (tracked files only).
func gitHasChanges(dir string) bool {
	// --quiet exits 1 when there are changes.
	return cmdGit(dir, "diff", "--quiet", "HEAD").Run() != nil
}

func gitStash(msg, dir string) error {
	return cmdGit(dir, "stash", "push", "-m", msg).Run()
}

// gitStageDir stages a specific path. path is the argument passed to git add;
// dir is the repository root used as cmd.Dir (empty means process CWD).
func gitStageDir(path, dir string) error {
	return cmdGit(dir, "add", path).Run()
}

func gitCommit(msg, dir string) error {
	return cmdGit(dir, "commit", "--no-verify", "-m", msg).Run()
}

func gitCommitAllowEmpty(msg, dir string) error {
	return cmdGit(dir, "commit", "--no-verify", "-m", msg, "--allow-empty").Run()
}

func gitRevParseHEAD(dir string) (string, error) {
	out, err := cmdGit(dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitResetSoft(ref, dir string) error {
	return cmdGit(dir, "reset", "--soft", ref).Run()
}

func gitMergeCmd(branch, dir string) *exec.Cmd {
	return cmdGit(dir, "merge", branch, "--no-edit")
}

func gitWorktreePrune(dir string) error {
	return cmdGit(dir, "worktree", "prune").Run()
}

// gitWorktreeAdd returns a Cmd that adds a worktree at worktreeDir on branch.
// dir is the repository root used as cmd.Dir (empty means process CWD).
func gitWorktreeAdd(worktreeDir, branch, dir string) *exec.Cmd {
	return cmdGit(dir, "worktree", "add", worktreeDir, branch)
}

// gitWorktreeRemove removes the worktree at worktreeDir.
// dir is the repository root used as cmd.Dir (empty means process CWD).
func gitWorktreeRemove(worktreeDir, dir string) error {
	return cmdGit(dir, "worktree", "remove", worktreeDir, "--force").Run()
}

func gitCurrentBranch(dir string) (string, error) {
	out, err := cmdGit(dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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

// gitLsTreeFiles returns the list of file paths tracked at the given ref.
func gitLsTreeFiles(ref, dir string) ([]string, error) {
	out, err := cmdGit(dir, "ls-tree", "-r", "--name-only", ref).Output()
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

// gitShowFileContent returns the raw content of a file at the given ref.
func gitShowFileContent(ref, path, dir string) ([]byte, error) {
	return cmdGit(dir, "show", ref+":"+path).Output()
}

// FileChange holds per-file diff information from git diff --name-status
// combined with insertion/deletion counts from git diff --numstat.
type FileChange struct {
	Path       string `yaml:"path"`
	Status     string `yaml:"status"`
	Insertions int    `yaml:"insertions"`
	Deletions  int    `yaml:"deletions"`
}

// diffStat holds parsed output from git diff --shortstat.
type diffStat struct {
	FilesChanged int
	Insertions   int
	Deletions    int
}

// gitDiffShortstat runs git diff --shortstat against the given ref and
// parses the output (e.g. "5 files changed, 100 insertions(+), 20 deletions(-)").
func gitDiffShortstat(ref, dir string) (diffStat, error) {
	out, err := cmdGit(dir, "diff", "--shortstat", ref).Output()
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

// gitDiffNameStatus runs git diff --name-status and --numstat against the
// given ref and returns per-file entries with path, status, insertions, and
// deletions. The two commands are combined to produce complete file-level
// change records.
func gitDiffNameStatus(ref, dir string) ([]FileChange, error) {
	nsOut, err := cmdGit(dir, "diff", "--name-status", ref).Output()
	if err != nil {
		return nil, err
	}

	numOut, _ := cmdGit(dir, "diff", "--numstat", ref).Output()
	numMap := parseNumstat(string(numOut))

	return parseNameStatus(string(nsOut), numMap), nil
}

// parseNameStatus parses git diff --name-status output and merges it with
// numstat data to produce FileChange entries.
func parseNameStatus(output string, numMap map[string]numstatEntry) []FileChange {
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
			fc.Insertions = ns.ins
			fc.Deletions = ns.del
		}
		files = append(files, fc)
	}
	return files
}

type numstatEntry struct {
	ins int
	del int
}

// parseNumstat parses git diff --numstat output into a map keyed by file path.
// Binary files show "-\t-\tpath" and are recorded with zero counts.
func parseNumstat(output string) map[string]numstatEntry {
	m := make(map[string]numstatEntry)
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		// Binary files use "-" for insertions and deletions.
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		path := parts[len(parts)-1]
		m[path] = numstatEntry{ins: ins, del: del}
	}
	return m
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
	return exec.Command(binGo, "mod", "init", o.cfg.Project.ModulePath).Run()
}

func goModEditReplace(old, new string) error {
	return exec.Command(binGo, "mod", "edit", "-replace", old+"="+new).Run()
}

func goModTidy() error {
	return exec.Command(binGo, "mod", "tidy").Run()
}
