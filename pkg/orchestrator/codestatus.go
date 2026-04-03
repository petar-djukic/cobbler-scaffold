// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
)

// Type aliases for backward-compatible re-exports.
type UCCodeStatus = an.UCCodeStatus
type ReleaseCodeStatus = an.ReleaseCodeStatus
type CodeStatusReport = an.CodeStatusReport

// CodeStatus reports the code implementation status per use case and
// release by comparing road-map.yaml spec status with test file presence.
func (a *Analyzer) CodeStatus() error {
	return an.PrintCodeStatus()
}
