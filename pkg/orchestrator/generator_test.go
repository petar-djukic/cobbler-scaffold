// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestGitRepo creates a bare-minimum git repo in a temp directory,
// changes the working directory to it, and registers cleanup.
// Tests calling this MUST NOT use t.Parallel() because they share the
// process-wide working directory.
func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v failed: %v\n%s", args, err, out)
		}
	}
	return dir
}

// chdirTemp changes the working directory to a temp directory and
// registers cleanup. Tests calling this MUST NOT use t.Parallel().
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	return dir
}

// --- generationName (pure, parallelizable) ---

func TestGenerationName_StripsSuffixes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tag  string
		want string
	}{
		{"generation-2026-02-12-07-13-55-start", "generation-2026-02-12-07-13-55"},
		{"generation-2026-02-12-07-13-55-finished", "generation-2026-02-12-07-13-55"},
		{"generation-2026-02-12-07-13-55-merged", "generation-2026-02-12-07-13-55"},
		{"generation-2026-02-12-07-13-55-abandoned", "generation-2026-02-12-07-13-55"},
		{"generation-2026-02-12-07-13-55", "generation-2026-02-12-07-13-55"},
		{"unrelated-tag", "unrelated-tag"},
	}
	for _, tt := range tests {
		got := generationName(tt.tag)
		if got != tt.want {
			t.Errorf("generationName(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

// --- writeBaseBranch / readBaseBranch (filesystem only, parallelizable) ---

func TestWriteBaseBranch_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cobblerDir := filepath.Join(dir, ".cobbler")
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: cobblerDir}}}

	if err := o.writeBaseBranch("feature-branch"); err != nil {
		t.Fatalf("writeBaseBranch() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cobblerDir, baseBranchFile))
	if err != nil {
		t.Fatalf("reading base-branch file: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "feature-branch" {
		t.Errorf("file content = %q, want %q", got, "feature-branch")
	}
}

func TestWriteBaseBranch_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cobblerDir := filepath.Join(dir, "nested", "cobbler")
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: cobblerDir}}}

	if err := o.writeBaseBranch("main"); err != nil {
		t.Fatalf("writeBaseBranch() error = %v", err)
	}

	if _, err := os.Stat(cobblerDir); os.IsNotExist(err) {
		t.Error("cobbler directory was not created")
	}
}

func TestReadBaseBranch_FileExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cobblerDir := filepath.Join(dir, ".cobbler")
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cobblerDir, baseBranchFile), []byte("develop\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: cobblerDir}}}
	got := o.readBaseBranch()
	if got != "develop" {
		t.Errorf("readBaseBranch() = %q, want %q", got, "develop")
	}
}

func TestReadBaseBranch_FileMissing_ReturnsMain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: filepath.Join(dir, "nonexistent")}}}

	got := o.readBaseBranch()
	if got != "main" {
		t.Errorf("readBaseBranch() = %q, want %q", got, "main")
	}
}

func TestReadBaseBranch_EmptyFile_ReturnsMain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cobblerDir := filepath.Join(dir, ".cobbler")
	if err := os.MkdirAll(cobblerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cobblerDir, baseBranchFile), []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: cobblerDir}}}
	got := o.readBaseBranch()
	if got != "main" {
		t.Errorf("readBaseBranch() = %q, want %q", got, "main")
	}
}

func TestWriteReadBaseBranch_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cobblerDir := filepath.Join(dir, ".cobbler")
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: cobblerDir}}}

	branches := []string{"main", "develop", "feature/my-feature", "release-1.0"}
	for _, branch := range branches {
		if err := o.writeBaseBranch(branch); err != nil {
			t.Fatalf("writeBaseBranch(%q) error = %v", branch, err)
		}
		got := o.readBaseBranch()
		if got != branch {
			t.Errorf("round-trip: wrote %q, read %q", branch, got)
		}
	}
}

// --- seedFiles (uses cwd, NOT parallel) ---

func TestSeedFiles_CreatesFiles(t *testing.T) {
	dir := chdirTemp(t)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			ModulePath: "example.com/test",
			SeedFiles: map[string]string{
				"hello.go": "package main\n",
			},
		},
	}}

	if err := o.seedFiles("v1"); err != nil {
		t.Fatalf("seedFiles() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.go"))
	if err != nil {
		t.Fatalf("reading seeded file: %v", err)
	}
	if string(data) != "package main\n" {
		t.Errorf("seeded file content = %q, want %q", string(data), "package main\n")
	}
}

func TestSeedFiles_TemplateExpansion(t *testing.T) {
	dir := chdirTemp(t)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			ModulePath: "example.com/myproject",
			SeedFiles: map[string]string{
				"version.go": `package main
const Version = "{{.Version}}"
const Module = "{{.ModulePath}}"
`,
			},
		},
	}}

	if err := o.seedFiles("v2.0.0"); err != nil {
		t.Fatalf("seedFiles() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "version.go"))
	if err != nil {
		t.Fatalf("reading seeded file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `"v2.0.0"`) {
		t.Errorf("expected version v2.0.0 in content: %s", content)
	}
	if !strings.Contains(content, `"example.com/myproject"`) {
		t.Errorf("expected module path in content: %s", content)
	}
}

func TestSeedFiles_CreatesSubdirectories(t *testing.T) {
	dir := chdirTemp(t)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			ModulePath: "example.com/test",
			SeedFiles: map[string]string{
				"pkg/internal/deep/file.go": "package deep\n",
			},
		},
	}}

	if err := o.seedFiles("main"); err != nil {
		t.Fatalf("seedFiles() error = %v", err)
	}

	path := filepath.Join(dir, "pkg", "internal", "deep", "file.go")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file at %s to exist", path)
	}
}

func TestSeedFiles_InvalidTemplate(t *testing.T) {
	chdirTemp(t)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			SeedFiles: map[string]string{
				"bad.go": "{{.Invalid",
			},
		},
	}}

	err := o.seedFiles("main")
	if err == nil {
		t.Error("seedFiles() expected error for invalid template, got nil")
	}
}

func TestSeedFiles_EmptyMap(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{
			SeedFiles: map[string]string{},
		},
	}}

	if err := o.seedFiles("main"); err != nil {
		t.Fatalf("seedFiles() with empty map error = %v", err)
	}
}

// --- deleteGoFiles (uses cwd, NOT parallel) ---

