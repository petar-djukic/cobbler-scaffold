// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// stats.go delegates LOC and documentation word counting to the
// internal/stats sub-package.

package orchestrator

import (
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// StatsRecord holds collected LOC and documentation word counts.
type StatsRecord = st.StatsRecord

// CollectStats gathers Go LOC and documentation word counts.
func (o *Orchestrator) CollectStats() (StatsRecord, error) {
	return st.CollectStats(o.statsDeps())
}

// Stats prints Go lines of code and documentation word counts as YAML.
func (o *Orchestrator) Stats() error {
	return st.PrintStats(o.statsDeps())
}

// statsDeps constructs the StatsDeps struct from orchestrator state.
func (o *Orchestrator) statsDeps() st.StatsDeps {
	return st.StatsDeps{
		BinaryDir:            o.cfg.Project.BinaryDir,
		MagefilesDir:         o.cfg.Project.MagefilesDir,
		ResolveStandardFiles: resolveStandardFiles,
		ClassifyContextFile:  classifyContextFile,
	}
}

