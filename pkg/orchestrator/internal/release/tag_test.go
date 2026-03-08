// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package release

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
	rev := NextDocRevision("v0.", "29991231")
	if rev != 0 {
		t.Errorf("NextDocRevision(\"v0.\", \"29991231\") = %d, want 0", rev)
	}
}

func TestNextDocRevision_CustomPrefix(t *testing.T) {
	rev := NextDocRevision("myproj.", "29991231")
	if rev != 0 {
		t.Errorf("NextDocRevision(\"myproj.\", \"29991231\") = %d, want 0", rev)
	}
}

// --- NextDocRevision edge cases ---

func TestNextDocRevision_SameDate_Increments(t *testing.T) {
	setupTagRepo(t, []string{"v0.29991231.0"})
	rev := NextDocRevision("v0.", "29991231")
	if rev != 1 {
		t.Errorf("NextDocRevision with existing .0 tag: got %d, want 1", rev)
	}
}

func TestNextDocRevision_SameDate_MultipleRevisions(t *testing.T) {
	setupTagRepo(t, []string{"v0.29991231.0", "v0.29991231.3", "v0.29991231.1"})
	rev := NextDocRevision("v0.", "29991231")
	if rev != 4 {
		t.Errorf("NextDocRevision with .0/.1/.3 tags: got %d, want 4", rev)
	}
}

func TestNextDocRevision_DifferentDate_ReturnsZero(t *testing.T) {
	setupTagRepo(t, []string{"v0.29991230.0", "v0.29991230.5"})
	rev := NextDocRevision("v0.", "29991231")
	if rev != 0 {
		t.Errorf("NextDocRevision with tags for different date: got %d, want 0", rev)
	}
}

func TestNextDocRevision_MalformedRevision_ReturnsZero(t *testing.T) {
	setupTagRepo(t, []string{"v0.29991231.xyz"})
	rev := NextDocRevision("v0.", "29991231")
	if rev != 0 {
		t.Errorf("NextDocRevision with malformed tag revision: got %d, want 0", rev)
	}
}

func TestNextDocRevision_CustomPrefix_Increments(t *testing.T) {
	setupTagRepo(t, []string{"docs.29991231.0", "docs.29991231.2"})
	rev := NextDocRevision("docs.", "29991231")
	if rev != 3 {
		t.Errorf("NextDocRevision with custom prefix: got %d, want 3", rev)
	}
}

func TestTag_WrongBranch(t *testing.T) {
	// Override GitCurrentBranchFn to return a known branch.
	origFn := GitCurrentBranchFn
	GitCurrentBranchFn = func(dir string) (string, error) {
		return "feature-branch", nil
	}
	defer func() { GitCurrentBranchFn = origFn }()

	err := Tag(TagParams{
		BaseBranch:   "release",
		DocTagPrefix: "v0.",
		BuildImageFn: func() error { return nil },
	})
	if err == nil {
		t.Fatal("Tag() expected error for wrong branch, got nil")
	}
	if !strings.Contains(err.Error(), "release") {
		t.Errorf("Tag() error = %q, want it to mention the expected branch name", err.Error())
	}
}

func TestTag_CreatesGitTag(t *testing.T) {
	setupTagRepo(t, nil)

	current, err := GitCurrentBranchFn(".")
	if err != nil {
		t.Fatal(err)
	}

	buildCalled := false
	err = Tag(TagParams{
		BaseBranch:   current,
		DocTagPrefix: "v0.",
		BuildImageFn: func() error {
			buildCalled = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Tag() unexpected error: %v", err)
	}
	if !buildCalled {
		t.Error("expected BuildImageFn to be called")
	}

	// Verify the git tag was created.
	tags := GitListTagsFn("v0.*", ".")
	if len(tags) == 0 {
		t.Error("expected at least one v0.* tag after Tag()")
	}
}

func TestTag_VersionFileWriteError(t *testing.T) {
	setupTagRepo(t, nil)

	current, err := GitCurrentBranchFn(".")
	if err != nil {
		t.Fatal(err)
	}

	err = Tag(TagParams{
		BaseBranch:   current,
		DocTagPrefix: "v0.",
		VersionFile:  "/dev/null/impossible/version.go", // will fail
		BuildImageFn: func() error { return nil },
	})

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
