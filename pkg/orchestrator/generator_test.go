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

// --- generationDate (pure, parallelizable) ---

func TestGenerationDate_ValidBranch(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationDate("generation-2026-02-12-07-13-55")
	want := "2026-02-12"
	if got != want {
		t.Errorf("generationDate() = %q, want %q", got, want)
	}
}

func TestGenerationDate_NoPrefix(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationDate("main")
	if got != "" {
		t.Errorf("generationDate(main) = %q, want empty", got)
	}
}

func TestGenerationDate_ShortRest(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationDate("generation-20")
	if got != "" {
		t.Errorf("generationDate(short) = %q, want empty", got)
	}
}

func TestGenerationDate_CustomPrefix(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "gen-"}}}
	got := o.generationDate("gen-2026-03-01-12-00-00")
	want := "2026-03-01"
	if got != want {
		t.Errorf("generationDate() = %q, want %q", got, want)
	}
}

// --- generationDateCompact (pure, parallelizable) ---

func TestGenerationDateCompact_Valid(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationDateCompact("generation-2026-02-12-07-13-55")
	want := "20260212"
	if got != want {
		t.Errorf("generationDateCompact() = %q, want %q", got, want)
	}
}

func TestGenerationDateCompact_Invalid(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationDateCompact("main")
	if got != "" {
		t.Errorf("generationDateCompact(main) = %q, want empty", got)
	}
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

	current, err := gitCurrentBranch()
	if err != nil {
		t.Fatal(err)
	}

	if err := ensureOnBranch(current); err != nil {
		t.Errorf("ensureOnBranch(current) error = %v", err)
	}
}

func TestEnsureOnBranch_SwitchesBranch(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("test-branch"); err != nil {
		t.Fatal(err)
	}

	if err := ensureOnBranch("test-branch"); err != nil {
		t.Fatalf("ensureOnBranch(test-branch) error = %v", err)
	}

	current, err := gitCurrentBranch()
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

	current, _ := gitCurrentBranch()
	if err := saveAndSwitchBranch(current); err != nil {
		t.Errorf("saveAndSwitchBranch(current) error = %v", err)
	}
}

func TestSaveAndSwitchBranch_CleanSwitch(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("target"); err != nil {
		t.Fatal(err)
	}

	if err := saveAndSwitchBranch("target"); err != nil {
		t.Fatalf("saveAndSwitchBranch(target) error = %v", err)
	}

	current, err := gitCurrentBranch()
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

	if err := gitCreateBranch("other"); err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified"), 0o644)

	if err := saveAndSwitchBranch("other"); err != nil {
		t.Fatalf("saveAndSwitchBranch with dirty tree error = %v", err)
	}

	current, err := gitCurrentBranch()
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

	if err := gitCreateBranch("explicit-branch"); err != nil {
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

	current, _ := gitCurrentBranch()
	if got != current {
		t.Errorf("resolveBranch() = %q, want current branch %q", got, current)
	}
}

func TestResolveBranch_SingleGenerationBranch(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCreateBranch("generation-2026-02-28-12-00-00"); err != nil {
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

	gitCreateBranch("generation-2026-02-28-12-00-00")
	gitCreateBranch("generation-2026-02-28-13-00-00")

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

	gitCreateBranch("generation-2026-02-28-12-00-00")
	gitCreateBranch("generation-2026-02-28-13-00-00")
	gitCreateBranch("other-branch")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.listGenerationBranches()
	if len(got) != 2 {
		t.Errorf("listGenerationBranches() returned %d branches, want 2: %v", len(got), got)
	}
}

// --- generationRevision (git, NOT parallel) ---

func TestGenerationRevision_SingleGeneration(t *testing.T) {
	initTestGitRepo(t)

	branch := "generation-2026-02-28-12-00-00"
	if err := gitCreateBranch(branch); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationRevision(branch)
	if got != 0 {
		t.Errorf("generationRevision() = %d, want 0", got)
	}
}

func TestGenerationRevision_MultipleGenerationsSameDay(t *testing.T) {
	initTestGitRepo(t)

	gitCreateBranch("generation-2026-02-28-10-00-00")
	gitCreateBranch("generation-2026-02-28-12-00-00")
	gitCreateBranch("generation-2026-02-28-14-00-00")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}

	got := o.generationRevision("generation-2026-02-28-10-00-00")
	if got != 0 {
		t.Errorf("generationRevision(10-00-00) = %d, want 0", got)
	}

	got = o.generationRevision("generation-2026-02-28-12-00-00")
	if got != 1 {
		t.Errorf("generationRevision(12-00-00) = %d, want 1", got)
	}

	got = o.generationRevision("generation-2026-02-28-14-00-00")
	if got != 2 {
		t.Errorf("generationRevision(14-00-00) = %d, want 2", got)
	}
}

func TestGenerationRevision_InvalidBranch(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	got := o.generationRevision("main")
	if got != 0 {
		t.Errorf("generationRevision(main) = %d, want 0", got)
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
	if err := gitCheckoutNew(branch); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: branch},
	}}

	if err := o.GeneratorSwitch(); err != nil {
		t.Errorf("GeneratorSwitch() already on branch error = %v", err)
	}

	current, _ := gitCurrentBranch()
	if current != branch {
		t.Errorf("current branch = %q, want %q", current, branch)
	}
}

func TestGeneratorSwitch_SwitchToMain(t *testing.T) {
	initTestGitRepo(t)

	if err := gitCheckoutNew("generation-2026-02-28-12-00-00"); err != nil {
		t.Fatal(err)
	}

	o := &Orchestrator{cfg: Config{
		Generation: GenerationConfig{Prefix: "generation-", Branch: "main"},
	}}

	if err := o.GeneratorSwitch(); err != nil {
		t.Errorf("GeneratorSwitch() to main error = %v", err)
	}

	current, _ := gitCurrentBranch()
	if current != "main" {
		t.Errorf("current branch = %q, want %q", current, "main")
	}
}

// --- cleanupUnmergedTags (git, NOT parallel) ---

func TestCleanupUnmergedTags_MergedNotTouched(t *testing.T) {
	initTestGitRepo(t)

	gitTag("generation-2026-02-28-12-00-00-start")
	gitTag("generation-2026-02-28-12-00-00-finished")
	gitTag("generation-2026-02-28-12-00-00-merged")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	o.cleanupUnmergedTags()

	tags := gitListTags("generation-2026-02-28-12-00-00-*")
	if len(tags) != 3 {
		t.Errorf("expected 3 tags after cleanup, got %d: %v", len(tags), tags)
	}
}

func TestCleanupUnmergedTags_UnmergedAbandoned(t *testing.T) {
	initTestGitRepo(t)

	gitTag("generation-2026-02-28-12-00-00-start")
	gitTag("generation-2026-02-28-12-00-00-finished")

	o := &Orchestrator{cfg: Config{Generation: GenerationConfig{Prefix: "generation-"}}}
	o.cleanupUnmergedTags()

	tags := gitListTags("generation-2026-02-28-12-00-00-*")
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

// --- Init (pure, parallelizable) ---

func TestInit_NoOp(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{}}
	if err := o.Init(); err != nil {
		t.Errorf("Init() error = %v", err)
	}
}