func TestDeleteGoFiles_RemovesGoFiles(t *testing.T) {
	dir := chdirTemp(t)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(dir, "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "pkg", "lib.go"), []byte("package pkg"), 0o644)

	o := &Orchestrator{cfg: Config{Project: ProjectConfig{MagefilesDir: "magefiles"}}}
	o.deleteGoFiles(".")

	if _, err := os.Stat(filepath.Join(dir, "main.go")); !os.IsNotExist(err) {
		t.Error("main.go should have been deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg", "lib.go")); !os.IsNotExist(err) {
		t.Error("pkg/lib.go should have been deleted")
	}
}

func TestDeleteGoFiles_SkipsMagefilesDir(t *testing.T) {
	dir := chdirTemp(t)

	os.MkdirAll(filepath.Join(dir, "magefiles"), 0o755)
	os.WriteFile(filepath.Join(dir, "magefiles", "magefile.go"), []byte("package main"), 0o644)

	o := &Orchestrator{cfg: Config{Project: ProjectConfig{MagefilesDir: "magefiles"}}}
	o.deleteGoFiles(".")

	if _, err := os.Stat(filepath.Join(dir, "magefiles", "magefile.go")); os.IsNotExist(err) {
		t.Error("magefiles/magefile.go should have been preserved")
	}
}

func TestDeleteGoFiles_SkipsGitDir(t *testing.T) {
	dir := chdirTemp(t)

	os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-commit.go"), []byte("package hooks"), 0o644)

	o := &Orchestrator{cfg: Config{Project: ProjectConfig{MagefilesDir: "magefiles"}}}
	o.deleteGoFiles(".")

	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit.go")); os.IsNotExist(err) {
		t.Error(".git/hooks/pre-commit.go should have been preserved")
	}
}

func TestDeleteGoFiles_PreservesNonGoFiles(t *testing.T) {
	dir := chdirTemp(t)

	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0o644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("key: value"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	o := &Orchestrator{cfg: Config{Project: ProjectConfig{MagefilesDir: "magefiles"}}}
	o.deleteGoFiles(".")

	if _, err := os.Stat(filepath.Join(dir, "README.md")); os.IsNotExist(err) {
		t.Error("README.md should have been preserved")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); os.IsNotExist(err) {
		t.Error("config.yaml should have been preserved")
	}
}

// --- removeEmptyDirs (uses absolute paths, parallelizable) ---

func TestRemoveEmptyDirs_RemovesEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	empty := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	removeEmptyDirs(filepath.Join(dir, "a"))

	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("empty directory tree should have been removed")
	}
}

func TestRemoveEmptyDirs_PreservesNonEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	subdir := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("content"), 0o644)
	emptyDir := filepath.Join(dir, "a", "empty")
	os.MkdirAll(emptyDir, 0o755)

	removeEmptyDirs(filepath.Join(dir, "a"))

	if _, err := os.Stat(filepath.Join(subdir, "file.txt")); os.IsNotExist(err) {
		t.Error("file in non-empty dir should have been preserved")
	}
	if _, err := os.Stat(emptyDir); !os.IsNotExist(err) {
		t.Error("empty sibling dir should have been removed")
	}
}

func TestRemoveEmptyDirs_NonExistentRoot(t *testing.T) {
	t.Parallel()
	removeEmptyDirs("/nonexistent/path/that/does/not/exist")
}

// --- cleanupDirs (uses absolute paths, parallelizable) ---

func TestCleanupDirs_RemovesDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	d1 := filepath.Join(dir, "cleanup1")
	d2 := filepath.Join(dir, "cleanup2")
	os.MkdirAll(d1, 0o755)
	os.MkdirAll(d2, 0o755)
	os.WriteFile(filepath.Join(d1, "file.txt"), []byte("data"), 0o644)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{CleanupDirs: []string{d1, d2}},
	}}
	o.cleanupDirs()

	if _, err := os.Stat(d1); !os.IsNotExist(err) {
		t.Error("cleanup dir 1 should have been removed")
	}
	if _, err := os.Stat(d2); !os.IsNotExist(err) {
		t.Error("cleanup dir 2 should have been removed")
	}
}

func TestCleanupDirs_IgnoresNonExistent(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{CleanupDirs: []string{"/nonexistent/dir/abc123"}},
	}}
	o.cleanupDirs()
}

func TestCleanupDirs_EmptyList(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{CleanupDirs: nil},
	}}
	o.cleanupDirs()
}

// --- ensureOnBranch (git, NOT parallel) ---

func TestEnsureOnBranch_AlreadyOnBranch(t *testing.T) {
	initTestGitRepo(t)

	current, err := gitCurrentBranch("")
	if err != nil {
		t.Fatal(err)
	}

	if err := ensureOnBranch(current); err != nil {
		t.Errorf("ensureOnBranch(current) error = %v", err)
	}
}

func TestEnsureOnBranch_SwitchesBranch(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("test-branch", ""); err != nil {
		t.Fatal(err)
	}

	if err := ensureOnBranch("test-branch"); err != nil {
		t.Fatalf("ensureOnBranch(test-branch) error = %v", err)
	}

	current, err := gitCurrentBranch("")
	if err != nil {
		t.Fatal(err)
	}
	if current != "test-branch" {
		t.Errorf("current branch = %q, want %q", current, "test-branch")
	}
}

// --- saveAndSwitchBranch (git, NOT parallel) ---

func TestSaveAndSwitchBranch_AlreadyOnTarget(t *testing.T) {
	initTestGitRepo(t)

	current, _ := gitCurrentBranch("")
	if err := saveAndSwitchBranch(current); err != nil {
		t.Errorf("saveAndSwitchBranch(current) error = %v", err)
	}
}

func TestSaveAndSwitchBranch_CleanSwitch(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("target", ""); err != nil {
		t.Fatal(err)
	}

	if err := saveAndSwitchBranch("target"); err != nil {
		t.Fatalf("saveAndSwitchBranch(target) error = %v", err)
	}

	current, err := gitCurrentBranch("")
	if err != nil {
		t.Fatal(err)
	}
	if current != "target" {
		t.Errorf("current branch = %q, want %q", current, "target")
	}
}

func TestSaveAndSwitchBranch_DirtyWorkingTree(t *testing.T) {
	dir := initTestGitRepo(t)

	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original"), 0o644)
	exec.Command("git", "add", "tracked.txt").Run()
	exec.Command("git", "commit", "--no-verify", "-m", "add tracked file").Run()

	if err := gitCreateBranch("other", ""); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified"), 0o644)

	if err := saveAndSwitchBranch("other"); err != nil {
		t.Fatalf("saveAndSwitchBranch with dirty tree error = %v", err)
	}

	current, err := gitCurrentBranch("")
	if err != nil {
		t.Fatal(err)
	}
	if current != "other" {
		t.Errorf("current branch = %q, want %q", current, "other")
	}
}

// --- resolveBranch (git, NOT parallel) ---

func TestResolveBranch_ExplicitBranchExists(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("explicit-branch", ""); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got, err := o.resolveBranch("explicit-branch")
	if err != nil {
		t.Fatalf("resolveBranch() error = %v", err)
	}
	if got != "explicit-branch" {
		t.Errorf("resolveBranch() = %q, want %q", got, "explicit-branch")
	}
}

func TestResolveBranch_ExplicitBranchNotExist(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	_, err := o.resolveBranch("nonexistent-branch")
	if err == nil {
		t.Error("resolveBranch() expected error for nonexistent branch")
	}
}

func TestResolveBranch_NoGenerationBranches(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got, err := o.resolveBranch("")
	if err != nil {
		t.Fatalf("resolveBranch() error = %v", err)
	}

	current, _ := gitCurrentBranch("")
	if got != current {
		t.Errorf("resolveBranch() = %q, want current branch %q", got, current)
	}
}

