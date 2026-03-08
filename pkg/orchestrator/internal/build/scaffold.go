// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package build

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Package-level variables for scaffold operations.
var (
	BinMage = "mage"
	BinGit  = "git"
)

// OrchestratorModule is the Go module path for this orchestrator library.
const OrchestratorModule = "github.com/mesh-intelligence/cobbler-scaffold"

// GoModDownloadResult holds the fields needed from go mod download -json.
type GoModDownloadResult struct {
	Dir string `json:"Dir"`
}

// ---------------------------------------------------------------------------
// Scaffold helper functions
// ---------------------------------------------------------------------------

// ClearMageGoFiles removes all .go files from mageDir, preserving
// go.mod, go.sum, and non-Go files. If mageDir does not exist, this
// is a no-op.
func ClearMageGoFiles(mageDir string) error {
	entries, err := os.ReadDir(mageDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(mageDir, e.Name())
		Log("scaffold: removing existing %s", path)
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}
	return nil
}

// CopyFile copies src to dst, creating parent directories as needed.
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// DetectModulePath reads go.mod in the target directory and extracts
// the module path from the first "module" directive.
func DetectModulePath(targetDir string) (string, error) {
	modPath := filepath.Join(targetDir, "go.mod")
	f, err := os.Open(modPath)
	if err != nil {
		return "", fmt.Errorf("opening go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading go.mod: %w", err)
	}
	return "", fmt.Errorf("no module directive in %s", modPath)
}

// DetectMainPackage scans cmd/ for directories containing main.go.
// Returns the module-relative import path of the first main package
// found, or empty string if none exist.
func DetectMainPackage(targetDir, modulePath string) string {
	cmdDir := filepath.Join(targetDir, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		mainGo := filepath.Join(cmdDir, e.Name(), "main.go")
		if _, err := os.Stat(mainGo); err == nil {
			return modulePath + "/cmd/" + e.Name()
		}
	}
	// Check for main.go directly in cmd/.
	if _, err := os.Stat(filepath.Join(cmdDir, "main.go")); err == nil {
		return modulePath + "/cmd"
	}
	return ""
}

// DetectSourceDirs returns existing Go source directories in the target.
func DetectSourceDirs(targetDir string) []string {
	candidates := []string{"cmd/", "pkg/", "internal/", "tests/"}
	var found []string
	for _, d := range candidates {
		if _, err := os.Stat(filepath.Join(targetDir, d)); err == nil {
			found = append(found, d)
		}
	}
	return found
}

// DetectBinaryName extracts a binary name from the module path by
// using its last path component.
func DetectBinaryName(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) == 0 {
		return "app"
	}
	return parts[len(parts)-1]
}

