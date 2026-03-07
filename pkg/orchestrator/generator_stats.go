// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// generatorIssueStats holds per-issue stats derived from labels and comments.
type generatorIssueStats struct {
	cobblerIssue
	status    string  // "done", "failed", "in-progress", "pending"
	costUSD   float64
	durationS int
	prds      []string
}

// GeneratorStats prints a status report for the current generation run.
// It discovers active generation branches, fetches all task issues, parses
// progress comments, and prints an issue table with aggregate totals.
func (o *Orchestrator) GeneratorStats() error {
	branches := o.listGenerationBranches()
	if len(branches) == 0 {
		fmt.Println("no active generation branches found")
		return nil
	}

	// Prefer the configured branch; fall back to the first detected branch.
	genBranch := o.cfg.Generation.Branch
	if genBranch == "" {
		genBranch = branches[0]
	}

	repo, err := detectGitHubRepo(".", o.cfg)
	if err != nil || repo == "" {
		return fmt.Errorf("detecting GitHub repo: %w", err)
	}

	issues, err := listAllCobblerIssues(repo, genBranch)
	if err != nil {
		return fmt.Errorf("listing cobbler issues for %s: %w", genBranch, err)
	}
	if len(issues) == 0 {
		fmt.Printf("generation %s: no task issues found\n", genBranch)
		return nil
	}

	// Collect per-issue stats.
	rows := make([]generatorIssueStats, 0, len(issues))
	var totalCost float64
	var nDone, nFailed, nInProgress, nPending int
	prdStatus := make(map[string]string) // prd name → highest-priority status

	for _, iss := range issues {
		s := generatorIssueStats{cobblerIssue: iss}

		switch {
		case iss.State == "closed" && !hasLabel(iss, "failed"):
			s.status = "done"
			nDone++
		case iss.State == "closed":
			s.status = "failed"
			nFailed++
		case hasLabel(iss, cobblerLabelInProgress):
			s.status = "in-progress"
			nInProgress++
		default:
			s.status = "pending"
			nPending++
		}

		// Parse stitch progress comments for cost and duration.
		comments, _ := fetchIssueComments(repo, iss.Number)
		for _, c := range comments {
			if p := parseStitchComment(c); p.costUSD > 0 {
				s.costUSD += p.costUSD
			}
			if p := parseStitchComment(c); p.durationS > 0 {
				s.durationS = p.durationS
			}
		}
		totalCost += s.costUSD

		// Extract PRD references and track coverage.
		s.prds = extractPRDRefs(iss.Title + " " + iss.Description)
		for _, prd := range s.prds {
			existing := prdStatus[prd]
			switch s.status {
			case "in-progress":
				prdStatus[prd] = "in-progress"
			case "pending":
				if existing == "" {
					prdStatus[prd] = "pending"
				}
			case "done", "failed":
				if existing == "" {
					prdStatus[prd] = s.status
				}
			}
		}

		rows = append(rows, s)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Index < rows[j].Index })

	// Header.
	fmt.Printf("Generation: %s\n", genBranch)
	if len(branches) > 1 {
		fmt.Printf("Other branches: %s\n", strings.Join(branches[1:], ", "))
	}
	fmt.Printf("Tasks: %d done, %d in-progress, %d pending", nDone, nInProgress, nPending)
	if nFailed > 0 {
		fmt.Printf(", %d failed", nFailed)
	}
	fmt.Println()
	fmt.Printf("Total cost: $%.2f\n\n", totalCost)

	// Issue table.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tIdx\tStatus\tCost\tDuration\tTitle")
	for _, r := range rows {
		cost := "-"
		if r.costUSD > 0 {
			cost = fmt.Sprintf("$%.2f", r.costUSD)
		}
		dur := "-"
		if r.durationS > 0 {
			dur = formatDuration(r.durationS)
		}
		title := r.Title
		if len(title) > 48 {
			title = title[:45] + "..."
		}
		fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%s\n",
			r.Number, r.Index, r.status, cost, dur, title)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// PRD coverage table.
	if len(prdStatus) > 0 {
		prds := make([]string, 0, len(prdStatus))
		for prd := range prdStatus {
			prds = append(prds, prd)
		}
		sort.Strings(prds)

		fmt.Println()
		pw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(pw, "PRD\tStatus")
		for _, prd := range prds {
			fmt.Fprintf(pw, "%s\t%s\n", prd, prdStatus[prd])
		}
		if err := pw.Flush(); err != nil {
			return err
		}
	}

	// Requirements progress.
	total, byPRD := countTotalPRDRequirements()
	if total > 0 {
		addressed := 0
		for prd, status := range prdStatus {
			if status == "done" || status == "in-progress" {
				addressed += byPRD[prd]
			}
		}
		pct := 0
		if total > 0 {
			pct = addressed * 100 / total
		}
		fmt.Printf("\nRequirements: %d/%d addressed by this generation (%d%%)\n", addressed, total, pct)
	}

	return nil
}

