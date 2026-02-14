// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// orchestratorModule is the Go module path for this orchestrator library.
const orchestratorModule = "github.com/mesh-intelligence/mage-claude-orchestrator"

// Scaffold sets up a target Go repository to use the orchestrator.
// It copies the orchestrator.go template into magefiles/, detects
// project structure, generates configuration.yaml, and wires the
// Go module dependencies.
func (o *Orchestrator) Scaffold(targetDir, orchestratorRoot string) error {
	logf("scaffold: targetDir=%s orchestratorRoot=%s", targetDir, orchestratorRoot)

	mageDir := filepath.Join(targetDir, "magefiles")

	// 1. Remove existing .go files in magefiles/ (the orchestrator
	//    template replaces the target's build system) and copy ours.
	if err := clearMageGoFiles(mageDir); err != nil {
		return fmt.Errorf("clearing magefiles: %w", err)
	}
	src := filepath.Join(orchestratorRoot, "orchestrator.go")
	dst := filepath.Join(mageDir, "orchestrator.go")
	logf("scaffold: copying %s -> %s", src, dst)
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("copying orchestrator.go: %w", err)
	}

	// 2. Detect project structure.
	modulePath, err := detectModulePath(targetDir)
	if err != nil {
		return fmt.Errorf("detecting module path: %w", err)
	}
	logf("scaffold: detected module_path=%s", modulePath)

	mainPkg := detectMainPackage(targetDir, modulePath)
	logf("scaffold: detected main_package=%s", mainPkg)

	srcDirs := detectSourceDirs(targetDir)
	logf("scaffold: detected go_source_dirs=%v", srcDirs)

	binName := detectBinaryName(modulePath)
	logf("scaffold: detected binary_name=%s", binName)

	// 3. Generate configuration.yaml in the target root.
	cfg := DefaultConfig()
	cfg.ModulePath = modulePath
	cfg.BinaryName = binName
	cfg.MainPackage = mainPkg
	cfg.GoSourceDirs = srcDirs

	cfgPath := filepath.Join(targetDir, DefaultConfigFile)
	logf("scaffold: writing %s", cfgPath)
	if err := writeScaffoldConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("writing configuration.yaml: %w", err)
	}

	// 4. Wire magefiles/go.mod.
	logf("scaffold: wiring magefiles/go.mod")
	absOrch, err := filepath.Abs(orchestratorRoot)
	if err != nil {
		return fmt.Errorf("resolving orchestrator path: %w", err)
	}
	if err := scaffoldMageGoMod(mageDir, modulePath, absOrch); err != nil {
		return fmt.Errorf("wiring magefiles/go.mod: %w", err)
	}

	// 5. Verify.
	logf("scaffold: verifying with mage -l")
	if err := verifyMage(targetDir); err != nil {
		return fmt.Errorf("mage verification: %w", err)
	}

	logf("scaffold: done")
	return nil
}

// clearMageGoFiles removes all .go files from mageDir, preserving
// go.mod, go.sum, and non-Go files. If mageDir does not exist, this
// is a no-op.
func clearMageGoFiles(mageDir string) error {
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
		logf("scaffold: removing existing %s", path)
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing %s: %w", path, err)
		}
	}
	return nil
}

// copyFile copies src to dst, creating parent directories as needed.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// detectModulePath reads go.mod in the target directory and extracts
// the module path from the first "module" directive.
func detectModulePath(targetDir string) (string, error) {
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
	return "", fmt.Errorf("no module directive in %s", modPath)
}

// detectMainPackage scans cmd/ for directories containing main.go.
// Returns the module-relative import path of the first main package
// found, or empty string if none exist.
func detectMainPackage(targetDir, modulePath string) string {
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

// detectSourceDirs returns existing Go source directories in the target.
func detectSourceDirs(targetDir string) []string {
	candidates := []string{"cmd/", "pkg/", "internal/", "tests/"}
	var found []string
	for _, d := range candidates {
		if _, err := os.Stat(filepath.Join(targetDir, d)); err == nil {
			found = append(found, d)
		}
	}
	return found
}

// detectBinaryName extracts a binary name from the module path by
// using its last path component.
func detectBinaryName(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) == 0 {
		return "app"
	}
	return parts[len(parts)-1]
}

// writeScaffoldConfig marshals cfg as YAML and writes it to path.
func writeScaffoldConfig(path string, cfg Config) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	header := "# Orchestrator configuration — generated by scaffold.\n# See docs/ARCHITECTURE.md for field descriptions.\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

// scaffoldMageGoMod ensures magefiles/go.mod exists with the orchestrator
// dependency and replace directive pointing to the local checkout.
// If magefiles/go.mod does not exist, it creates one.
func scaffoldMageGoMod(mageDir, rootModule, orchestratorRoot string) error {
	goMod := filepath.Join(mageDir, "go.mod")

	// Create magefiles/go.mod if it does not exist.
	if _, err := os.Stat(goMod); os.IsNotExist(err) {
		mageModule := rootModule + "/magefiles"
		logf("scaffold: creating %s (module %s)", goMod, mageModule)
		initCmd := exec.Command(binGo, "mod", "init", mageModule)
		initCmd.Dir = mageDir
		if err := initCmd.Run(); err != nil {
			return fmt.Errorf("go mod init: %w", err)
		}
	}

	// Add replace directive.
	replaceCmd := exec.Command(binGo, "mod", "edit",
		"-replace", orchestratorModule+"="+orchestratorRoot)
	replaceCmd.Dir = mageDir
	if err := replaceCmd.Run(); err != nil {
		return fmt.Errorf("go mod edit -replace: %w", err)
	}

	// Tidy resolves imports from orchestrator.go and adds required modules.
	tidyCmd := exec.Command(binGo, "mod", "tidy")
	tidyCmd.Dir = mageDir
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	return nil
}

