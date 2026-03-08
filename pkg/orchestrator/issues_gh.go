// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// issues_gh.go delegates GitHub issue management to the internal/github
// sub-package. It re-exports types and provides thin delegation functions
// that wire the orchestrator's global dependencies into the sub-package.

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
)

// GitHubIssueManager defines the interface for GitHub issue operations
// used by the orchestrator.
type GitHubIssueManager interface {
	DetectGitHubRepo(repoRoot string) (string, error)
	ListOpenIssues(repo, generation string) ([]gh.CobblerIssue, error)
	PickReadyIssue(repo, generation string) (gh.CobblerIssue, error)
}

// Type aliases for backward compatibility within the package.
type cobblerIssue = gh.CobblerIssue
type cobblerFrontMatter = gh.CobblerFrontMatter
type proposedIssue = gh.ProposedIssue

// Label constants re-exported for use within the package.
const (
	cobblerLabelReady      = gh.LabelReady
	cobblerLabelInProgress = gh.LabelInProgress
	cobblerGenLabelPrefix  = gh.GenLabelPrefix
)

// ghDeps constructs the Deps struct for the github sub-package from
// the orchestrator's globals.
func ghDeps() gh.Deps {
	return gh.Deps{
		Log:          logf,
		GhBin:        binGh,
		BranchExists: gitBranchExists,
	}
}

// ghRepoConfig builds a RepoConfig from the orchestrator Config.
func ghRepoConfig(cfg Config) gh.RepoConfig {
	return gh.RepoConfig{
		IssuesRepo: cfg.Cobbler.IssuesRepo,
		ModulePath: cfg.Project.ModulePath,
		TargetRepo: cfg.Project.TargetRepo,
	}
}

// --- Delegation functions ---

func cobblerGenLabel(generation string) string {
	return gh.GenLabel(generation)
}

func formatIssueFrontMatter(generation string, index, dependsOn int) string {
	return gh.FormatIssueFrontMatter(generation, index, dependsOn)
}

func parseIssueFrontMatter(body string) (cobblerFrontMatter, string) {
	return gh.ParseIssueFrontMatter(body)
}

func detectGitHubRepo(repoRoot string, cfg Config) (string, error) {
	return gh.DetectGitHubRepo(repoRoot, ghRepoConfig(cfg), ghDeps())
}

func ensureCobblerLabels(repo string) error {
	return gh.EnsureCobblerLabels(repo, ghDeps())
}

func listRepoLabels(repo string) []string {
	return gh.ListRepoLabels(repo, ghDeps())
}

func ensureCobblerGenLabel(repo, generation string) error {
	return gh.EnsureCobblerGenLabel(repo, generation, ghDeps())
}

func createMeasuringPlaceholder(repo, generation string, iteration int) (int, error) {
	return gh.CreateMeasuringPlaceholder(repo, generation, iteration, ghDeps())
}

func closeMeasuringPlaceholder(repo string, number int) {
	gh.CloseMeasuringPlaceholder(repo, number, ghDeps())
}

func closeMeasuringPlaceholderWithComment(repo string, number int, comment string) {
	gh.CloseMeasuringPlaceholderWithComment(repo, number, comment, ghDeps())
}

func upgradeMeasuringPlaceholder(repo string, number int, generation string, issue proposedIssue) error {
	return gh.UpgradeMeasuringPlaceholder(repo, number, generation, issue, ghDeps())
}

func createCobblerIssue(repo, generation string, issue proposedIssue) (int, error) {
	return gh.CreateCobblerIssue(repo, generation, issue, ghDeps())
}

func extractParentIssueNumber(generation string) int {
	return gh.ExtractParentIssueNumber(generation)
}

func linkSubIssue(repo string, parentNumber, childNumber int) error {
	return gh.LinkSubIssue(repo, parentNumber, childNumber, ghDeps())
}

func parseIssueURL(raw string) (int, error) {
	return gh.ParseIssueURL(raw)
}

func listOpenCobblerIssues(repo, generation string) ([]cobblerIssue, error) {
	return gh.ListOpenCobblerIssues(repo, generation, ghDeps())
}

func listAllCobblerIssues(repo, generation string) ([]cobblerIssue, error) {
	return gh.ListAllCobblerIssues(repo, generation, ghDeps())
}

func fetchIssueComments(repo string, number int) ([]string, error) {
	return gh.FetchIssueComments(repo, number, ghDeps())
}

func parseCobblerIssuesJSON(data []byte) ([]cobblerIssue, error) {
	return gh.ParseCobblerIssuesJSON(data)
}

func waitForIssuesVisible(repo, generation string, expected int) {
	gh.WaitForIssuesVisible(repo, generation, expected, ghDeps())
}

func hasLabel(issue cobblerIssue, label string) bool {
	return gh.HasLabel(issue, label)
}

func promoteReadyIssues(repo, generation string) error {
	return gh.PromoteReadyIssues(repo, generation, ghDeps())
}

func pickReadyIssue(repo, generation string) (cobblerIssue, error) {
	return gh.PickReadyIssue(repo, generation, ghDeps())
}

func editIssueTitle(repo string, number int, title string) error {
	return gh.EditIssueTitle(repo, number, title, ghDeps())
}

func normalizeIssueTitle(title string) string {
	return gh.NormalizeIssueTitle(title)
}

func closeCobblerIssue(repo string, number int, generation string) error {
	return gh.CloseCobblerIssue(repo, number, generation, ghDeps())
}

func removeInProgressLabel(repo string, number int) error {
	return gh.RemoveInProgressLabel(repo, number, ghDeps())
}

func closeGenerationIssues(repo, generation string) error {
	return gh.CloseGenerationIssues(repo, generation, ghDeps())
}

func gcStaleGenerationIssues(repo, generationPrefix string) {
	gh.GcStaleGenerationIssues(repo, generationPrefix, ghDeps())
}

func listActiveIssuesContext(repo, generation string) (string, error) {
	return gh.ListActiveIssuesContext(repo, generation, ghDeps())
}

func issuesContextJSON(issues []cobblerIssue) (string, error) {
	return gh.IssuesContextJSON(issues)
}

func addIssueLabel(repo string, number int, label string) error {
	return gh.AddIssueLabel(repo, number, label, ghDeps())
}

func removeIssueLabel(repo string, number int, label string) error {
	return gh.RemoveIssueLabel(repo, number, label, ghDeps())
}

func ghExec(repoRoot string, args ...string) (string, error) {
	return gh.GhExec(repoRoot, ghDeps(), args...)
}

func goModModulePath(repoRoot string) string {
	return gh.GoModModulePath(repoRoot)
}

func resolveTargetRepo(cfg Config) string {
	return gh.ResolveTargetRepo(ghRepoConfig(cfg))
}

func commentCobblerIssue(repo string, number int, body string) {
	gh.CommentCobblerIssue(repo, number, body, ghDeps())
}

func fileTargetRepoDefects(repo string, defects []string) {
	gh.FileTargetRepoDefects(repo, defects, ghDeps())
}
