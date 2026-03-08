// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	rel "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/release"
)

// Tag creates a documentation-only release tag (v0.YYYYMMDD.N) for the current
// state of the repository, builds the container image with that tag, and tags
// the image as :latest. The revision number increments for each tag created on
// the same date. Optionally updates the version file if configured.
//
// Tag convention:
//   - v0.* = documentation-only releases on main (manual)
//   - v1.* = Claude-generated code (created by GeneratorStop)
//
// Exposed as a mage target (e.g., mage tag).
func (o *Orchestrator) Tag() error {
	return rel.Tag(rel.TagParams{
		BaseBranch:   o.cfg.Cobbler.BaseBranch,
		DocTagPrefix: o.cfg.Cobbler.DocTagPrefix,
		VersionFile:  o.cfg.Project.VersionFile,
		BuildImageFn: o.BuildImage,
	})
}

// nextDocRevision delegates to the internal/release package.
func nextDocRevision(prefix, date string) int {
	return rel.NextDocRevision(prefix, date)
}
