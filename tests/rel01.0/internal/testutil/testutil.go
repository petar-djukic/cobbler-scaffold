//go:build usecase || benchmark

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package testutil provides shared helpers for E2E use-case tests.
// It lives under internal/ so only test packages within tests/rel01.0/
// can import it.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"gopkg.in/yaml.v3"
)

// ClaudeTestTimeout is the per-invocation timeout for mage targets that call
// Claude. Individual Claude calls should complete well within this limit;
// if they don't, the test fails fast rather than burning the full 30-minute
// package timeout.
const ClaudeTestTimeout = 5 * time.Minute

// SetupRepo copies the global snapshot to a fresh temp directory, initialises
// a new git repo inside it, and registers t.Cleanup to remove the directory.
// Each test gets an isolated, fully-scaffolded repo in a few seconds.
func SetupRepo(t testing.TB, snapshotDir string) string {
	t.Helper()
	// Use workDir directly as testDir so filepath.Base(testDir) is unique
	// per test (e.g. "e2e-test-123456"). All tests previously nested the
	// repo at workDir/repo, making filepath.Base always "repo" and causing
	// every test to share /tmp/repo-worktrees/ as the worktree base directory.
	// Parallel tests racing on that shared directory caused stale worktree
	// registrations that made git checkout main fail in generator:stop.
	testDir, err := os.MkdirTemp("", "e2e-test-*")
	if err != nil {
		t.Fatalf("SetupRepo: MkdirTemp: %v", err)
	}

	if err := CopyDir(snapshotDir, testDir); err != nil {
		os.RemoveAll(testDir)
		t.Fatalf("SetupRepo: copy snapshot: %v", err)
	}

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "e2e@test.local"},
		{"git", "config", "user.name", "E2E Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "config", "tag.gpgsign", "false"},
		{"git", "config", "gc.auto", "0"},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "Initial scaffold"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = testDir
		if out, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(testDir)
			t.Fatalf("SetupRepo: git %v: %v\n%s", args[1:], err, out)
		}
	}

	// Also clean up directories that the orchestrator creates alongside the repo:
	// - /tmp/e2e-test-123456-worktrees/ (stitch worktree base)
	// - generation worktree recorded in .cobbler/generation-worktree
	worktreeBase := filepath.Join(os.TempDir(), filepath.Base(testDir)+"-worktrees")
	t.Cleanup(func() {
		// Remove generation worktree if GeneratorStart created one.
		// Parse `git worktree list --porcelain` to find it.
		if out, err := exec.Command("git", "-C", testDir, "worktree", "list", "--porcelain").Output(); err == nil {
			var wtPath string
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "worktree ") {
					wtPath = strings.TrimPrefix(line, "worktree ")
				}
				if strings.HasPrefix(line, "branch refs/heads/generation-") && wtPath != "" {
					exec.Command("git", "-C", testDir, "worktree", "remove", "--force", wtPath).Run() //nolint:errcheck
					os.RemoveAll(wtPath)
				}
			}
		}
		os.RemoveAll(testDir)
		os.RemoveAll(worktreeBase)
	})
	return testDir
}

// RunMageEnv runs a mage target in dir with extra environment variables.
func RunMageEnv(t testing.TB, dir string, env []string, target ...string) error {
	t.Helper()
	_, err := RunMageOutEnv(t, dir, env, target...)
	return err
}

// RunMageOutEnv runs a mage target in dir with extra environment variables
// and returns combined stdout+stderr.
func RunMageOutEnv(t testing.TB, dir string, env []string, target ...string) (string, error) {
	t.Helper()
	args := append([]string{"-d", "."}, target...)
	cmd := exec.CommandContext(context.Background(), "mage", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)

	tag := "[" + t.Name() + "] "
	var buf bytes.Buffer
	pw := &prefixWriter{tag: tag, w: os.Stderr}
	cmd.Stdout = io.MultiWriter(pw, &buf)
	cmd.Stderr = io.MultiWriter(pw, &buf)

	err := cmd.Run()
	return buf.String(), err
}