// verifyMage runs mage -l in the target directory to confirm the
// orchestrator template is correctly wired.
func verifyMage(targetDir string) error {
	magePath, err := findMage()
	if err != nil {
		return err
	}
	cmd := exec.Command(magePath, "-l")
	cmd.Dir = targetDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findMage locates the mage binary. It checks PATH first, then falls
// back to $(go env GOPATH)/bin/mage for installations via go install.
func findMage() (string, error) {
	if p, err := exec.LookPath(binMage); err == nil {
		return p, nil
	}
	out, err := exec.Command(binGo, "env", "GOPATH").Output()
	if err != nil {
		return "", fmt.Errorf("mage not found on PATH and cannot determine GOPATH: %w", err)
	}
	gopath := strings.TrimSpace(string(out))
	candidate := filepath.Join(gopath, "bin", binMage)
	if _, err := os.Stat(candidate); err != nil {
		return "", fmt.Errorf("mage not found on PATH or at %s", candidate)
	}
	return candidate, nil
}

// goModDownloadResult holds the fields we need from go mod download -json.
type goModDownloadResult struct {
	Dir string `json:"Dir"`
}

// goModDownload fetches a Go module at the specified version using the
// Go module proxy and returns the path to the cached source directory.
// The cache directory is read-only; callers must copy before modifying.
func goModDownload(module, version string) (string, error) {
	// go mod download requires a module context; create a temporary one.
	tmpDir, err := os.MkdirTemp("", "gomod-dl-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	initCmd := exec.Command(binGo, "mod", "init", "temp")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		return "", fmt.Errorf("go mod init: %w", err)
	}

	ref := module + "@" + version
	dlCmd := exec.Command(binGo, "mod", "download", "-json", ref)
	dlCmd.Dir = tmpDir
	out, err := dlCmd.Output()
	if err != nil {
		return "", fmt.Errorf("go mod download %s: %w", ref, err)
	}

	var result goModDownloadResult
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing go mod download output: %w", err)
	}
	if result.Dir == "" {
		return "", fmt.Errorf("go mod download %s: empty Dir in output", ref)
	}
	return result.Dir, nil
}

// copyDir recursively copies src to dst, making all files writable.
// The Go module cache is read-only, so this produces a mutable copy.
func copyDir(src, dst string) error {
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

// PrepareTestRepo downloads a Go module at the given version, copies it
// to a temporary working directory, initializes a fresh git repository,
// and runs Scaffold. Returns the path to the ready-to-use repo directory.
// The caller is responsible for removing the parent temp directory when done.
func (o *Orchestrator) PrepareTestRepo(module, version, orchestratorRoot string) (string, error) {
	logf("prepareTestRepo: downloading %s@%s", module, version)

	cacheDir, err := goModDownload(module, version)
	if err != nil {
		return "", fmt.Errorf("downloading module: %w", err)
	}
	logf("prepareTestRepo: cached at %s", cacheDir)

	workDir, err := os.MkdirTemp("", "test-clone-*")
	if err != nil {
		return "", fmt.Errorf("creating work dir: %w", err)
	}
	repoDir := filepath.Join(workDir, "repo")

	logf("prepareTestRepo: copying to %s", repoDir)
	if err := copyDir(cacheDir, repoDir); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("copying module source: %w", err)
	}

	// Initialize a fresh git repository.
	logf("prepareTestRepo: initializing git")
	initCmd := exec.Command(binGit, "init")
	initCmd.Dir = repoDir
	if err := initCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("git init: %w", err)
	}

	addCmd := exec.Command(binGit, "add", "-A")
	addCmd.Dir = repoDir
	if err := addCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("git add: %w", err)
	}

	commitCmd := exec.Command(binGit, "commit", "-m", "Initial commit from test-clone")
	commitCmd.Dir = repoDir
	if err := commitCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("git commit: %w", err)
	}

	// Scaffold the orchestrator into the repo.
	logf("prepareTestRepo: scaffolding")
	if err := o.Scaffold(repoDir, orchestratorRoot); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("scaffold: %w", err)
	}

	// Commit scaffold artifacts so the working tree is clean.
	addCmd2 := exec.Command(binGit, "add", "-A")
	addCmd2.Dir = repoDir
	if err := addCmd2.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("git add scaffold: %w", err)
	}

	commitCmd2 := exec.Command(binGit, "commit", "-m", "Add orchestrator scaffold")
	commitCmd2.Dir = repoDir
	if err := commitCmd2.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("git commit scaffold: %w", err)
	}

	logf("prepareTestRepo: ready at %s", repoDir)
	return repoDir, nil
}
