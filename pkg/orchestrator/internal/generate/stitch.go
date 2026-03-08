// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package generate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrTaskReset is returned by DoOneTask when a task fails but the stitch
// loop should continue to the next task (e.g., Claude failure, worktree
// commit failure, merge failure). The task has been reset to open.
var ErrTaskReset = errors.New("task reset to open")

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// StitchTask holds the state for a single stitch work item.
type StitchTask struct {
	ID          string // cobbler_index as string — used for branch/worktree naming
	Title       string
	Description string
	IssueType   string
	BranchName  string
	WorktreeDir string
	GhNumber    int    // GitHub issue number — used for closing/labelling
	Generation  string // generation label value
	Repo        string // GitHub owner/repo
}

// ---------------------------------------------------------------------------
// Branch naming
// ---------------------------------------------------------------------------

// TaskBranchName returns the git branch name for a stitch task.
// Uses "task/<base>-<id>" instead of "<base>/task/<id>" to avoid
// ref conflicts when the base branch is "main".
func TaskBranchName(baseBranch, issueID string) string {
	return "task/" + baseBranch + "-" + issueID
}

// TaskBranchPattern returns the glob pattern for listing task branches.
func TaskBranchPattern(baseBranch string) string {
	return "task/" + baseBranch + "-*"
}

// ---------------------------------------------------------------------------
// Issue description parsing & validation
// ---------------------------------------------------------------------------

