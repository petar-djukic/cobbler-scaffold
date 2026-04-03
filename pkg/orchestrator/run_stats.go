// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// RunStats prints aggregate statistics for a completed generation run.
// When name is empty, it lists available generations.
func (o *Orchestrator) RunStats(name string) error {
	return st.PrintRunStats(name, o.runStatsDeps())
}

// CompareRunStats prints a side-by-side comparison of two generation runs.
func (o *Orchestrator) CompareRunStats(name1, name2 string) error {
	return st.PrintCompareStats(name1, name2, o.runStatsDeps())
}

func (o *Orchestrator) runStatsDeps() st.RunStatsDeps {
	return st.RunStatsDeps{
		Log: logf,
		ListTags: func(pattern string) []string {
			return defaultGitOps.ListTags(pattern, ".")
		},
		ShowFile: func(ref, path string) ([]byte, error) {
			return defaultGitOps.ShowFileContent(ref, path, ".")
		},
		GenerationPrefix: o.cfg.Generation.Prefix,
		CobblerDir:       o.cfg.Cobbler.Dir,
		HistorySubdir:    o.cfg.Cobbler.HistoryDir,
	}
}
