// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// cobblerIssue holds the parsed representation of a GitHub issue created by
// the orchestrator. Fields are populated from the issue's YAML front-matter.
type cobblerIssue struct {
	Number      int    // GitHub issue number
	Title       string // Issue title
	State       string // "open" or "closed"
	Index       int    // cobbler_index from front-matter
	DependsOn   int    // cobbler_depends_on (-1 = no dependency)
	Generation  string // cobbler_generation label value
	Description string // Body text below the front-matter block
	Labels      []string
}

// cobblerFrontMatter is the YAML front-matter embedded at the top of every
// GitHub issue created by the orchestrator.
type cobblerFrontMatter struct {
	Generation string `yaml:"cobbler_generation"`
	Index      int    `yaml:"cobbler_index"`
	DependsOn  int    `yaml:"cobbler_depends_on"`
}

// cobblerLabelReady and cobblerLabelInProgress are the two status labels
// applied to orchestrator issues during their lifecycle.
const (
	cobblerLabelReady      = "cobbler-ready"
	cobblerLabelInProgress = "cobbler-in-progress"
)

// cobblerGenLabelPrefix is the prefix for generation-scoped labels.
const cobblerGenLabelPrefix = "cobbler-gen-"

// cobblerGenLabel returns the generation label for a given generation name.
// GitHub enforces a 50-character maximum on label names. When the full label
// would exceed 50 chars, we keep the prefix (12 chars) plus the first 29 chars
// of the generation name, a hyphen, and an 8-char FNV-32 hex digest of the
// full generation name — yielding exactly 50 chars and remaining deterministic.
func cobblerGenLabel(generation string) string {
	const maxLen = 50
	label := cobblerGenLabelPrefix + generation
	if len(label) <= maxLen {
		return label
	}
	// Available space after the prefix: 50 - 12 (prefix) - 1 (hyphen) - 8 (hash) = 29.
	const bodyLen = 29
	h := fnv.New32a()
	h.Write([]byte(generation))
	truncated := generation
	if len(truncated) > bodyLen {
		truncated = truncated[:bodyLen]
	}
	return fmt.Sprintf("%s%s-%08x", cobblerGenLabelPrefix, truncated, h.Sum32())
}

// formatIssueFrontMatter formats the YAML front-matter block for an issue body.
func formatIssueFrontMatter(generation string, index, dependsOn int) string {
	if dependsOn < 0 {
		return fmt.Sprintf("---\ncobbler_generation: %s\ncobbler_index: %d\n---\n\n",
			generation, index)
	}
	return fmt.Sprintf("---\ncobbler_generation: %s\ncobbler_index: %d\ncobbler_depends_on: %d\n---\n\n",
		generation, index, dependsOn)
}

// parseIssueFrontMatter splits a GitHub issue body into its YAML front-matter
// and description parts. Returns zero-value front-matter on parse failure.
func parseIssueFrontMatter(body string) (cobblerFrontMatter, string) {
	// Expect body to start with "---\n".
	if !strings.HasPrefix(body, "---\n") {
		return cobblerFrontMatter{DependsOn: -1}, body
	}
	// Find the closing "---".
	rest := body[4:] // skip opening ---\n
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return cobblerFrontMatter{DependsOn: -1}, body
	}
	yamlBlock := rest[:idx]
	description := strings.TrimPrefix(rest[idx+5:], "\n") // skip \n---\n and leading newline

	var fm cobblerFrontMatter
	fm.DependsOn = -1 // default: no dependency
	for _, line := range strings.Split(yamlBlock, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cobbler_generation:") {
			fm.Generation = strings.TrimSpace(strings.TrimPrefix(line, "cobbler_generation:"))
		} else if strings.HasPrefix(line, "cobbler_index:") {
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "cobbler_index:")), "%d", &fm.Index)
		} else if strings.HasPrefix(line, "cobbler_depends_on:") {
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "cobbler_depends_on:")), "%d", &fm.DependsOn)
		}
	}
	return fm, description
}

