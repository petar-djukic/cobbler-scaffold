//go:build usecase

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package testutil provides shared helpers for E2E use-case tests.
// It lives under internal/ so only test packages within tests/rel01.0/
// can import it.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// CountReadyIssues calls bd ready in dir and returns the number of available
// tasks.
func CountReadyIssues(t testing.TB, dir string) int {
	t.Helper()
	cmd := exec.Command("bd", "ready", "--json", "--type", "task")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(out, &tasks); err != nil {
		return 0
	}
	return len(tasks)
}

// CreateIssue creates a beads issue via the bd CLI and returns the issue ID.
func CreateIssue(t testing.TB, dir, title string) string {
	t.Helper()
	cmd := exec.Command("bd", "create", "--json", "--type", "task",
		"--title", title, "--description", "created by e2e test")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd create: %v\n%s", err, out)
	}
	var issue struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &issue); err != nil {
		t.Fatalf("bd create: JSON parse failed: %v\noutput: %s", err, out)
	}
	if issue.ID == "" {
		t.Fatalf("bd create returned empty ID\noutput: %s", out)
	}
	return issue.ID
}

// IssueHasField checks whether any issue listed by "bd list --json" contains
// the given field name in its JSON output.
func IssueHasField(t testing.TB, dir, field string) bool {
	t.Helper()
	cmd := exec.Command("bd", "list", "--json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd list --json: %v\n%s", err, out)
	}
	return strings.Contains(string(out), field)
}

// ContainsField checks whether a JSON string contains the given field value.
func ContainsField(jsonStr, value string) bool {
	return strings.Contains(jsonStr, value)
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

// CountIssuesByStatus calls bd list with the given status and returns the count.
func CountIssuesByStatus(t testing.TB, dir, status string) int {
	t.Helper()
	cmd := exec.Command("bd", "list", "--json", "--status", status, "--type", "task")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Logf("CountIssuesByStatus: bd list --status %s: %v (stdout=%d bytes)", status, err, len(out))
		return 0
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal(out, &tasks); err != nil {
		t.Logf("CountIssuesByStatus: JSON unmarshal: %v (output=%q)", err, string(out))
		return 0
	}
	return len(tasks)
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
