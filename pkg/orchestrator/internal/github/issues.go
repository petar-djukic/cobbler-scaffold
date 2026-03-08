// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package github implements GitHub issue management for the orchestrator.
// It handles cobbler issue creation, listing, labeling, DAG promotion,
// and lifecycle management via the gh CLI.
package github

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

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// BranchChecker returns true if a git branch exists in the given directory.
type BranchChecker func(branch, dir string) bool

// Deps holds external dependencies injected by the parent package.
type Deps struct {
	Log            Logger
	GhBin          string
	BranchExists   BranchChecker
}

// RepoConfig holds the minimal configuration fields needed by this package.
type RepoConfig struct {
	IssuesRepo string // cobbler.issues_repo override
	ModulePath string // project.module_path for fallback detection
	TargetRepo string // project.target_repo for defect filing
}

// CobblerIssue holds the parsed representation of a GitHub issue created by
// the orchestrator. Fields are populated from the issue's YAML front-matter.
type CobblerIssue struct {
	Number      int    // GitHub issue number
	Title       string // Issue title
	State       string // "open" or "closed"
	Index       int    // cobbler_index from front-matter
	DependsOn   int    // cobbler_depends_on (-1 = no dependency)
	Generation  string // cobbler_generation label value
	Description string // Body text below the front-matter block
	Labels      []string
}

// CobblerFrontMatter is the YAML front-matter embedded at the top of every
// GitHub issue created by the orchestrator.
type CobblerFrontMatter struct {
	Generation string `yaml:"cobbler_generation"`
	Index      int    `yaml:"cobbler_index"`
	DependsOn  int    `yaml:"cobbler_depends_on"`
}