// detectGitHubRepo resolves the GitHub owner/repo string for the target project.
// Resolution order:
//  1. cfg.Cobbler.IssuesRepo if set (explicit override, used for testing)
//  2. `gh repo view --json nameWithOwner` run in repoRoot (reads git remote)
//  3. Strip "github.com/" from go.mod module path
func detectGitHubRepo(repoRoot string, cfg Config) (string, error) {
	if cfg.Cobbler.IssuesRepo != "" {
		return cfg.Cobbler.IssuesRepo, nil
	}

	// Try gh repo view in the repo root.
	cmd := exec.Command(binGh, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	cmd.Dir = repoRoot
	if out, err := cmd.Output(); err == nil {
		if repo := strings.TrimSpace(string(out)); repo != "" {
			return repo, nil
		}
	}

	// Fall back to module path.
	modPath := cfg.Project.ModulePath
	if strings.HasPrefix(modPath, "github.com/") {
		return strings.TrimPrefix(modPath, "github.com/"), nil
	}

	return "", fmt.Errorf("cannot determine GitHub repo: set cobbler.issues_repo in configuration.yaml or ensure the project has a github.com module path")
}

// ensureCobblerLabels creates the cobbler-ready and cobbler-in-progress labels
// on the target repo if they do not already exist. Idempotent.
func ensureCobblerLabels(repo string) error {
	existing := listRepoLabels(repo)
	existingSet := make(map[string]bool, len(existing))
	for _, l := range existing {
		existingSet[l] = true
	}

	labels := []struct {
		name  string
		color string
		desc  string
	}{
		{cobblerLabelReady, "0075ca", "Cobbler task ready to be picked by stitch"},
		{cobblerLabelInProgress, "e4e669", "Cobbler task currently being worked on"},
	}

	for _, l := range labels {
		if existingSet[l.name] {
			continue
		}
		cmd := exec.Command(binGh, "api", "repos/"+repo+"/labels",
			"--method", "POST",
			"--field", "name="+l.name,
			"--field", "color="+l.color,
			"--field", "description="+l.desc,
		)
		if out, err := cmd.Output(); err != nil {
			logf("ensureCobblerLabels: could not create label %q: %v (output: %s)", l.name, err, string(out))
		} else {
			logf("ensureCobblerLabels: created label %q on %s", l.name, repo)
		}
	}
	return nil
}

// listRepoLabels returns the names of all labels on the repo.
func listRepoLabels(repo string) []string {
	out, err := exec.Command(binGh, "label", "list", "--repo", repo, "--json", "name", "--limit", "100").Output()
	if err != nil {
		return nil
	}
	var labels []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &labels); err != nil {
		return nil
	}
	names := make([]string, 0, len(labels))
	for _, l := range labels {
		names = append(names, l.Name)
	}
	return names
}

// ensureCobblerGenLabel creates the generation-scoped label on the repo if
// it does not already exist.
func ensureCobblerGenLabel(repo, generation string) error {
	label := cobblerGenLabel(generation)
	cmd := exec.Command(binGh, "api", "repos/"+repo+"/labels",
		"--method", "POST",
		"--field", "name="+label,
		"--field", "color=ededed", // light grey; GitHub API requires a valid 6-char hex color
		"--field", "description=Cobbler generation "+generation,
	)
	// Ignore error — label may already exist (422 Unprocessable Entity).
	cmd.Run() //nolint:errcheck // best-effort
	return nil
}

// createMeasuringPlaceholder creates a transient GitHub issue that signals
// the measure agent is actively calling Claude for iteration i (1-based).
// The issue carries no cobbler-ready label so stitch won't pick it up.
// Callers must call closeMeasuringPlaceholder after the iteration completes.
func createMeasuringPlaceholder(repo, generation string, iteration int) (int, error) {
	title := fmt.Sprintf("[measuring] %s task %d", generation, iteration)
	body := fmt.Sprintf("Cobbler measure is calling Claude to propose task %d for generation %s.\n\nThis issue will be closed automatically when measure completes.", iteration, generation)
	// No cobbler labels: stitch ignores issues without a gen label, and the
	// placeholder must not appear in the existing-issues context sent to Claude.
	out, err := exec.Command(binGh, "issue", "create",
		"--repo", repo,
		"--title", title,
		"--body", body,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("gh issue create placeholder: %w", err)
	}
	number, err := parseIssueURL(string(out))
	if err != nil {
		return 0, err
	}
	logf("createMeasuringPlaceholder: created #%d for iteration %d", number, iteration)
	return number, nil
}