// GeneratorStart runs mage generator:start with a unique generation name
// derived from the test name to avoid worktree path collisions between
// parallel tests. Returns the worktree directory path by parsing
// `git worktree list` output.
func GeneratorStart(t testing.TB, dir string) string {
	t.Helper()
	// Use a sanitized test name as the generation name to ensure
	// uniqueness across parallel tests.
	genName := sanitizeGenName(t.Name())
	env := []string{"COBBLER_GEN_NAME=" + genName}
	if err := RunMageEnv(t, dir, env, "generator:start"); err != nil {
		t.Fatalf("generator:start: %v", err)
	}
	return FindGenerationWorktree(t, dir)
}

// FindGenerationWorktree finds the generation worktree path by parsing
// `git worktree list --porcelain`. Returns the worktree path on a
// generation branch.
func FindGenerationWorktree(t testing.TB, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("FindGenerationWorktree: git worktree list: %v", err)
	}
	var currentPath string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch refs/heads/generation-") && currentPath != "" {
			return currentPath
		}
	}
	t.Fatalf("FindGenerationWorktree: no generation worktree found in:\n%s", string(out))
	return ""
}

// ReadWorktreeDir reads the generation worktree path via git worktree list.
// Alias for FindGenerationWorktree for backward compatibility.
func ReadWorktreeDir(t testing.TB, dir string) string {
	t.Helper()
	return FindGenerationWorktree(t, dir)
}

// sanitizeGenName creates a short, filesystem-safe generation name from
// a test name by replacing path separators and special chars.
func sanitizeGenName(testName string) string {
	// Replace slashes and non-alphanumeric chars with hyphens.
	var b strings.Builder
	for _, r := range testName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	name := strings.ToLower(b.String())
	// Truncate to avoid overly long paths.
	if len(name) > 40 {
		name = name[:40]
	}
	return name
}

// RunMage runs a mage target in dir and returns an error on non-zero exit.
func RunMage(t testing.TB, dir string, target ...string) error {
	t.Helper()
	_, err := RunMageOut(t, dir, target...)
	return err
}

// RunMageOut runs a mage target in dir and returns combined stdout+stderr.
// Output is streamed to os.Stderr in real-time (visible with go test -v)
// so that long-running Claude invocations show progress. Each line is
// prefixed with the test name so parallel output is attributable.
func RunMageOut(t testing.TB, dir string, target ...string) (string, error) {
	t.Helper()
	return RunMageOutCtx(context.Background(), t, dir, target...)
}

// RunMageOutTimeout is like RunMageOut but cancels the process after timeout.
// Use this for Claude-calling targets (cobbler:measure, cobbler:stitch,
// generator:run) so a hung API call fails fast instead of consuming the
// full package timeout.
func RunMageOutTimeout(t testing.TB, dir string, timeout time.Duration, target ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return RunMageOutCtx(ctx, t, dir, target...)
}

// RunMageOutCtx runs a mage target with a context for cancellation/timeout.
func RunMageOutCtx(ctx context.Context, t testing.TB, dir string, target ...string) (string, error) {
	t.Helper()
	args := append([]string{"-d", "."}, target...)
	cmd := exec.CommandContext(ctx, "mage", args...)
	cmd.Dir = dir

	tag := "[" + t.Name() + "] "
	var buf bytes.Buffer
	pw := &prefixWriter{tag: tag, w: os.Stderr}
	cmd.Stdout = io.MultiWriter(pw, &buf)
	cmd.Stderr = io.MultiWriter(pw, &buf)

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return buf.String(), fmt.Errorf("mage %s timed out after %v: %w", strings.Join(target, " "), ctx.Err(), err)
	}
	return buf.String(), err
}

// RunMageTimeout runs a mage target with a timeout. Returns error on failure.
func RunMageTimeout(t testing.TB, dir string, timeout time.Duration, target ...string) error {
	t.Helper()
	_, err := RunMageOutTimeout(t, dir, timeout, target...)
	return err
}

