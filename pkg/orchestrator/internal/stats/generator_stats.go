// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	cl "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"gopkg.in/yaml.v3"
)

// GeneratorIssueStats holds per-issue stats derived from labels and comments.
type GeneratorIssueStats struct {
	gh.CobblerIssue
	Status       string  // "done", "failed", "in-progress", "pending"
	CostUSD      float64
	DurationS    int
	NumTurns     int
	LocDeltaProd int
	LocDeltaTest int
	NumReqs      int // number of requirements in the task description
	PromptBytes  int // prompt size in bytes from "Stitch started" comment
	InputTokens  int // total input tokens from stitch completion comments
	OutputTokens int // total output tokens from stitch completion comments
	PRDs         []string
	Release      string // roadmap release version, e.g. "01.0"
}

// GeneratorStatsDeps holds dependencies for generator stats collection.
type GeneratorStatsDeps struct {
	Log                    Logger
	ListGenerationBranches func() []string
	GenerationBranch       string // from config, "" means auto-detect
	DetectGitHubRepo       func() (string, error)
	ListAllIssues          func(repo, generation string) ([]gh.CobblerIssue, error)
	FetchIssueComments     func(repo string, number int) ([]string, error)
	HistoryDir             string // path to .cobbler/history for local stats files
}

// LoadHistoryStats reads all *-stats.yaml files from dir and returns the
// parsed entries. Returns nil, nil when dir is empty or does not exist.
func LoadHistoryStats(dir string) ([]cl.HistoryStats, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading history dir %s: %w", dir, err)
	}
	var result []cl.HistoryStats
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "-stats.yaml") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue
		}
		var hs cl.HistoryStats
		if parseErr := yaml.Unmarshal(data, &hs); parseErr != nil {
			continue
		}
		result = append(result, hs)
	}
	return result, nil
}