// closeMeasuringPlaceholder closes the placeholder issue created by
// createMeasuringPlaceholder. Best-effort: logs and ignores errors.
func closeMeasuringPlaceholder(repo string, number int) {
	if err := exec.Command(binGh, "issue", "close",
		"--repo", repo,
		fmt.Sprintf("%d", number),
	).Run(); err != nil {
		logf("closeMeasuringPlaceholder: close #%d warning: %v", number, err)
		return
	}
	logf("closeMeasuringPlaceholder: closed #%d", number)
}

// upgradeMeasuringPlaceholder converts the transient measuring placeholder
// into the task issue in-place. It edits the placeholder's title and body
// to match the proposed issue, adds the cobbler-gen label so stitch can
// pick it up, and links it as a sub-issue of the parent generation issue
// if the generation name encodes one (GH-578).
func upgradeMeasuringPlaceholder(repo string, number int, generation string, issue proposedIssue) error {
	body := formatIssueFrontMatter(generation, issue.Index, issue.Dependency) + issue.Description

	// Edit title and body in one command.
	if err := exec.Command(binGh, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--title", issue.Title,
		"--body", body,
	).Run(); err != nil {
		return fmt.Errorf("gh issue edit placeholder #%d: %w", number, err)
	}

	// Add cobbler-gen label so stitch can pick it up.
	if err := addIssueLabel(repo, number, cobblerGenLabel(generation)); err != nil {
		return fmt.Errorf("adding gen label to #%d: %w", number, err)
	}

	logf("upgradeMeasuringPlaceholder: upgraded #%d %q gen=%s index=%d dep=%d",
		number, issue.Title, generation, issue.Index, issue.Dependency)

	// Link as sub-issue of the parent if the generation name encodes one.
	if parentNumber := extractParentIssueNumber(generation); parentNumber > 0 {
		if err := linkSubIssue(repo, parentNumber, number); err != nil {
			logf("upgradeMeasuringPlaceholder: linkSubIssue warning for #%d -> parent #%d: %v", number, parentNumber, err)
		}
	}
	return nil
}

// createCobblerIssue creates a GitHub issue on repo for the given generation
// and proposedIssue. Returns the GitHub issue number.
//
// Note: gh issue create (v2.87.3) does not support --json; it outputs the
// issue URL (https://github.com/owner/repo/issues/123) on success.
func createCobblerIssue(repo, generation string, issue proposedIssue) (int, error) {
	body := formatIssueFrontMatter(generation, issue.Index, issue.Dependency) + issue.Description

	genLabel := cobblerGenLabel(generation)
	out, err := exec.Command(binGh, "issue", "create",
		"--repo", repo,
		"--title", issue.Title,
		"--body", body,
		"--label", genLabel,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("gh issue create: %w", err)
	}

	number, err := parseIssueURL(string(out))
	if err != nil {
		return 0, err
	}
	logf("createCobblerIssue: created #%d %q gen=%s index=%d dep=%d",
		number, issue.Title, generation, issue.Index, issue.Dependency)

	// Link as sub-issue of the parent, if the generation name encodes one (GH-566).
	if parentNumber := extractParentIssueNumber(generation); parentNumber > 0 {
		if err := linkSubIssue(repo, parentNumber, number); err != nil {
			logf("createCobblerIssue: linkSubIssue warning for #%d -> parent #%d: %v", number, parentNumber, err)
		}
	}

	return number, nil
}

// extractParentIssueNumber parses a GitHub issue number from a generation name
// that follows the pattern "...-gh-<N>-..." (e.g., "generation-gh-206-slug"
// → 206). Returns 0 if the pattern is not found.
func extractParentIssueNumber(generation string) int {
	const marker = "-gh-"
	idx := strings.Index(generation, marker)
	if idx < 0 {
		return 0
	}
	rest := generation[idx+len(marker):]
	var n int
	if _, err := fmt.Sscanf(rest, "%d", &n); err != nil || n <= 0 {
		return 0
	}
	return n
}