func TestResolveBranch_SingleGenerationBranch(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("generation-2026-02-28-12-00-00", ""); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got, err := o.resolveBranch("")
	if err != nil {
		t.Fatalf("resolveBranch() error = %v", err)
	}
	if got != "generation-2026-02-28-12-00-00" {
		t.Errorf("resolveBranch() = %q, want %q", got, "generation-2026-02-28-12-00-00")
	}
}

func TestResolveBranch_MultipleGenerationBranches(t *testing.T) {
	initTestGitRepo(t)

	gitCreateBranch("generation-2026-02-28-12-00-00", "")
	gitCreateBranch("generation-2026-02-28-13-00-00", "")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	_, err := o.resolveBranch("")
	if err == nil {
		t.Error("resolveBranch() expected error for multiple generation branches")
	}
	if !strings.Contains(err.Error(), "multiple generation branches") {
		t.Errorf("resolveBranch() error = %q, want to contain 'multiple generation branches'", err.Error())
	}
}

// --- listGenerationBranches (git, NOT parallel) ---

func TestListGenerationBranches_NoBranches(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.listGenerationBranches()
	if len(got) != 0 {
		t.Errorf("listGenerationBranches() = %v, want empty", got)
	}
}

func TestListGenerationBranches_WithBranches(t *testing.T) {
	initTestGitRepo(t)

	gitCreateBranch("generation-2026-02-28-12-00-00", "")
	gitCreateBranch("generation-2026-02-28-13-00-00", "")
	gitCreateBranch("other-branch", "")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.listGenerationBranches()
	if len(got) != 2 {
		t.Errorf("listGenerationBranches() returned %d branches, want 2: %v", len(got), got)
	}
}

// --- GeneratorResume validation (pure validation, parallelizable) ---

func TestGeneratorResume_NotGenerationBranch(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{
			Prefix: "generation-",
			Branch: "main",
		},
	}}
	err := o.GeneratorResume()
	if err == nil {
		t.Error("GeneratorResume() expected error for non-generation branch")
	}
	if !strings.Contains(err.Error(), "not a generation branch") {
		t.Errorf("error = %q, want to contain 'not a generation branch'", err.Error())
	}
}

// --- GeneratorStart validation (git, NOT parallel) ---

func TestGeneratorStart_DirtyWorkingTree(t *testing.T) {
	dir := initTestGitRepo(t)

	os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("uncommitted"), 0o644)
	exec.Command("git", "add", "dirty.txt").Run()

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-"},
		Project:    ProjectConfig{MagefilesDir: "magefiles"},
	}}

	err := o.GeneratorStart()
	if err == nil {
		t.Error("GeneratorStart() expected error for dirty working tree")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("error = %q, want to contain 'uncommitted changes'", err.Error())
	}
}

// TestGeneratorStart_PreserveSources verifies that with PreserveSources=true,
// GeneratorStart does not delete existing .go files (prd002 R10.1).
// This test MUST NOT call t.Parallel() because it uses initTestGitRepo / os.Chdir.
func TestGeneratorStart_PreserveSources(t *testing.T) {
	dir := initTestGitRepo(t)

	// Create a Go source file that must survive GeneratorStart.
	goFile := filepath.Join(dir, "pkg", "foo.go")
	if err := os.MkdirAll(filepath.Dir(goFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goFile, []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stage and commit so the working tree is clean.
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "add foo.go"},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{
			Prefix:          "generation-",
			PreserveSources: true,
		},
		Project: ProjectConfig{MagefilesDir: "magefiles"},
		Cobbler: CobblerConfig{Dir: ".cobbler/"},
	}}

	if err := o.GeneratorStart(); err != nil {
		t.Fatalf("GeneratorStart() error = %v", err)
	}

	if _, err := os.Stat(goFile); os.IsNotExist(err) {
		t.Error("GeneratorStart() deleted Go source file; want file preserved when PreserveSources=true")
	}
}

// --- GeneratorStart custom name (git, NOT parallel) ---

func TestGeneratorStart_CustomName(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{
			Prefix:          "generation-",
			Name:            "gh-42",
			PreserveSources: true,
		},
		Project: ProjectConfig{MagefilesDir: "magefiles"},
		Cobbler: CobblerConfig{Dir: ".cobbler/"},
	}}

	if err := o.GeneratorStart(); err != nil {
		t.Fatalf("GeneratorStart() error = %v", err)
	}

	branch, err := gitCurrentBranch("")
	if err != nil {
		t.Fatalf("gitCurrentBranch: %v", err)
	}
	if branch != "generation-gh-42" {
		t.Errorf("branch = %q, want %q", branch, "generation-gh-42")
	}

	// Verify lifecycle tag was created.
	tags := gitListTags("generation-gh-42-start", "")
	if len(tags) != 1 {
		t.Errorf("expected start tag, got %v", tags)
	}
}

func TestGeneratorStart_EnvNameOverridesConfig(t *testing.T) {
	initTestGitRepo(t)
	t.Setenv("COBBLER_GEN_NAME", "my-feature")

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{
			Prefix:          "generation-",
			Name:            "config-name",
			PreserveSources: true,
		},
		Project: ProjectConfig{MagefilesDir: "magefiles"},
		Cobbler: CobblerConfig{Dir: ".cobbler/"},
	}}

	if err := o.GeneratorStart(); err != nil {
		t.Fatalf("GeneratorStart() error = %v", err)
	}

	branch, err := gitCurrentBranch("")
	if err != nil {
		t.Fatalf("gitCurrentBranch: %v", err)
	}
	if branch != "generation-my-feature" {
		t.Errorf("branch = %q, want %q", branch, "generation-my-feature")
	}
}

// TestGeneratorStart_AddsBinToGitignore verifies that GeneratorStart appends
// bin/ to .gitignore on the generation branch (GH-469).
// MUST NOT call t.Parallel() — uses initTestGitRepo / os.Chdir.
func TestGeneratorStart_AddsBinToGitignore(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{
			Prefix:          "generation-",
			PreserveSources: true,
		},
		Project: ProjectConfig{MagefilesDir: "magefiles", BinaryDir: "bin"},
		Cobbler: CobblerConfig{Dir: ".cobbler/"},
	}}

	if err := o.GeneratorStart(); err != nil {
		t.Fatalf("GeneratorStart() error = %v", err)
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "bin/") {
		t.Errorf(".gitignore = %q, want to contain 'bin/'", string(data))
	}
}

// --- appendToGitignore (parallel-safe, no git) ---

func TestAppendToGitignore_CreatesFileWhenMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := appendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("appendToGitignore error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "bin/") {
		t.Errorf(".gitignore = %q, want 'bin/'", string(data))
	}
}

func TestAppendToGitignore_SkipsWhenAlreadyPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	os.WriteFile(path, []byte("bin/\n"), 0o644)

	if err := appendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("appendToGitignore error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	// Should have exactly one occurrence.
	count := strings.Count(string(data), "bin/")
	if count != 1 {
		t.Errorf(".gitignore has %d occurrences of 'bin/', want 1:\n%s", count, data)
	}
}

