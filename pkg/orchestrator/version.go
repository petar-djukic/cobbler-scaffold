// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	rel "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/release"
)

// versionConstRe is kept as a package-level reference for backward compatibility.
var versionConstRe = rel.VersionConstRe

// readVersionConst delegates to the internal/release package.
func readVersionConst(filePath string) string {
	return rel.ReadVersionConst(filePath)
}

// writeVersionConst delegates to the internal/release package.
func writeVersionConst(filePath, version string) error {
	return rel.WriteVersionConst(filePath, version)
}
