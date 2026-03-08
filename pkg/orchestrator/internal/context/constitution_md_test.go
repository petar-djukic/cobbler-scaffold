// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConstitutionToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		sections []ConstitutionSection
		want     string
	}{
		{
			name:     "nil sections returns empty string",
			sections: nil,
			want:     "",
		},
		{
			name:     "empty sections returns empty string",
			sections: []ConstitutionSection{},
			want:     "",
		},
		{
			name: "single section with trailing newline in content",
			sections: []ConstitutionSection{
				{Tag: "articles", Title: "Core Principles", Content: "Five principles govern.\n"},
			},
			want: "## Core Principles\n\nFive principles govern.\n\n",
		},
		{
			name: "single section without trailing newline in content",
			sections: []ConstitutionSection{
				{Tag: "x", Title: "Title", Content: "No trailing newline"},
			},
			want: "## Title\n\nNo trailing newline\n\n",
		},
		{
			name: "multiple sections produce contiguous headings",
			sections: []ConstitutionSection{
				{Tag: "articles", Title: "First", Content: "First content.\n"},
				{Tag: "coding", Title: "Second", Content: "Second content.\n"},
			},
			want: "## First\n\nFirst content.\n\n## Second\n\nSecond content.\n\n",
		},
		{
			name: "multi-line content is preserved",
			sections: []ConstitutionSection{
				{Tag: "s1", Title: "Multi", Content: "Line one.\nLine two.\nLine three.\n"},
			},
			want: "## Multi\n\nLine one.\nLine two.\nLine three.\n\n",
		},
		{
			name: "extra trailing newlines in content are collapsed",
			sections: []ConstitutionSection{
				{Tag: "s1", Title: "Heading", Content: "Body text.\n\n\n"},
			},
			want: "## Heading\n\nBody text.\n\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConstitutionToMarkdown(tc.sections)
			if got != tc.want {
				t.Errorf("ConstitutionToMarkdown() mismatch\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestPreviewConstitutionFile_Success(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test-constitution.yaml")
	content := "sections:\n  - tag: articles\n    title: Core Principles\n    content: |\n      Five principles govern.\n"
	os.WriteFile(path, []byte(content), 0o644)

	if err := PreviewConstitutionFile(path); err != nil {
		t.Errorf("PreviewConstitutionFile() unexpected error: %v", err)
	}
}

func TestPreviewConstitutionFile_EmptySections(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.yaml")
	os.WriteFile(path, []byte("id: no-sections\ntitle: Empty\n"), 0o644)

	err := PreviewConstitutionFile(path)
	if err == nil {
		t.Error("PreviewConstitutionFile() expected error for file with no sections, got nil")
	} else if !strings.Contains(err.Error(), "no sections") {
		t.Errorf("PreviewConstitutionFile() error = %q, want it to mention 'no sections'", err.Error())
	}
}

func TestPreviewConstitutionFile_MissingFile(t *testing.T) {
	err := PreviewConstitutionFile("/nonexistent/path/constitution.yaml")
	if err == nil {
		t.Error("PreviewConstitutionFile() expected error for missing file, got nil")
	}
}