func TestAppendToGitignore_AppendsToExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	os.WriteFile(path, []byte("*.log\n"), 0o644)

	if err := appendToGitignore(dir, "bin/"); err != nil {
		t.Fatalf("appendToGitignore error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "*.log") {
		t.Error("appendToGitignore removed existing content")
	}
	if !strings.Contains(string(data), "bin/") {
		t.Errorf(".gitignore = %q, want to contain 'bin/'", string(data))
	}
}

// --- GeneratorSwitch validation (git, NOT parallel) ---

func TestGeneratorSwitch_NoBranchConfigured(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: ""},
	}}

	err := o.GeneratorSwitch()
	if err == nil {
		t.Error("GeneratorSwitch() expected error when no branch configured")
	}
	if !strings.Contains(err.Error(), "set generation.branch") {
		t.Errorf("error = %q, want to contain 'set generation.branch'", err.Error())
	}
}

func TestGeneratorSwitch_NotGenerationBranch(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: "feature-branch"},
	}}

	err := o.GeneratorSwitch()
	if err == nil {
		t.Error("GeneratorSwitch() expected error for non-generation branch")
	}
	if !strings.Contains(err.Error(), "not a generation branch") {
		t.Errorf("error = %q, want to contain 'not a generation branch'", err.Error())
	}
}

func TestGeneratorSwitch_BranchNotExist(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: "generation-2026-01-01-00-00-00"},
	}}

	err := o.GeneratorSwitch()
	if err == nil {
		t.Error("GeneratorSwitch() expected error for nonexistent branch")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want to contain 'does not exist'", err.Error())
	}
}

func TestGeneratorSwitch_AlreadyOnBranch(t *testing.T) {
	initTestGitRepo(t)

	branch := "generation-2026-02-28-12-00-00"
	if err := gitCheckoutNew(branch, ""); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: branch},
	}}

	if err := o.GeneratorSwitch(); err != nil {
		t.Errorf("GeneratorSwitch() already on branch error = %v", err)
	}

	current, _ := gitCurrentBranch("")
	if current != branch {
		t.Errorf("current branch = %q, want %q", current, branch)
	}
}

func TestGeneratorSwitch_SwitchToMain(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCheckoutNew("generation-2026-02-28-12-00-00", ""); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: "main"},
		Cobbler:    CobblerConfig{BaseBranch: "main"},
	}}

	if err := o.GeneratorSwitch(); err != nil {
		t.Errorf("GeneratorSwitch() to main error = %v", err)
	}

	current, _ := gitCurrentBranch("")
	if current != "main" {
		t.Errorf("current branch = %q, want %q", current, "main")
	}
}

func TestGeneratorReset_UsesConfiguredBaseBranch(t *testing.T) {
	initTestGitRepo(t)

	// Configure a non-existent base branch; GeneratorReset must attempt to switch
	// to it (not to the hardcoded string "main") and return an error that names it.
	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-"},
		Cobbler:    CobblerConfig{BaseBranch: "trunk", Dir: ".cobbler/"},
		Project:    ProjectConfig{MagefilesDir: "magefiles"},
	}}

	err := o.GeneratorReset()
	if err == nil {
		t.Fatal("GeneratorReset() expected error when configured base branch 'trunk' does not exist")
	}
	if !strings.Contains(err.Error(), "trunk") {
		t.Errorf("error = %q, want to mention configured base branch 'trunk'", err.Error())
	}
	if strings.Contains(err.Error(), "switching to main") {
		t.Errorf("error = %q, must not say 'switching to main' when BaseBranch is 'trunk'", err.Error())
	}
}

// --- cleanupUnmergedTags (git, NOT parallel) ---

func TestCleanupUnmergedTags_MergedNotTouched(t *testing.T) {
	initTestGitRepo(t)

	gitTag("generation-2026-02-28-12-00-00-start", "")
	gitTag("generation-2026-02-28-12-00-00-finished", "")
	gitTag("generation-2026-02-28-12-00-00-merged", "")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	o.cleanupUnmergedTags()

	tags := gitListTags("generation-2026-02-28-12-00-00-*", "")
	if len(tags) != 3 {
		t.Errorf("expected 3 tags after cleanup, got %d: %v", len(tags), tags)
	}
}

func TestCleanupUnmergedTags_UnmergedAbandoned(t *testing.T) {
	initTestGitRepo(t)

	gitTag("generation-2026-02-28-12-00-00-start", "")
	gitTag("generation-2026-02-28-12-00-00-finished", "")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	o.cleanupUnmergedTags()

	tags := gitListTags("generation-2026-02-28-12-00-00-*", "")
	found := false
	for _, tag := range tags {
		if tag == "generation-2026-02-28-12-00-00-abandoned" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected -abandoned tag, got: %v", tags)
	}
}

// --- GeneratorInit (uses cwd, NOT parallel) ---

func TestGeneratorInit_CreatesConfigFile(t *testing.T) {
	dir := chdirTemp(t)

	if err := GeneratorInit(); err != nil {
		t.Fatalf("GeneratorInit() error = %v", err)
	}

	path := filepath.Join(dir, DefaultConfigFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected %s to exist after GeneratorInit()", DefaultConfigFile)
	}
}

// --- cleanGoSources ---

func TestCleanGoSources_RemovesGoFiles(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	// Create Go files and a non-Go file.
	os.MkdirAll("pkg/sub", 0o755)
	os.MkdirAll("magefiles", 0o755)
	os.MkdirAll("bin", 0o755)
	os.WriteFile("main.go", []byte("package main"), 0o644)
	os.WriteFile("pkg/sub/lib.go", []byte("package sub"), 0o644)
	os.WriteFile("pkg/sub/lib_test.go", []byte("package sub"), 0o644)
	os.WriteFile("pkg/sub/README.md", []byte("# readme"), 0o644)
	os.WriteFile("magefiles/build.go", []byte("package main"), 0o644) // should be preserved
	os.WriteFile("bin/binary", []byte("binary"), 0o644)

	o := &Orchestrator{cfg: Config{}}
	o.cfg.applyDefaults()
	o.cfg.Project.GoSourceDirs = []string{"pkg"}
	o.cleanGoSources()

	// Go files outside magefiles/ should be deleted.
	if _, err := os.Stat("main.go"); !os.IsNotExist(err) {
		t.Error("main.go should be deleted")
	}
	if _, err := os.Stat("pkg/sub/lib.go"); !os.IsNotExist(err) {
		t.Error("pkg/sub/lib.go should be deleted")
	}
	// magefiles/ should be preserved.
	if _, err := os.Stat("magefiles/build.go"); os.IsNotExist(err) {
		t.Error("magefiles/build.go should be preserved")
	}
	// Non-Go files should remain.
	if _, err := os.Stat("pkg/sub/README.md"); os.IsNotExist(err) {
		t.Error("README.md should be preserved")
	}
	// Binary dir should be removed.
	if _, err := os.Stat("bin"); !os.IsNotExist(err) {
		t.Error("bin/ should be removed")
	}
}

