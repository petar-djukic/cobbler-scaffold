// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// generatorIssueStats holds per-issue stats derived from labels and comments.
type generatorIssueStats struct {
	cobblerIssue
	status       string  // "done", "failed", "in-progress", "pending"
	costUSD      float64
	durationS    int
	numTurns     int
	locDeltaProd int
	locDeltaTest int
	numReqs      int // number of requirements in the task description
	promptBytes  int // prompt size in bytes from "Stitch started" comment
	prds         []string
	release      string // roadmap release version, e.g. "01.0"
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
	var totalTurns, totalLocProd, totalLocTest, totalReqs, totalPromptBytes int
	var nDone, nFailed, nInProgress, nPending int
	prdStatus := make(map[string]string) // prd name → highest-priority status
	prdReleaseMap := buildPRDReleaseMap()

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

		// Parse stitch progress comments for cost, duration, and turns.
		comments, _ := fetchIssueComments(repo, iss.Number)
		for _, c := range comments {
			p := parseStitchComment(c)
			if p.costUSD > 0 {
				s.costUSD += p.costUSD
			}
			if p.durationS > 0 {
				s.durationS = p.durationS
			}
			if p.numTurns > 0 {
				s.numTurns += p.numTurns
			}
			s.locDeltaProd += p.locDeltaProd
			s.locDeltaTest += p.locDeltaTest
			if p.promptBytes > 0 {
				s.promptBytes = p.promptBytes
			}
		}
		totalCost += s.costUSD
		totalTurns += s.numTurns
		totalLocProd += s.locDeltaProd
		totalLocTest += s.locDeltaTest
		totalPromptBytes += s.promptBytes

		s.numReqs = countDescriptionReqs(iss.Description)
		totalReqs += s.numReqs

		// Extract release directly from title; fall back to PRD mapping.
		s.release = extractRelease(iss.Title)
		s.prds = extractPRDRefs(iss.Title + " " + iss.Description)
		for _, prd := range s.prds {
			if s.release == "" {
				if rel, ok := prdReleaseMap[prd]; ok {
					s.release = rel
				}
			}
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

	sort.Slice(rows, func(i, j int) bool { return rows[i].Number < rows[j].Number })

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
	fmt.Printf("Total cost: $%.2f, %d turns\n", totalCost, totalTurns)
	fmt.Printf("LOC created: %+d prod, %+d test\n", totalLocProd, totalLocTest)
	fmt.Printf("Requirements: %d total in tasks\n", totalReqs)
	if totalPromptBytes > 0 {
		fmt.Printf("Prompt total: %s\n", formatBytes(totalPromptBytes))
	}

	// Per-release breakdown.
	type relCounts struct{ done, inProgress, pending, failed int }
	byRelease := make(map[string]*relCounts)
	for _, r := range rows {
		rel := r.release
		if rel == "" {
			rel = "-"
		}
		rc := byRelease[rel]
		if rc == nil {
			rc = &relCounts{}
			byRelease[rel] = rc
		}
		switch r.status {
		case "done":
			rc.done++
		case "in-progress":
			rc.inProgress++
		case "pending":
			rc.pending++
		case "failed":
			rc.failed++
		}
	}
	if len(byRelease) > 1 || (len(byRelease) == 1 && byRelease["-"] == nil) {
		rels := make([]string, 0, len(byRelease))
		for rel := range byRelease {
			rels = append(rels, rel)
		}
		sort.Strings(rels)
		for _, rel := range rels {
			rc := byRelease[rel]
			fmt.Printf("  rel %s: %d done, %d in-progress, %d pending", rel, rc.done, rc.inProgress, rc.pending)
			if rc.failed > 0 {
				fmt.Printf(", %d failed", rc.failed)
			}
			fmt.Println()
		}
	}
	fmt.Println()

	// Issue table.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tStatus\tRel\tReqs\tPrompt\tCost\tDuration\tTurns\tProd\tTest\tTitle")
	for _, r := range rows {
		prompt := "-"
		if r.promptBytes > 0 {
			prompt = formatBytes(r.promptBytes)
		}
		cost := "-"
		if r.costUSD > 0 {
			cost = fmt.Sprintf("$%.2f", r.costUSD)
		}
		dur := "-"
		if r.durationS > 0 {
			dur = formatDuration(r.durationS)
		}
		turns := "-"
		if r.numTurns > 0 {
			turns = strconv.Itoa(r.numTurns)
		}
		prod := "-"
		if r.locDeltaProd != 0 {
			prod = fmt.Sprintf("%+d", r.locDeltaProd)
		}
		test := "-"
		if r.locDeltaTest != 0 {
			test = fmt.Sprintf("%+d", r.locDeltaTest)
		}
		reqs := "-"
		if r.numReqs > 0 {
			reqs = strconv.Itoa(r.numReqs)
		}
		rel := r.release
		if rel == "" {
			rel = "-"
		}
		title := r.Title
		if len(title) > 48 {
			title = title[:45] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Number, r.status, rel, reqs, prompt, cost, dur, turns, prod, test, title)
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
	costUSD      float64
	durationS    int
	numTurns     int
	locDeltaProd int
	locDeltaTest int
	promptBytes  int
}

// parseStitchComment extracts cost and duration from a stitch progress comment
// produced by closeStitchTask or failTask (GH-567 format):
//
//	"Stitch completed in 5m 32s. LOC delta: +45 prod, +17 test. Cost: $0.42. Turns: 12."
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

	// Parse "LOC delta: +N prod, +N test"
	if i := strings.Index(body, "LOC delta: "); i >= 0 {
		rest := body[i+11:]
		var prod, test int
		if n, _ := fmt.Sscanf(rest, "%d prod, %d test", &prod, &test); n == 2 {
			d.locDeltaProd = prod
			d.locDeltaTest = test
		}
	}

	// Parse "Turns: N"
	if i := strings.Index(body, "Turns: "); i >= 0 {
		rest := body[i+7:]
		var turnsStr string
		fmt.Sscanf(rest, "%s", &turnsStr)
		turnsStr = strings.TrimRight(turnsStr, ".,;")
		if v, err := strconv.Atoi(turnsStr); err == nil {
			d.numTurns = v
		}
	}

	// Parse "prompt: N bytes" from Stitch started comment.
	if i := strings.Index(body, "prompt: "); i >= 0 {
		rest := body[i+8:]
		var bytesStr string
		fmt.Sscanf(rest, "%s", &bytesStr)
		bytesStr = strings.TrimRight(bytesStr, ".,;")
		if v, err := strconv.Atoi(bytesStr); err == nil {
			d.promptBytes = v
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

// buildPRDReleaseMap loads use case files and maps PRD short names (e.g.
// "prd-003") to their roadmap release version by parsing touchpoint references.
// Use case filenames encode the release: "rel01.0-uc003-measure-workflow.yaml".
func buildPRDReleaseMap() map[string]string {
	paths, _ := filepath.Glob("docs/specs/use-cases/rel*.yaml")
	prdRelease := make(map[string]string)
	for _, path := range paths {
		base := filepath.Base(path)
		// Extract release from filename: "rel01.0-uc003-..." → "01.0"
		rel := ""
		if strings.HasPrefix(base, "rel") {
			if dash := strings.Index(base, "-uc"); dash > 3 {
				rel = base[3:dash]
			}
		}
		if rel == "" {
			continue
		}

		uc := loadYAML[UseCaseDoc](path)
		if uc == nil {
			continue
		}
		// Touchpoints reference PRDs like "prd003-cobbler-workflows R1, R2".
		for _, tp := range uc.Touchpoints {
			for _, v := range tp {
				for _, word := range strings.Fields(v) {
					w := strings.ToLower(strings.Trim(word, ".,;:()[]`\"'"))
					if !strings.HasPrefix(w, "prd") || len(w) < 6 {
						continue
					}
					// Convert "prd003-cobbler-workflows" → "prd-003".
					if w[3] >= '0' && w[3] <= '9' {
						short := "prd-" + w[3:6]
						if _, exists := prdRelease[short]; !exists {
							prdRelease[short] = rel
						}
					}
				}
			}
		}
	}
	return prdRelease
}

// countDescriptionReqs counts the number of requirements in a task description
// by parsing the YAML requirements list. Each item with an "id" field (e.g.
// "R1", "R2") counts as one requirement.
func countDescriptionReqs(description string) int {
	var parsed struct {
		Requirements []struct {
			ID string `yaml:"id"`
		} `yaml:"requirements"`
	}
	if err := yaml.Unmarshal([]byte(description), &parsed); err != nil {
		return 0
	}
	return len(parsed.Requirements)
}

// reRelease matches release patterns like "rel01.0" or "rel02.1" in text.
var reRelease = regexp.MustCompile(`rel(\d{2}\.\d)`)

// extractRelease returns the first release version (e.g. "01.0") found in
// text by matching relNN.N patterns. Returns "" if no match.
func extractRelease(text string) string {
	m := reRelease.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// formatBytes returns a human-readable byte size, e.g. "125K" or "1.2M".
func formatBytes(b int) string {
	switch {
	case b >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(b)/1_000_000)
	case b >= 1_000:
		return fmt.Sprintf("%dK", b/1_000)
	default:
		return fmt.Sprintf("%dB", b)
	}
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
