// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- DetectBinaryName ---

func TestDetectBinaryName_LastSegment(t *testing.T) {
	cases := []struct {
		module string
		want   string
	}{
		{"github.com/org/myproject", "myproject"},
		{"github.com/org/my-tool", "my-tool"},
		{"example.com/foo/bar/baz", "baz"},
		{"singleword", "singleword"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := DetectBinaryName(tc.module); got != tc.want {
			t.Errorf("DetectBinaryName(%q) = %q, want %q", tc.module, got, tc.want)
		}
	}
}

// --- DetectModulePath ---

func TestDetectModulePath_ReadsGoMod(t *testing.T) {
	dir := t.TempDir()
	gomod := "module github.com/org/repo\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DetectModulePath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "github.com/org/repo" {
		t.Errorf("got %q, want %q", got, "github.com/org/repo")
	}
}

func TestDetectModulePath_MissingGoMod(t *testing.T) {
	_, err := DetectModulePath(t.TempDir())
	if err == nil {
		t.Error("expected error for missing go.mod, got nil")
	}
}

func TestDetectModulePath_NoModuleDirective(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := DetectModulePath(dir)
	if err == nil {
		t.Error("expected error for go.mod without module directive, got nil")
	}
}

// --- DetectMainPackage ---

func TestDetectMainPackage_CmdSubdir(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "cmd", "myapp")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := DetectMainPackage(dir, "github.com/org/repo")
	if got != "github.com/org/repo/cmd/myapp" {
		t.Errorf("got %q, want %q", got, "github.com/org/repo/cmd/myapp")
	}
}

func TestDetectMainPackage_CmdDirect(t *testing.T) {
	dir := t.TempDir()
	cmdDir := filepath.Join(dir, "cmd")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := DetectMainPackage(dir, "github.com/org/repo")
	if got != "github.com/org/repo/cmd" {
		t.Errorf("got %q, want %q", got, "github.com/org/repo/cmd")
	}
}

func TestDetectMainPackage_NoCmdDir(t *testing.T) {
	got := DetectMainPackage(t.TempDir(), "github.com/org/repo")
	if got != "" {
		t.Errorf("got %q, want empty string when no cmd/ exists", got)
	}
}

func TestDetectMainPackage_CmdDirNoMainGo(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "cmd", "app")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got := DetectMainPackage(dir, "github.com/org/repo")
	if got != "" {
		t.Errorf("got %q, want empty string when cmd/app/ has no main.go", got)
	}
}

// --- DetectSourceDirs ---

func TestDetectSourceDirs_ReturnsExisting(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"cmd/", "pkg/"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := DetectSourceDirs(dir)
	if len(got) != 2 {
		t.Fatalf("got %v, want [cmd/ pkg/]", got)
	}
	if got[0] != "cmd/" || got[1] != "pkg/" {
		t.Errorf("got %v, want [cmd/ pkg/]", got)
	}
}

func TestDetectSourceDirs_NoneExist(t *testing.T) {
	got := DetectSourceDirs(t.TempDir())
	if len(got) != 0 {
		t.Errorf("got %v, want empty slice when no source dirs exist", got)
	}
}

// --- ClearMageGoFiles ---

func TestClearMageGoFiles_RemovesGoFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "go.mod", "go.sum", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ClearMageGoFiles(dir); err != nil {
		t.Fatalf("ClearMageGoFiles: %v", err)
	}
	for _, name := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s should have been removed", name)
		}
	}
	for _, name := range []string{"go.mod", "go.sum", "README.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should still exist: %v", name, err)
		}
	}
}

func TestClearMageGoFiles_MissingDir_IsNoOp(t *testing.T) {
	err := ClearMageGoFiles(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Errorf("ClearMageGoFiles on missing dir should be no-op, got: %v", err)
	}
}

// --- RemoveIfExists ---

func TestRemoveIfExists_RemovesFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveIfExists(f); err != nil {
		t.Fatalf("RemoveIfExists: %v", err)
	}
	if _, err := os.Stat(f); err == nil {
		t.Error("file should have been removed")
	}
}

func TestRemoveIfExists_MissingFile_IsNoOp(t *testing.T) {
	err := RemoveIfExists(filepath.Join(t.TempDir(), "nonexistent.txt"))
	if err != nil {
		t.Errorf("RemoveIfExists on missing file should be no-op, got: %v", err)
	}
}

// --- ScaffoldSeedTemplate ---

func TestScaffoldSeedTemplate_CreatesFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	destPath, tmplPath, err := ScaffoldSeedTemplate(dir, "github.com/org/repo", "github.com/org/repo/cmd/app", "magefiles")
	if err != nil {
		t.Fatalf("ScaffoldSeedTemplate: %v", err)
	}

	if destPath != "cmd/app/version.go" {
		t.Errorf("destPath = %q, want cmd/app/version.go", destPath)
	}
	if tmplPath != "magefiles/version.go.tmpl" {
		t.Errorf("tmplPath = %q, want magefiles/version.go.tmpl", tmplPath)
	}

	absPath := filepath.Join(dir, tmplPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "package main") {
		t.Error("template missing 'package main'")
	}
	if !strings.Contains(content, "{{.Version}}") {
		t.Error("template missing Version placeholder")
	}
	if strings.Contains(content, "func main") {
		t.Error("template must not contain func main() — version.go is constants-only")
	}
}

func TestScaffoldSeedTemplate_RootMainPkg(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	destPath, _, err := ScaffoldSeedTemplate(dir, "github.com/org/tool", "github.com/org/tool", "magefiles")
	if err != nil {
		t.Fatalf("ScaffoldSeedTemplate: %v", err)
	}
	if destPath != "version.go" {
		t.Errorf("destPath = %q, want version.go for root main pkg", destPath)
	}
}

// --- CopyFile ---

func TestCopyFile_Success(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "src.txt")
	os.WriteFile(src, []byte("hello"), 0o644)

	dst := filepath.Join(t.TempDir(), "sub", "dir", "dst.txt")
	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want hello", got)
	}
}

func TestCopyFile_MissingSrc(t *testing.T) {
	t.Parallel()
	dst := filepath.Join(t.TempDir(), "dst.txt")
	if err := CopyFile("/nonexistent/file.txt", dst); err == nil {
		t.Error("expected error for missing source")
	}
}

// --- CopyDir ---

func TestCopyDir_CopiesRecursively(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("root"), 0o644)
	os.WriteFile(filepath.Join(src, "a", "mid.txt"), []byte("mid"), 0o644)
	os.WriteFile(filepath.Join(src, "a", "b", "deep.txt"), []byte("deep"), 0o644)

	dst := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	for _, rel := range []string{"root.txt", "a/mid.txt", "a/b/deep.txt"} {
		path := filepath.Join(dst, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", rel)
		}
	}
	got, _ := os.ReadFile(filepath.Join(dst, "a", "b", "deep.txt"))
	if string(got) != "deep" {
		t.Errorf("deep.txt content = %q, want deep", got)
	}
}

func TestCopyDir_EmptySrc(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Errorf("expected empty dst, got %d entries", len(entries))
	}
}

func TestCopyDir_PreservesContent(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(src, "b.txt"), []byte("beta"), 0o644)

	dst := filepath.Join(t.TempDir(), "out")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	for _, tc := range []struct{ name, want string }{
		{"a.txt", "alpha"},
		{"b.txt", "beta"},
	} {
		got, err := os.ReadFile(filepath.Join(dst, tc.name))
		if err != nil {
			t.Errorf("ReadFile(%s): %v", tc.name, err)
		} else if string(got) != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}