// initTestGitRepoInDir sets up a bare-minimum git repo in dir without calling
// os.Chdir. Tests using this helper can safely call t.Parallel() because they
// do not modify the process-wide working directory.
func initTestGitRepoInDir(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v failed: %v\n%s", args, err, out)
		}
	}
}

// --- gitCurrentBranch with explicit dir (git, parallel-safe) ---

// TestGitCurrentBranch_ExplicitDir demonstrates that git helpers can run in
// parallel when an explicit dir is passed instead of relying on os.Chdir.
func TestGitCurrentBranch_ExplicitDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepoInDir(t, dir)

	branch, err := gitCurrentBranch(dir)
	if err != nil {
		t.Fatalf("gitCurrentBranch(%q) error = %v", dir, err)
	}
	if branch == "" {
		t.Errorf("gitCurrentBranch(%q) returned empty branch", dir)
	}
}

// --- Init (pure, parallelizable) ---

func TestInit_NoOp(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{}}
	if err := o.Init(); err != nil {
		t.Errorf("Init() error = %v", err)
	}
}

// --- GeneratorList (git-dependent, no t.Parallel) ---

func TestGeneratorList_NoGenerations(t *testing.T) {
	initTestGitRepo(t)
	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-"},
	}}
	if err := o.GeneratorList(); err != nil {
		t.Fatalf("GeneratorList() error = %v", err)
	}
}

func TestGeneratorList_WithBranchAndTags(t *testing.T) {
	initTestGitRepo(t)
	gitRun(t, "checkout", "-b", "generation-2026-01-01")
	gitRun(t, "checkout", "main")
	gitRun(t, "tag", "generation-2026-01-01-start")

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-"},
	}}
	if err := o.GeneratorList(); err != nil {
		t.Fatalf("GeneratorList() error = %v", err)
	}
}

// --- restoreFromStartTag (git-dependent, no t.Parallel) ---

func TestRestoreFromStartTag_RestoresMissingGoFiles(t *testing.T) {
	initTestGitRepo(t)

	// Commit a .go file and tag that commit as the generation start.
	if err := os.WriteFile("foo.go", []byte("package foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add foo.go")
	gitRun(t, "tag", "gen-start")

	// Delete the file and commit the deletion to simulate a generation run
	// that removed Go source files. restoreFromStartTag restores files that
	// are absent from both the working tree and the index.
	gitRun(t, "rm", "foo.go")
	gitRun(t, "commit", "--no-verify", "-m", "delete foo.go")

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{MagefilesDir: "magefiles"},
	}}
	if err := o.restoreFromStartTag("gen-start"); err != nil {
		t.Fatalf("restoreFromStartTag: %v", err)
	}

	got, err := os.ReadFile("foo.go")
	if err != nil {
		t.Fatalf("foo.go was not restored: %v", err)
	}
	if string(got) != "package foo\n" {
		t.Errorf("restored content = %q, want %q", string(got), "package foo\n")
	}
}

func TestRestoreFromStartTag_SkipsExistingFiles(t *testing.T) {
	initTestGitRepo(t)

	// Create a Go file, commit, and tag as start.
	os.MkdirAll("pkg", 0o755)
	os.WriteFile(filepath.Join("pkg", "existing.go"), []byte("package pkg\n// original\n"), 0o644)
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add existing.go")
	gitRun(t, "tag", "gen-start-exist")

	// Modify the file (should NOT be overwritten by restore).
	os.WriteFile(filepath.Join("pkg", "existing.go"), []byte("package pkg\n// modified\n"), 0o644)

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{MagefilesDir: "magefiles"},
	}}
	if err := o.restoreFromStartTag("gen-start-exist"); err != nil {
		t.Fatalf("restoreFromStartTag: %v", err)
	}

	// File should keep its modified content (not overwritten).
	got, err := os.ReadFile(filepath.Join("pkg", "existing.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package pkg\n// modified\n" {
		t.Errorf("existing file was overwritten: got %q", string(got))
	}
}

func TestRestoreFromStartTag_SkipsNonGoFiles(t *testing.T) {
	initTestGitRepo(t)

	// Create a non-Go file, commit, and tag.
	os.WriteFile("readme.md", []byte("# Hello\n"), 0o644)
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add readme")
	gitRun(t, "tag", "gen-start-nongo")

	// Delete it so restore would try to bring it back.
	os.Remove("readme.md")

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{MagefilesDir: "magefiles"},
	}}
	if err := o.restoreFromStartTag("gen-start-nongo"); err != nil {
		t.Fatalf("restoreFromStartTag: %v", err)
	}

	// Non-Go file should NOT have been restored.
	if _, err := os.Stat("readme.md"); err == nil {
		t.Error("readme.md was restored, but non-Go files should be skipped")
	}
}

func TestCleanupUnmergedTags_NoTags(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-"},
	}}
	// Must not panic with no tags.
	o.cleanupUnmergedTags()
}

func TestCleanupUnmergedTags_AllMerged(t *testing.T) {
	initTestGitRepo(t)

	// Create tags that look merged (has -merged suffix).
	gitRun(t, "tag", "generation-2026-01-01-start")
	gitRun(t, "tag", "generation-2026-01-01-finished")
	gitRun(t, "tag", "generation-2026-01-01-merged")

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-"},
	}}
	o.cleanupUnmergedTags()

	// All tags should still exist (nothing abandoned).
	tags := gitListTags("generation-*", ".")
	if len(tags) != 3 {
		t.Errorf("expected 3 tags after cleanup of all-merged, got %d: %v", len(tags), tags)
	}
}

func TestListGenerationBranches_CustomPrefix(t *testing.T) {
	initTestGitRepo(t)

	gitRun(t, "branch", "myprefix-2026-01-01")
	gitRun(t, "branch", "myprefix-2026-01-02")
	gitRun(t, "branch", "other-branch")

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "myprefix-"},
	}}
	branches := o.listGenerationBranches()

	if len(branches) != 2 {
		t.Errorf("expected 2 branches with prefix myprefix-, got %d: %v", len(branches), branches)
	}
}

func TestGenerationName_NoSuffix(t *testing.T) {
	t.Parallel()
	got := generationName("generation-2026-01-01")
	if got != "generation-2026-01-01" {
		t.Errorf("generationName without suffix = %q, want unchanged", got)
	}
}

