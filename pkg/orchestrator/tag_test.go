// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"strings"
	"testing"
)

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
