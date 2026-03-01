// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed constitutions/design.yaml
var designConstitution string

//go:embed constitutions/testing.yaml
var testingConstitution string

// orchestratorModule is the Go module path for this orchestrator library.
const orchestratorModule = "github.com/mesh-intelligence/cobbler-scaffold"

// Scaffold sets up a target Go repository to use the orchestrator.
// It copies the orchestrator.go template into magefiles/, detects
// project structure, generates configuration.yaml, and wires the
// Go module dependencies.
func (o *Orchestrator) Scaffold(targetDir, orchestratorRoot string) error {
	logf("scaffold: targetDir=%s orchestratorRoot=%s", targetDir, orchestratorRoot)

	mageDir := filepath.Join(targetDir, dirMagefiles)

	// 1. Remove existing .go files in magefiles/ (the orchestrator
	//    template replaces the target's build system) and copy ours.
	if err := clearMageGoFiles(mageDir); err != nil {
		return fmt.Errorf("clearing magefiles: %w", err)
	}
	src := filepath.Join(orchestratorRoot, "orchestrator.go.tmpl")
	dst := filepath.Join(mageDir, "orchestrator.go")
	logf("scaffold: copying %s -> %s", src, dst)
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("copying orchestrator.go: %w", err)
	}

	// 1b. Copy all constitutions to docs/constitutions/ so users can
	//    read and modify them. Config paths point here by default.
	docsDir := filepath.Join(targetDir, "docs")
	constitutionsDir := filepath.Join(docsDir, "constitutions")
	if err := os.MkdirAll(constitutionsDir, 0o755); err != nil {
		return fmt.Errorf("creating docs/constitutions directory: %w", err)
	}
	constitutionFiles := map[string]string{
		"design.yaml":    designConstitution,
		"planning.yaml":  planningConstitution,
		"execution.yaml": executionConstitution,
		"go-style.yaml":  goStyleConstitution,
		"testing.yaml":   testingConstitution,
	}
	for _, name := range slices.Sorted(maps.Keys(constitutionFiles)) {
		p := filepath.Join(constitutionsDir, name)
		logf("scaffold: writing constitution to %s", p)
		if err := os.WriteFile(p, []byte(constitutionFiles[name]), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	// 1c. Copy prompt templates to docs/prompts/ so users can read and
	//    modify them. Config paths point here by default.
	promptsDir := filepath.Join(docsDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		return fmt.Errorf("creating docs/prompts directory: %w", err)
	}
	promptFiles := map[string]string{
		"measure.yaml": defaultMeasurePrompt,
		"stitch.yaml":  defaultStitchPrompt,
	}
	for _, name := range slices.Sorted(maps.Keys(promptFiles)) {
		p := filepath.Join(promptsDir, name)
		logf("scaffold: writing prompt to %s", p)
		if err := os.WriteFile(p, []byte(promptFiles[name]), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	// 1d. Write default phase context files to the cobbler directory.
	// These files are optional; when absent, Config defaults apply.
	cobblerDir := filepath.Join(targetDir, dirCobbler)
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		return fmt.Errorf("creating cobbler directory: %w", err)
	}
	contextFiles := map[string]string{
		"measure_context.yaml": defaultMeasureContext,
		"stitch_context.yaml":  defaultStitchContext,
	}
	for _, name := range slices.Sorted(maps.Keys(contextFiles)) {
		p := filepath.Join(cobblerDir, name)
		logf("scaffold: writing context file to %s", p)
		if err := os.WriteFile(p, []byte(contextFiles[name]), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
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

	// 3. Generate seed files and configuration.yaml in the target root.
	cfg := DefaultConfig()
	cfg.Project.ModulePath = modulePath
	cfg.Project.BinaryName = binName
	cfg.Project.MainPackage = mainPkg
	cfg.Project.GoSourceDirs = srcDirs
	cfg.Cobbler.PlanningConstitution = "docs/constitutions/planning.yaml"
	cfg.Cobbler.ExecutionConstitution = "docs/constitutions/execution.yaml"
	cfg.Cobbler.DesignConstitution = "docs/constitutions/design.yaml"
	cfg.Cobbler.GoStyleConstitution = "docs/constitutions/go-style.yaml"
	cfg.Cobbler.MeasurePrompt = "docs/prompts/measure.yaml"
	cfg.Cobbler.StitchPrompt = "docs/prompts/stitch.yaml"

	// When a main package is detected, create a version.go seed template
	// so that after generator:reset the project has a minimal compilable
	// binary. The template is stored in magefiles/ and referenced by
	// seed_files in configuration.yaml.
	if mainPkg != "" {
		seedPath, tmplPath, err := scaffoldSeedTemplate(targetDir, modulePath, mainPkg)
		if err != nil {
			return fmt.Errorf("creating seed template: %w", err)
		}
		cfg.Project.SeedFiles = map[string]string{seedPath: tmplPath}
		cfg.Project.VersionFile = seedPath
		logf("scaffold: created seed template %s -> %s", seedPath, tmplPath)
	}

	cfgPath := filepath.Join(targetDir, DefaultConfigFile)
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		logf("scaffold: writing %s", cfgPath)
		if err := writeScaffoldConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("writing configuration.yaml: %w", err)
		}
	} else {
		logf("scaffold: %s already exists, skipping", DefaultConfigFile)

		// When configuration.yaml already exists but was written by a
		// previous scaffold (or generation), the seed_files entry may
		// reference a template that does not exist on disk — e.g. a
		// specs-only tag that deleted magefiles/version.go.tmpl. Read
		// the existing config (raw YAML, not LoadConfig which would
		// fail on the missing file) and re-create any absent templates.
		existData, err := os.ReadFile(cfgPath)
		if err == nil {
			var existCfg Config
			if err := yaml.Unmarshal(existData, &existCfg); err == nil {
				if existCfg.Project.MainPackage != "" {
					for _, src := range existCfg.Project.SeedFiles {
						absFile := filepath.Join(targetDir, src)
						if _, err := os.Stat(absFile); os.IsNotExist(err) {
							logf("scaffold: re-creating missing seed template %s", src)
							if _, _, err := scaffoldSeedTemplate(targetDir, modulePath, existCfg.Project.MainPackage); err != nil {
								return fmt.Errorf("re-creating seed template: %w", err)
							}
						}
					}
				}
			}
		}
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

	// 5. Verify. If verification fails and we used a published version,
	// retry with a local replace — the published module may be missing
	// methods that the scaffolded orchestrator.go references.
	logf("scaffold: verifying with mage -l")
	if err := verifyMage(targetDir); err != nil {
		logf("scaffold: verification failed; retrying with local replace -> %s", absOrch)
		retryReplace := exec.Command(binGo, "mod", "edit",
			"-replace", orchestratorModule+"="+absOrch)
		retryReplace.Dir = mageDir
		if replaceErr := retryReplace.Run(); replaceErr != nil {
			return fmt.Errorf("mage verification: %w (replace fallback: %v)", err, replaceErr)
		}
		retryTidy := exec.Command(binGo, "mod", "tidy")
		retryTidy.Dir = mageDir
		if tidyErr := retryTidy.Run(); tidyErr != nil {
			return fmt.Errorf("mage verification: %w (tidy fallback: %v)", err, tidyErr)
		}
		if err := verifyMage(targetDir); err != nil {
			return fmt.Errorf("mage verification (after local replace): %w", err)
		}
	}

	logf("scaffold: done")
	return nil
}

// Uninstall removes the files added by Scaffold from targetDir:
// magefiles/orchestrator.go, docs/constitutions/, docs/prompts/,
// configuration.yaml, and .cobbler/. It also removes the orchestrator replace
// directive from magefiles/go.mod and runs go mod tidy to clean up unused
// dependencies.
func (o *Orchestrator) Uninstall(targetDir string) error {
	logf("uninstall: removing orchestrator files from %s", targetDir)

	// Remove magefiles/orchestrator.go.
	orchGo := filepath.Join(targetDir, dirMagefiles, "orchestrator.go")
	if err := removeIfExists(orchGo); err != nil {
		return fmt.Errorf("removing orchestrator.go: %w", err)
	}

	// Remove docs/constitutions/ and docs/prompts/ directories.
	constitutionsDir := filepath.Join(targetDir, "docs", "constitutions")
	if err := os.RemoveAll(constitutionsDir); err != nil {
		return fmt.Errorf("removing docs/constitutions: %w", err)
	}
	logf("uninstall: removed %s", constitutionsDir)

	promptsDir := filepath.Join(targetDir, "docs", "prompts")
	if err := os.RemoveAll(promptsDir); err != nil {
		return fmt.Errorf("removing docs/prompts: %w", err)
	}
	logf("uninstall: removed %s", promptsDir)

	// Remove .cobbler/ directory written by Scaffold.
	cobblerDir := filepath.Join(targetDir, dirCobbler)
	if err := os.RemoveAll(cobblerDir); err != nil {
		return fmt.Errorf("removing .cobbler: %w", err)
	}
	logf("uninstall: removed %s", cobblerDir)

	// Remove configuration.yaml.
	cfgPath := filepath.Join(targetDir, DefaultConfigFile)
	if err := removeIfExists(cfgPath); err != nil {
		return fmt.Errorf("removing configuration.yaml: %w", err)
	}

	// Remove the orchestrator replace directive from magefiles/go.mod.
	mageDir := filepath.Join(targetDir, dirMagefiles)
	goMod := filepath.Join(mageDir, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		dropCmd := exec.Command(binGo, "mod", "edit",
			"-dropreplace", orchestratorModule)
		dropCmd.Dir = mageDir
		if err := dropCmd.Run(); err != nil {
			logf("uninstall: warning: could not drop replace directive: %v", err)
		} else {
			tidyCmd := exec.Command(binGo, "mod", "tidy")
			tidyCmd.Dir = mageDir
			tidyCmd.Stdout = os.Stdout
			tidyCmd.Stderr = os.Stderr
			if err := tidyCmd.Run(); err != nil {
				logf("uninstall: warning: go mod tidy failed: %v", err)
			}
		}
	}

	logf("uninstall: done")
	return nil
}

// removeIfExists removes path if it exists, logging the action.
func removeIfExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	logf("uninstall: removing %s", path)
	return os.Remove(path)
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
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading go.mod: %w", err)
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

// scaffoldSeedTemplate creates a version.go.tmpl in the magefiles directory
// and returns the destination path (relative to repo root) and the template
// source path (relative to repo root) for use in seed_files configuration.
func scaffoldSeedTemplate(targetDir, modulePath, mainPkg string) (destPath, tmplPath string, err error) {
	// Derive the relative directory for the main package.
	// e.g. modulePath="github.com/org/repo", mainPkg="github.com/org/repo/cmd/app"
	// → relDir="cmd/app"
	relDir := strings.TrimPrefix(mainPkg, modulePath+"/")
	if relDir == mainPkg {
		// mainPkg equals modulePath — main is at repo root.
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

// writeScaffoldConfig marshals cfg as YAML and writes it to path.
func writeScaffoldConfig(path string, cfg Config) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	header := "# Orchestrator configuration — generated by scaffold.\n# See docs/ARCHITECTURE.yaml for field descriptions.\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

// clearGenerationBranch reads configuration.yaml in repoDir, clears the
// generation.branch field, and writes the file back. This prevents stale
// branch references from a module's own generation cycle from interfering
// with test repos that start from a clean git state.
func clearGenerationBranch(repoDir string) error {
	cfgPath := filepath.Join(repoDir, DefaultConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.Generation.Branch == "" {
		return nil
	}
	logf("prepareTestRepo: clearing stale generation.branch=%s", cfg.Generation.Branch)
	cfg.Generation.Branch = ""
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0o644)
}

// scaffoldMageGoMod ensures magefiles/go.mod exists with the orchestrator
// dependency. If a published version of the orchestrator module is available
// on the Go module proxy, it is required directly. Otherwise the function
// falls back to a local replace directive pointing at orchestratorRoot.
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

	// Prefer a published version over a local replace directive.
	// A local replace bakes a machine-specific absolute path into the
	// target repo, which is meaningless to other machines and fails
	// inside containers.
	usedPublished := false
	if version := latestPublishedVersion(orchestratorModule); version != "" {
		logf("scaffold: trying published %s@%s", orchestratorModule, version)

		// Drop any stale replace directive from a previous scaffold.
		dropCmd := exec.Command(binGo, "mod", "edit",
			"-dropreplace", orchestratorModule)
		dropCmd.Dir = mageDir
		_ = dropCmd.Run() // ignore error if no replace exists

		requireCmd := exec.Command(binGo, "mod", "edit",
			"-require", orchestratorModule+"@"+version)
		requireCmd.Dir = mageDir
		if err := requireCmd.Run(); err != nil {
			return fmt.Errorf("go mod edit -require: %w", err)
		}

		// Verify the published version is usable (module path may have
		// changed across tags — the proxy will reject mismatches).
		tidyCmd := exec.Command(binGo, "mod", "tidy")
		tidyCmd.Dir = mageDir
		if err := tidyCmd.Run(); err != nil {
			logf("scaffold: published %s@%s unusable (%v); falling back to local replace", orchestratorModule, version, err)
		} else {
			usedPublished = true
		}
	}

	if !usedPublished {
		logf("scaffold: using local replace for %s", orchestratorModule)
		replaceCmd := exec.Command(binGo, "mod", "edit",
			"-replace", orchestratorModule+"="+orchestratorRoot)
		replaceCmd.Dir = mageDir
		if err := replaceCmd.Run(); err != nil {
			return fmt.Errorf("go mod edit -replace: %w", err)
		}

		tidyCmd := exec.Command(binGo, "mod", "tidy")
		tidyCmd.Dir = mageDir
		tidyCmd.Stdout = os.Stdout
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			return fmt.Errorf("go mod tidy: %w", err)
		}
	}

	return nil
}

// latestPublishedVersion queries the Go module proxy for the latest
// published version of module. Returns empty string if no versions
// are available or the proxy cannot be reached.
func latestPublishedVersion(module string) string {
	tmpDir, err := os.MkdirTemp("", "version-check-*")
	if err != nil {
		return ""
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			logf("scaffold: warning: removing temp dir: %v", err)
		}
	}()

	initCmd := exec.Command(binGo, "mod", "init", "temp")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		return ""
	}

	cmd := exec.Command(binGo, "list", "-m", "-versions", module)
	cmd.Dir = tmpDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output format: "module v0.1.0 v0.2.0 v0.3.0"
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
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
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			logf("scaffold: warning: removing temp dir: %v", err)
		}
	}()

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

	// Remove development artifacts from the copied source. Module
	// sources may include .cobbler/ or other local state
	// directories that interfere with a clean test environment.
	for _, artifact := range []string{dirCobbler} {
		p := filepath.Join(repoDir, artifact)
		if _, err := os.Stat(p); err == nil {
			logf("prepareTestRepo: removing artifact %s", artifact)
			os.RemoveAll(p)
		}
	}

	// Clear stale generation branch from configuration.yaml. The
	// upstream repo may have a generation.branch value left over
	// from its own generation cycle; that branch does not exist in
	// the fresh test repo and causes generator:stop to fail.
	if err := clearGenerationBranch(repoDir); err != nil {
		logf("prepareTestRepo: warning: could not clear generation branch: %v", err)
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

	// Override with a local replace so the test repo compiles against
	// the current orchestrator source, not a published release.
	mageDir := filepath.Join(repoDir, dirMagefiles)
	logf("prepareTestRepo: overriding with local replace -> %s", orchestratorRoot)
	replaceCmd := exec.Command(binGo, "mod", "edit",
		"-replace", orchestratorModule+"="+orchestratorRoot)
	replaceCmd.Dir = mageDir
	if err := replaceCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("go mod edit -replace: %w", err)
	}
	tidyCmd := exec.Command(binGo, "mod", "tidy")
	tidyCmd.Dir = mageDir
	if err := tidyCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("go mod tidy (test replace): %w", err)
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