func TestRestoreFromStartTag_SkipsMagefiles(t *testing.T) {
	initTestGitRepo(t)

	// Commit a magefiles .go file and tag that commit as the generation start.
	if err := os.MkdirAll("magefiles", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("magefiles", "build.go"), []byte("package magefiles\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, "add", "-A")
	gitRun(t, "commit", "--no-verify", "-m", "add magefiles/build.go")
	gitRun(t, "tag", "gen-start-mf")

	// Delete the file so restoreFromStartTag would try to restore it if not skipped.
	if err := os.Remove(filepath.Join("magefiles", "build.go")); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{
		Project: ProjectConfig{MagefilesDir: "magefiles"},
	}}
	if err := o.restoreFromStartTag("gen-start-mf"); err != nil {
		t.Fatalf("restoreFromStartTag: %v", err)
	}

	// magefiles/build.go must NOT have been restored.
	if _, err := os.Stat(filepath.Join("magefiles", "build.go")); err == nil {
		t.Error("magefiles/build.go was restored, but should have been skipped")
	}
}

// --- resolveStopTarget (pure, parallelizable) ---

func TestResolveStopTarget_CallerOnFeatureBranch_UsesFeatureBranch(t *testing.T) {
	t.Parallel()
	// Caller checked out a feature branch before running generator:stop.
	// Expected: merge into the feature branch, not the recorded base.
	got := resolveStopTarget("gh-168-feature", "generation-2026-03-03", "main")
	if got != "gh-168-feature" {
		t.Errorf("resolveStopTarget = %q, want %q", got, "gh-168-feature")
	}
}

func TestResolveStopTarget_CallerOnRecordedBase_UsesRecordedBase(t *testing.T) {
	t.Parallel()
	// Caller is on the recorded base branch (normal workflow, no change).
	got := resolveStopTarget("main", "generation-2026-03-03", "main")
	if got != "main" {
		t.Errorf("resolveStopTarget = %q, want %q", got, "main")
	}
}

func TestResolveStopTarget_CallerOnGenBranch_UsesRecordedBase(t *testing.T) {
	t.Parallel()
	// Caller is already on the generation branch itself; fall back to recorded base.
	got := resolveStopTarget("generation-2026-03-03", "generation-2026-03-03", "main")
	if got != "main" {
		t.Errorf("resolveStopTarget = %q, want %q", got, "main")
	}
}

func TestResolveStopTarget_CustomRecordedBase_FeatureBranchOverrides(t *testing.T) {
	t.Parallel()
	// Project uses a custom base branch (develop) and caller is on a feature branch.
	got := resolveStopTarget("gh-42-my-fix", "generation-2026-03-03", "develop")
	if got != "gh-42-my-fix" {
		t.Errorf("resolveStopTarget = %q, want %q", got, "gh-42-my-fix")
	}
}

func TestResolveStopTarget_CallerOnCustomBase_UsesCustomBase(t *testing.T) {
	t.Parallel()
	// Caller is on the custom base branch itself; no feature branch override.
	got := resolveStopTarget("develop", "generation-2026-03-03", "develop")
	if got != "develop" {
		t.Errorf("resolveStopTarget = %q, want %q", got, "develop")
	}
}

// --- validateAndMarkUCs (GH-1361, replaces GH-1187 markActiveReleaseUCsDone) ---

func TestValidateAndMarkUCs_OnlyMarksUCsWithCompleteRequirements(t *testing.T) {
	dir := initTestGitRepo(t)

	roadmapContent := `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: in_progress
    use_cases:
      - id: rel00.0-uc001-format
        status: spec_complete
        summary: Format output
      - id: rel00.0-uc002-build
        status: spec_complete
        summary: Build pipeline
  - version: "01.0"
    name: Release 1
    status: pending
    use_cases:
      - id: rel01.0-uc001-ext
        status: spec_complete
        summary: Extension
`
	writeRoadmapFile(t, dir, roadmapContent)

	// Create go.mod so `go test` can run in this directory.
	goMod := "module testmod\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create use case files with touchpoints.
	ucDir := filepath.Join(dir, "docs", "specs", "use-cases")
	if err := os.MkdirAll(ucDir, 0o755); err != nil {
		t.Fatal(err)
	}
	uc001 := `id: rel00.0-uc001-format
title: Format output
touchpoints:
  - T1: "Format per prd001-core R1"
`
	uc002 := `id: rel00.0-uc002-build
title: Build pipeline
touchpoints:
  - T1: "Build per prd002-build R1"
`
	os.WriteFile(filepath.Join(ucDir, "rel00.0-uc001-format.yaml"), []byte(uc001), 0o644)
	os.WriteFile(filepath.Join(ucDir, "rel00.0-uc002-build.yaml"), []byte(uc002), 0o644)

	// Create requirements.yaml: uc001's PRD is complete, uc002's is not.
	cobblerDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(cobblerDir, 0o755)
	reqContent := `requirements:
  prd001-core:
    R1.1:
      status: complete
      issue: 10
    R1.2:
      status: complete
      issue: 11
  prd002-build:
    R1.1:
      status: ready
    R1.2:
      status: complete
      issue: 12
`
	os.WriteFile(filepath.Join(cobblerDir, "requirements.yaml"), []byte(reqContent), 0o644)

	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: cobblerDir}}}
	o.validateAndMarkUCs()

	rmPath := filepath.Join(dir, "docs", "road-map.yaml")
	statuses, err := roadmapUCStatuses(rmPath, "00.0")
	if err != nil {
		t.Fatalf("read UC statuses for 00.0: %v", err)
	}
	// uc001 has all requirements complete → should be implemented.
	if statuses["rel00.0-uc001-format"] != "implemented" {
		t.Errorf("UC rel00.0-uc001-format: want implemented, got %q", statuses["rel00.0-uc001-format"])
	}
	// uc002 has pending requirements → should remain spec_complete.
	if statuses["rel00.0-uc002-build"] != "spec_complete" {
		t.Errorf("UC rel00.0-uc002-build: want spec_complete, got %q", statuses["rel00.0-uc002-build"])
	}

	// Verify 01.0 is unchanged.
	statuses01, err := roadmapUCStatuses(rmPath, "01.0")
	if err != nil {
		t.Fatalf("read UC statuses for 01.0: %v", err)
	}
	for id, status := range statuses01 {
		if status != "spec_complete" {
			t.Errorf("UC %s (rel 01.0): want spec_complete, got %q", id, status)
		}
	}
}

func TestValidateAndMarkUCs_SkipsAlreadyImplemented(t *testing.T) {
	dir := initTestGitRepo(t)

	roadmapContent := `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: implemented
    use_cases:
      - id: rel00.0-uc001-format
        status: implemented
        summary: Format output
  - version: "01.0"
    name: Release 1
    status: in_progress
    use_cases:
      - id: rel01.0-uc001-ext
        status: spec_complete
        summary: Extension
`
	writeRoadmapFile(t, dir, roadmapContent)

	o := &Orchestrator{cfg: Config{}}
	o.validateAndMarkUCs()

	// 01.0 UC has no tests → should remain spec_complete (not blindly marked).
	rmPath := filepath.Join(dir, "docs", "road-map.yaml")
	statuses, err := roadmapUCStatuses(rmPath, "01.0")
	if err != nil {
		t.Fatalf("read UC statuses for 01.0: %v", err)
	}
	for id, status := range statuses {
		if status != "spec_complete" {
			t.Errorf("UC %s (rel 01.0): want spec_complete, got %q", id, status)
		}
	}
}

func TestValidateAndMarkUCs_NoRoadmap(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{}}
	// Should be a no-op, no panic.
	o.validateAndMarkUCs()
}

