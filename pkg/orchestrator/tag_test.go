// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// setupTagRepo creates a temp git repo with an initial commit and the given
// tags, then chdirs into it. Returns the original directory; the caller is
// responsible for restoring via t.Cleanup.
func setupTagRepo(t *testing.T, tags []string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "tag-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	runIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	runIn("git", "init")
	runIn("git", "config", "user.email", "test@test.local")
	runIn("git", "config", "user.name", "Test")
	runIn("git", "config", "commit.gpgsign", "false")
	runIn("git", "commit", "--allow-empty", "-m", "initial")
	for _, tag := range tags {
		runIn("git", "tag", tag)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	return origDir
}

func TestNextDocRevision_DefaultPrefix(t *testing.T) {
	// With no matching tags in the repo for a far-future date, revision is 0.
	// Use a date unlikely to have real tags.
	rev := nextDocRevision("v0.", "29991231")
	if rev != 0 {
		t.Errorf("nextDocRevision(\"v0.\", \"29991231\") = %d, want 0", rev)
	}
}

func TestNextDocRevision_CustomPrefix(t *testing.T) {
	// With no matching tags for a custom prefix + far-future date, revision is 0.
	rev := nextDocRevision("myproj.", "29991231")
	if rev != 0 {
		t.Errorf("nextDocRevision(\"myproj.\", \"29991231\") = %d, want 0", rev)
	}
}

// --- nextDocRevision edge cases ---

func TestNextDocRevision_SameDate_Increments(t *testing.T) {
	// Not parallel: uses os.Chdir.
	setupTagRepo(t, []string{"v0.29991231.0"})
	rev := nextDocRevision("v0.", "29991231")
	if rev != 1 {
		t.Errorf("nextDocRevision with existing .0 tag: got %d, want 1", rev)
	}
}

func TestNextDocRevision_SameDate_MultipleRevisions(t *testing.T) {
	// Not parallel: uses os.Chdir.
	setupTagRepo(t, []string{"v0.29991231.0", "v0.29991231.3", "v0.29991231.1"})
	rev := nextDocRevision("v0.", "29991231")
	if rev != 4 {
		t.Errorf("nextDocRevision with .0/.1/.3 tags: got %d, want 4", rev)
	}
}

func TestNextDocRevision_DifferentDate_ReturnsZero(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// Tags for date 29991230 must not affect revision for 29991231.
	setupTagRepo(t, []string{"v0.29991230.0", "v0.29991230.5"})
	rev := nextDocRevision("v0.", "29991231")
	if rev != 0 {
		t.Errorf("nextDocRevision with tags for different date: got %d, want 0", rev)
	}
}

func TestNextDocRevision_MalformedRevision_ReturnsZero(t *testing.T) {
	// Not parallel: uses os.Chdir.
	// A tag that matches the glob but has a non-numeric revision should be skipped;
	// with no valid revisions found, nextDocRevision returns 0.
	setupTagRepo(t, []string{"v0.29991231.xyz"})
	rev := nextDocRevision("v0.", "29991231")
	if rev != 0 {
		t.Errorf("nextDocRevision with malformed tag revision: got %d, want 0", rev)
	}
}

func TestNextDocRevision_CustomPrefix_Increments(t *testing.T) {
	// Not parallel: uses os.Chdir.
	setupTagRepo(t, []string{"docs.29991231.0", "docs.29991231.2"})
	rev := nextDocRevision("docs.", "29991231")
	if rev != 3 {
		t.Errorf("nextDocRevision with custom prefix: got %d, want 3", rev)
	}
}

func TestTag_WrongBranch(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	// Override BaseBranch to something that won't match the current branch.
	cfg.Cobbler.BaseBranch = "release"
	o := New(cfg)
	err := o.Tag()
	if err == nil {
		t.Fatal("Tag() expected error for wrong branch, got nil")
	}
	if !strings.Contains(err.Error(), "release") {
		t.Errorf("Tag() error = %q, want it to mention the expected branch name", err.Error())
	}
}

func TestTag_CreatesGitTag(t *testing.T) {
	// Not parallel: uses os.Chdir via setupTagRepo.
	setupTagRepo(t, nil)

	cfg := Config{}
	cfg.applyDefaults()
	// Set BaseBranch to whatever our test repo branch is.
	current, err := gitCurrentBranch(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Cobbler.BaseBranch = current
	cfg.Cobbler.DocTagPrefix = "v0."
	// No version file, no podman image → Tag will fail at BuildImage.
	o := &Orchestrator{cfg: cfg}

	err = o.Tag()
	// Tag will fail at BuildImage (podman not available), but the tag
	// should already have been created before that step.
	if err == nil || !strings.Contains(err.Error(), "building image") {
		// If err is nil, tag succeeded fully (unlikely in test).
		// If err doesn't mention "building image", something else failed.
		if err != nil {
			t.Fatalf("Tag() unexpected error: %v", err)
		}
	}

	// Verify the git tag was created.
	tags := gitListTags("v0.*", ".")
	if len(tags) == 0 {
		t.Error("expected at least one v0.* tag after Tag()")
	}
}

func TestTag_VersionFileWriteError(t *testing.T) {
	// Not parallel: uses os.Chdir via setupTagRepo.
	setupTagRepo(t, nil)

	current, err := gitCurrentBranch(".")
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{}
	cfg.applyDefaults()
	cfg.Cobbler.BaseBranch = current
	cfg.Cobbler.DocTagPrefix = "v0."
	cfg.Project.VersionFile = "/dev/null/impossible/version.go" // will fail

	o := &Orchestrator{cfg: cfg}
	err = o.Tag()

	// Should fail with a version file error that mentions the tag was created.
	if err == nil {
		t.Fatal("expected error for invalid version file path")
	}
	if !strings.Contains(err.Error(), "version file") {
		t.Errorf("error = %q, want it to mention version file", err.Error())
	}
	if !strings.Contains(err.Error(), "tag") {
		t.Errorf("error = %q, want it to mention the tag was created", err.Error())
	}
}
