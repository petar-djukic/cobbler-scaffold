// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// cobblerIssue holds the parsed representation of a GitHub issue created by
// the orchestrator. Fields are populated from the issue's YAML front-matter.
type cobblerIssue struct {
	Number      int    // GitHub issue number
	Title       string // Issue title
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
func cobblerGenLabel(generation string) string {
	return cobblerGenLabelPrefix + generation
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

	// Parse the issue number from the URL output (e.g. "https://github.com/owner/repo/issues/123\n").
	url := strings.TrimSpace(string(out))
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("parsing gh issue create output: empty URL (raw: %s)", string(out))
	}
	var number int
	if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &number); err != nil || number == 0 {
		return 0, fmt.Errorf("parsing gh issue create output: could not extract number from %q", url)
	}
	logf("createCobblerIssue: created #%d %q gen=%s index=%d dep=%d",
		number, issue.Title, generation, issue.Index, issue.Dependency)
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
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=open",
		"-f", "labels="+label,
		"-f", "per_page=100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api repos issues: %w", err)
	}

	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
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
			Index:       fm.Index,
			DependsOn:   fm.DependsOn,
			Generation:  fm.Generation,
			Description: desc,
			Labels:      labelNames,
		})
	}
	return issues, nil
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
	logf("pickReadyIssue: picked #%d %q gen=%s", picked.Number, picked.Title, generation)
	return picked, nil
}

// closeCobblerIssue closes a GitHub issue and re-runs promoteReadyIssues so
// any unblocked issues become ready.
func closeCobblerIssue(repo string, number int, generation string) error {
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

// listActiveIssuesContext returns a human-readable summary of all open
// issues for the generation, suitable for injection into the measure prompt.
func listActiveIssuesContext(repo, generation string) (string, error) {
	issues, err := listOpenCobblerIssues(repo, generation)
	if err != nil {
		return "", fmt.Errorf("listActiveIssuesContext: %w", err)
	}
	if len(issues) == 0 {
		return "", nil
	}

	sort.Slice(issues, func(i, j int) bool { return issues[i].Index < issues[j].Index })

	var sb strings.Builder
	for _, iss := range issues {
		status := "backfill"
		if hasLabel(iss, cobblerLabelInProgress) {
			status = "in_progress"
		} else if hasLabel(iss, cobblerLabelReady) {
			status = "ready"
		}
		fmt.Fprintf(&sb, "#%d (index=%d, status=%s): %s\n", iss.Number, iss.Index, status, iss.Title)
	}
	return sb.String(), nil
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
