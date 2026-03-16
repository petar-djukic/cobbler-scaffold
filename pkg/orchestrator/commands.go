// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"os/exec"
	"strings"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

// Binary names.
const (
	binGit      = "git"
	binClaude   = "claude"
	binGh       = "gh"
	binGo       = "go"
	binLint     = "golangci-lint"
	binMage     = "mage"
	binSecurity = "security"
)

// Directory and file path constants.
const (
	dirMagefiles = "magefiles"
	dirCobbler   = ".cobbler"
)

// defaultGitOps is the package-level Repository instance used by all git
// helper functions. It shells out to the "git" binary and implements all
// role-based interfaces (GH-1439).
var defaultGitOps = gitops.NewRepository("")

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

// Git helpers.
// Each function accepts a dir string parameter; when dir is non-empty it is
// forwarded to exec.Cmd.Dir so the command runs in that directory rather than
// the process-wide working directory. Pass "" to use the existing CWD (the
// original behaviour, preserved for callers that rely on os.Chdir).
//
// All git functions delegate to the defaultGitOps instance.

func gitCheckout(branch, dir string) error        { return defaultGitOps.Checkout(branch, dir) }
func gitCheckoutNew(branch, dir string) error      { return defaultGitOps.CheckoutNew(branch, dir) }
func gitCreateBranch(name, dir string) error       { return defaultGitOps.CreateBranch(name, dir) }
func gitDeleteBranch(name, dir string) error       { return defaultGitOps.DeleteBranch(name, dir) }
func gitForceDeleteBranch(name, dir string) error  { return defaultGitOps.ForceDeleteBranch(name, dir) }
func gitBranchExists(name, dir string) bool        { return defaultGitOps.BranchExists(name, dir) }
func gitListBranches(pattern, dir string) []string { return defaultGitOps.ListBranches(pattern, dir) }
func gitTag(name, dir string) error                { return defaultGitOps.Tag(name, dir) }
func gitDeleteTag(name, dir string) error          { return defaultGitOps.DeleteTag(name, dir) }
func gitListTags(pattern, dir string) []string     { return defaultGitOps.ListTags(pattern, dir) }
func gitLsFiles(dir string) []string               { return defaultGitOps.LsFiles(dir) }
func gitStageAll(dir string) error                 { return defaultGitOps.StageAll(dir) }
func gitStageDir(path, dir string) error           { return defaultGitOps.StageDir(path, dir) }
func gitUnstageAll(dir string) error               { return defaultGitOps.UnstageAll(dir) }
func gitHasChanges(dir string) bool                { return defaultGitOps.HasChanges(dir) }
func gitStash(msg, dir string) error               { return defaultGitOps.Stash(msg, dir) }
func gitCommit(msg, dir string) error              { return defaultGitOps.Commit(msg, dir) }
func gitCommitAllowEmpty(msg, dir string) error    { return defaultGitOps.CommitAllowEmpty(msg, dir) }
func gitResetSoft(ref, dir string) error           { return defaultGitOps.ResetSoft(ref, dir) }
func gitWorktreePrune(dir string) error            { return defaultGitOps.WorktreePrune(dir) }

// gitTagAt creates a tag pointing at the given ref (commit, tag, or branch).
func gitTagAt(name, ref, dir string) error { return defaultGitOps.TagAt(name, ref, dir) }

// gitRenameTag creates newName at the same commit as oldName, then
// deletes oldName. Returns an error if the new tag cannot be created.
func gitRenameTag(oldName, newName, dir string) error {
	return defaultGitOps.RenameTag(oldName, newName, dir)
}

func gitRevParseHEAD(dir string) (string, error) { return defaultGitOps.RevParseHEAD(dir) }

func gitMergeCmd(branch, dir string) *exec.Cmd { return defaultGitOps.MergeCmd(branch, dir) }

// gitWorktreeAdd returns a Cmd that adds a worktree at worktreeDir on branch.
// dir is the repository root used as cmd.Dir (empty means process CWD).
func gitWorktreeAdd(worktreeDir, branch, dir string) *exec.Cmd {
	return defaultGitOps.WorktreeAdd(worktreeDir, branch, dir)
}

// gitWorktreeRemove removes the worktree at worktreeDir.
// dir is the repository root used as cmd.Dir (empty means process CWD).
func gitWorktreeRemove(worktreeDir, dir string) error {
	return defaultGitOps.WorktreeRemove(worktreeDir, dir)
}

func gitCurrentBranch(dir string) (string, error) { return defaultGitOps.CurrentBranch(dir) }

// gitLsTreeFiles returns the list of file paths tracked at the given ref.
func gitLsTreeFiles(ref, dir string) ([]string, error) {
	return defaultGitOps.LsTreeFiles(ref, dir)
}

// gitShowFileContent returns the raw content of a file at the given ref.
func gitShowFileContent(ref, path, dir string) ([]byte, error) {
	return defaultGitOps.ShowFileContent(ref, path, dir)
}

// FileChange is defined in internal/claude and aliased in cobbler.go.

// diffStat holds parsed output from git diff --shortstat.
type diffStat = gitops.DiffStat

// gitDiffShortstat runs git diff --shortstat against the given ref and
// parses the output (e.g. "5 files changed, 100 insertions(+), 20 deletions(-)").
func gitDiffShortstat(ref, dir string) (diffStat, error) {
	return defaultGitOps.DiffShortstat(ref, dir)
}

// gitDiffNameStatus runs git diff --name-status and --numstat against the
// given ref and returns per-file entries with path, status, insertions, and
// deletions. The two commands are combined to produce complete file-level
// change records.
func gitDiffNameStatus(ref, dir string) ([]FileChange, error) {
	gfc, err := defaultGitOps.DiffNameStatus(ref, dir)
	if err != nil {
		return nil, err
	}
	files := make([]FileChange, len(gfc))
	for i, fc := range gfc {
		files[i] = FileChange{
			Path:       fc.Path,
			Status:     fc.Status,
			Insertions: fc.Insertions,
			Deletions:  fc.Deletions,
		}
	}
	return files, nil
}

// parseBranchList parses the output of git branch --list or git tag --list.
func parseBranchList(output string) []string {
	return gitops.ParseBranchList(output)
}

// parseDiffShortstat extracts file/insertion/deletion counts from
// git diff --shortstat output.
func parseDiffShortstat(s string) diffStat {
	return gitops.ParseDiffShortstat(s)
}

// parseNumstat parses git diff --numstat output into a map keyed by file path.
func parseNumstat(output string) map[string]gitops.NumstatEntry {
	return gitops.ParseNumstat(output)
}

// parseNameStatus parses git diff --name-status output and merges it with
// numstat data to produce FileChange entries.
func parseNameStatus(output string, numMap map[string]gitops.NumstatEntry) []gitops.FileChange {
	return gitops.ParseNameStatus(output, numMap)
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