// linkSubIssue attaches childNumber as a GitHub sub-issue of parentNumber.
// It first fetches the child's database ID, then POSTs to the sub_issues API.
// Errors are returned so the caller can log them as warnings.
func linkSubIssue(repo string, parentNumber, childNumber int) error {
	// Fetch the child issue's database ID (different from the display number).
	dbIDOut, err := exec.Command(binGh, "api",
		fmt.Sprintf("repos/%s/issues/%d", repo, childNumber),
		"--jq", ".id",
	).Output()
	if err != nil {
		return fmt.Errorf("fetching database id for #%d: %w", childNumber, err)
	}
	dbIDStr := strings.TrimSpace(string(dbIDOut))
	var dbID int
	if _, err := fmt.Sscanf(dbIDStr, "%d", &dbID); err != nil || dbID <= 0 {
		return fmt.Errorf("parsing database id %q for #%d: %w", dbIDStr, childNumber, err)
	}

	// POST to the parent's sub_issues endpoint.
	out, err := exec.Command(binGh, "api",
		fmt.Sprintf("repos/%s/issues/%d/sub_issues", repo, parentNumber),
		"--method", "POST",
		"--field", fmt.Sprintf("sub_issue_id=%d", dbID),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("linking #%d as sub-issue of #%d: %w (output: %s)",
			childNumber, parentNumber, err, strings.TrimSpace(string(out)))
	}
	logf("linkSubIssue: linked #%d as sub-issue of #%d", childNumber, parentNumber)
	return nil
}

// parseIssueURL extracts a GitHub issue number from a URL string like
// "https://github.com/owner/repo/issues/123\n". Returns an error for
// malformed or empty output.
func parseIssueURL(raw string) (int, error) {
	url := strings.TrimSpace(raw)
	parts := strings.Split(url, "/")
	// A valid GitHub issue URL has at least 7 segments: ["https:", "", "github.com", "owner", "repo", "issues", "123"].
	if len(parts) < 7 {
		return 0, fmt.Errorf("parsing gh issue create output: expected URL, got %q", url)
	}
	var number int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &number); err != nil || number == 0 {
		return 0, fmt.Errorf("parsing gh issue create output: could not extract number from %q", url)
	}
	return number, nil
}

// listOpenCobblerIssues returns all open GitHub issues for a generation.
// It uses the REST API endpoint (gh api repos/.../issues) rather than
// gh issue list, because gh issue list uses GitHub's search API which is
// eventually consistent and can return stale results immediately after
// label changes. The REST endpoint reads directly from the database.
func listOpenCobblerIssues(repo, generation string) ([]cobblerIssue, error) {
	label := cobblerGenLabel(generation)
	out, err := exec.Command(binGh, "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=open",
		"-f", "labels="+label,
		"-f", "per_page=100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api repos issues: %w", err)
	}

	return parseCobblerIssuesJSON(out)
}

// listAllCobblerIssues returns all GitHub issues (open and closed) for a
// generation. Used by GeneratorStats to report completed tasks.
func listAllCobblerIssues(repo, generation string) ([]cobblerIssue, error) {
	label := cobblerGenLabel(generation)
	out, err := exec.Command(binGh, "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=all",
		"-f", "labels="+label,
		"-f", "per_page=100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api repos issues: %w", err)
	}
	return parseCobblerIssuesJSON(out)
}

// fetchIssueComments returns the body text of all comments on the given issue.
func fetchIssueComments(repo string, number int) ([]string, error) {
	out, err := exec.Command(binGh, "api",
		fmt.Sprintf("repos/%s/issues/%d/comments", repo, number),
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api issue comments for #%d: %w", number, err)
	}
	var raw []struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing issue comments for #%d: %w", number, err)
	}
	bodies := make([]string, 0, len(raw))
	for _, r := range raw {
		bodies = append(bodies, r.Body)
	}
	return bodies, nil
}

// parseCobblerIssuesJSON parses the JSON output from the GitHub REST API issues
// endpoint into a slice of cobblerIssue structs.
func parseCobblerIssuesJSON(data []byte) ([]cobblerIssue, error) {
	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing gh api repos issues: %w", err)
	}

	issues := make([]cobblerIssue, 0, len(raw))
	for _, r := range raw {
		fm, desc := parseIssueFrontMatter(r.Body)
		labelNames := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labelNames = append(labelNames, l.Name)
		}
		issues = append(issues, cobblerIssue{
			Number:      r.Number,
			Title:       r.Title,
			State:       r.State,
			Index:       fm.Index,
			DependsOn:   fm.DependsOn,
			Generation:  fm.Generation,
			Description: desc,
			Labels:      labelNames,
		})
	}
	return issues, nil
}

