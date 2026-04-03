// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
)

// Type aliases so the rest of the package can refer to internal/github
// types without a qualified name.
type cobblerIssue = gh.CobblerIssue
type cobblerFrontMatter = gh.CobblerFrontMatter
type proposedIssue = gh.ProposedIssue

// CobblerGenLabel returns the GitHub label name for a generation. Exported
// for use by e2e tests that need label-safe generation names (GH-1644).
func CobblerGenLabel(generation string) string {
	return gh.GenLabel(generation)
}