// --- checkAutoAdvanceRelease (GH-1006) ---

func TestCheckAutoAdvanceRelease_AllUCsDone(t *testing.T) {
	dir := initTestGitRepo(t)

	// Write a roadmap where release 00.0 has all UCs done.
	roadmapContent := `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: in_progress
    use_cases:
      - id: rel00.0-uc001-format
        status: done
        summary: Format output
      - id: rel00.0-uc002-build
        status: done
        summary: Build pipeline
  - version: "01.0"
    name: Release 1
    status: pending
    use_cases:
      - id: rel01.0-uc001-ext
        status: spec_complete
        summary: Extension
`
	writeRoadmapFile(t, dir, roadmapContent)
	cfgPath := filepath.Join(dir, DefaultConfigFile)
	writeConfigFile(t, cfgPath, []string{"00.0", "01.0"})

	o := &Orchestrator{cfg: Config{}}
	advanced, ver := o.checkAutoAdvanceRelease()
	if !advanced {
		t.Fatal("expected auto-advance, got false")
	}
	if ver != "00.0" {
		t.Errorf("advanced version = %q, want %q", ver, "00.0")
	}

	// Verify road-map.yaml: release 00.0 UCs should be "implemented".
	statuses, err := roadmapUCStatuses(filepath.Join(dir, "docs", "road-map.yaml"), "00.0")
	if err != nil {
		t.Fatalf("read statuses: %v", err)
	}
	for id, status := range statuses {
		if status != "implemented" {
			t.Errorf("UC %s: want implemented, got %q", id, status)
		}
	}

	// Verify configuration.yaml: "00.0" should be removed.
	versions, err := releaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, v := range versions {
		if v == "00.0" {
			t.Error("00.0 should have been removed from config releases")
		}
	}
}

func TestCheckAutoAdvanceRelease_NotAllDone(t *testing.T) {
	dir := initTestGitRepo(t)

	// Release 00.0 has one pending UC.
	writeRoadmapFile(t, dir, sampleRoadmap)
	cfgPath := filepath.Join(dir, DefaultConfigFile)
	writeConfigFile(t, cfgPath, []string{"00.0"})

	o := &Orchestrator{cfg: Config{}}
	advanced, ver := o.checkAutoAdvanceRelease()
	if advanced {
		t.Errorf("should not advance when UCs are pending, got version=%q", ver)
	}
}

func TestCheckAutoAdvanceRelease_NoRoadmap(t *testing.T) {
	_ = chdirTemp(t)

	o := &Orchestrator{cfg: Config{}}
	advanced, _ := o.checkAutoAdvanceRelease()
	if advanced {
		t.Error("should not advance when no road-map.yaml exists")
	}
}

func TestCheckAutoAdvanceRelease_AllReleasesAlreadyDone(t *testing.T) {
	dir := initTestGitRepo(t)

	roadmapContent := `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: done
    use_cases:
      - id: rel00.0-uc001-format
        status: done
        summary: Format output
`
	writeRoadmapFile(t, dir, roadmapContent)

	o := &Orchestrator{cfg: Config{}}
	advanced, _ := o.checkAutoAdvanceRelease()
	if advanced {
		t.Error("should not advance when all releases already done")
	}
}

// --- resetImplementedReleases (GH-1021) ---

func TestResetImplementedReleases(t *testing.T) {
	dir := initTestGitRepo(t)

	roadmapContent := `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: implemented
    use_cases:
      - id: rel00.0-uc001-format
        status: implemented
        summary: Format output
  - version: "01.0"
    name: Release 1
    status: implemented
    use_cases:
      - id: rel01.0-uc001-ext
        status: implemented
        summary: Extension
  - version: "02.0"
    name: Release 2
    status: spec_complete
    use_cases:
      - id: rel02.0-uc001-tee
        status: spec_complete
        summary: Tee
`
	writeRoadmapFile(t, dir, roadmapContent)
	cfgPath := filepath.Join(dir, DefaultConfigFile)
	writeConfigFile(t, cfgPath, []string{"02.0"})

	o := &Orchestrator{cfg: Config{}}
	if err := o.resetImplementedReleases(); err != nil {
		t.Fatalf("resetImplementedReleases error: %v", err)
	}

	// Verify 00.0 and 01.0 are reset to spec_complete (both release and UC level).
	rmPath := filepath.Join(dir, "docs", "road-map.yaml")
	for _, ver := range []string{"00.0", "01.0"} {
		relStatus, err := roadmapReleaseStatus(rmPath, ver)
		if err != nil {
			t.Fatalf("read release status for %s: %v", ver, err)
		}
		if relStatus != "spec_complete" {
			t.Errorf("release %s status: want spec_complete, got %q", ver, relStatus)
		}
		statuses, err := roadmapUCStatuses(rmPath, ver)
		if err != nil {
			t.Fatalf("read UC statuses for %s: %v", ver, err)
		}
		for id, status := range statuses {
			if status != "spec_complete" {
				t.Errorf("UC %s (rel %s): want spec_complete, got %q", id, ver, status)
			}
		}
	}

	// Verify 02.0 is unchanged (both release and UC level).
	relStatus02, err := roadmapReleaseStatus(rmPath, "02.0")
	if err != nil {
		t.Fatalf("read release status for 02.0: %v", err)
	}
	if relStatus02 != "spec_complete" {
		t.Errorf("release 02.0 status: want spec_complete, got %q", relStatus02)
	}
	statuses, err := roadmapUCStatuses(rmPath, "02.0")
	if err != nil {
		t.Fatalf("read statuses for 02.0: %v", err)
	}
	for id, status := range statuses {
		if status != "spec_complete" {
			t.Errorf("UC %s (rel 02.0): want spec_complete, got %q", id, status)
		}
	}

	// Verify configuration.yaml has 00.0 and 01.0 added to releases.
	versions, err := releaseVersionsFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	found := map[string]bool{}
	for _, v := range versions {
		found[v] = true
	}
	if !found["00.0"] {
		t.Error("00.0 should be in config releases after reset")
	}
	if !found["01.0"] {
		t.Error("01.0 should be in config releases after reset")
	}
}

func TestResetImplementedReleases_NoneImplemented(t *testing.T) {
	dir := initTestGitRepo(t)

	roadmapContent := `id: rm1
title: Test Roadmap
releases:
  - version: "00.0"
    name: Release 0
    status: spec_complete
    use_cases:
      - id: rel00.0-uc001-format
        status: spec_complete
        summary: Format
`
	writeRoadmapFile(t, dir, roadmapContent)

	o := &Orchestrator{cfg: Config{}}
	if err := o.resetImplementedReleases(); err != nil {
		t.Fatalf("resetImplementedReleases error: %v", err)
	}
	// No error, no commit — just a no-op.
}