// ProposedIssue represents an issue proposed by measure for creation on GitHub.
type ProposedIssue struct {
	Index       int    `yaml:"index"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Dependency  int    `yaml:"dependency"`
}

// ContextIssue represents an issue tracker entry in the project context.
// It captures the fields needed for Claude to avoid creating duplicate
// issues during measure.
type ContextIssue struct {
	ID     string `yaml:"id"     json:"id"`
	Title  string `yaml:"title"  json:"title"`
	Status string `yaml:"status" json:"status"`
	Type   string `yaml:"type"   json:"type"`
}

// LabelReady and LabelInProgress are the two status labels
// applied to orchestrator issues during their lifecycle.
const (
	LabelReady      = "cobbler-ready"
	LabelInProgress = "cobbler-in-progress"
)

// GenLabelPrefix is the prefix for generation-scoped labels.
const GenLabelPrefix = "cobbler-gen-"

// GenLabel returns the generation label for a given generation name.
// GitHub enforces a 50-character maximum on label names. When the full label
// would exceed 50 chars, we keep the prefix (12 chars) plus the first 29 chars
// of the generation name, a hyphen, and an 8-char FNV-32 hex digest of the
// full generation name — yielding exactly 50 chars and remaining deterministic.
func GenLabel(generation string) string {
	const maxLen = 50
	label := GenLabelPrefix + generation
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
	return fmt.Sprintf("%s%s-%08x", GenLabelPrefix, truncated, h.Sum32())
}

// FormatIssueFrontMatter formats the YAML front-matter block for an issue body.
func FormatIssueFrontMatter(generation string, index, dependsOn int) string {
	if dependsOn < 0 {
		return fmt.Sprintf("---\ncobbler_generation: %s\ncobbler_index: %d\n---\n\n",
			generation, index)
	}
	return fmt.Sprintf("---\ncobbler_generation: %s\ncobbler_index: %d\ncobbler_depends_on: %d\n---\n\n",
		generation, index, dependsOn)
}

// ParseIssueFrontMatter splits a GitHub issue body into its YAML front-matter
// and description parts. Returns zero-value front-matter on parse failure.
func ParseIssueFrontMatter(body string) (CobblerFrontMatter, string) {
	// Expect body to start with "---\n".
	if !strings.HasPrefix(body, "---\n") {
		return CobblerFrontMatter{DependsOn: -1}, body
	}
	// Find the closing "---".
	rest := body[4:] // skip opening ---\n
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return CobblerFrontMatter{DependsOn: -1}, body
	}
	yamlBlock := rest[:idx]
	description := strings.TrimPrefix(rest[idx+5:], "\n") // skip \n---\n and leading newline

	var fm CobblerFrontMatter
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

// DetectGitHubRepo resolves the GitHub owner/repo string for the target project.
// Resolution order:
//  1. cfg.IssuesRepo if set (explicit override, used for testing)
//  2. `gh repo view --json nameWithOwner` run in repoRoot (reads git remote)
//  3. Strip "github.com/" from cfg.ModulePath
func DetectGitHubRepo(repoRoot string, cfg RepoConfig, deps Deps) (string, error) {
	if cfg.IssuesRepo != "" {
		return cfg.IssuesRepo, nil
	}

	// Try gh repo view in the repo root.
	cmd := exec.Command(deps.GhBin, "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner")
	cmd.Dir = repoRoot
	if out, err := cmd.Output(); err == nil {
		if repo := strings.TrimSpace(string(out)); repo != "" {
			return repo, nil
		}
	}

	// Fall back to module path.
	if strings.HasPrefix(cfg.ModulePath, "github.com/") {
		return strings.TrimPrefix(cfg.ModulePath, "github.com/"), nil
	}

	return "", fmt.Errorf("cannot determine GitHub repo: set cobbler.issues_repo in configuration.yaml or ensure the project has a github.com module path")
}

// EnsureCobblerLabels creates the cobbler-ready and cobbler-in-progress labels
// on the target repo if they do not already exist. Idempotent.
func EnsureCobblerLabels(repo string, deps Deps) error {
	existing := ListRepoLabels(repo, deps)
	existingSet := make(map[string]bool, len(existing))
	for _, l := range existing {
		existingSet[l] = true
	}

	labels := []struct {
		name  string
		color string
		desc  string
	}{
		{LabelReady, "0075ca", "Cobbler task ready to be picked by stitch"},
		{LabelInProgress, "e4e669", "Cobbler task currently being worked on"},
	}

	for _, l := range labels {
		if existingSet[l.name] {
			continue
		}
		cmd := exec.Command(deps.GhBin, "api", "repos/"+repo+"/labels",
			"--method", "POST",
			"--field", "name="+l.name,
			"--field", "color="+l.color,
			"--field", "description="+l.desc,
		)
		if out, err := cmd.Output(); err != nil {
			deps.Log("ensureCobblerLabels: could not create label %q: %v (output: %s)", l.name, err, string(out))
		} else {
			deps.Log("ensureCobblerLabels: created label %q on %s", l.name, repo)
		}
	}
	return nil
}

// ListRepoLabels returns the names of all labels on the repo.
func ListRepoLabels(repo string, deps Deps) []string {
	out, err := exec.Command(deps.GhBin, "label", "list", "--repo", repo, "--json", "name", "--limit", "100").Output()
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

// EnsureCobblerGenLabel creates the generation-scoped label on the repo if
// it does not already exist.
func EnsureCobblerGenLabel(repo, generation string, deps Deps) error {
	label := GenLabel(generation)
	cmd := exec.Command(deps.GhBin, "api", "repos/"+repo+"/labels",
		"--method", "POST",
		"--field", "name="+label,
		"--field", "color=ededed", // light grey; GitHub API requires a valid 6-char hex color
		"--field", "description=Cobbler generation "+generation,
	)
	// Ignore error — label may already exist (422 Unprocessable Entity).
	cmd.Run() //nolint:errcheck // best-effort
	return nil
}

// CreateMeasuringPlaceholder creates a transient GitHub issue that signals
// the measure agent is actively calling Claude for iteration i (1-based).
// The issue carries no cobbler-ready label so stitch won't pick it up.
// Callers must call CloseMeasuringPlaceholder after the iteration completes.
func CreateMeasuringPlaceholder(repo, generation string, iteration int, deps Deps) (int, error) {
	title := fmt.Sprintf("[measuring] %s task %d", generation, iteration)
	body := fmt.Sprintf("Cobbler measure is calling Claude to propose task %d for generation %s.\n\nThis issue will be closed automatically when measure completes.", iteration, generation)
	// No cobbler labels: stitch ignores issues without a gen label, and the
	// placeholder must not appear in the existing-issues context sent to Claude.
	out, err := exec.Command(deps.GhBin, "issue", "create",
		"--repo", repo,
		"--title", title,
		"--body", body,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("gh issue create placeholder: %w", err)
	}
	number, err := ParseIssueURL(string(out))
	if err != nil {
		return 0, err
	}
	deps.Log("createMeasuringPlaceholder: created #%d for iteration %d", number, iteration)
	return number, nil
}

// CloseMeasuringPlaceholder closes the placeholder issue created by
// CreateMeasuringPlaceholder. Best-effort: logs and ignores errors.
func CloseMeasuringPlaceholder(repo string, number int, deps Deps) {
	if err := exec.Command(deps.GhBin, "issue", "close",
		"--repo", repo,
		fmt.Sprintf("%d", number),
	).Run(); err != nil {
		deps.Log("closeMeasuringPlaceholder: close #%d warning: %v", number, err)
		return
	}
	deps.Log("closeMeasuringPlaceholder: closed #%d", number)
}

// CloseMeasuringPlaceholderWithComment closes the placeholder issue and adds a
// comment explaining why it was closed. Used on error paths to avoid orphans
// (GH-747). Best-effort: logs and ignores errors.
func CloseMeasuringPlaceholderWithComment(repo string, number int, comment string, deps Deps) {
	if err := exec.Command(deps.GhBin, "issue", "comment",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--body", comment,
	).Run(); err != nil {
		deps.Log("closeMeasuringPlaceholderWithComment: comment on #%d warning: %v", number, err)
	}
	CloseMeasuringPlaceholder(repo, number, deps)
}

// UpgradeMeasuringPlaceholder converts the transient measuring placeholder
// into the task issue in-place. It edits the placeholder's title and body
// to match the proposed issue, adds the cobbler-gen label so stitch can
// pick it up, and links it as a sub-issue of the parent generation issue
// if the generation name encodes one (GH-578).
func UpgradeMeasuringPlaceholder(repo string, number int, generation string, issue ProposedIssue, deps Deps) error {
	body := FormatIssueFrontMatter(generation, issue.Index, issue.Dependency) + issue.Description
	title := "[measure] " + issue.Title

	// Edit title and body in one command.
	if err := exec.Command(deps.GhBin, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--title", title,
		"--body", body,
	).Run(); err != nil {
		return fmt.Errorf("gh issue edit placeholder #%d: %w", number, err)
	}

	// Add cobbler-gen label so stitch can pick it up.
	if err := AddIssueLabel(repo, number, GenLabel(generation), deps); err != nil {
		return fmt.Errorf("adding gen label to #%d: %w", number, err)
	}

	deps.Log("upgradeMeasuringPlaceholder: upgraded #%d %q gen=%s index=%d dep=%d",
		number, title, generation, issue.Index, issue.Dependency)

	// Link as sub-issue of the parent if the generation name encodes one.
	if parentNumber := ExtractParentIssueNumber(generation); parentNumber > 0 {
		if err := LinkSubIssue(repo, parentNumber, number, deps); err != nil {
			deps.Log("upgradeMeasuringPlaceholder: linkSubIssue warning for #%d -> parent #%d: %v", number, parentNumber, err)
		}
	}
	return nil
}

// CreateCobblerIssue creates a GitHub issue on repo for the given generation
// and ProposedIssue. Returns the GitHub issue number.
//
// Note: gh issue create (v2.87.3) does not support --json; it outputs the
// issue URL (https://github.com/owner/repo/issues/123) on success.
func CreateCobblerIssue(repo, generation string, issue ProposedIssue, deps Deps) (int, error) {
	body := FormatIssueFrontMatter(generation, issue.Index, issue.Dependency) + issue.Description
	title := "[measure] " + issue.Title

	genLabel := GenLabel(generation)
	out, err := exec.Command(deps.GhBin, "issue", "create",
		"--repo", repo,
		"--title", title,
		"--body", body,
		"--label", genLabel,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("gh issue create: %w", err)
	}

	number, err := ParseIssueURL(string(out))
	if err != nil {
		return 0, err
	}
	deps.Log("createCobblerIssue: created #%d %q gen=%s index=%d dep=%d",
		number, title, generation, issue.Index, issue.Dependency)

	// Link as sub-issue of the parent, if the generation name encodes one (GH-566).
	if parentNumber := ExtractParentIssueNumber(generation); parentNumber > 0 {
		if err := LinkSubIssue(repo, parentNumber, number, deps); err != nil {
			deps.Log("createCobblerIssue: linkSubIssue warning for #%d -> parent #%d: %v", number, parentNumber, err)
		}
	}

	return number, nil
}

// ExtractParentIssueNumber parses a GitHub issue number from a generation name
// that follows the pattern "...-gh-<N>-..." (e.g., "generation-gh-206-slug"
// → 206). Returns 0 if the pattern is not found.
func ExtractParentIssueNumber(generation string) int {
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

// LinkSubIssue attaches childNumber as a GitHub sub-issue of parentNumber.
// It first fetches the child's database ID, then POSTs to the sub_issues API.
// Errors are returned so the caller can log them as warnings.
func LinkSubIssue(repo string, parentNumber, childNumber int, deps Deps) error {
	// Fetch the child issue's database ID (different from the display number).
	dbIDOut, err := exec.Command(deps.GhBin, "api",
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
	out, err := exec.Command(deps.GhBin, "api",
		fmt.Sprintf("repos/%s/issues/%d/sub_issues", repo, parentNumber),
		"--method", "POST",
		"--field", fmt.Sprintf("sub_issue_id=%d", dbID),
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("linking #%d as sub-issue of #%d: %w (output: %s)",
			childNumber, parentNumber, err, strings.TrimSpace(string(out)))
	}
	deps.Log("linkSubIssue: linked #%d as sub-issue of #%d", childNumber, parentNumber)
	return nil
}

// ParseIssueURL extracts a GitHub issue number from a URL string like
// "https://github.com/owner/repo/issues/123\n". Returns an error for
// malformed or empty output.
func ParseIssueURL(raw string) (int, error) {
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

// ListOpenCobblerIssues returns all open GitHub issues for a generation.
// It uses the REST API endpoint (gh api repos/.../issues) rather than
// gh issue list, because gh issue list uses GitHub's search API which is
// eventually consistent and can return stale results immediately after
// label changes. The REST endpoint reads directly from the database.
func ListOpenCobblerIssues(repo, generation string, deps Deps) ([]CobblerIssue, error) {
	label := GenLabel(generation)
	out, err := exec.Command(deps.GhBin, "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=open",
		"-f", "labels="+label,
		"-f", "per_page=100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api repos issues: %w", err)
	}

	return ParseCobblerIssuesJSON(out)
}

// ListAllCobblerIssues returns all GitHub issues (open and closed) for a
// generation. Used by GeneratorStats to report completed tasks.
func ListAllCobblerIssues(repo, generation string, deps Deps) ([]CobblerIssue, error) {
	label := GenLabel(generation)
	out, err := exec.Command(deps.GhBin, "api",
		"--method", "GET",
		fmt.Sprintf("repos/%s/issues", repo),
		"-f", "state=all",
		"-f", "labels="+label,
		"-f", "per_page=100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api repos issues: %w", err)
	}
	return ParseCobblerIssuesJSON(out)
}

// FetchIssueComments returns the body text of all comments on the given issue.
func FetchIssueComments(repo string, number int, deps Deps) ([]string, error) {
	out, err := exec.Command(deps.GhBin, "api",
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

// ParseCobblerIssuesJSON parses the JSON output from the GitHub REST API issues
// endpoint into a slice of CobblerIssue structs.
func ParseCobblerIssuesJSON(data []byte) ([]CobblerIssue, error) {
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

	issues := make([]CobblerIssue, 0, len(raw))
	for _, r := range raw {
		fm, desc := ParseIssueFrontMatter(r.Body)
		labelNames := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labelNames = append(labelNames, l.Name)
		}
		issues = append(issues, CobblerIssue{
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

// WaitForIssuesVisible polls ListOpenCobblerIssues until at least
// expected issues appear or the timeout expires. The REST API label
// index may lag briefly after issue creation, so this function
// ensures all issues are visible before promotion or DAG resolution.
func WaitForIssuesVisible(repo, generation string, expected int, deps Deps) {
	const maxWait = 15 * time.Second
	const interval = time.Second
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		issues, err := ListOpenCobblerIssues(repo, generation, deps)
		if err == nil && len(issues) >= expected {
			return
		}
		deps.Log("waitForIssuesVisible: %d/%d visible, retrying...", len(issues), expected)
		time.Sleep(interval)
	}
	deps.Log("waitForIssuesVisible: timed out waiting for %d issues (generation=%s)", expected, generation)
}

// HasLabel returns true if the issue has the given label.
func HasLabel(issue CobblerIssue, label string) bool {
	for _, l := range issue.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// PromoteReadyIssues builds the DAG from open issues and applies
// cobbler-ready to unblocked issues. Issues whose dependency is still open
// have cobbler-ready removed.
func PromoteReadyIssues(repo, generation string, deps Deps) error {
	issues, err := ListOpenCobblerIssues(repo, generation, deps)
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
		currentlyReady := HasLabel(iss, LabelReady)

		if !blocked && !currentlyReady {
			if err := AddIssueLabel(repo, iss.Number, LabelReady, deps); err != nil {
				deps.Log("promoteReadyIssues: add ready label to #%d: %v", iss.Number, err)
			}
		} else if blocked && currentlyReady {
			if err := RemoveIssueLabel(repo, iss.Number, LabelReady, deps); err != nil {
				deps.Log("promoteReadyIssues: remove ready label from #%d: %v", iss.Number, err)
			}
		}
	}
	return nil
}

// PickReadyIssue promotes ready issues then picks the lowest-numbered
// cobbler-ready issue, adds cobbler-in-progress, and returns it.
func PickReadyIssue(repo, generation string, deps Deps) (CobblerIssue, error) {
	if err := PromoteReadyIssues(repo, generation, deps); err != nil {
		return CobblerIssue{}, fmt.Errorf("pickReadyIssue promote: %w", err)
	}

	issues, err := ListOpenCobblerIssues(repo, generation, deps)
	if err != nil {
		return CobblerIssue{}, fmt.Errorf("pickReadyIssue list: %w", err)
	}

	// Filter to ready issues and sort by number ascending.
	var ready []CobblerIssue
	for _, iss := range issues {
		if HasLabel(iss, LabelReady) && !HasLabel(iss, LabelInProgress) {
			ready = append(ready, iss)
		}
	}
	if len(ready) == 0 {
		return CobblerIssue{}, fmt.Errorf("no ready issues for generation %s", generation)
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Number < ready[j].Number })

	picked := ready[0]
	if err := AddIssueLabel(repo, picked.Number, LabelInProgress, deps); err != nil {
		deps.Log("pickReadyIssue: add in-progress label to #%d: %v", picked.Number, err)
	}
	if err := RemoveIssueLabel(repo, picked.Number, LabelReady, deps); err != nil {
		deps.Log("pickReadyIssue: remove ready label from #%d: %v", picked.Number, err)
	}

	// Rename [measure] → [stitch] so stats:generator shows which phase executed the task.
	if strings.HasPrefix(picked.Title, "[measure] ") {
		picked.Title = "[stitch] " + strings.TrimPrefix(picked.Title, "[measure] ")
		if err := EditIssueTitle(repo, picked.Number, picked.Title, deps); err != nil {
			deps.Log("pickReadyIssue: rename title warning for #%d: %v", picked.Number, err)
		}
	}

	deps.Log("pickReadyIssue: picked #%d %q gen=%s", picked.Number, picked.Title, generation)
	return picked, nil
}

// EditIssueTitle updates the title of a GitHub issue.
func EditIssueTitle(repo string, number int, title string, deps Deps) error {
	return exec.Command(deps.GhBin, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--title", title,
	).Run()
}

// NormalizeIssueTitle strips [measure]/[stitch] prefixes and trims whitespace
// so that proposed titles can be compared against existing issues (GH-1026).
func NormalizeIssueTitle(title string) string {
	t := strings.TrimSpace(title)
	for _, prefix := range []string{"[measure] ", "[stitch] "} {
		t = strings.TrimPrefix(t, prefix)
	}
	return strings.TrimSpace(t)
}

// CloseCobblerIssue closes a GitHub issue and re-runs PromoteReadyIssues so
// any unblocked issues become ready.
func CloseCobblerIssue(repo string, number int, generation string, deps Deps) error {
	if err := RemoveIssueLabel(repo, number, LabelInProgress, deps); err != nil {
		deps.Log("closeCobblerIssue: remove in-progress label from #%d: %v", number, err)
	}
	if err := exec.Command(deps.GhBin, "issue", "close",
		"--repo", repo,
		fmt.Sprintf("%d", number),
	).Run(); err != nil {
		return fmt.Errorf("gh issue close #%d: %w", number, err)
	}
	deps.Log("closeCobblerIssue: closed #%d", number)

	if err := PromoteReadyIssues(repo, generation, deps); err != nil {
		deps.Log("closeCobblerIssue: promoteReadyIssues warning: %v", err)
	}
	return nil
}

// RemoveInProgressLabel removes the cobbler-in-progress label from an issue,
// returning it to cobbler-ready state. Used by resetTask.
func RemoveInProgressLabel(repo string, number int, deps Deps) error {
	return RemoveIssueLabel(repo, number, LabelInProgress, deps)
}

// CloseGenerationIssues closes all open issues scoped to a generation.
// Used during reset or cleanup of a failed generation.
func CloseGenerationIssues(repo, generation string, deps Deps) error {
	if generation == "" {
		return nil
	}
	issues, err := ListOpenCobblerIssues(repo, generation, deps)
	if err != nil {
		return fmt.Errorf("closeGenerationIssues: list: %w", err)
	}
	if len(issues) == 0 {
		deps.Log("closeGenerationIssues: no open issues for generation %s", generation)
		return nil
	}
	deps.Log("closeGenerationIssues: closing %d issue(s) for generation %s", len(issues), generation)
	for _, iss := range issues {
		if err := exec.Command(deps.GhBin, "issue", "close",
			"--repo", repo,
			fmt.Sprintf("%d", iss.Number),
		).Run(); err != nil {
			deps.Log("closeGenerationIssues: close #%d warning: %v", iss.Number, err)
		}
	}
	return nil
}

// GcStaleGenerationIssues closes open issues whose generation branch no
// longer exists locally. This catches leaked issues from crashed tests,
// killed processes, or GeneratorStop runs that predated the cleanup fix.
// It fetches all open issues in a single API call, filters locally for
// cobbler-gen-* labels, groups by generation, and closes issues for
// missing branches. Cost: 1 API call for discovery + 1 per stale issue.
func GcStaleGenerationIssues(repo, generationPrefix string, deps Deps) {
	// Fetch all open issues in a single API call and filter locally for
	// cobbler-gen-* labels. This replaces the previous O(labels) approach
	// that listed all labels then queried issues per label.
	out, err := exec.Command(deps.GhBin, "api",
		fmt.Sprintf("repos/%s/issues", repo),
		"--method", "GET",
		"-f", "state=open",
		"-f", "per_page=100",
	).Output()
	if err != nil {
		deps.Log("gcStaleGenerationIssues: list issues: %v", err)
		return
	}

	var raw []struct {
		Number int    `json:"number"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		deps.Log("gcStaleGenerationIssues: parse issues: %v", err)
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
			if strings.HasPrefix(label.Name, GenLabelPrefix) {
				hasGenLabel = true
				break
			}
		}
		if !hasGenLabel {
			continue
		}
		fm, _ := ParseIssueFrontMatter(issue.Body)
		gen := fm.Generation
		if gen == "" || !strings.HasPrefix(gen, generationPrefix) {
			continue
		}
		byGeneration[gen] = append(byGeneration[gen], issue.Number)
	}

	// Close issues for generations whose branch no longer exists locally.
	for generation, numbers := range byGeneration {
		if deps.BranchExists(generation, ".") {
			continue
		}
		deps.Log("gcStaleGenerationIssues: branch %s gone, closing %d issue(s)", generation, len(numbers))
		for _, num := range numbers {
			if err := exec.Command(deps.GhBin, "issue", "close",
				"--repo", repo,
				fmt.Sprintf("%d", num),
			).Run(); err != nil {
				deps.Log("gcStaleGenerationIssues: close #%d: %v", num, err)
			}
		}
	}
}

