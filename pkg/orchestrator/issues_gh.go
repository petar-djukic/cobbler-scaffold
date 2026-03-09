// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// issues_gh.go delegates GitHub issue management to the internal/github
// sub-package. It re-exports types and provides thin delegation functions
// that wire the orchestrator's global dependencies into the sub-package.

package orchestrator

import (
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
)

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

// ghTracker constructs a GitHubTracker from the orchestrator's globals and
// the given Config. Replaces the former ghDeps() + ghRepoConfig() pair.
func ghTracker(cfg Config) *gh.GitHubTracker {
	return &gh.GitHubTracker{
		Log:          logf,
		GhBin:        binGh,
		BranchExists: gitBranchExists,
		Cfg: gh.RepoConfig{
			IssuesRepo: cfg.Cobbler.IssuesRepo,
			ModulePath: cfg.Project.ModulePath,
			TargetRepo: cfg.Project.TargetRepo,
		},
	}
}

// ghTrackerNoCfg constructs a GitHubTracker without RepoConfig, for
// operations that do not need configuration (label ops, issue CRUD, etc.).
func ghTrackerNoCfg() *gh.GitHubTracker {
	return &gh.GitHubTracker{
		Log:          logf,
		GhBin:        binGh,
		BranchExists: gitBranchExists,
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
	return ghTracker(cfg).DetectGitHubRepo(repoRoot)
}

func ensureCobblerLabels(repo string) error {
	return ghTrackerNoCfg().EnsureCobblerLabels(repo)
}

func listRepoLabels(repo string) []string {
	return ghTrackerNoCfg().ListRepoLabels(repo)
}

func ensureCobblerGenLabel(repo, generation string) error {
	return ghTrackerNoCfg().EnsureCobblerGenLabel(repo, generation)
}

func createMeasuringPlaceholder(repo, generation string, iteration int) (int, error) {
	return ghTrackerNoCfg().CreateMeasuringPlaceholder(repo, generation, iteration)
}

func closeMeasuringPlaceholder(repo string, number int) {
	ghTrackerNoCfg().CloseMeasuringPlaceholder(repo, number)
}

func closeMeasuringPlaceholderWithComment(repo string, number int, comment string) {
	ghTrackerNoCfg().CloseMeasuringPlaceholderWithComment(repo, number, comment)
}

func finalizeMeasurePlaceholder(repo string, number int, generation, comment string, childIssues []int) {
	ghTrackerNoCfg().FinalizeMeasurePlaceholder(repo, number, generation, comment, childIssues)
}

func createCobblerIssue(repo, generation string, issue proposedIssue) (int, error) {
	return ghTrackerNoCfg().CreateCobblerIssue(repo, generation, issue)
}

func extractParentIssueNumber(generation string) int {
	return gh.ExtractParentIssueNumber(generation)
}

func linkSubIssue(repo string, parentNumber, childNumber int) error {
	return ghTrackerNoCfg().LinkSubIssue(repo, parentNumber, childNumber)
}

func parseIssueURL(raw string) (int, error) {
	return gh.ParseIssueURL(raw)
}

func listOpenCobblerIssues(repo, generation string) ([]cobblerIssue, error) {
	return ghTrackerNoCfg().ListOpenCobblerIssues(repo, generation)
}

func listAllCobblerIssues(repo, generation string) ([]cobblerIssue, error) {
	return ghTrackerNoCfg().ListAllCobblerIssues(repo, generation)
}

func fetchIssueComments(repo string, number int) ([]string, error) {
	return ghTrackerNoCfg().FetchIssueComments(repo, number)
}

func parseCobblerIssuesJSON(data []byte) ([]cobblerIssue, error) {
	return gh.ParseCobblerIssuesJSON(data)
}

func waitForIssuesVisible(repo, generation string, expected int) {
	ghTrackerNoCfg().WaitForIssuesVisible(repo, generation, expected)
}

func hasLabel(issue cobblerIssue, label string) bool {
	return gh.HasLabel(issue, label)
}

func promoteReadyIssues(repo, generation string) error {
	return ghTrackerNoCfg().PromoteReadyIssues(repo, generation)
}

func pickReadyIssue(repo, generation string) (cobblerIssue, error) {
	return ghTrackerNoCfg().PickReadyIssue(repo, generation)
}

func editIssueTitle(repo string, number int, title string) error {
	return ghTrackerNoCfg().EditIssueTitle(repo, number, title)
}

func normalizeIssueTitle(title string) string {
	return gh.NormalizeIssueTitle(title)
}

func closeCobblerIssue(repo string, number int, generation string) error {
	return ghTrackerNoCfg().CloseCobblerIssue(repo, number, generation)
}

func removeInProgressLabel(repo string, number int) error {
	return ghTrackerNoCfg().RemoveIssueLabel(repo, number, gh.LabelInProgress)
}

func closeGenerationIssues(repo, generation string) error {
	return ghTrackerNoCfg().CloseGenerationIssues(repo, generation)
}

func gcStaleGenerationIssues(repo, generationPrefix string) {
	ghTrackerNoCfg().GcStaleGenerationIssues(repo, generationPrefix)
}

func listActiveIssuesContext(repo, generation string) (string, error) {
	return ghTrackerNoCfg().ListActiveIssuesContext(repo, generation)
}

func issuesContextJSON(issues []cobblerIssue) (string, error) {
	return gh.IssuesContextJSON(issues)
}

func addIssueLabel(repo string, number int, label string) error {
	return ghTrackerNoCfg().AddIssueLabel(repo, number, label)
}

func removeIssueLabel(repo string, number int, label string) error {
	return ghTrackerNoCfg().RemoveIssueLabel(repo, number, label)
}

func ghExec(repoRoot string, args ...string) (string, error) {
	return ghTrackerNoCfg().GhExec(repoRoot, args...)
}

func goModModulePath(repoRoot string) string {
	return gh.GoModModulePath(repoRoot)
}

func resolveTargetRepo(cfg Config) string {
	return ghTracker(cfg).ResolveTargetRepo()
}

func commentCobblerIssue(repo string, number int, body string) {
	ghTrackerNoCfg().CommentCobblerIssue(repo, number, body)
}

func fileTargetRepoDefects(repo string, defects []string) {
	ghTrackerNoCfg().FileTargetRepoDefects(repo, defects)
}