// PrintGeneratorStats prints a status report for the current generation run.
func PrintGeneratorStats(deps GeneratorStatsDeps) error {
	branches := deps.ListGenerationBranches()
	if len(branches) == 0 {
		fmt.Println("no active generation branches found")
		return nil
	}

	// Prefer the configured branch; fall back to the first detected branch.
	genBranch := deps.GenerationBranch
	if genBranch == "" {
		genBranch = branches[0]
	}

	repo, err := deps.DetectGitHubRepo()
	if err != nil || repo == "" {
		return fmt.Errorf("detecting GitHub repo: %w", err)
	}

	issues, err := deps.ListAllIssues(repo, genBranch)
	if err != nil {
		return fmt.Errorf("listing cobbler issues for %s: %w", genBranch, err)
	}
	if len(issues) == 0 {
		fmt.Printf("generation %s: no task issues found\n", genBranch)
		return nil
	}

	// Load local history stats if available.
	historyStats, _ := LoadHistoryStats(deps.HistoryDir)

	// Build lookup maps from history data: task_id → aggregated stitch stats.
	type stitchAgg struct {
		CostUSD      float64
		DurationS    int
		NumTurns     int
		InputTokens  int
		OutputTokens int
	}
	stitchByTask := make(map[string]*stitchAgg)
	var measureEntries []cl.HistoryStats
	for _, hs := range historyStats {
		switch hs.Caller {
		case "stitch":
			tid := hs.TaskID
			if tid == "" {
				continue
			}
			agg := stitchByTask[tid]
			if agg == nil {
				agg = &stitchAgg{}
				stitchByTask[tid] = agg
			}
			agg.CostUSD += hs.CostUSD
			if hs.DurationS > agg.DurationS {
				agg.DurationS = hs.DurationS
			}
			agg.NumTurns += hs.NumTurns
			agg.InputTokens += hs.Tokens.Input
			agg.OutputTokens += hs.Tokens.Output
		case "measure":
			measureEntries = append(measureEntries, hs)
		}
	}

	// Collect per-issue stats.
	rows := make([]GeneratorIssueStats, 0, len(issues))
	var totalStitchCost float64
	var totalTurns, totalLocProd, totalLocTest, totalReqs, totalPromptBytes int
	var totalInputTokens, totalOutputTokens int
	var nDone, nFailed, nInProgress, nPending int
	prdStatus := make(map[string]string) // prd name → highest-priority status
	prdReleaseMap := BuildPRDReleaseMap()

	for _, iss := range issues {
		s := GeneratorIssueStats{CobblerIssue: iss}

		switch {
		case iss.State == "closed" && !gh.HasLabel(iss, "failed"):
			s.Status = "done"
			nDone++
		case iss.State == "closed":
			s.Status = "failed"
			nFailed++
		case gh.HasLabel(iss, gh.LabelInProgress):
			s.Status = "in-progress"
			nInProgress++
		default:
			s.Status = "pending"
			nPending++
		}

		// Prefer local history data over comment parsing.
		taskID := fmt.Sprintf("%d", iss.Number)
		if agg, ok := stitchByTask[taskID]; ok {
			s.CostUSD = agg.CostUSD
			s.DurationS = agg.DurationS
			s.NumTurns = agg.NumTurns
			s.InputTokens = agg.InputTokens
			s.OutputTokens = agg.OutputTokens
		} else {
			// Fallback: parse stitch progress comments.
			comments, _ := deps.FetchIssueComments(repo, iss.Number)
			for _, c := range comments {
				p := ParseStitchComment(c)
				if p.CostUSD > 0 {
					s.CostUSD += p.CostUSD
				}
				if p.DurationS > 0 {
					s.DurationS = p.DurationS
				}
				if p.NumTurns > 0 {
					s.NumTurns += p.NumTurns
				}
				s.LocDeltaProd += p.LocDeltaProd
				s.LocDeltaTest += p.LocDeltaTest
				if p.PromptBytes > 0 {
					s.PromptBytes = p.PromptBytes
				}
				s.InputTokens += p.InputTokens
				s.OutputTokens += p.OutputTokens
			}
		}
		totalStitchCost += s.CostUSD
		totalTurns += s.NumTurns
		totalLocProd += s.LocDeltaProd
		totalLocTest += s.LocDeltaTest
		totalPromptBytes += s.PromptBytes
		totalInputTokens += s.InputTokens
		totalOutputTokens += s.OutputTokens

		s.NumReqs = CountDescriptionReqs(iss.Description)
		totalReqs += s.NumReqs

		// Extract release directly from title; fall back to PRD mapping.
		s.Release = ExtractRelease(iss.Title)
		s.PRDs = ExtractPRDRefs(iss.Title + " " + iss.Description)
		for _, prd := range s.PRDs {
			if s.Release == "" {
				if rel, ok := prdReleaseMap[prd]; ok {
					s.Release = rel
				}
			}
			existing := prdStatus[prd]
			switch s.Status {
			case "in-progress":
				prdStatus[prd] = "in-progress"
			case "pending":
				if existing == "" {
					prdStatus[prd] = "pending"
				}
			case "done", "failed":
				if existing == "" {
					prdStatus[prd] = s.Status
				}
			}
		}

		rows = append(rows, s)
	}

	// Aggregate measure costs.
	var totalMeasureCost float64
	var totalMeasureTurns, totalMeasureIn, totalMeasureOut, totalMeasureDurS int
	for _, m := range measureEntries {
		totalMeasureCost += m.CostUSD
		totalMeasureTurns += m.NumTurns
		totalMeasureIn += m.Tokens.Input
		totalMeasureOut += m.Tokens.Output
		totalMeasureDurS += m.DurationS
	}
	totalCost := totalStitchCost + totalMeasureCost

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
	fmt.Printf("Total cost: $%.2f", totalCost)
	if totalMeasureCost > 0 {
		fmt.Printf(" (stitch $%.2f + measure $%.2f)", totalStitchCost, totalMeasureCost)
	}
	fmt.Printf(", %d turns\n", totalTurns)
	fmt.Printf("LOC created: %+d prod, %+d test\n", totalLocProd, totalLocTest)
	fmt.Printf("Requirements: %d total in tasks\n", totalReqs)
	if totalPromptBytes > 0 {
		fmt.Printf("Prompt total: %s\n", FormatBytes(totalPromptBytes))
	}
	if totalInputTokens > 0 || totalOutputTokens > 0 {
		fmt.Printf("Tokens: %s in, %s out\n", FormatTokens(totalInputTokens), FormatTokens(totalOutputTokens))
	}

	// Per-release breakdown.
	type relCounts struct{ done, inProgress, pending, failed int }
	byRelease := make(map[string]*relCounts)
	for _, r := range rows {
		rel := r.Release
		if rel == "" {
			rel = "-"
		}
		rc := byRelease[rel]
		if rc == nil {
			rc = &relCounts{}
			byRelease[rel] = rc
		}
		switch r.Status {
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
	fmt.Fprintln(w, "#\tStatus\tRel\tReqs\tPrompt\tCost\tDuration\tTurns\tTokIn\tTokOut\tProd\tTest\tTitle")
	for _, r := range rows {
		prompt := "-"
		if r.PromptBytes > 0 {
			prompt = FormatBytes(r.PromptBytes)
		}
		cost := "-"
		if r.CostUSD > 0 {
			cost = fmt.Sprintf("$%.2f", r.CostUSD)
		}
		dur := "-"
		if r.DurationS > 0 {
			dur = FormatDuration(r.DurationS)
		}
		turns := "-"
		if r.NumTurns > 0 {
			turns = strconv.Itoa(r.NumTurns)
		}
		tokIn := "-"
		if r.InputTokens > 0 {
			tokIn = FormatTokens(r.InputTokens)
		}
		tokOut := "-"
		if r.OutputTokens > 0 {
			tokOut = FormatTokens(r.OutputTokens)
		}
		prod := "-"
		if r.LocDeltaProd != 0 {
			prod = fmt.Sprintf("%+d", r.LocDeltaProd)
		}
		test := "-"
		if r.LocDeltaTest != 0 {
			test = fmt.Sprintf("%+d", r.LocDeltaTest)
		}
		reqs := "-"
		if r.NumReqs > 0 {
			reqs = strconv.Itoa(r.NumReqs)
		}
		rel := r.Release
		if rel == "" {
			rel = "-"
		}
		title := r.Title
		if len(title) > 48 {
			title = title[:45] + "..."
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Number, r.Status, rel, reqs, prompt, cost, dur, turns, tokIn, tokOut, prod, test, title)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// Measure invocations summary.
	if len(measureEntries) > 0 {
		fmt.Printf("\nMeasure invocations: %d, cost $%.2f, %d turns, %s duration\n",
			len(measureEntries), totalMeasureCost, totalMeasureTurns,
			FormatDuration(totalMeasureDurS))
		if totalMeasureIn > 0 || totalMeasureOut > 0 {
			fmt.Printf("Measure tokens: %s in, %s out\n",
				FormatTokens(totalMeasureIn), FormatTokens(totalMeasureOut))
		}
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
	total, byPRD := CountTotalPRDRequirements()
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

// StitchCommentData holds metrics extracted from a stitch progress comment.
type StitchCommentData struct {
	CostUSD      float64
	DurationS    int
	NumTurns     int
	LocDeltaProd int
	LocDeltaTest int
	PromptBytes  int
	InputTokens  int
	OutputTokens int
}

// ParseStitchComment extracts cost and duration from a stitch progress comment.
func ParseStitchComment(body string) StitchCommentData {
	var d StitchCommentData

	// Parse "Cost: $X.XX"
	if i := strings.Index(body, "Cost: $"); i >= 0 {
		rest := body[i+7:]
		var costStr string
		fmt.Sscanf(rest, "%s", &costStr)
		costStr = strings.TrimRight(costStr, ".,;")
		if v, err := strconv.ParseFloat(costStr, 64); err == nil {
			d.CostUSD = v
		}
	}

	// Parse "LOC delta: +N prod, +N test"
	if i := strings.Index(body, "LOC delta: "); i >= 0 {
		rest := body[i+11:]
		var prod, test int
		if n, _ := fmt.Sscanf(rest, "%d prod, %d test", &prod, &test); n == 2 {
			d.LocDeltaProd = prod
			d.LocDeltaTest = test
		}
	}

	// Parse "Turns: N"
	if i := strings.Index(body, "Turns: "); i >= 0 {
		rest := body[i+7:]
		var turnsStr string
		fmt.Sscanf(rest, "%s", &turnsStr)
		turnsStr = strings.TrimRight(turnsStr, ".,;")
		if v, err := strconv.Atoi(turnsStr); err == nil {
			d.NumTurns = v
		}
	}

	// Parse "prompt: N bytes" from Stitch started comment.
	if i := strings.Index(body, "prompt: "); i >= 0 {
		rest := body[i+8:]
		var bytesStr string
		fmt.Sscanf(rest, "%s", &bytesStr)
		bytesStr = strings.TrimRight(bytesStr, ".,;")
		if v, err := strconv.Atoi(bytesStr); err == nil {
			d.PromptBytes = v
		}
	}

	// Parse "Tokens: Nin Nout" from stitch completion comment.
	if i := strings.Index(body, "Tokens: "); i >= 0 {
		rest := body[i+8:]
		var in, out int
		if n, _ := fmt.Sscanf(rest, "%din %dout", &in, &out); n == 2 {
			d.InputTokens = in
			d.OutputTokens = out
		}
	}

	// Parse "in Xm Ys" or "after Xm Ys" for duration.
	for _, marker := range []string{"in ", "after "} {
		if i := strings.Index(body, marker); i >= 0 {
			rest := body[i+len(marker):]
			var mins, secs int
			if n, _ := fmt.Sscanf(rest, "%dm %ds", &mins, &secs); n == 2 {
				d.DurationS = mins*60 + secs
				break
			}
			if n, _ := fmt.Sscanf(rest, "%ds", &secs); n == 1 {
				d.DurationS = secs
				break
			}
		}
	}

	return d
}

// CountDescriptionReqs counts the number of requirements in a task description
// by parsing the YAML requirements list.
func CountDescriptionReqs(description string) int {
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

// ExtractRelease returns the first release version (e.g. "01.0") found in
// text by matching relNN.N patterns.
func ExtractRelease(text string) string {
	m := reRelease.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// FormatTokens returns a human-readable token count, e.g. "125K" or "1.2M".
func FormatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dK", n/1_000)
	default:
		return strconv.Itoa(n)
	}
}

// FormatBytes returns a human-readable byte size, e.g. "125K" or "1.2M".
func FormatBytes(b int) string {
	switch {
	case b >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(b)/1_000_000)
	case b >= 1_000:
		return fmt.Sprintf("%dK", b/1_000)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// ExtractPRDRefs returns deduplicated prd-* tokens found in text.
func ExtractPRDRefs(text string) []string {
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