// prefixWriter wraps an io.Writer and inserts a test-name tag into each
// line of output. If the line starts with a bracketed timestamp (the
// orchestrator's log format), the tag is inserted after the timestamp:
//
//	[2026-02-23T08:22:35-05:00] [TestName] message
//
// Otherwise the tag is prepended to the line.
type prefixWriter struct {
	tag string
	w   io.Writer
}

func (pw *prefixWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		var line []byte
		if idx < 0 {
			line = p
			p = nil
		} else {
			line = p[:idx+1]
			p = p[idx+1:]
		}
		// Insert tag after first "] " if the line starts with '[' (timestamp).
		if len(line) > 0 && line[0] == '[' {
			if pos := bytes.Index(line, []byte("] ")); pos >= 0 {
				if _, err := pw.w.Write(line[:pos+2]); err != nil {
					return n, err
				}
				if _, err := io.WriteString(pw.w, pw.tag); err != nil {
					return n, err
				}
				if _, err := pw.w.Write(line[pos+2:]); err != nil {
					return n, err
				}
				continue
			}
		}
		// No timestamp — prepend the tag.
		if _, err := io.WriteString(pw.w, pw.tag); err != nil {
			return n, err
		}
		if _, err := pw.w.Write(line); err != nil {
			return n, err
		}
	}
	return n, nil
}

// FileExists returns true if the path relative to dir exists on disk.
func FileExists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, rel))
	return err == nil
}

