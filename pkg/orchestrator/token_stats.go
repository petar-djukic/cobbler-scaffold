// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// token_stats.go delegates token statistics to the internal/stats sub-package.

package orchestrator

import (
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// FileTokenStat holds size information for a single file that
// contributes to an assembled Claude prompt.
type FileTokenStat = st.FileTokenStat

// TokenStats enumerates all files that buildProjectContext would load,
// outputs their sizes grouped by category as YAML, and optionally calls
// the Anthropic Token Counting API for exact prompt token counts.
func (o *Orchestrator) TokenStats() error {
	return st.TokenStats(st.TokenStatsDeps{
		Log:            logf,
		EnumerateFiles: o.enumerateContextFiles,
		BuildMeasurePrompt: func(analysis, issues string, iteration int) (string, error) {
			return o.buildMeasurePrompt(analysis, issues, iteration)
		},
	})
}

// enumerateContextFiles lists all files that buildProjectContext loads,
// grouped by category.
func (o *Orchestrator) enumerateContextFiles() []st.FileTokenStat {
	entries := o.resolveContextFileEntries()
	files := make([]st.FileTokenStat, 0, len(entries))
	for _, e := range entries {
		files = append(files, st.FileTokenStat{
			Category: e.Category,
			Path:     e.Path,
			Bytes:    e.Bytes,
		})
	}
	return files
}

