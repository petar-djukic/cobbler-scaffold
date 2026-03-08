// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package build

import "testing"

// --- ShortID ---

func TestShortID_LongID(t *testing.T) {
	t.Parallel()
	got := ShortID("e60ba5bdd19ddb026f7afa4919e45757d10c609bce112586ee6c4d8ba05bda64")
	want := "e60ba5bdd19d"
	if got != want {
		t.Errorf("ShortID(long) = %q, want %q", got, want)
	}
}

func TestShortID_ShortID(t *testing.T) {
	t.Parallel()
	got := ShortID("abc123")
	want := "abc123"
	if got != want {
		t.Errorf("ShortID(short) = %q, want %q", got, want)
	}
}

func TestShortID_Exactly12(t *testing.T) {
	t.Parallel()
	got := ShortID("123456789012")
	want := "123456789012"
	if got != want {
		t.Errorf("ShortID(12) = %q, want %q", got, want)
	}
}

func TestShortID_Empty(t *testing.T) {
	t.Parallel()
	got := ShortID("")
	if got != "" {
		t.Errorf("ShortID(empty) = %q, want empty", got)
	}
}

// --- ImageBaseName ---

func TestImageBaseName_WithTag(t *testing.T) {
	t.Parallel()
	got := ImageBaseName("cobbler-scaffold:latest")
	want := "cobbler-scaffold"
	if got != want {
		t.Errorf("ImageBaseName() = %q, want %q", got, want)
	}
}

func TestImageBaseName_WithVersionTag(t *testing.T) {
	t.Parallel()
	got := ImageBaseName("claude-cli:v2026-02-13.1")
	want := "claude-cli"
	if got != want {
		t.Errorf("ImageBaseName() = %q, want %q", got, want)
	}
}

func TestImageBaseName_NoTag(t *testing.T) {
	t.Parallel()
	got := ImageBaseName("my-image")
	want := "my-image"
	if got != want {
		t.Errorf("ImageBaseName() = %q, want %q", got, want)
	}
}

func TestImageBaseName_Empty(t *testing.T) {
	t.Parallel()
	got := ImageBaseName("")
	if got != "" {
		t.Errorf("ImageBaseName(empty) = %q, want empty", got)
	}
}
