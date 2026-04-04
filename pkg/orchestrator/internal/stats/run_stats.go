// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package stats

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	cl "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
	"gopkg.in/yaml.v3"
)

// RunSummary holds aggregate statistics for a completed generation run.
type RunSummary struct {
	Name      string
	StartedAt time.Time
	FinishedAt time.Time

	// Requirements
	TotalReqs    int
	Complete     int
	CompleteWithFailures int
	Ready        int
	Skipped      int

	// Tasks
	StitchTasks  int
	MeasureTasks int
	TotalCostUSD float64
	StitchCostUSD float64
	MeasureCostUSD float64

	// Per-task averages (stitch only)
	AvgCostPerTask  float64
	AvgCostPerReq   float64
	AvgTurnsPerTask float64
	AvgTimePerTaskS int
	AvgTokensIn     int
	AvgTokensOut    int

	// Top expensive tasks
	TopTasks []RunTaskSummary
}

// RunTaskSummary holds per-task info for the "most expensive" list.
type RunTaskSummary struct {
	TaskID  string
	CostUSD float64
	Title   string
}

// RunStatsDeps holds dependencies for run stats collection.
type RunStatsDeps struct {
	Log            Logger
	ListTags       func(pattern string) []string
	ShowFile       func(ref, path string) ([]byte, error)
	GenerationPrefix string // e.g. "generation-"
	CobblerDir       string // e.g. ".cobbler"
	HistorySubdir    string // e.g. "history"
}

