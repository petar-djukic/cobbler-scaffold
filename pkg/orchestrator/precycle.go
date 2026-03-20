// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
)

// Type aliases for backward-compatible re-exports.
type AnalysisDoc = an.AnalysisDoc

// RunPreCycleAnalysis performs cross-artifact consistency checks and code
// status detection, writes the combined result to {ScratchDir}/analysis.yaml,
// and logs a summary.
func (o *Orchestrator) RunPreCycleAnalysis() {
	an.RunPreCycleAnalysis(an.PreCycleDeps{
		Log:        logf,
		CobblerDir: o.cfg.Cobbler.Dir,
		AnalyzeDeps: an.AnalyzeDeps{
			Log:                    logf,
			Releases:               o.cfg.Project.Releases,
			ValidateDocSchemas:     o.validateDocSchemas,
			ValidatePromptTemplate: ictx.ValidatePromptTemplate,
		},
	})
}

// loadAnalysisDoc loads an AnalysisDoc from {cobblerDir}/analysis.yaml.
// Returns nil if the file does not exist or cannot be parsed.
func loadAnalysisDoc(cobblerDir string) *AnalysisDoc {
	return an.LoadAnalysisDoc(cobblerDir)
}