// TestGeneratorStop_CommitsHistoryBeforeTag verifies that uncommitted history
// files (e.g. orchestrator logs) are committed before the -finished tag so
// they are preserved for post-hoc analysis (GH-1452).
func TestGeneratorStop_CommitsHistoryBeforeTag(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := initTestGitRepo(t)

	// Create .cobbler/history with a committed stats file.
	cobblerDir := filepath.Join(dir, ".cobbler")
	histDir := filepath.Join(cobblerDir, "history")
	os.MkdirAll(histDir, 0o755)
	os.WriteFile(filepath.Join(histDir, "stitch-stats.yaml"), []byte("caller: stitch\n"), 0o644)

	// Write base-branch so GeneratorStop knows where to merge.
	os.WriteFile(filepath.Join(cobblerDir, "base-branch"), []byte("main\n"), 0o644)

	// Commit everything on main, then create a generation branch.
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "add cobbler dir")
	cmd.Dir = dir
	cmd.CombinedOutput()

	cmd = exec.Command("git", "checkout", "-b", "generation-test")
	cmd.Dir = dir
	cmd.CombinedOutput()

	// Add a committed file on the generation branch.
	os.WriteFile(filepath.Join(histDir, "measure-stats.yaml"), []byte("caller: measure\n"), 0o644)
	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "generation work")
	cmd.Dir = dir
	cmd.CombinedOutput()

	// Add an UNCOMMITTED orchestrator log — this is the file GH-1452 is about.
	os.WriteFile(filepath.Join(histDir, "stitch-orchestrator.log"), []byte("orchestrator log content\n"), 0o644)

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{
			Prefix:          "generation-",
			Branch:          "generation-test",
			PreserveSources: true,
		},
		Cobbler: CobblerConfig{Dir: ".cobbler", HistoryDir: "history"},
	}}

	err := o.GeneratorStop()
	if err != nil {
		t.Fatalf("GeneratorStop error: %v", err)
	}

	// The -finished tag should contain the orchestrator log.
	out, err := exec.Command("git", "show", "generation-test-finished:.cobbler/history/stitch-orchestrator.log").CombinedOutput()
	if err != nil {
		t.Fatalf("orchestrator log not in -finished tag: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "orchestrator log content") {
		t.Errorf("orchestrator log content mismatch: %s", out)
	}

	// The committed stats file should also be in the tag.
	out, err = exec.Command("git", "show", "generation-test-finished:.cobbler/history/measure-stats.yaml").CombinedOutput()
	if err != nil {
		t.Fatalf("measure-stats.yaml not in -finished tag: %v\n%s", err, out)
	}
}

// TestResetImplementedReleases_RevertsUCStatuses verifies that individual
// UC statuses are reverted to spec_complete even when the release itself
// is not yet marked implemented (GH-1469).
func TestResetImplementedReleases_RevertsUCStatuses(t *testing.T) {
	// Uses os.Chdir — do NOT use t.Parallel()
	dir := initTestGitRepo(t)

	// Create road-map.yaml with a release that has some UCs implemented
	// but the release itself is still spec_complete.
	docsDir := filepath.Join(dir, "docs")
	os.MkdirAll(docsDir, 0o755)
	roadmap := `releases:
  - version: "00.0"
    name: "Foundation"
    status: spec_complete
    use_cases:
      - id: rel00.0-uc001-format
        status: implemented
      - id: rel00.0-uc002-sys
        status: implemented
      - id: rel00.0-uc003-testutils
        status: spec_complete
`
	os.WriteFile(filepath.Join(docsDir, "road-map.yaml"), []byte(roadmap), 0o644)

	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "add roadmap")
	cmd.Dir = dir
	cmd.CombinedOutput()

	o := &Orchestrator{cfg: Config{}}
	if err := o.resetImplementedReleases(); err != nil {
		t.Fatalf("resetImplementedReleases error: %v", err)
	}

	// Read the road-map.yaml and verify UC statuses were reverted.
	data, err := os.ReadFile(filepath.Join(docsDir, "road-map.yaml"))
	if err != nil {
		t.Fatalf("cannot read road-map.yaml: %v", err)
	}
	content := string(data)

	// Both implemented UCs should now be spec_complete.
	// The file should not contain "implemented" for any UC.
	if strings.Contains(content, "implemented") {
		t.Errorf("road-map.yaml still contains 'implemented' after reset:\n%s", content)
	}
}

// TestStripGoModRequires verifies that require and replace blocks are
// removed from go.mod, keeping only module and go version (GH-1468).
func TestStripGoModRequires(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gomod := filepath.Join(dir, "go.mod")

	content := `module example.com/test

go 1.25.7

require (
	github.com/stretchr/testify v1.9.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/single/dep v1.0.0

replace github.com/foo/bar => ./bar
`
	os.WriteFile(gomod, []byte(content), 0o644)

	stripGoModRequires(gomod)

	data, err := os.ReadFile(gomod)
	if err != nil {
		t.Fatalf("cannot read go.mod: %v", err)
	}
	result := string(data)

	if strings.Contains(result, "require") {
		t.Errorf("go.mod still contains require after strip:\n%s", result)
	}
	if strings.Contains(result, "replace") {
		t.Errorf("go.mod still contains replace after strip:\n%s", result)
	}
	if !strings.Contains(result, "module example.com/test") {
		t.Errorf("go.mod missing module line:\n%s", result)
	}
	if !strings.Contains(result, "go 1.25.7") {
		t.Errorf("go.mod missing go version:\n%s", result)
	}
}

func TestStripGoModRequires_NoFile(t *testing.T) {
	t.Parallel()
	// Should not panic on missing file.
	stripGoModRequires("/nonexistent/go.mod")
}

func TestResetImplementedReleases_NoRoadmap(t *testing.T) {
	initTestGitRepo(t)

	o := &Orchestrator{cfg: Config{}}
	if err := o.resetImplementedReleases(); err != nil {
		t.Fatalf("resetImplementedReleases error: %v", err)
	}
}

// --- hasUnresolvedRequirements (GH-1475) ---

func TestHasUnresolvedRequirements_ReadyItems(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".cobbler"), 0o755)
	reqYAML := `requirements:
  prd001-core:
    R1.1:
      status: complete
      issue: 10
    R1.2:
      status: ready
`
	os.WriteFile(filepath.Join(dir, ".cobbler", "requirements.yaml"), []byte(reqYAML), 0o644)

	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: filepath.Join(dir, ".cobbler")}}}
	if !o.hasUnresolvedRequirements() {
		t.Error("expected true: R1.2 is still ready")
	}
}

func TestHasUnresolvedRequirements_AllComplete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".cobbler"), 0o755)
	reqYAML := `requirements:
  prd001-core:
    R1.1:
      status: complete
      issue: 10
    R1.2:
      status: skip
`
	os.WriteFile(filepath.Join(dir, ".cobbler", "requirements.yaml"), []byte(reqYAML), 0o644)

	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: filepath.Join(dir, ".cobbler")}}}
	if o.hasUnresolvedRequirements() {
		t.Error("expected false: all items are complete or skip")
	}
}

func TestHasUnresolvedRequirements_NoFile(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: "/nonexistent"}}}
	if o.hasUnresolvedRequirements() {
		t.Error("expected false: no requirements file")
	}
}
