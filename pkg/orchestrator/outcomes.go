// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// outcomes.go delegates outcome reporting to the internal/stats sub-package.

package orchestrator

import (
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// OutcomeRecord holds parsed outcome trailer data from a single task commit.
type OutcomeRecord = st.OutcomeRecord

// Outcomes scans all git branches for commits that carry outcome trailers
// and prints a summary table to stdout.
func (o *Orchestrator) Outcomes() error {
	return st.PrintOutcomes(st.OutcomesDeps{
		Log:    logf,
		GitBin: binGit,
	})
}
