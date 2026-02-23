//go:build e2e

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package e2e_test contains end-to-end tests that run the orchestrator
// against real target repositories. Tests in this package require:
//   - mage on PATH
//   - bd (beads CLI) on PATH
//   - go on PATH
//   - The orchestrator source at the repo root (two levels up)
package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"gopkg.in/yaml.v3"
)

// scaffoldModule is the Go module used as the E2E test target.
const scaffoldModule = "github.com/petar-djukic/sdd-hello-world"

// orchRoot is the absolute path to the orchestrator repository root.
var orchRoot string

// snapshotDir holds a scaffolded repo tree (without .git) created once in
// TestMain. setupRepo copies it per-test to a fresh temp directory and
// reinitialises git, avoiding the expensive PrepareTestRepo for every test.
var snapshotDir string

func TestMain(m *testing.M) {
	var err error
	orchRoot, err = filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: resolving orchRoot: %v\n", err)
		os.Exit(1)
	}

	// Prepare the test repo once and extract a reusable snapshot.
	snapshot, cleanup, prepErr := prepareSnapshot()
	if prepErr != nil {
		fmt.Fprintf(os.Stderr, "e2e: preparing snapshot: %v\n", prepErr)
		os.Exit(1)
	}
	snapshotDir = snapshot
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

// prepareSnapshot runs PrepareTestRepo once, copies the working tree (minus
// .git) to a temp directory, and returns that directory plus a cleanup func.
func prepareSnapshot() (string, func(), error) {
	cfg, err := orchestrator.LoadConfig(filepath.Join(orchRoot, "configuration.yaml"))
	if err != nil {
		return "", nil, fmt.Errorf("load config: %w", err)
	}
	orch := orchestrator.New(cfg)

	version, err := latestModuleVersion(scaffoldModule)
	if err != nil {
		return "", nil, fmt.Errorf("resolving latest version of %s: %w", scaffoldModule, err)
	}
	fmt.Fprintf(os.Stderr, "e2e: using %s@%s\n", scaffoldModule, version)

	repoDir, err := orch.PrepareTestRepo(
		scaffoldModule,
		version,
		orchRoot,
	)
	if err != nil {
		return "", nil, fmt.Errorf("PrepareTestRepo: %w", err)
	}
	// workDir is the parent temp directory created by PrepareTestRepo.
	workDir := filepath.Dir(repoDir)

	// Copy the working tree without .git to a separate snapshot dir.
	snap, err := os.MkdirTemp("", "e2e-snapshot-*")
	if err != nil {
		os.RemoveAll(workDir)
		return "", nil, fmt.Errorf("creating snapshot dir: %w", err)
	}
	if err := copyDirSkipGit(repoDir, snap); err != nil {
		os.RemoveAll(workDir)
		os.RemoveAll(snap)
		return "", nil, fmt.Errorf("copying snapshot: %w", err)
	}
	// The original PrepareTestRepo work dir is no longer needed.
	os.RemoveAll(workDir)

	cleanup := func() { os.RemoveAll(snap) }
	return snap, cleanup, nil
}

// setupRepo copies the global snapshot to a fresh temp directory, initialises
// a new git repo inside it, and registers t.Cleanup to remove the directory.
// Each test gets an isolated, fully-scaffolded repo in a few seconds.
func setupRepo(t *testing.T) string {
	t.Helper()
	workDir, err := os.MkdirTemp("", "e2e-test-*")
	if err != nil {
		t.Fatalf("setupRepo: MkdirTemp: %v", err)
	}
	testDir := filepath.Join(workDir, "repo")

	if err := copyDir(snapshotDir, testDir); err != nil {
		os.RemoveAll(workDir)
		t.Fatalf("setupRepo: copy snapshot: %v", err)
	}

	// Initialise a fresh git repo so lifecycle commands work.
	// Disable GPG commit signing so git commits in the test repo are fast.
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
			t.Fatalf("setupRepo: git %v: %v\n%s", args[1:], err, out)
		}
	}

	t.Cleanup(func() { os.RemoveAll(workDir) })
	return testDir
}

// runMage runs a mage target in dir and returns an error on non-zero exit.
func runMage(t *testing.T, dir string, target ...string) error {
	t.Helper()
	_, err := runMageOut(t, dir, target...)
	return err
}