// ListGenerations returns the names of all known generations discovered
// from git tags with a -start suffix.
func ListGenerations(deps RunStatsDeps) []string {
	tags := deps.ListTags("*-start")
	var names []string
	for _, tag := range tags {
		name := generate.GenerationName(tag)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// CollectRunSummary gathers aggregate statistics for a named generation
// by reading history stats and requirements from git tags/refs.
func CollectRunSummary(name string, deps RunStatsDeps) (*RunSummary, error) {
	summary := &RunSummary{Name: name}

	// 1. Resolve generation timing from tag timestamps.
	startTag := name + "-start"
	finishedTag := name + "-finished"
	mergedTag := name + "-merged"

	summary.StartedAt = tagTimestamp(startTag)
	summary.FinishedAt = tagTimestamp(finishedTag)
	if summary.FinishedAt.IsZero() {
		summary.FinishedAt = tagTimestamp(mergedTag)
	}

	// 2. Find the best ref to read history from.
	// Prefer -merged > -finished > branch name.
	histRef := ""
	for _, ref := range []string{mergedTag, finishedTag, name} {
		if deps.ShowFile != nil {
			if _, err := deps.ShowFile(ref, ".cobbler"); err == nil {
				histRef = ref
				break
			}
		}
	}
	if histRef == "" {
		histRef = name // fallback to branch/tag name
	}

	// 3. Load history stats from the ref.
	historyEntries := loadHistoryFromRef(deps, histRef)

	// 4. Aggregate stitch and measure stats.
	type taskAgg struct {
		ID       string
		Title    string
		CostUSD  float64
		Turns    int
		DurationS int
		TokensIn int
		TokensOut int
	}
	tasksByID := make(map[string]*taskAgg)
	var taskOrder []string

	for _, hs := range historyEntries {
		switch hs.Caller {
		case "stitch":
			tid := hs.TaskID
			if tid == "" {
				continue
			}
			agg := tasksByID[tid]
			if agg == nil {
				agg = &taskAgg{ID: tid}
				tasksByID[tid] = agg
				taskOrder = append(taskOrder, tid)
			}
			if hs.TaskTitle != "" {
				agg.Title = hs.TaskTitle
			}
			agg.CostUSD += hs.CostUSD
			agg.Turns += hs.NumTurns
			if hs.DurationS > agg.DurationS {
				agg.DurationS = hs.DurationS
			}
			agg.TokensIn += hs.Tokens.Input
			agg.TokensOut += hs.Tokens.Output
		case "measure":
			summary.MeasureTasks++
			summary.MeasureCostUSD += hs.CostUSD
		}
	}

	summary.StitchTasks = len(tasksByID)
	var totalTurns, totalDurS, totalIn, totalOut int
	for _, tid := range taskOrder {
		agg := tasksByID[tid]
		summary.StitchCostUSD += agg.CostUSD
		totalTurns += agg.Turns
		totalDurS += agg.DurationS
		totalIn += agg.TokensIn
		totalOut += agg.TokensOut
	}
	summary.TotalCostUSD = summary.StitchCostUSD + summary.MeasureCostUSD

	if summary.StitchTasks > 0 {
		summary.AvgCostPerTask = summary.StitchCostUSD / float64(summary.StitchTasks)
		summary.AvgTurnsPerTask = float64(totalTurns) / float64(summary.StitchTasks)
		summary.AvgTimePerTaskS = totalDurS / summary.StitchTasks
		summary.AvgTokensIn = totalIn / summary.StitchTasks
		summary.AvgTokensOut = totalOut / summary.StitchTasks
	}

	// 5. Build top-5 most expensive tasks.
	type costEntry struct {
		id    string
		cost  float64
		title string
	}
	var costs []costEntry
	for _, tid := range taskOrder {
		agg := tasksByID[tid]
		costs = append(costs, costEntry{tid, agg.CostUSD, agg.Title})
	}
	sort.Slice(costs, func(i, j int) bool { return costs[i].cost > costs[j].cost })
	topN := 5
	if len(costs) < topN {
		topN = len(costs)
	}
	for i := 0; i < topN; i++ {
		summary.TopTasks = append(summary.TopTasks, RunTaskSummary{
			TaskID:  costs[i].id,
			CostUSD: costs[i].cost,
			Title:   costs[i].title,
		})
	}

	// 6. Load requirements from the ref.
	reqPath := deps.CobblerDir + "/" + generate.RequirementsFileName
	if deps.ShowFile != nil {
		if data, err := deps.ShowFile(histRef, reqPath); err == nil {
			reqStates := generate.ParseRequirementStates(data)
			for _, srdReqs := range reqStates {
				for _, st := range srdReqs {
					summary.TotalReqs++
					switch st.Status {
					case "complete":
						summary.Complete++
					case "complete_with_failures":
						summary.CompleteWithFailures++
					case "ready":
						summary.Ready++
					case "skipped":
						summary.Skipped++
					default:
						// Other statuses (e.g., "in_progress") count toward total
						// but not any specific bucket.
						summary.Complete++ // assume addressed
					}
				}
			}
		}
	}

	addressed := summary.Complete + summary.CompleteWithFailures
	if addressed > 0 {
		summary.AvgCostPerReq = summary.TotalCostUSD / float64(addressed)
	}

	return summary, nil
}

// PrintRunStats prints a summary report for a completed generation run.
func PrintRunStats(name string, deps RunStatsDeps) error {
	if name == "" {
		// List available generations.
		gens := ListGenerations(deps)
		if len(gens) == 0 {
			fmt.Println("No generations found (no *-start tags)")
			return nil
		}
		fmt.Println("Available generations:")
		for _, g := range gens {
			fmt.Printf("  %s\n", g)
		}
		return nil
	}

	summary, err := CollectRunSummary(name, deps)
	if err != nil {
		return err
	}

	// Header
	fmt.Printf("Generation: %s\n", summary.Name)
	if !summary.StartedAt.IsZero() {
		fmt.Printf("Started:    %s\n", summary.StartedAt.Local().Format("2006-01-02 15:04"))
	}
	if !summary.FinishedAt.IsZero() {
		fmt.Printf("Finished:   %s\n", summary.FinishedAt.Local().Format("2006-01-02 15:04"))
	}
	if !summary.StartedAt.IsZero() && !summary.FinishedAt.IsZero() {
		wallTime := summary.FinishedAt.Sub(summary.StartedAt)
		fmt.Printf("Wall time:  %s\n", FormatDurationLong(int(wallTime.Seconds())))
	}
	fmt.Println()

	// Requirements
	if summary.TotalReqs > 0 {
		addressed := summary.Complete + summary.CompleteWithFailures
		pct := float64(addressed) * 100 / float64(summary.TotalReqs)
		fmt.Printf("Requirements: %d/%d (%.1f%%)\n", addressed, summary.TotalReqs, pct)
		fmt.Printf("  Complete:            %d\n", summary.Complete)
		if summary.CompleteWithFailures > 0 {
			fmt.Printf("  Complete w/failures: %d\n", summary.CompleteWithFailures)
		}
		fmt.Printf("  Ready (remaining):   %d\n", summary.Ready)
		if summary.Skipped > 0 {
			fmt.Printf("  Skipped:             %d\n", summary.Skipped)
		}
		fmt.Println()
	}

	// Tasks
	fmt.Printf("Tasks:     %d stitch, %d measure\n", summary.StitchTasks, summary.MeasureTasks)
	fmt.Printf("Total cost: $%.0f\n", summary.TotalCostUSD)
	if summary.StitchCostUSD > 0 && summary.MeasureCostUSD > 0 {
		fmt.Printf("  Stitch:  $%.0f\n", summary.StitchCostUSD)
		fmt.Printf("  Measure: $%.0f\n", summary.MeasureCostUSD)
	}
	if summary.AvgCostPerTask > 0 {
		fmt.Printf("  Avg cost/task:  $%.2f\n", summary.AvgCostPerTask)
	}
	if summary.AvgCostPerReq > 0 {
		fmt.Printf("  Avg cost/req:   $%.2f\n", summary.AvgCostPerReq)
	}
	fmt.Println()

	// Averages
	if summary.StitchTasks > 0 {
		fmt.Printf("Avg turns/task:   %.1f\n", summary.AvgTurnsPerTask)
		fmt.Printf("Avg time/task:    %s\n", FormatDuration(summary.AvgTimePerTaskS))
		fmt.Printf("Avg tokens in:    %s\n", FormatTokens(summary.AvgTokensIn))
		fmt.Printf("Avg tokens out:   %s\n", FormatTokens(summary.AvgTokensOut))
		fmt.Println()
	}

	// Top 5 most expensive tasks
	if len(summary.TopTasks) > 0 {
		fmt.Println("Top 5 most expensive tasks:")
		for _, t := range summary.TopTasks {
			title := t.Title
			if len(title) > 60 {
				title = title[:57] + "..."
			}
			fmt.Printf("  #%-6s $%.2f  %s\n", t.TaskID, t.CostUSD, title)
		}
	}

	return nil
}

// PrintCompareStats prints a side-by-side comparison of two generation runs.
func PrintCompareStats(name1, name2 string, deps RunStatsDeps) error {
	s1, err := CollectRunSummary(name1, deps)
	if err != nil {
		return fmt.Errorf("collecting %s: %w", name1, err)
	}
	s2, err := CollectRunSummary(name2, deps)
	if err != nil {
		return fmt.Errorf("collecting %s: %w", name2, err)
	}

	// Compute short labels from generation names.
	label1 := shortLabel(name1)
	label2 := shortLabel(name2)

	// Column widths: metric label (24), col1 (20), col2 (20)
	hdr := fmt.Sprintf("%-24s %-20s %-20s", "", label1, label2)
	fmt.Println(hdr)
	fmt.Println(strings.Repeat("-", len(hdr)))

	addr1 := s1.Complete + s1.CompleteWithFailures
	addr2 := s2.Complete + s2.CompleteWithFailures
	printCompareRow("Requirements", fmtReqPct(addr1, s1.TotalReqs), fmtReqPct(addr2, s2.TotalReqs))
	printCompareRow("Tasks", fmt.Sprintf("%d", s1.StitchTasks), fmt.Sprintf("%d", s2.StitchTasks))
	printCompareRow("Total cost", fmt.Sprintf("$%.0f", s1.TotalCostUSD), fmt.Sprintf("$%.0f", s2.TotalCostUSD))
	printCompareRow("Cost/req", fmtCostPerReq(s1.AvgCostPerReq), fmtCostPerReq(s2.AvgCostPerReq))
	printCompareRow("Avg turns/task", fmtFloat1(s1.AvgTurnsPerTask), fmtFloat1(s2.AvgTurnsPerTask))
	printCompareRow("Avg time/task", fmtDurOrDash(s1.AvgTimePerTaskS), fmtDurOrDash(s2.AvgTimePerTaskS))
	printCompareRow("Avg tokens in", fmtTokOrDash(s1.AvgTokensIn), fmtTokOrDash(s2.AvgTokensIn))
	printCompareRow("Avg tokens out", fmtTokOrDash(s1.AvgTokensOut), fmtTokOrDash(s2.AvgTokensOut))

	return nil
}

func printCompareRow(label, val1, val2 string) {
	fmt.Printf("%-24s %-20s %-20s\n", label, val1, val2)
}

func shortLabel(name string) string {
	name = strings.TrimPrefix(name, "generation-")
	if len(name) > 18 {
		name = name[:15] + "..."
	}
	return name
}

func fmtReqPct(addressed, total int) string {
	if total == 0 {
		return "—"
	}
	pct := float64(addressed) * 100 / float64(total)
	return fmt.Sprintf("%d/%d (%.0f%%)", addressed, total, pct)
}

func fmtCostPerReq(v float64) string {
	if v == 0 {
		return "—"
	}
	return fmt.Sprintf("$%.2f", v)
}

func fmtFloat1(v float64) string {
	if v == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f", v)
}

func fmtDurOrDash(s int) string {
	if s == 0 {
		return "—"
	}
	return FormatDuration(s)
}

func fmtTokOrDash(n int) string {
	if n == 0 {
		return "—"
	}
	return FormatTokens(n)
}

// loadHistoryFromRef reads all *-stats.yaml files from .cobbler/history/
// on the given git ref by listing the tree and reading each file.
func loadHistoryFromRef(deps RunStatsDeps, ref string) []cl.HistoryStats {
	if deps.ShowFile == nil {
		return nil
	}
	histPath := deps.CobblerDir + "/" + deps.HistorySubdir
	// List files in the history directory via git ls-tree.
	files := listTreeFiles(ref, histPath)
	var entries []cl.HistoryStats
	for _, f := range files {
		if !strings.HasSuffix(f, "-stats.yaml") {
			continue
		}
		data, err := deps.ShowFile(ref, histPath+"/"+f)
		if err != nil {
			continue
		}
		var hs cl.HistoryStats
		if err := yaml.Unmarshal(data, &hs); err != nil {
			continue
		}
		entries = append(entries, hs)
	}
	return entries
}

// listTreeFiles runs git ls-tree to list files at a path under a ref.
func listTreeFiles(ref, path string) []string {
	out, err := exec.Command("git", "ls-tree", "--name-only", ref+":"+path).Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// tagTimestamp returns the commit timestamp of a git tag.
func tagTimestamp(tag string) time.Time {
	out, err := exec.Command("git", "log", "-1", "--format=%aI", tag).Output()
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
	if err != nil {
		return time.Time{}
	}
	return t
}
