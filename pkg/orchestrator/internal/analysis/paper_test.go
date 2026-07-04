// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func noopLog(string, ...any) {}

// writePaperProject builds a temp project with a paper.yaml carrying the given
// project_registry body and the supplied files, then chdirs into it. The
// working directory is restored on cleanup.
func writePaperProject(t *testing.T, registry string, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	consts := filepath.Join(dir, "docs", "constitutions")
	if err := os.MkdirAll(consts, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "articles:\n  - id: P1\n    title: Vocabulary registry\n    rule: x\nproject_registry:\n" + registry
	if err := os.WriteFile(filepath.Join(consts, "paper.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestRunPaperChecks_NoConstitutionIsNoop(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	got := RunPaperChecks(noopLog)
	if len(got.VocabularyIssues)+len(got.PlaceholderErrors)+len(got.BrokenCitations)+len(got.ForbiddenTerms) != 0 {
		t.Errorf("expected no findings without paper.yaml, got %+v", got)
	}
}

func TestPaperVocabulary_P1(t *testing.T) {
	cases := []struct {
		name     string
		registry string
		want     int
	}{
		{
			name:     "empty registry with prose flagged",
			registry: "  vocabulary: {}\n  prose_globs:\n    - paper/*.md\n",
			want:     1,
		},
		{
			name:     "populated registry passes",
			registry: "  vocabulary:\n    spindle: the state-machine engine\n  prose_globs:\n    - paper/*.md\n",
			want:     0,
		},
		{
			name:     "no prose passes",
			registry: "  vocabulary: {}\n  prose_globs:\n    - paper/*.md\n",
			want:     0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			if tc.name != "no prose passes" {
				files["paper/intro.md"] = "The system works.\n"
			}
			writePaperProject(t, tc.registry, files)
			got := RunPaperChecks(noopLog)
			if len(got.VocabularyIssues) != tc.want {
				t.Errorf("VocabularyIssues = %v, want %d", got.VocabularyIssues, tc.want)
			}
		})
	}
}

func TestPaperPlaceholders_P3(t *testing.T) {
	registry := "  placeholder_pattern: '\\{\\{DATA:([^}]+)\\}\\}'\n  prose_globs:\n    - paper/*.md\n"

	t.Run("missing artifact fails", func(t *testing.T) {
		writePaperProject(t, registry, map[string]string{
			"paper/results.md": "Accuracy was {{DATA:results/run1.json}}.\n",
		})
		got := RunPaperChecks(noopLog)
		if len(got.PlaceholderErrors) != 1 {
			t.Fatalf("PlaceholderErrors = %v, want 1", got.PlaceholderErrors)
		}
	})

	t.Run("present artifact passes", func(t *testing.T) {
		writePaperProject(t, registry, map[string]string{
			"paper/results.md":  "Accuracy was {{DATA:results/run1.json}}.\n",
			"results/run1.json": "{}\n",
		})
		got := RunPaperChecks(noopLog)
		if len(got.PlaceholderErrors) != 0 {
			t.Errorf("PlaceholderErrors = %v, want 0", got.PlaceholderErrors)
		}
	})
}

func TestPaperCitations_P4(t *testing.T) {
	registry := "  prose_globs:\n    - paper/*.tex\n  bibliography:\n    - paper/refs.bib\n"
	files := map[string]string{
		"paper/refs.bib":  "@article{known2020, title={A}}\n",
		"paper/paper.tex": "As shown \\cite{known2020} and \\citep{missing2021}.\n",
	}
	writePaperProject(t, registry, files)
	got := RunPaperChecks(noopLog)
	if len(got.BrokenCitations) != 1 {
		t.Fatalf("BrokenCitations = %v, want 1 (missing2021)", got.BrokenCitations)
	}
}

func TestPaperForbiddenTerms_P5(t *testing.T) {
	registry := "  forbidden_terms:\n    - novel\n    - groundbreaking\n  prose_globs:\n    - paper/*.md\n"
	files := map[string]string{
		"paper/intro.md": "We present a novel approach that works.\n",
	}
	writePaperProject(t, registry, files)
	got := RunPaperChecks(noopLog)
	if len(got.ForbiddenTerms) != 1 {
		t.Errorf("ForbiddenTerms = %v, want 1 (novel)", got.ForbiddenTerms)
	}
}

func TestExpandGlob_DoubleStar(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "paper", "sections")
	os.MkdirAll(nested, 0o755)
	os.WriteFile(filepath.Join(dir, "paper", "top.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(nested, "deep.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(nested, "skip.tex"), []byte("x"), 0o644)

	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	got := expandGlob("paper/**/*.md")
	if len(got) != 2 {
		t.Errorf("expandGlob matched %v, want 2 .md files", got)
	}
}