// ListActiveIssuesContext returns a JSON array of ContextIssue objects for all
// open issues in the generation, suitable for injection into the measure prompt.
// The JSON format matches what parseIssuesJSON expects.
func ListActiveIssuesContext(repo, generation string, deps Deps) (string, error) {
	issues, err := ListOpenCobblerIssues(repo, generation, deps)
	if err != nil {
		return "", fmt.Errorf("listActiveIssuesContext: %w", err)
	}
	if len(issues) == 0 {
		return "", nil
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Index < issues[j].Index })
	return IssuesContextJSON(issues)
}

// IssuesContextJSON converts a slice of CobblerIssue into the JSON string
// expected by parseIssuesJSON. Exported for testing.
func IssuesContextJSON(issues []CobblerIssue) (string, error) {
	ctx := make([]ContextIssue, len(issues))
	for i, iss := range issues {
		status := "backfill"
		if HasLabel(iss, LabelInProgress) {
			status = "in_progress"
		} else if HasLabel(iss, LabelReady) {
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

// AddIssueLabel adds a label to a GitHub issue via the API.
func AddIssueLabel(repo string, number int, label string, deps Deps) error {
	return exec.Command(deps.GhBin, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--add-label", label,
	).Run()
}

// RemoveIssueLabel removes a label from a GitHub issue via the API.
func RemoveIssueLabel(repo string, number int, label string, deps Deps) error {
	return exec.Command(deps.GhBin, "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", number),
		"--remove-label", label,
	).Run()
}

// GhExec runs a gh subcommand with dir set to repoRoot and returns stdout.
func GhExec(repoRoot string, deps Deps, args ...string) (string, error) {
	cmd := exec.Command(deps.GhBin, args...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// GoModModulePath reads the module path from the go.mod in repoRoot.
func GoModModulePath(repoRoot string) string {
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

// ResolveTargetRepo returns the GitHub owner/repo string for the project being
// developed. It checks cfg.TargetRepo first; if empty it strips
// "github.com/" from cfg.ModulePath. Returns "" if neither yields a
// non-empty value. Intentionally separate from DetectGitHubRepo to avoid
// cobbler.issues_repo contaminating target resolution (prd003 R11.4, D2).
func ResolveTargetRepo(cfg RepoConfig) string {
	if cfg.TargetRepo != "" {
		return cfg.TargetRepo
	}
	if strings.HasPrefix(cfg.ModulePath, "github.com/") {
		return strings.TrimPrefix(cfg.ModulePath, "github.com/")
	}
	return ""
}

// CommentCobblerIssue posts a comment on a GitHub issue. Errors are logged
// but do not fail the caller — commenting is best-effort.
func CommentCobblerIssue(repo string, number int, body string, deps Deps) {
	if repo == "" || number <= 0 {
		return
	}
	out, err := exec.Command(deps.GhBin, "issue", "comment",
		fmt.Sprintf("%d", number),
		"--repo", repo,
		"--body", body,
	).CombinedOutput()
	if err != nil {
		deps.Log("commentCobblerIssue: gh issue comment failed for #%d: %v (output: %s)", number, err, strings.TrimSpace(string(out)))
		return
	}
	deps.Log("commentCobblerIssue: posted comment on #%d", number)
}

// FileTargetRepoDefects files each defect as a GitHub bug issue in repo.
// Errors are logged but do not fail the caller — filing is best-effort
// (prd003 R11.5, R11.6). If repo is empty the call is a no-op with a
// warning log (prd003 R11.7).
func FileTargetRepoDefects(repo string, defects []string, deps Deps) {
	if repo == "" {
		deps.Log("fileTargetRepoDefects: no target repo configured; skipping %d defect(s)", len(defects))
		return
	}
	for _, defect := range defects {
		title := "Defect: " + defect
		if len(title) > 68 { // keep title under ~70 chars
			title = title[:68] + "..."
		}
		body := "## Defect detected by cobbler:measure\n\n" + defect
		out, err := exec.Command(deps.GhBin, "issue", "create",
			"--repo", repo,
			"--title", title,
			"--body", body,
			"--label", "bug",
		).CombinedOutput()
		if err != nil {
			deps.Log("fileTargetRepoDefects: gh issue create failed for %q: %v (output: %s)", defect, err, string(out))
			continue
		}
		deps.Log("fileTargetRepoDefects: filed defect issue in %s: %s", repo, strings.TrimSpace(string(out)))
	}
}
