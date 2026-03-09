// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// generator_stats.go delegates generator statistics to the internal/stats
// sub-package.

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	st "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/stats"
)

// generatorIssueStats type alias for backward compatibility.
type generatorIssueStats = st.GeneratorIssueStats

// stitchCommentData type alias for backward compatibility.
type stitchCommentData = st.StitchCommentData

// GeneratorStats prints a status report for the current generation run.
func (o *Orchestrator) GeneratorStats() error {
	currentBranch, _ := gitCurrentBranch(".")
	return st.PrintGeneratorStats(st.GeneratorStatsDeps{
		Log:                    logf,
		ListGenerationBranches: o.listGenerationBranches,
		GenerationBranch:       o.cfg.Generation.Branch,
		CurrentBranch:          currentBranch,
		DetectGitHubRepo: func() (string, error) {
			return detectGitHubRepo(".", o.cfg)
		},
		ListAllIssues: func(repo, generation string) ([]gh.CobblerIssue, error) {
			return listAllCobblerIssues(repo, generation)
		},
		FetchIssueComments: func(repo string, number int) ([]string, error) {
			return fetchIssueComments(repo, number)
		},
		HistoryDir: o.historyDir(),
	})
}

// parseStitchComment delegates to the internal/stats package.
func parseStitchComment(body string) stitchCommentData {
	return st.ParseStitchComment(body)
}

// countTotalPRDRequirements delegates to the internal/stats package.
func countTotalPRDRequirements() (int, map[string]int) {
	return st.CountTotalPRDRequirements()
}

// buildPRDReleaseMap delegates to the internal/stats package.
func buildPRDReleaseMap() map[string]string {
	return st.BuildPRDReleaseMap()
}

// countDescriptionReqs delegates to the internal/stats package.
func countDescriptionReqs(description string) int {
	return st.CountDescriptionReqs(description)
}

// extractRelease delegates to the internal/stats package.
func extractRelease(text string) string {
	return st.ExtractRelease(text)
}

// formatTokens delegates to the internal/stats package.
func formatTokens(n int) string {
	return st.FormatTokens(n)
}

// formatBytes delegates to the internal/stats package.
func formatBytes(b int) string {
	return st.FormatBytes(b)
}

// extractPRDRefs delegates to the internal/stats package.
func extractPRDRefs(text string) []string {
	return st.ExtractPRDRefs(text)
}
