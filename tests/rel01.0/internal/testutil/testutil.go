//go:build usecase || benchmark

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package testutil provides shared helpers for E2E use-case tests.
// It lives under internal/ so only test packages within tests/rel01.0/
// can import it.
package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"gopkg.in/yaml.v3"
)

// ClaudeImage is the container image used for Claude in E2E tests.
const ClaudeImage = "localhost/cobbler-scaffold:latest"

// SetupRepo copies the global snapshot to a fresh temp directory, initialises
// a new git repo inside it, and registers t.Cleanup to remove the directory.
// Each test gets an isolated, fully-scaffolded repo in a few seconds.
func SetupRepo(t testing.TB, snapshotDir string) string {
	t.Helper()
	workDir, err := os.MkdirTemp("", "e2e-test-*")
	if err != nil {
		t.Fatalf("SetupRepo: MkdirTemp: %v", err)
	}
	testDir := filepath.Join(workDir, "repo")

	if err := CopyDir(snapshotDir, testDir); err != nil {
		os.RemoveAll(workDir)
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
			os.RemoveAll(workDir)
			t.Fatalf("SetupRepo: git %v: %v\n%s", args[1:], err, out)
		}
	}

	t.Cleanup(func() { os.RemoveAll(workDir) })
	return testDir
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
	args := append([]string{"-d", "."}, target...)
	cmd := exec.Command("mage", args...)
	cmd.Dir = dir

	tag := "[" + t.Name() + "] "
	var buf bytes.Buffer
	pw := &prefixWriter{tag: tag, w: os.Stderr}
	cmd.Stdout = io.MultiWriter(pw, &buf)
	cmd.Stderr = io.MultiWriter(pw, &buf)

	err := cmd.Run()
	return buf.String(), err
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

// CountReadyIssues queries GitHub for open issues with the cobbler-ready label
// and the generation label for the current branch in dir.
func CountReadyIssues(t testing.TB, dir string) int {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		return 0
	}
	generation := GitBranch(t, dir)
	genLabel := "cobbler-gen-" + generation
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--label", "cobbler-ready",
		"--label", genLabel,
		"--json", "number")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("CountReadyIssues: gh issue list: %v", err)
		return 0
	}
	var issues []struct{ Number int `json:"number"` }
	if err := json.Unmarshal(out, &issues); err != nil {
		t.Logf("CountReadyIssues: parse: %v (output=%q)", err, string(out))
		return 0
	}
	return len(issues)
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

	// Ensure all required labels exist before creating the issue.
	ensureGitHubLabel(repo, "cobbler-ready", "0075ca", "Cobbler task ready to be picked by stitch")
	ensureGitHubLabel(repo, "cobbler-in-progress", "e4e669", "Cobbler task currently being worked on")
	ensureGitHubLabel(repo, "cobbler-gen-"+generation, "ededed", "Cobbler generation "+generation)

	body := fmt.Sprintf("---\ncobbler_generation: %s\ncobbler_index: 0\n---\n\ncreated by e2e test",
		generation)
	cmd := exec.Command("gh", "issue", "create",
		"--repo", repo,
		"--title", title,
		"--label", "cobbler-gen-"+generation,
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
// the podman image in configuration.yaml.
func SetupClaude(t testing.TB, dir string) {
	t.Helper()
	if err := RunMage(t, dir, "credentials"); err != nil {
		t.Fatalf("SetupClaude: mage credentials: %v", err)
	}
	WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Podman.Image = ClaudeImage
	})
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
func CountIssuesByStatus(t testing.TB, dir, status string) int {
	t.Helper()
	repo := readIssuesRepo(t, dir)
	if repo == "" {
		return 0
	}
	generation := GitBranch(t, dir)
	statusLabel := "cobbler-" + strings.ReplaceAll(status, "_", "-")
	genLabel := "cobbler-gen-" + generation
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--label", statusLabel,
		"--label", genLabel,
		"--json", "number")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("CountIssuesByStatus: gh issue list --label %s: %v", statusLabel, err)
		return 0
	}
	var issues []struct{ Number int `json:"number"` }
	if err := json.Unmarshal(out, &issues); err != nil {
		t.Logf("CountIssuesByStatus: parse: %v (output=%q)", err, string(out))
		return 0
	}
	return len(issues)
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
