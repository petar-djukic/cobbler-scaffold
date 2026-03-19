// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"os/exec"
	"strings"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops" // diffStat alias, diffNameStatus conversion
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

// diffStat holds parsed output from git diff --shortstat.
type diffStat = gitops.DiffStat

// diffNameStatus runs git diff --name-status and returns per-file entries,
// converting from gitops.FileChange to claude.FileChange (aliased as
// FileChange in cobbler.go).
func diffNameStatus(ref, dir string) ([]claude.FileChange, error) {
	gfc, err := defaultGitOps.DiffNameStatus(ref, dir)
	if err != nil {
		return nil, err
	}
	files := make([]claude.FileChange, len(gfc))
	for i, fc := range gfc {
		files[i] = claude.FileChange{
			Path:       fc.Path,
			Status:     fc.Status,
			Insertions: fc.Insertions,
			Deletions:  fc.Deletions,
		}
	}
	return files, nil
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
