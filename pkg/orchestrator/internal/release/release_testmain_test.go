// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package release

import (
	"os"
	"testing"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

func TestMain(m *testing.M) {
	// Wire up git interfaces for tests using ShellGitOps, which mirrors
	// the real implementation in the parent orchestrator package.
	git := &gitops.ShellGitOps{}
	GitReader = git
	GitTags = git
	GitCommitter = git

	os.Exit(m.Run())
}