// GitBranch returns the current branch name in dir.
func GitBranch(t testing.TB, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("GitBranch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// GitHead returns the full SHA of HEAD in dir.
func GitHead(t testing.TB, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("GitHead: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// GitTagExists returns true if the named tag exists in the repo at dir.
func GitTagExists(t testing.TB, dir, tag string) bool {
	t.Helper()
	cmd := exec.Command("git", "tag", "-l", tag)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("GitTagExists(%q): %v", tag, err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// GitListBranchesMatching returns branches in dir whose names contain substr.
func GitListBranchesMatching(t testing.TB, dir, substr string) []string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", "*"+substr+"*")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("GitListBranchesMatching(%q): %v", substr, err)
	}
	var branches []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// readIssuesRepo reads cobbler.issues_repo from configuration.yaml in dir.
// Returns empty string and logs a warning if the file cannot be read.
func readIssuesRepo(t testing.TB, dir string) string {
	t.Helper()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Logf("readIssuesRepo: read %s: %v", cfgPath, err)
		return ""
	}
	var cfg struct {
		Cobbler struct {
			IssuesRepo string `yaml:"issues_repo"`
		} `yaml:"cobbler"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Logf("readIssuesRepo: unmarshal: %v", err)
		return ""
	}
	return cfg.Cobbler.IssuesRepo
}

// CountReadyIssues returns the number of open cobbler issues that have the
// cobbler-ready label and the generation label for the current branch in dir.
//
// Implementation: list issues by generation label via the REST API
// (gh api repos/.../issues --method GET, strongly consistent), then check
// cobbler-ready on each issue via gh issue view (also REST, strongly consistent).
// Both steps avoid GitHub's search API, which is eventually consistent and
// can return stale results immediately after label changes.
func CountReadyIssues(t testing.TB, dir string) int {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		return 0
	}
	generation := GitBranch(t, dir)
	genLabel := orchestrator.CobblerGenLabel(generation)
	cmd := exec.Command("gh", "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=open",
		"-f", "labels="+genLabel,
		"-f", "per_page=100",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("CountReadyIssues: gh api repos issues: %v", err)
		return 0
	}
	var issues []struct{ Number int `json:"number"` }
	if err := json.Unmarshal(out, &issues); err != nil {
		t.Logf("CountReadyIssues: parse: %v (output=%q)", err, string(out))
		return 0
	}
	count := 0
	for _, iss := range issues {
		if IssueHasLabel(t, dir, strconv.Itoa(iss.Number), "cobbler-ready") {
			count++
		}
	}
	return count
}

// WaitForReadyIssues polls CountReadyIssues until at least min issues are
// ready or timeout elapses. Returns the final count. On each iteration,
// adds cobbler-ready to any open issue that lacks it and has no unresolved
// dependencies (GH-1682). This compensates for the GitHub API eventual-
// consistency lag where PromoteReadyIssues during import may not see
// newly created issues.
func WaitForReadyIssues(t testing.TB, dir string, min int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		promoteOpenIssues(t, dir)
		n := CountReadyIssues(t, dir)
		if n >= min {
			return n
		}
		if time.Now().After(deadline) {
			t.Logf("WaitForReadyIssues: timed out after %v with %d/%d ready", timeout, n, min)
			return n
		}
		time.Sleep(2 * time.Second)
	}
}

// promoteOpenIssues adds cobbler-ready to open generation issues that lack
// both cobbler-ready and cobbler-in-progress labels. This is a test-only
// helper that compensates for GitHub API eventual consistency — the
// orchestrator's PromoteReadyIssues may have run before the issue was
// visible in the API listing.
func promoteOpenIssues(t testing.TB, dir string) {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		return
	}
	generation := GitBranch(t, dir)
	genLabel := orchestrator.CobblerGenLabel(generation)
	out, err := exec.Command("gh", "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=open",
		"-f", "labels="+genLabel,
		"-f", "per_page=100",
	).Output()
	if err != nil {
		return
	}
	var issues []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return
	}
	for _, iss := range issues {
		num := strconv.Itoa(iss.Number)
		if !IssueHasLabel(t, dir, num, "cobbler-ready") && !IssueHasLabel(t, dir, num, "cobbler-in-progress") {
			_ = exec.Command("gh", "issue", "edit", "--repo", repo, num, "--add-label", "cobbler-ready").Run()
		}
	}
}

// ensureGitHubLabel creates label on repo if it does not already exist.
// A 422 response (label exists) is silently ignored.
func ensureGitHubLabel(repo, name, color, description string) {
	exec.Command("gh", "api", "repos/"+repo+"/labels", //nolint:errcheck
		"--method", "POST",
		"--field", "name="+name,
		"--field", "color="+color,
		"--field", "description="+description,
	).Run()
}

// CreateIssue creates a GitHub issue with cobbler labels for the current
// generation in dir. Returns the issue number as a string.
func CreateIssue(t testing.TB, dir, title string) string {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		t.Fatalf("CreateIssue: no issues_repo in configuration.yaml")
	}
	generation := GitBranch(t, dir)

	// Use GenLabel to get the label-safe generation name (handles 50-char GitHub limit).
	genLabel := orchestrator.CobblerGenLabel(generation)

	// Ensure all required labels exist before creating the issue.
	ensureGitHubLabel(repo, "cobbler-ready", "0075ca", "Cobbler task ready to be picked by stitch")
	ensureGitHubLabel(repo, "cobbler-in-progress", "e4e669", "Cobbler task currently being worked on")
	ensureGitHubLabel(repo, genLabel, "ededed", "Cobbler generation "+generation)

	body := fmt.Sprintf("---\ncobbler_generation: %s\ncobbler_index: 0\n---\n\ncreated by e2e test",
		generation)
	cmd := exec.Command("gh", "issue", "create",
		"--repo", repo,
		"--title", title,
		"--label", genLabel,
		"--label", "cobbler-ready",
		"--body", body)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CreateIssue: gh issue create: %v\n%s", err, out)
	}
	// Output is a URL like https://github.com/owner/repo/issues/123
	url := strings.TrimSpace(string(out))
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		t.Fatalf("CreateIssue: unexpected output %q", url)
	}
	num := parts[len(parts)-1]
	if _, err := strconv.Atoi(num); err != nil {
		t.Fatalf("CreateIssue: could not parse issue number from %q: %v", url, err)
	}
	// Close the issue when the test ends so it does not leak into other test
	// runs that happen to share the same generation label (same second).
	t.Cleanup(func() {
		exec.Command("gh", "issue", "close", num, "--repo", repo).Run() //nolint:errcheck
	})
	return num
}

// SetIssueInProgress adds the cobbler-in-progress label to the issue and
// removes cobbler-ready. issueNumber is the string form of the GitHub issue
// number returned by CreateIssue.
func SetIssueInProgress(t testing.TB, dir, issueNumber string) {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		t.Fatalf("SetIssueInProgress: no issues_repo in configuration.yaml")
	}
	n, err := strconv.Atoi(issueNumber)
	if err != nil {
		t.Fatalf("SetIssueInProgress: invalid issue number %q: %v", issueNumber, err)
	}
	add := exec.Command("gh", "issue", "edit", strconv.Itoa(n),
		"--repo", repo, "--add-label", "cobbler-in-progress")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("SetIssueInProgress: add label: %v\n%s", err, out)
	}
	rm := exec.Command("gh", "issue", "edit", strconv.Itoa(n),
		"--repo", repo, "--remove-label", "cobbler-ready")
	if out, err := rm.CombinedOutput(); err != nil {
		t.Logf("SetIssueInProgress: remove cobbler-ready (non-fatal): %v\n%s", err, out)
	}
}

// IssueHasLabel returns true if the GitHub issue identified by issueNumber
// currently has the given label. It fetches the issue directly via
// gh issue view (REST endpoint) which is strongly consistent, avoiding the
// eventual-consistency lag of gh issue list.
func IssueHasLabel(t testing.TB, dir, issueNumber, label string) bool {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		t.Logf("IssueHasLabel: no issues_repo in configuration.yaml")
		return false
	}
	cmd := exec.Command("gh", "issue", "view", issueNumber,
		"--repo", repo, "--json", "labels")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("IssueHasLabel: gh issue view %s: %v", issueNumber, err)
		return false
	}
	var resp struct {
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Logf("IssueHasLabel: parse: %v (output=%q)", err, string(out))
		return false
	}
	for _, l := range resp.Labels {
		if l.Name == label {
			return true
		}
	}
	return false
}

// SetupClaude extracts Claude credentials into the test repo and configures
// the idle timeout in configuration.yaml. It also registers a cleanup that
// closes any GitHub issues created for this test's generation so that
// stale issues do not accumulate in the repository.
func SetupClaude(t testing.TB, dir string) {
	t.Helper()
	if err := RunMage(t, dir, "credentials"); err != nil {
		t.Fatalf("SetupClaude: mage credentials: %v", err)
	}
	WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		// Use a 120s idle timeout so back-to-back Claude sessions are not killed
		// by rate limiting that causes the API to delay responding for >60s.
		cfg.Cobbler.IdleTimeoutSeconds = 120
	})
	t.Cleanup(func() {
		closeTestGenerationIssues(t, dir)
	})
}

// closeTestGenerationIssues closes all open cobbler GitHub issues for the
// test repo's current generation (branch). Called as a t.Cleanup to prevent
// stale issues from accumulating in the issues repository after each test run.
func closeTestGenerationIssues(t testing.TB, dir string) {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		return
	}
	branch := GitBranch(t, dir)
	if branch == "" {
		return
	}

	seen := map[int]bool{}

	// Close labelled cobbler issues (normal task issues created by measure/stitch).
	label := "cobbler-gen-" + branch
	if out, err := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--label", label,
		"--state", "open",
		"--json", "number",
		"--limit", "200",
	).Output(); err != nil {
		t.Logf("closeTestGenerationIssues: list labelled: %v", err)
	} else {
		var issues []struct{ Number int `json:"number"` }
		if err := json.Unmarshal(out, &issues); err == nil {
			for _, iss := range issues {
				seen[iss.Number] = true
			}
		}
	}

	// Also close unlabelled [measuring] placeholders for this generation.
	// These are created without cobbler labels (so stitch ignores them) and
	// are missed by the label-based query above when Claude fails mid-run.
	placeholderPrefix := fmt.Sprintf("[measuring] %s task", branch)
	if out, err := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--state", "open",
		"--search", placeholderPrefix+" in:title",
		"--json", "number,title",
		"--limit", "200",
	).Output(); err != nil {
		t.Logf("closeTestGenerationIssues: list placeholders: %v", err)
	} else {
		var issues []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		}
		if err := json.Unmarshal(out, &issues); err == nil {
			for _, iss := range issues {
				if strings.HasPrefix(iss.Title, placeholderPrefix) {
					seen[iss.Number] = true
				}
			}
		}
	}

	closed := 0
	for num := range seen {
		if err := exec.Command("gh", "issue", "close",
			"--repo", repo,
			fmt.Sprintf("%d", num),
		).Run(); err != nil {
			t.Logf("closeTestGenerationIssues: close #%d: %v", num, err)
		} else {
			closed++
		}
	}
	if closed > 0 {
		t.Logf("closeTestGenerationIssues: closed %d issue(s) for %s on %s", closed, repo, branch)
	}
}

// WriteConfigOverride reads configuration.yaml in dir, applies modify, and
// writes the result back.
func WriteConfigOverride(t testing.TB, dir string, modify func(*orchestrator.Config)) {
	t.Helper()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("WriteConfigOverride: read: %v", err)
	}
	var cfg orchestrator.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("WriteConfigOverride: unmarshal: %v", err)
	}
	modify(&cfg)
	newData, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("WriteConfigOverride: marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, newData, 0o644); err != nil {
		t.Fatalf("WriteConfigOverride: write: %v", err)
	}
}

// HistoryStatsFiles returns all *-{phase}-stats.yaml files under .cobbler/history/ in dir.
func HistoryStatsFiles(t testing.TB, dir, phase string) []string {
	t.Helper()
	pattern := filepath.Join(dir, ".cobbler", "history", "*-"+phase+"-stats.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("HistoryStatsFiles: glob: %v", err)
	}
	return matches
}

// HistoryReportFiles returns all *-{phase}-report.yaml files under .cobbler/history/ in dir.
func HistoryReportFiles(t testing.TB, dir, phase string) []string {
	t.Helper()
	pattern := filepath.Join(dir, ".cobbler", "history", "*-"+phase+"-report.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("HistoryReportFiles: glob: %v", err)
	}
	return matches
}

// ReadFileContains returns true if the file at path contains substr.
func ReadFileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}

// CountIssuesByStatus queries GitHub for open cobbler issues with the given
// status label ("ready" or "in_progress") for the current generation in dir.
//
// Implementation: list issues by generation label via the REST API
// (gh api repos/.../issues --method GET, strongly consistent), then check the
// status label on each issue via gh issue view (also REST, strongly consistent).
// This avoids GitHub's search API, which is eventually consistent and would
// miss recently-applied labels (e.g. cobbler-in-progress added by gh issue edit).
func CountIssuesByStatus(t testing.TB, dir, status string) int {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		return 0
	}
	generation := GitBranch(t, dir)
	statusLabel := "cobbler-" + strings.ReplaceAll(status, "_", "-")
	genLabel := orchestrator.CobblerGenLabel(generation)
	cmd := exec.Command("gh", "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=open",
		"-f", "labels="+genLabel,
		"-f", "per_page=100",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("CountIssuesByStatus: gh api repos issues --label %s: %v", statusLabel, err)
		return 0
	}
	var issues []struct{ Number int `json:"number"` }
	if err := json.Unmarshal(out, &issues); err != nil {
		t.Logf("CountIssuesByStatus: parse: %v (output=%q)", err, string(out))
		return 0
	}
	count := 0
	for _, iss := range issues {
		if IssueHasLabel(t, dir, strconv.Itoa(iss.Number), statusLabel) {
			count++
		}
	}
	return count
}

// CopyDir copies src to dst recursively.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return CopyFile(path, target)
	})
}

// CopyDirSkipGit copies src to dst recursively, skipping the .git directory.
func CopyDirSkipGit(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return CopyFile(path, target)
	})
}

// CopyFile copies a single file from src to dst.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// MarkAllRequirementsComplete reads .cobbler/requirements.yaml (generating it
// first via generator:start if absent) and overwrites every R-item status to
// "complete". This creates a deterministic precondition for tests that expect
// measure to return zero tasks (GH-1798).
func MarkAllRequirementsComplete(t testing.TB, dir string) {
	t.Helper()
	reqFile := filepath.Join(dir, ".cobbler", "requirements.yaml")

	data, err := os.ReadFile(reqFile)
	if err != nil {
		t.Fatalf("MarkAllRequirementsComplete: read %s: %v", reqFile, err)
	}

	type reqState struct {
		Status string `yaml:"status"`
		Issue  int    `yaml:"issue,omitempty"`
	}
	type reqFile_ struct {
		Requirements map[string]map[string]reqState `yaml:"requirements"`
	}
	var rf reqFile_
	if err := yaml.Unmarshal(data, &rf); err != nil {
		t.Fatalf("MarkAllRequirementsComplete: parse: %v", err)
	}
	for prd, items := range rf.Requirements {
		for id, st := range items {
			st.Status = "complete"
			items[id] = st
		}
		rf.Requirements[prd] = items
	}
	out, err := yaml.Marshal(&rf)
	if err != nil {
		t.Fatalf("MarkAllRequirementsComplete: marshal: %v", err)
	}
	if err := os.WriteFile(reqFile, out, 0o644); err != nil {
		t.Fatalf("MarkAllRequirementsComplete: write: %v", err)
	}
	// Commit so measure sees the change.
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "Mark all requirements complete for test")
	cmd.Dir = dir
	cmd.Run()
}

// HasUnresolvedRequirements returns true if .cobbler/requirements.yaml contains
// any R-item with status "ready". Returns false if the file does not exist.
func HasUnresolvedRequirements(t testing.TB, dir string) bool {
	t.Helper()
	reqFile := filepath.Join(dir, ".cobbler", "requirements.yaml")
	data, err := os.ReadFile(reqFile)
	if err != nil {
		return false
	}
	type reqState struct {
		Status string `yaml:"status"`
	}
	type reqFile_ struct {
		Requirements map[string]map[string]reqState `yaml:"requirements"`
	}
	var rf reqFile_
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return false
	}
	for _, items := range rf.Requirements {
		for _, st := range items {
			if st.Status == "ready" {
				return true
			}
		}
	}
	return false
}

// MeasureAndExpectIssues runs cobbler:measure and waits for at least one ready
// issue. If measure returns zero issues despite unresolved requirements, it
// retries once (Claude non-determinism). Returns the issue count. Fatals if
// both attempts return zero (GH-1798).
func MeasureAndExpectIssues(t testing.TB, dir string, timeout time.Duration) int {
	t.Helper()
	if !HasUnresolvedRequirements(t, dir) {
		t.Fatal("MeasureAndExpectIssues: precondition failed — no unresolved requirements in requirements.yaml")
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := RunMageTimeout(t, dir, ClaudeTestTimeout, "cobbler:measure"); err != nil {
			t.Fatalf("cobbler:measure (attempt %d): %v", attempt, err)
		}
		n := WaitForReadyIssues(t, dir, 1, timeout)
		if n > 0 {
			return n
		}
		if attempt == 1 {
			t.Logf("MeasureAndExpectIssues: Claude returned empty list despite unresolved requirements, retrying (attempt 2)")
		}
	}
	t.Fatal("MeasureAndExpectIssues: Claude returned empty list on both attempts despite unresolved requirements")
	return 0
}
