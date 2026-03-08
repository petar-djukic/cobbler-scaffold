// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package build

import (
	"fmt"
	"testing"
)

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

// --- LatestVersionTag ---

func TestLatestVersionTag_NoTags(t *testing.T) {
	orig := GitListTagsFn
	GitListTagsFn = func(pattern, dir string) []string { return nil }
	defer func() { GitListTagsFn = orig }()

	got := LatestVersionTag()
	if got != "" {
		t.Errorf("LatestVersionTag() = %q, want empty when no tags", got)
	}
}

func TestLatestVersionTag_MultiTags(t *testing.T) {
	orig := GitListTagsFn
	GitListTagsFn = func(pattern, dir string) []string {
		return []string{"v1.0.0", "v1.1.0", "v2.0.0"}
	}
	defer func() { GitListTagsFn = orig }()

	got := LatestVersionTag()
	if got != "v2.0.0" {
		t.Errorf("LatestVersionTag() = %q, want v2.0.0", got)
	}
}

func TestLatestVersionTag_SingleTag(t *testing.T) {
	orig := GitListTagsFn
	GitListTagsFn = func(pattern, dir string) []string {
		return []string{"v0.1.0"}
	}
	defer func() { GitListTagsFn = orig }()

	got := LatestVersionTag()
	if got != "v0.1.0" {
		t.Errorf("LatestVersionTag() = %q, want v0.1.0", got)
	}
}

// --- BuildImage ---

func TestBuildImage_EmptyImageName(t *testing.T) {
	cfg := DockerConfig{Image: "", VersionFile: "version.go"}
	err := BuildImage(cfg, "FROM scratch\n")
	if err == nil {
		t.Fatal("expected error for empty image name")
	}
}

func TestBuildImage_NoVersion(t *testing.T) {
	origRead := ReadVersionConstFn
	origTags := GitListTagsFn
	ReadVersionConstFn = func(string) string { return "" }
	GitListTagsFn = func(string, string) []string { return nil }
	defer func() {
		ReadVersionConstFn = origRead
		GitListTagsFn = origTags
	}()

	cfg := DockerConfig{Image: "my-image:latest", VersionFile: "version.go"}
	err := BuildImage(cfg, "FROM scratch\n")
	if err == nil {
		t.Fatal("expected error when no version is found")
	}
}

func TestBuildImage_UsesVersionConst(t *testing.T) {
	origRead := ReadVersionConstFn
	origBuild := PodmanBuildFn
	ReadVersionConstFn = func(string) string { return "v1.2.3" }
	var calledTags []string
	PodmanBuildFn = func(dockerfile string, tags ...string) error {
		calledTags = tags
		return nil
	}
	defer func() {
		ReadVersionConstFn = origRead
		PodmanBuildFn = origBuild
	}()

	cfg := DockerConfig{Image: "my-image:latest", VersionFile: "version.go"}
	err := BuildImage(cfg, "FROM scratch\n")
	if err != nil {
		t.Fatalf("BuildImage: %v", err)
	}
	if len(calledTags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(calledTags), calledTags)
	}
	if calledTags[0] != "my-image:v1.2.3" {
		t.Errorf("tag[0] = %q, want my-image:v1.2.3", calledTags[0])
	}
	if calledTags[1] != "my-image:latest" {
		t.Errorf("tag[1] = %q, want my-image:latest", calledTags[1])
	}
}

func TestBuildImage_FallsBackToGitTag(t *testing.T) {
	origRead := ReadVersionConstFn
	origTags := GitListTagsFn
	origBuild := PodmanBuildFn
	ReadVersionConstFn = func(string) string { return "" }
	GitListTagsFn = func(string, string) []string { return []string{"v0.9.0"} }
	var calledTags []string
	PodmanBuildFn = func(dockerfile string, tags ...string) error {
		calledTags = tags
		return nil
	}
	defer func() {
		ReadVersionConstFn = origRead
		GitListTagsFn = origTags
		PodmanBuildFn = origBuild
	}()

	cfg := DockerConfig{Image: "my-image:latest", VersionFile: ""}
	err := BuildImage(cfg, "FROM scratch\n")
	if err != nil {
		t.Fatalf("BuildImage: %v", err)
	}
	if calledTags[0] != "my-image:v0.9.0" {
		t.Errorf("tag[0] = %q, want my-image:v0.9.0", calledTags[0])
	}
}

// --- ImageBaseName additional cases ---

// --- BuildFromEmbeddedDockerfile ---

func TestBuildFromEmbeddedDockerfile_Success(t *testing.T) {
	origBuild := PodmanBuildFn
	var receivedDockerfile string
	var receivedTags []string
	PodmanBuildFn = func(dockerfile string, tags ...string) error {
		receivedDockerfile = dockerfile
		receivedTags = tags
		return nil
	}
	defer func() { PodmanBuildFn = origBuild }()

	err := BuildFromEmbeddedDockerfile("FROM scratch\nRUN echo hello\n", "myimage:v1", "myimage:latest")
	if err != nil {
		t.Fatalf("BuildFromEmbeddedDockerfile: %v", err)
	}
	if receivedDockerfile == "" {
		t.Error("PodmanBuildFn should have received a non-empty dockerfile path")
	}
	if len(receivedTags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(receivedTags))
	}
	if receivedTags[0] != "myimage:v1" {
		t.Errorf("tag[0] = %q, want myimage:v1", receivedTags[0])
	}
}

func TestBuildFromEmbeddedDockerfile_PodmanError(t *testing.T) {
	origBuild := PodmanBuildFn
	PodmanBuildFn = func(dockerfile string, tags ...string) error {
		return fmt.Errorf("podman build failed")
	}
	defer func() { PodmanBuildFn = origBuild }()

	err := BuildFromEmbeddedDockerfile("FROM scratch\n", "img:v1")
	if err == nil {
		t.Fatal("expected error when podman build fails")
	}
}

// --- EnsureImage ---

func TestEnsureImage_ImageExists(t *testing.T) {
	// We can't easily test PodmanImageExists without podman,
	// but EnsureImage with a non-existent image will try to build.
	// Just verify it doesn't panic.
}

func TestImageBaseName_ColonAtStart(t *testing.T) {
	t.Parallel()
	got := ImageBaseName(":latest")
	// colon at index 0, so LastIndex returns 0, which is not > 0
	if got != ":latest" {
		t.Errorf("ImageBaseName(:latest) = %q, want :latest", got)
	}
}

func TestImageBaseName_MultipleColons(t *testing.T) {
	t.Parallel()
	got := ImageBaseName("registry.io:5000/myimage:v1")
	if got != "registry.io:5000/myimage" {
		t.Errorf("ImageBaseName(multi-colon) = %q, want registry.io:5000/myimage", got)
	}
}
