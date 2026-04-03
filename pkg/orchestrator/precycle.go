// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
)

// Type aliases for backward-compatible re-exports.
type AnalysisDoc = an.AnalysisDoc

// RunPreCycleAnalysis performs cross-artifact consistency checks and code
// status detection, writes the combined result to {ScratchDir}/analysis.yaml,
// and logs a summary.
func (a *Analyzer) RunPreCycleAnalysis() {
	an.RunPreCycleAnalysis(an.PreCycleDeps{
		Log:         a.logf,
		CobblerDir:  a.cfg.Cobbler.Dir,
		AnalyzeDeps: a.analyzeDeps(),
	})
}