// waitForIssuesVisible polls listOpenCobblerIssues until at least
// expected issues appear or the timeout expires. The REST API label
// index may lag briefly after issue creation, so this function
// ensures all issues are visible before promotion or DAG resolution.
func waitForIssuesVisible(repo, generation string, expected int) {
	const maxWait = 15 * time.Second
	const interval = time.Second
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		issues, err := listOpenCobblerIssues(repo, generation)
		if err == nil && len(issues) >= expected {
			return
		}
		logf("waitForIssuesVisible: %d/%d visible, retrying...", len(issues), expected)
		time.Sleep(interval)
	}
	logf("waitForIssuesVisible: timed out waiting for %d issues (generation=%s)", expected, generation)
}

// hasLabel returns true if the issue has the given label.
func hasLabel(issue cobblerIssue, label string) bool {
	for _, l := range issue.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// promoteReadyIssues builds the DAG from open issues and applies
// cobbler-ready to unblocked issues. Issues whose dependency is still open
// have cobbler-ready removed.
func promoteReadyIssues(repo, generation string) error {
	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		return fmt.Errorf("promoteReadyIssues: %w", err)
	}
	if len(issues) == 0 {
		return nil
	}

	// Build set of open cobbler indices.
	openIndices := make(map[int]bool, len(issues))
	for _, iss := range issues {
		openIndices[iss.Index] = true
	}

	for _, iss := range issues {
		blocked := iss.DependsOn >= 0 && openIndices[iss.DependsOn]
		currentlyReady := hasLabel(iss, cobblerLabelReady)

		if !blocked && !currentlyReady {
			if err := addIssueLabel(repo, iss.Number, cobblerLabelReady); err != nil {
				logf("promoteReadyIssues: add ready label to #%d: %v", iss.Number, err)
			}
		} else if blocked && currentlyReady {
			if err := removeIssueLabel(repo, iss.Number, cobblerLabelReady); err != nil {
				logf("promoteReadyIssues: remove ready label from #%d: %v", iss.Number, err)
			}
		}
	}
	return nil
}

// pickReadyIssue promotes ready issues then picks the lowest-numbered
// cobbler-ready issue, adds cobbler-in-progress, and returns it.
func pickReadyIssue(repo, generation string) (cobblerIssue, error) {
	if err := promoteReadyIssues(repo, generation); err != nil {
		return cobblerIssue{}, fmt.Errorf("pickReadyIssue promote: %w", err)
	}

	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		return cobblerIssue{}, fmt.Errorf("pickReadyIssue list: %w", err)
	}

	// Filter to ready issues and sort by number ascending.
	var ready []cobblerIssue
	for _, iss := range issues {
		if hasLabel(iss, cobblerLabelReady) && !hasLabel(iss, cobblerLabelInProgress) {
			ready = append(ready, iss)
		}
	}
	if len(ready) == 0 {
		return cobblerIssue{}, fmt.Errorf("no ready issues for generation %s", generation)
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Number < ready[j].Number })

	picked := ready[0]
	if err := addIssueLabel(repo, picked.Number, cobblerLabelInProgress); err != nil {
		logf("pickReadyIssue: add in-progress label to #%d: %v", picked.Number, err)
	}
	if err := removeIssueLabel(repo, picked.Number, cobblerLabelReady); err != nil {
		logf("pickReadyIssue: remove ready label from #%d: %v", picked.Number, err)
	}
	logf("pickReadyIssue: picked #%d %q gen=%s", picked.Number, picked.Title, generation)
	return picked, nil
}

// closeCobblerIssue closes a GitHub issue and re-runs promoteReadyIssues so
// any unblocked issues become ready.
func closeCobblerIssue(repo string, number int, generation string) error {
	if err := removeIssueLabel(repo, number, cobblerLabelInProgress); err != nil {
		logf("closeCobblerIssue: remove in-progress label from #%d: %v", number, err)
	}
	if err := exec.Command(binGh, "issue", "close",
		"--repo", repo,
		fmt.Sprintf("%d", number),
	).Run(); err != nil {
		return fmt.Errorf("gh issue close #%d: %w", number, err)
	}
	logf("closeCobblerIssue: closed #%d", number)

	if err := promoteReadyIssues(repo, generation); err != nil {
		logf("closeCobblerIssue: promoteReadyIssues warning: %v", err)
	}
	return nil
}

