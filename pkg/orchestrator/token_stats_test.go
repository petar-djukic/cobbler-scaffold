// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// --- sortedKeys ---

func TestSortedKeys_Empty(t *testing.T) {
	t.Parallel()
	got := st.SortedKeys(map[string]int{})
	if len(got) != 0 {
		t.Errorf("st.SortedKeys(empty) = %v, want []", got)
	}
}

func TestSortedKeys_Sorted(t *testing.T) {
	t.Parallel()
	got := st.SortedKeys(map[string]int{"c": 3, "a": 1, "b": 2})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("sortedKeys len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("sortedKeys[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestSortedKeys_SingleKey(t *testing.T) {
	t.Parallel()
	got := st.SortedKeys(map[string]int{"only": 42})
	if len(got) != 1 || got[0] != "only" {
		t.Errorf("st.SortedKeys(single) = %v, want [only]", got)
	}
}

// --- enumerateContextFiles ---

func TestEnumerateContextFiles_IncludesSourceFiles(t *testing.T) {
	t.Parallel()
	// Create a temp source directory with a .go file.
	srcDir := t.TempDir()
	goFile := filepath.Join(srcDir, "main.go")
	os.WriteFile(goFile, []byte("package main\n"), 0644)

	o := New(Config{
		Project: ProjectConfig{
			GoSourceDirs: []string{srcDir},
		},
	})

	files := o.enumerateContextFiles()

	var found bool
	for _, f := range files {
		if f.Category == "source" && strings.HasSuffix(f.Path, "main.go") {
			found = true
			if f.Bytes <= 0 {
				t.Errorf("source file %q has bytes=%d, want > 0", f.Path, f.Bytes)
			}
		}
	}
	if !found {
		t.Error("expected main.go to appear as category=source in enumerateContextFiles")
	}
}

func TestEnumerateContextFiles_NoPromptFilesAbsent(t *testing.T) {
	// Not parallel: uses os.Chdir to a temp dir with no prompt files.
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	o := New(Config{})
	files := o.enumerateContextFiles()

	for _, f := range files {
		if f.Category == "prompts" {
			t.Errorf("expected no prompts category when template files absent, got %+v", f)
		}
	}
}