// runMageOut runs a mage target in dir and returns combined stdout+stderr.
// Output is streamed to os.Stderr in real-time (visible with go test -v)
// so that long-running Claude invocations show progress.
func runMageOut(t *testing.T, dir string, target ...string) (string, error) {
	t.Helper()
	args := append([]string{"-d", "."}, target...)
	cmd := exec.Command("mage", args...)
	cmd.Dir = dir

	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)

	err := cmd.Run()
	return buf.String(), err
}


// fileExists returns true if the path relative to dir exists on disk.
func fileExists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, rel))
	return err == nil
}

// gitBranch returns the current branch name in dir.
func gitBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("gitBranch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// gitTagExists returns true if the named tag exists in the repo at dir.
func gitTagExists(t *testing.T, dir, tag string) bool {
	t.Helper()
	cmd := exec.Command("git", "tag", "-l", tag)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("gitTagExists(%q): %v", tag, err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// gitListBranchesMatching returns branches in dir whose names contain substr.
func gitListBranchesMatching(t *testing.T, dir, substr string) []string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", "*"+substr+"*")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("gitListBranchesMatching(%q): %v", substr, err)
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

// gitHead returns the full SHA of HEAD in dir.
func gitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("gitHead: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// countReadyIssues calls bd ready in dir and returns the number of available
// tasks. Uses the same command the orchestrator uses (bd ready --json --type
// task) rather than bd list --status ready, since tasks may be in states
// visible to bd ready (pending, ready) but not to bd list --status ready.
func countReadyIssues(t *testing.T, dir string) int {
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

// claudeImage is the container image used for Claude in E2E tests.
const claudeImage = "localhost/cobbler-scaffold:latest"

// setupClaude extracts Claude credentials into the test repo and configures
// the podman image in configuration.yaml. Call this at the start of every
// Claude-gated test before running any mage cobbler or generator targets.
func setupClaude(t *testing.T, dir string) {
	t.Helper()
	if err := runMage(t, dir, "credentials"); err != nil {
		t.Fatalf("setupClaude: mage credentials: %v", err)
	}
	writeConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Podman.Image = claudeImage
	})
}

// writeConfigOverride reads configuration.yaml in dir using raw YAML unmarshal
// (not LoadConfig, to avoid expanding constitution paths to their file content),
// applies modify, and writes the result back.
func writeConfigOverride(t *testing.T, dir string, modify func(*orchestrator.Config)) {
	t.Helper()
	cfgPath := filepath.Join(dir, "configuration.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("writeConfigOverride: read: %v", err)
	}
	var cfg orchestrator.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("writeConfigOverride: unmarshal: %v", err)
	}
	modify(&cfg)
	newData, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("writeConfigOverride: marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, newData, 0o644); err != nil {
		t.Fatalf("writeConfigOverride: write: %v", err)
	}
}

// latestModuleVersion resolves the latest tagged version of a Go module
// using `go list -m -versions`. Returns the last (highest) version.
func latestModuleVersion(module string) (string, error) {
	out, err := exec.Command("go", "list", "-m", "-versions", module).Output()
	if err != nil {
		return "", fmt.Errorf("go list -m -versions %s: %w", module, err)
	}
	// Output format: "module v0.1.0 v0.2.0 v0.3.0"
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return "", fmt.Errorf("no versions found for %s", module)
	}
	return parts[len(parts)-1], nil
}

// copyDirSkipGit copies src to dst recursively, skipping the .git directory.
func copyDirSkipGit(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Skip the .git directory and everything inside it.
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
		return copyFile(path, target)
	})
}

// copyDir copies src to dst recursively.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
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

// issueHasField checks whether any issue listed by "bd list --json" contains
// the given field name in its JSON output.
func issueHasField(t *testing.T, dir, field string) bool {
	t.Helper()
	cmd := exec.Command("bd", "list", "--json")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd list --json: %v\n%s", err, out)
	}
	return strings.Contains(string(out), field)
}

// containsField checks whether a JSON string contains the given field value.
func containsField(jsonStr, value string) bool {
	return strings.Contains(jsonStr, value)
}

// createIssue creates a beads issue via the bd CLI and returns the issue ID.
func createIssue(t *testing.T, dir, title string) string {
	t.Helper()
	cmd := exec.Command("bd", "create", "--type", "task",
		"--title", title, "--description", "created by e2e test")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd create: %v\n%s", err, out)
	}
	// bd create outputs the issue ID as the first word on the first line.
	output := strings.TrimSpace(string(out))
	parts := strings.Fields(output)
	if len(parts) == 0 {
		t.Fatalf("bd create returned empty output")
	}
	return parts[0]
}