// removeInProgressLabel removes the cobbler-in-progress label from an issue,
// returning it to cobbler-ready state. Used by resetTask.
func removeInProgressLabel(repo string, number int) error {
	return removeIssueLabel(repo, number, cobblerLabelInProgress)
}

// closeGenerationIssues closes all open issues scoped to a generation.
// Used during reset or cleanup of a failed generation.
func closeGenerationIssues(repo, generation string) error {
	if generation == "" {
		return nil
	}
	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		return fmt.Errorf("closeGenerationIssues: list: %w", err)
	}
	if len(issues) == 0 {
		logf("closeGenerationIssues: no open issues for generation %s", generation)
		return nil
	}
	logf("closeGenerationIssues: closing %d issue(s) for generation %s", len(issues), generation)
	for _, iss := range issues {
		if err := exec.Command(binGh, "issue", "close",
			"--repo", repo,
			fmt.Sprintf("%d", iss.Number),
		).Run(); err != nil {
			logf("closeGenerationIssues: close #%d warning: %v", iss.Number, err)
		}
	}
	return nil
}

// gcStaleGenerationIssues closes open issues whose generation branch no
// longer exists locally. This catches leaked issues from crashed tests,
// killed processes, or GeneratorStop runs that predated the cleanup fix.
// It fetches all open issues in a single API call, filters locally for
// cobbler-gen-* labels, groups by generation, and closes issues for
// missing branches. Cost: 1 API call for discovery + 1 per stale issue.
func gcStaleGenerationIssues(repo, generationPrefix string) {
	// Fetch all open issues in a single API call and filter locally for
	// cobbler-gen-* labels. This replaces the previous O(labels) approach
	// that listed all labels then queried issues per label.
	out, err := exec.Command(binGh, "api",
		fmt.Sprintf("repos/%s/issues", repo),
		"--method", "GET",
		"-f", "state=open",
		"-f", "per_page=100",
	).Output()
	if err != nil {
		logf("gcStaleGenerationIssues: list issues: %v", err)
		return
	}

	var raw []struct {
		Number int `json:"number"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		logf("gcStaleGenerationIssues: parse issues: %v", err)
		return
	}

	// Group issue numbers by generation name.
	// We read the generation from the YAML front-matter (cobbler_generation) in
	// the issue body. This is the source of truth; the label name may be a
	// truncated/hashed form when the full generation name exceeds 50 chars.
	byGeneration := make(map[string][]int)
	for _, issue := range raw {
		// Only consider issues that carry a cobbler-gen-* label.
		hasGenLabel := false
		for _, label := range issue.Labels {
			if strings.HasPrefix(label.Name, cobblerGenLabelPrefix) {
				hasGenLabel = true
				break
			}
		}
		if !hasGenLabel {
			continue
		}
		fm, _ := parseIssueFrontMatter(issue.Body)
		gen := fm.Generation
		if gen == "" || !strings.HasPrefix(gen, generationPrefix) {
			continue
		}
		byGeneration[gen] = append(byGeneration[gen], issue.Number)
	}

	// Close issues for generations whose branch no longer exists locally.
	for generation, numbers := range byGeneration {
		if gitBranchExists(generation, ".") {
			continue
		}
		logf("gcStaleGenerationIssues: branch %s gone, closing %d issue(s)", generation, len(numbers))
		for _, num := range numbers {
			if err := exec.Command(binGh, "issue", "close",
				"--repo", repo,
				fmt.Sprintf("%d", num),
			).Run(); err != nil {
				logf("gcStaleGenerationIssues: close #%d: %v", num, err)
			}
		}
	}
}

// listActiveIssuesContext returns a JSON array of ContextIssue objects for all
// open issues in the generation, suitable for injection into the measure prompt.
// The JSON format matches what parseIssuesJSON expects.
func listActiveIssuesContext(repo, generation string) (string, error) {
	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		return "", fmt.Errorf("listActiveIssuesContext: %w", err)
	}
	if len(issues) == 0 {
		return "", nil
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Index < issues[j].Index })
	return issuesContextJSON(issues)
}

// issuesContextJSON converts a slice of cobblerIssue into the JSON string
// expected by parseIssuesJSON. Exported for testing.
func issuesContextJSON(issues []cobblerIssue) (string, error) {
	ctx := make([]ContextIssue, len(issues))
	for i, iss := range issues {
		status := "backfill"
		if hasLabel(iss, cobblerLabelInProgress) {
			status = "in_progress"
		} else if hasLabel(iss, cobblerLabelReady) {
			status = "ready"
		}
		ctx[i] = ContextIssue{
			ID:     fmt.Sprintf("%d", iss.Number),
			Title:  iss.Title,
			Status: status,
		}
	}
	b, err := json.Marshal(ctx)
	if err != nil {
		return "", fmt.Errorf("issuesContextJSON: %w", err)
	}
	return string(b), nil
}

// addIssueLabel adds a label to a GitHub issue via the API.
func addIssueLabel(repo string, number int, label string) error {
	return exec.Command(binGh, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--add-label", label,
	).Run()
}

// removeIssueLabel removes a label from a GitHub issue via the API.
func removeIssueLabel(repo string, number int, label string) error {
	return exec.Command(binGh, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--remove-label", label,
	).Run()
}

// ghExec runs a gh subcommand with dir set to repoRoot and returns stdout.
// Used by detectGitHubRepo.
func ghExec(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command(binGh, args...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// goModModulePath reads the module path from the go.mod in repoRoot.
func goModModulePath(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// resolveTargetRepo returns the GitHub owner/repo string for the project being
// developed. It checks cfg.Project.TargetRepo first; if empty it strips
// "github.com/" from cfg.Project.ModulePath. Returns "" if neither yields a
// non-empty value. Intentionally separate from detectGitHubRepo to avoid
// cobbler.issues_repo contaminating target resolution (prd003 R11.4, D2).
func resolveTargetRepo(cfg Config) string {
	if cfg.Project.TargetRepo != "" {
		return cfg.Project.TargetRepo
	}
	if strings.HasPrefix(cfg.Project.ModulePath, "github.com/") {
		return strings.TrimPrefix(cfg.Project.ModulePath, "github.com/")
	}
	return ""
}

// commentCobblerIssue posts a comment on a GitHub issue. Errors are logged
// but do not fail the caller — commenting is best-effort.
func commentCobblerIssue(repo string, number int, body string) {
	if repo == "" || number <= 0 {
		return
	}
	out, err := exec.Command(binGh, "issue", "comment",
		fmt.Sprintf("%d", number),
		"--repo", repo,
		"--body", body,
	).CombinedOutput()
	if err != nil {
		logf("commentCobblerIssue: gh issue comment failed for #%d: %v (output: %s)", number, err, strings.TrimSpace(string(out)))
		return
	}
	logf("commentCobblerIssue: posted comment on #%d", number)
}

// fileTargetRepoDefects files each defect as a GitHub bug issue in repo.
// Errors are logged but do not fail the caller — filing is best-effort
// (prd003 R11.5, R11.6). If repo is empty the call is a no-op with a
// warning log (prd003 R11.7).
func fileTargetRepoDefects(repo string, defects []string) {
	if repo == "" {
		logf("fileTargetRepoDefects: no target repo configured; skipping %d defect(s)", len(defects))
		return
	}
	for _, defect := range defects {
		title := "Defect: " + defect
		if len(title) > 68 { // keep title under ~70 chars
			title = title[:68] + "..."
		}
		body := "## Defect detected by cobbler:measure\n\n" + defect
		out, err := exec.Command(binGh, "issue", "create",
			"--repo", repo,
			"--title", title,
			"--body", body,
			"--label", "bug",
		).CombinedOutput()
		if err != nil {
			logf("fileTargetRepoDefects: gh issue create failed for %q: %v (output: %s)", defect, err, string(out))
			continue
		}
		logf("fileTargetRepoDefects: filed defect issue in %s: %s", repo, strings.TrimSpace(string(out)))
	}
}
