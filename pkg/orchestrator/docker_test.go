// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import "testing"

// --- shortID ---

func TestShortID_LongID(t *testing.T) {
	t.Parallel()
	got := shortID("e60ba5bdd19ddb026f7afa4919e45757d10c609bce112586ee6c4d8ba05bda64")
	want := "e60ba5bdd19d"
	if got != want {
		t.Errorf("shortID(long) = %q, want %q", got, want)
	}
}

func TestShortID_ShortID(t *testing.T) {
	t.Parallel()
	got := shortID("abc123")
	want := "abc123"
	if got != want {
		t.Errorf("shortID(short) = %q, want %q", got, want)
	}
}

func TestShortID_Exactly12(t *testing.T) {
	t.Parallel()
	got := shortID("123456789012")
	want := "123456789012"
	if got != want {
		t.Errorf("shortID(12) = %q, want %q", got, want)
	}
}

func TestShortID_Empty(t *testing.T) {
	t.Parallel()
	got := shortID("")
	if got != "" {
		t.Errorf("shortID(empty) = %q, want empty", got)
	}
}

// --- imageBaseName ---

func TestImageBaseName_WithTag(t *testing.T) {
	t.Parallel()
	got := imageBaseName("cobbler-scaffold:latest")
	want := "cobbler-scaffold"
	if got != want {
		t.Errorf("imageBaseName() = %q, want %q", got, want)
	}
}

func TestImageBaseName_WithVersionTag(t *testing.T) {
	t.Parallel()
	got := imageBaseName("claude-cli:v2026-02-13.1")
	want := "claude-cli"
	if got != want {
		t.Errorf("imageBaseName() = %q, want %q", got, want)
	}
}

func TestImageBaseName_NoTag(t *testing.T) {
	t.Parallel()
	got := imageBaseName("my-image")
	want := "my-image"
	if got != want {
		t.Errorf("imageBaseName() = %q, want %q", got, want)
	}
}

func TestImageBaseName_Empty(t *testing.T) {
	t.Parallel()
	got := imageBaseName("")
	if got != "" {
		t.Errorf("imageBaseName(empty) = %q, want empty", got)
	}
}

// --- latestVersionTag (git-dependent, no t.Parallel) ---

func TestLatestVersionTag_NoTags(t *testing.T) {
	initTestGitRepo(t)
	got := latestVersionTag()
	if got != "" {
		t.Errorf("latestVersionTag() = %q, want empty in repo with no v* tags", got)
	}
}

func TestLatestVersionTag_WithTags(t *testing.T) {
	initTestGitRepo(t)
	gitRun(t, "tag", "v0.1")
	gitRun(t, "tag", "v0.2")
	gitRun(t, "tag", "v0.3")
	got := latestVersionTag()
	if got != "v0.3" {
		t.Errorf("latestVersionTag() = %q, want v0.3", got)
	}
}