// ParseRequiredReading extracts the required_reading list from a YAML task
// description. Handles both the canonical string format ("- path (reason)")
// and the map format ("- path: foo.go") that Claude sometimes emits.
// Returns nil if the field is absent or unparseable.
func ParseRequiredReading(description string) []string {
	if description == "" {
		return nil
	}

	// Try []string first (canonical format: "- path/to/file.go (reason)").
	var stringParsed struct {
		RequiredReading []string `yaml:"required_reading"`
	}
	if err := yaml.Unmarshal([]byte(description), &stringParsed); err == nil {
		return stringParsed.RequiredReading
	}

	// Fall back to []map format when Claude emits structured entries
	// like {path: "foo.go", reason: "..."}.
	var mapParsed struct {
		RequiredReading []map[string]string `yaml:"required_reading"`
	}
	if err := yaml.Unmarshal([]byte(description), &mapParsed); err != nil {
		Log("parseRequiredReading: YAML parse error: %v", err)
		return nil
	}
	var result []string
	for _, entry := range mapParsed.RequiredReading {
		if p := entry["path"]; p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ScopeSourceDirs narrows GoSourceDirs based on the task description's files
// field (GH-1005). For each configured dir, if the task's files reference a
// sub-directory two levels deep (e.g. "cmd/cat/main.go" under "cmd/"), only
// that sub-directory is included instead of the whole tree. Directories where
// all task files sit directly inside (e.g. "pkg/orchestrator/foo.go" under
// "pkg/") are kept as-is. Returns nil when no scoping is possible.
func ScopeSourceDirs(configDirs []string, description string) []string {
	if description == "" || len(configDirs) == 0 {
		return nil
	}
	var parsed struct {
		Files []string `yaml:"files"`
	}
	if err := yaml.Unmarshal([]byte(description), &parsed); err != nil || len(parsed.Files) == 0 {
		return nil
	}

	// Extract two-level prefixes from task files: "cmd/cat/main.go" → "cmd/cat".
	subDirs := make(map[string]bool)
	for _, f := range parsed.Files {
		f = strings.TrimPrefix(f, "./")
		parts := strings.SplitN(f, "/", 3)
		if len(parts) == 3 {
			subDirs[parts[0]+"/"+parts[1]] = true
		}
	}

	changed := false
	scoped := make([]string, 0, len(configDirs))
	for _, dir := range configDirs {
		clean := strings.TrimRight(strings.TrimPrefix(dir, "./"), "/")
		// Collect sub-directories of this config dir referenced by the task.
		var matches []string
		for sd := range subDirs {
			if strings.HasPrefix(sd, clean+"/") {
				matches = append(matches, sd)
			}
		}
		if len(matches) > 0 {
			sort.Strings(matches)
			scoped = append(scoped, matches...)
			changed = true
		} else {
			scoped = append(scoped, dir)
		}
	}

	if !changed {
		return nil
	}
	return scoped
}

// ValidateIssueDescription checks that a description parses as valid YAML
// and contains the required top-level keys defined by the issue-format
// constitution. Returns an error describing what is missing; callers
// should log a warning but not block on validation failures.
func ValidateIssueDescription(desc string) error {
	if desc == "" {
		return fmt.Errorf("empty description")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(desc), &parsed); err != nil {
		return fmt.Errorf("not valid YAML: %w", err)
	}

	required := []string{"deliverable_type", "required_reading", "files", "requirements", "acceptance_criteria"}
	var missing []string
	for _, key := range required {
		if _, ok := parsed[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stitch git helpers (standalone functions using DI)
// ---------------------------------------------------------------------------

// StitchGitDeps holds git helper functions needed by stitch operations.
type StitchGitDeps struct {
	BranchExists      func(name, dir string) bool
	CreateBranch      func(name, dir string) error
	DeleteBranch      func(name, dir string) error
	ForceDeleteBranch func(name, dir string) error
	ListBranches      func(pattern, dir string) []string
	WorktreeAdd       func(worktreeDir, branch, dir string) *exec.Cmd
	WorktreeRemove    func(worktreeDir, dir string) error
	WorktreePrune     func(dir string) error
	Checkout          func(branch, dir string) error
	CurrentBranch     func(dir string) (string, error)
	MergeCmd          func(branch, dir string) *exec.Cmd
	RevParseHEAD      func(dir string) (string, error)
}

// StitchIssueDeps holds GitHub issue helper functions needed by stitch.
type StitchIssueDeps struct {
	ListOpenCobblerIssues  func(repo, generation string) ([]StitchIssue, error)
	PickReadyIssue         func(repo, generation string) (StitchIssue, error)
	RemoveInProgressLabel  func(repo string, number int) error
	HasLabel               func(issue StitchIssue, label string) bool
	LabelInProgress        string
}

// StitchIssue is the minimal issue representation needed by stitch operations.
type StitchIssue struct {
	Number      int
	Title       string
	Description string
	State       string
	Labels      []string
}

// RecoverStaleBranches removes leftover task branches and worktrees,
// removing the in-progress label from their issues. Returns true if any were recovered.
func RecoverStaleBranches(baseBranch, worktreeBase, repo string, gitDeps StitchGitDeps, issueDeps StitchIssueDeps) bool {
	branches := gitDeps.ListBranches(TaskBranchPattern(baseBranch), ".")
	if len(branches) == 0 {
		Log("recoverStaleBranches: no stale branches found")
		return false
	}

	Log("recoverStaleBranches: found %d stale branch(es): %v", len(branches), branches)
	for _, branch := range branches {
		Log("recoverStaleBranches: recovering %s", branch)

		issueID := strings.TrimPrefix(branch, "task/"+baseBranch+"-")
		worktreeDir := filepath.Join(worktreeBase, issueID)

		if _, err := os.Stat(worktreeDir); err == nil {
			Log("recoverStaleBranches: removing worktree %s", worktreeDir)
			if err := gitDeps.WorktreeRemove(worktreeDir, "."); err != nil {
				Log("recoverStaleBranches: worktree remove warning: %v", err)
			}
		} else {
			Log("recoverStaleBranches: no worktree at %s", worktreeDir)
		}

		Log("recoverStaleBranches: deleting branch %s", branch)
		if err := gitDeps.ForceDeleteBranch(branch, "."); err != nil {
			Log("recoverStaleBranches: branch delete warning: %v", err)
		}

		if issueID != "" {
			var num int
			if _, err := fmt.Sscanf(issueID, "%d", &num); err == nil && num > 0 {
				Log("recoverStaleBranches: removing in-progress label from issue #%d", num)
				if err := issueDeps.RemoveInProgressLabel(repo, num); err != nil {
					Log("recoverStaleBranches: label removal warning: %v", err)
				}
			}
		}
	}
	return true
}

// ResetOrphanedIssues finds in_progress GitHub issues with no corresponding
// task branch and removes their in-progress label. Returns true if any were reset.
func ResetOrphanedIssues(baseBranch, repo, generation string, gitDeps StitchGitDeps, issueDeps StitchIssueDeps) bool {
	issues, err := issueDeps.ListOpenCobblerIssues(repo, generation)
	if err != nil {
		Log("resetOrphanedIssues: list issues failed: %v", err)
		return false
	}

	recovered := false
	for _, iss := range issues {
		if !issueDeps.HasLabel(iss, issueDeps.LabelInProgress) {
			continue
		}
		id := fmt.Sprintf("%d", iss.Number)
		taskBranch := TaskBranchName(baseBranch, id)
		if !gitDeps.BranchExists(taskBranch, ".") {
			recovered = true
			Log("resetOrphanedIssues: orphaned issue #%d (no branch %s), removing in-progress label", iss.Number, taskBranch)
			if err := issueDeps.RemoveInProgressLabel(repo, iss.Number); err != nil {
				Log("resetOrphanedIssues: label removal warning for #%d: %v", iss.Number, err)
			}
		} else {
			Log("resetOrphanedIssues: issue #%d has branch %s, skipping", iss.Number, taskBranch)
		}
	}
	return recovered
}

// PickTask selects the next ready task from GitHub Issues.
func PickTask(baseBranch, worktreeBase, repo, generation string, issueDeps StitchIssueDeps) (StitchTask, error) {
	Log("pickTask: calling pickReadyIssue repo=%s generation=%s", repo, generation)
	iss, err := issueDeps.PickReadyIssue(repo, generation)
	if err != nil {
		Log("pickTask: no tasks available: %v", err)
		return StitchTask{}, fmt.Errorf("no tasks available")
	}

	id := fmt.Sprintf("%d", iss.Number)
	task := StitchTask{
		ID:          id,
		Title:       iss.Title,
		Description: iss.Description,
		IssueType:   "task",
		BranchName:  TaskBranchName(baseBranch, id),
		WorktreeDir: filepath.Join(worktreeBase, id),
		GhNumber:    iss.Number,
		Generation:  generation,
		Repo:        repo,
	}

	// Validate the issue description as YAML with required fields.
	if err := ValidateIssueDescription(task.Description); err != nil {
		Log("pickTask: description validation warning: %v", err)
	}

	Log("pickTask: picked #%d id=%s branch=%s worktree=%s", iss.Number, task.ID, task.BranchName, task.WorktreeDir)
	Log("pickTask: title=%q", task.Title)
	Log("pickTask: descriptionLen=%d", len(task.Description))
	return task, nil
}

// CreateWorktree creates a git worktree for the given task.
func CreateWorktree(task StitchTask, gitDeps StitchGitDeps) error {
	Log("createWorktree: dir=%s branch=%s", task.WorktreeDir, task.BranchName)

	if err := os.MkdirAll(filepath.Dir(task.WorktreeDir), 0o755); err != nil {
		return fmt.Errorf("creating worktree parent directory: %w", err)
	}

	if !gitDeps.BranchExists(task.BranchName, ".") {
		Log("createWorktree: branch %s does not exist, creating", task.BranchName)
		if err := gitDeps.CreateBranch(task.BranchName, "."); err != nil {
			Log("createWorktree: gitCreateBranch failed: %v", err)
			return fmt.Errorf("creating branch %s: %w", task.BranchName, err)
		}
		Log("createWorktree: branch %s created", task.BranchName)
	} else {
		Log("createWorktree: branch %s already exists", task.BranchName)
	}

	Log("createWorktree: adding worktree")
	cmd := gitDeps.WorktreeAdd(task.WorktreeDir, task.BranchName, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		Log("createWorktree: worktree add failed: %v", err)
		return fmt.Errorf("adding worktree: %w", err)
	}

	Log("createWorktree: worktree ready at %s on branch %s", task.WorktreeDir, task.BranchName)
	return nil
}

// CleanGoBinaries removes untracked executable files with no extension from
// dir before staging. Go binaries produced by `go build ./cmd/<name>/` land
// in the working directory as extensionless executables; this prevents them
// from being committed to the generation branch (GH-456).
func CleanGoBinaries(dir string) {
	cmd := exec.Command(BinGit, "ls-files", "--others", "--exclude-standard")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		Log("cleanGoBinaries: git ls-files: %v", err)
		return
	}

	removed := 0
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name == "" || filepath.Ext(name) != "" {
			continue // skip empty lines and files with extensions
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue // skip missing, directories, and non-executables
		}
		if err := os.Remove(path); err != nil {
			Log("cleanGoBinaries: remove %s: %v", name, err)
		} else {
			Log("cleanGoBinaries: removed binary %s", name)
			removed++
		}
	}
	Log("cleanGoBinaries: removed %d binary file(s)", removed)
}

// CommitWorktreeChanges stages and commits all changes Claude made in the
// worktree. Claude does not run git commands; the orchestrator handles git
// externally. Returns nil if there are no changes to commit.
func CommitWorktreeChanges(task StitchTask) error {
	Log("commitWorktreeChanges: staging changes in %s", task.WorktreeDir)

	// Remove compiled Go binaries before staging so they are not committed.
	CleanGoBinaries(task.WorktreeDir)

	addCmd := exec.Command(BinGit, "add", "-A")
	addCmd.Dir = task.WorktreeDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A: %w\n%s", err, out)
	}

	// Check if there are staged changes to commit.
	diffCmd := exec.Command(BinGit, "diff", "--cached", "--quiet")
	diffCmd.Dir = task.WorktreeDir
	if diffCmd.Run() == nil {
		Log("commitWorktreeChanges: no changes to commit for %s", task.ID)
		return nil
	}

	msg := fmt.Sprintf("Task %s: %s", task.ID, task.Title)
	Log("commitWorktreeChanges: committing %q", msg)
	commitCmd := exec.Command(BinGit, "commit", "--no-verify", "-m", msg)
	commitCmd.Dir = task.WorktreeDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}

	Log("commitWorktreeChanges: committed in worktree for %s", task.ID)
	return nil
}

// MergeBranch merges the task branch into the base branch.
func MergeBranch(branchName, baseBranch, repoRoot string, gitDeps StitchGitDeps) error {
	Log("mergeBranch: %s into %s (repoRoot=%s)", branchName, baseBranch, repoRoot)

	Log("mergeBranch: checking out %s", baseBranch)
	if err := gitDeps.Checkout(baseBranch, "."); err != nil {
		Log("mergeBranch: checkout failed: %v", err)
		return fmt.Errorf("checking out %s: %w", baseBranch, err)
	}
	Log("mergeBranch: checked out %s", baseBranch)

	Log("mergeBranch: merging %s", branchName)
	cmd := gitDeps.MergeCmd(branchName, ".")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		Log("mergeBranch: merge failed: %v", err)
		return fmt.Errorf("merging %s: %w", branchName, err)
	}

	Log("mergeBranch: merge successful")
	return nil
}

// CleanupWorktree removes the worktree and its branch. Returns true if the
// worktree was removed successfully, false if removal failed (branch is left
// intact to avoid orphaning the worktree).
func CleanupWorktree(task StitchTask, gitDeps StitchGitDeps) bool {
	Log("cleanupWorktree: removing worktree %s", task.WorktreeDir)
	if err := gitDeps.WorktreeRemove(task.WorktreeDir, "."); err != nil {
		Log("cleanupWorktree: worktree remove failed, skipping branch delete: %v", err)
		return false
	}

	Log("cleanupWorktree: deleting branch %s", task.BranchName)
	if err := gitDeps.DeleteBranch(task.BranchName, "."); err != nil {
		Log("cleanupWorktree: branch delete warning: %v", err)
	}

	Log("cleanupWorktree: done for task %s", task.ID)
	return true
}