// ScaffoldSeedTemplate creates a version.go.tmpl in the magefiles directory
// and returns the destination path (relative to repo root) and the template
// source path (relative to repo root) for use in seed_files configuration.
// dirMagefiles is passed in by the caller.
func ScaffoldSeedTemplate(targetDir, modulePath, mainPkg, dirMagefiles string) (destPath, tmplPath string, err error) {
	relDir := strings.TrimPrefix(mainPkg, modulePath+"/")
	if relDir == mainPkg {
		relDir = "."
	}

	destPath = filepath.Join(relDir, "version.go")
	tmplPath = filepath.Join(dirMagefiles, "version.go.tmpl")

	tmplContent := `// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

// Version is set during the generation process.
const Version = "{{.Version}}"
`
	absPath := filepath.Join(targetDir, tmplPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(absPath, []byte(tmplContent), 0o644); err != nil {
		return "", "", err
	}
	return destPath, tmplPath, nil
}

// WriteScaffoldConfig marshals cfg as YAML and writes it to path.
// The marshalFn parameter allows the caller to provide the YAML
// marshalling without importing the Config type here.
func WriteScaffoldConfig(path string, marshalFn func() ([]byte, error)) error {
	data, err := marshalFn()
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	header := "# Orchestrator configuration — generated by scaffold.\n# See docs/ARCHITECTURE.yaml for field descriptions.\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

// ScaffoldMageGoMod ensures magefiles/go.mod exists with the orchestrator
// dependency. If a published version of the orchestrator module is available
// on the Go module proxy, it is required directly. Otherwise the function
// falls back to a local replace directive pointing at orchestratorRoot.
func ScaffoldMageGoMod(mageDir, rootModule, orchestratorRoot string) error {
	goMod := filepath.Join(mageDir, "go.mod")

	if _, err := os.Stat(goMod); os.IsNotExist(err) {
		mageModule := rootModule + "/magefiles"
		Log("scaffold: creating %s (module %s)", goMod, mageModule)
		initCmd := exec.Command(BinGo, "mod", "init", mageModule)
		initCmd.Dir = mageDir
		if err := initCmd.Run(); err != nil {
			return fmt.Errorf("go mod init: %w", err)
		}
	}

	usedPublished := false
	if version := LatestPublishedVersion(OrchestratorModule); version != "" {
		Log("scaffold: trying published %s@%s", OrchestratorModule, version)

		dropCmd := exec.Command(BinGo, "mod", "edit",
			"-dropreplace", OrchestratorModule)
		dropCmd.Dir = mageDir
		_ = dropCmd.Run()

		requireCmd := exec.Command(BinGo, "mod", "edit",
			"-require", OrchestratorModule+"@"+version)
		requireCmd.Dir = mageDir
		if err := requireCmd.Run(); err != nil {
			return fmt.Errorf("go mod edit -require: %w", err)
		}

		tidyCmd := exec.Command(BinGo, "mod", "tidy")
		tidyCmd.Dir = mageDir
		if err := tidyCmd.Run(); err != nil {
			Log("scaffold: published %s@%s unusable (%v); falling back to local replace", OrchestratorModule, version, err)
		} else {
			usedPublished = true
		}
	}

	if !usedPublished {
		Log("scaffold: using local replace for %s", OrchestratorModule)
		replaceCmd := exec.Command(BinGo, "mod", "edit",
			"-replace", OrchestratorModule+"="+orchestratorRoot)
		replaceCmd.Dir = mageDir
		if err := replaceCmd.Run(); err != nil {
			return fmt.Errorf("go mod edit -replace: %w", err)
		}

		tidyCmd := exec.Command(BinGo, "mod", "tidy")
		tidyCmd.Dir = mageDir
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			return fmt.Errorf("go mod tidy: %w", err)
		}
	}

	return nil
}

// LatestPublishedVersion queries the Go module proxy for the latest
// published version of module. Returns empty string if no versions
// are available or the proxy cannot be reached.
func LatestPublishedVersion(module string) string {
	tmpDir, err := os.MkdirTemp("", "version-check-*")
	if err != nil {
		return ""
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			Log("scaffold: warning: removing temp dir: %v", err)
		}
	}()

	initCmd := exec.Command(BinGo, "mod", "init", "temp")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		return ""
	}

	cmd := exec.Command(BinGo, "list", "-m", "-versions", module)
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// VerifyMage runs mage -l in the target directory to confirm the
// orchestrator template is correctly wired.
func VerifyMage(targetDir string) error {
	magePath, err := FindMage()
	if err != nil {
		return err
	}
	cmd := exec.Command(magePath, "-l")
	cmd.Dir = targetDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// FindMage locates the mage binary. It checks PATH first, then falls
// back to $(go env GOPATH)/bin/mage for installations via go install.
func FindMage() (string, error) {
	if p, err := exec.LookPath(BinMage); err == nil {
		return p, nil
	}
	out, err := exec.Command(BinGo, "env", "GOPATH").Output()
	if err != nil {
		return "", fmt.Errorf("mage not found on PATH and cannot determine GOPATH: %w", err)
	}
	gopath := strings.TrimSpace(string(out))
	candidate := filepath.Join(gopath, "bin", BinMage)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("mage not found on PATH or at %s", candidate)
	}
	return candidate, nil
}

// GoModDownload fetches a Go module at the specified version using the
// Go module proxy and returns the path to the cached source directory.
// The cache directory is read-only; callers must copy before modifying.
func GoModDownload(module, version string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "gomod-dl-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			Log("scaffold: warning: removing temp dir: %v", err)
		}
	}()

	initCmd := exec.Command(BinGo, "mod", "init", "temp")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		return "", fmt.Errorf("go mod init: %w", err)
	}

	ref := module + "@" + version
	dlCmd := exec.Command(BinGo, "mod", "download", "-json", ref)
	dlCmd.Dir = tmpDir
	out, err := dlCmd.Output()
	if err != nil {
		return "", fmt.Errorf("go mod download %s: %w", ref, err)
	}

	var result GoModDownloadResult
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing go mod download output: %w", err)
	}
	if result.Dir == "" {
		return "", fmt.Errorf("go mod download %s: empty Dir in output", ref)
	}
	return result.Dir, nil
}

// CopyDir recursively copies src to dst, making all files writable.
// The Go module cache is read-only, so this produces a mutable copy.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// RemoveIfExists removes path if it exists, logging the action.
func RemoveIfExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	Log("uninstall: removing %s", path)
	return os.Remove(path)
}