// stitchCommentData holds metrics extracted from a stitch progress comment.
type stitchCommentData struct {
	costUSD   float64
	durationS int
}

// parseStitchComment extracts cost and duration from a stitch progress comment
// produced by closeStitchTask or failTask (GH-567 format):
//
//	"Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42."
//	"Stitch failed after 2m 10s. Error: ..."
func parseStitchComment(body string) stitchCommentData {
	var d stitchCommentData

	// Parse "Cost: $X.XX"
	if i := strings.Index(body, "Cost: $"); i >= 0 {
		rest := body[i+7:]
		var costStr string
		fmt.Sscanf(rest, "%s", &costStr)
		costStr = strings.TrimRight(costStr, ".,;")
		if v, err := strconv.ParseFloat(costStr, 64); err == nil {
			d.costUSD = v
		}
	}

	// Parse "in Xm Ys" or "after Xm Ys" for duration.
	for _, marker := range []string{"in ", "after "} {
		if i := strings.Index(body, marker); i >= 0 {
			rest := body[i+len(marker):]
			var mins, secs int
			if n, _ := fmt.Sscanf(rest, "%dm %ds", &mins, &secs); n == 2 {
				d.durationS = mins*60 + secs
				break
			}
			if n, _ := fmt.Sscanf(rest, "%ds", &secs); n == 1 {
				d.durationS = secs
				break
			}
		}
	}

	return d
}

// countTotalPRDRequirements loads all PRD files and counts the total number of
// requirement items across all groups. Returns the total count and a map from
// PRD short name (e.g. "prd-003") to its item count for cross-referencing with
// generation task PRD references.
func countTotalPRDRequirements() (int, map[string]int) {
	paths, _ := filepath.Glob("docs/specs/product-requirements/prd*.yaml")
	byPRD := make(map[string]int, len(paths))
	total := 0
	for _, path := range paths {
		prd := loadYAML[PRDDoc](path)
		if prd == nil {
			continue
		}
		count := 0
		for _, group := range prd.Requirements {
			count += len(group.Items)
		}
		total += count
		// Store under the short prd-NNN name that extractPRDRefs produces.
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if idx := strings.IndexByte(stem, '-'); idx > 0 {
			// "prd003-cobbler-workflows" → "prd-003-cobbler-workflows" matches
			// extractPRDRefs output like "prd-003". Store both forms.
			byPRD[stem] = count
		}
		// extractPRDRefs produces "prd-NNN" form, so convert "prd003" → "prd-003".
		if len(stem) >= 6 && stem[:3] == "prd" {
			short := "prd-" + stem[3:6]
			byPRD[short] = count
		}
	}
	return total, byPRD
}

// extractPRDRefs returns deduplicated prd-* tokens found in text.
func extractPRDRefs(text string) []string {
	seen := make(map[string]bool)
	var prds []string
	for _, word := range strings.Fields(text) {
		w := strings.ToLower(strings.Trim(word, ".,;:()[]`\"'"))
		if strings.HasPrefix(w, "prd-") && len(w) > 4 {
			if !seen[w] {
				seen[w] = true
				prds = append(prds, w)
			}
		}
	}
	return prds
}
