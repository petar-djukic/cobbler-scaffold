// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// release_stats.go delegates release statistics to the internal/stats
// sub-package.

package orchestrator

import (
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// ReleaseStats prints a table of roadmap releases with per-release PRD and
// requirement counts.
func (o *Orchestrator) ReleaseStats() error {
	return st.PrintReleaseStats()
}
