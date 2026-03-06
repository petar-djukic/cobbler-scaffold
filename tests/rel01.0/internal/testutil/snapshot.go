//go:build usecase || benchmark

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"gopkg.in/yaml.v3"
)

// ScaffoldModule is the Go module used as the E2E test target.
const ScaffoldModule = "github.com/petar-djukic/sdd-hello-world"

// FindOrchestratorRoot returns the absolute path to the orchestrator
// repository root. It uses `go env GOMOD` to find the module root reliably,
// independent of working directory depth.
func FindOrchestratorRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", fmt.Errorf("go env GOMOD returned empty or /dev/null; not inside a Go module")
	}
	return filepath.Dir(gomod), nil
}

// PrepareSnapshot runs PrepareTestRepo once, copies the working tree (minus
// .git) to a temp directory, and returns that directory plus a cleanup func.
func PrepareSnapshot(orchRoot string) (string, func(), error) {
	cfg, err := orchestrator.LoadConfig(filepath.Join(orchRoot, "configuration.yaml"))
	if err != nil {
		return "", nil, fmt.Errorf("load config: %w", err)
	}
	orch := orchestrator.New(cfg)

	version, err := latestModuleVersion(ScaffoldModule, cfg.Cobbler.DocTagPrefix)
	if err != nil {
		return "", nil, fmt.Errorf("resolving latest version of %s: %w", ScaffoldModule, err)
	}
	fmt.Fprintf(os.Stderr, "e2e: using %s@%s\n", ScaffoldModule, version)

	repoDir, err := orch.PrepareTestRepo(ScaffoldModule, version, orchRoot)
	if err != nil {
		return "", nil, fmt.Errorf("PrepareTestRepo: %w", err)
	}
	workDir := filepath.Dir(repoDir)

	snap, err := os.MkdirTemp("", "e2e-snapshot-*")
	if err != nil {
		os.RemoveAll(workDir)
		return "", nil, fmt.Errorf("creating snapshot dir: %w", err)
	}
	if err := CopyDirSkipGit(repoDir, snap); err != nil {
		os.RemoveAll(workDir)
		os.RemoveAll(snap)
		return "", nil, fmt.Errorf("copying snapshot: %w", err)
	}
	os.RemoveAll(workDir)

	// Set cobbler.issues_repo so test repos use petar-djukic/cobbler-scaffold
	// for GitHub issue tracking. Issues created during usecase tests land here.
	if err := overrideSnapshotIssuesRepo(snap, "petar-djukic/cobbler-scaffold"); err != nil {
		os.RemoveAll(snap)
		return "", nil, fmt.Errorf("setting issues_repo in snapshot config: %w", err)
	}

	cleanup := func() { os.RemoveAll(snap) }
	return snap, cleanup, nil
}

// overrideSnapshotIssuesRepo writes cobbler.issues_repo into the snapshot's
// configuration.yaml so that all test repos created from it point to the
// correct GitHub repo for issue tracking. All other fields, including
// cobbler.mode, are preserved from the snapshot's configuration.yaml so
// that the execution mode is controlled by sdd-hello-world's config (GH-833).
func overrideSnapshotIssuesRepo(snapDir, issuesRepo string) error {
	cfgPath := filepath.Join(snapDir, orchestrator.DefaultConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	var cfg orchestrator.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	cfg.Cobbler.IssuesRepo = issuesRepo
	newData, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, newData, 0o644)
}

// latestModuleVersion resolves the latest tagged version of a Go module
// whose version string starts with prefix (e.g. "v0." for requirements-only
// releases). Uses `go list -m -versions` and filters to matching versions.
func latestModuleVersion(module, prefix string) (string, error) {
	out, err := exec.Command("go", "list", "-m", "-versions", module).Output()
	if err != nil {
		return "", fmt.Errorf("go list -m -versions %s: %w", module, err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return "", fmt.Errorf("no versions found for %s", module)
	}
	// Filter to versions matching the prefix and pick the last (highest).
	var best string
	for _, v := range parts[1:] {
		if strings.HasPrefix(v, prefix) {
			best = v
		}
	}
	if best == "" {
		return "", fmt.Errorf("no versions matching prefix %q found for %s", prefix, module)
	}
	return best, nil
}
