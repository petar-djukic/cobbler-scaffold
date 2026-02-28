// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
)

// outcomeSep delimits commit blocks in the git log output used by Outcomes.
const outcomeSep = "==OUTCOME=="

// OutcomeRecord holds parsed outcome trailer data from a single task commit.
type OutcomeRecord struct {
	TaskBranch          string
	TokensInput         int
	TokensOutput        int
	TokensCacheCreation int
	TokensCacheRead     int
	TokensCostUSD       float64
	LocProdBefore       int
	LocProdAfter        int
	LocTestBefore       int
	LocTestAfter        int
	DurationSeconds     int
}

// Outcomes scans all git branches for commits that carry outcome trailers
// written by appendOutcomeTrailers and prints a summary table to stdout.
// Returns nil (with a message) if no trailers are found.
func (o *Orchestrator) Outcomes() error {
	format := outcomeSep + "%n%D%n%(trailers:only)"
	out, err := exec.Command(binGit, "log", "--all", "--format="+format).Output()
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}

	records := parseOutcomeRecords(string(out))
	if len(records) == 0 {
		fmt.Println("no outcome records found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Branch\tTokens-In\tTokens-Out\tCost-USD\tLOC-Prod-Δ\tLOC-Test-Δ\tDuration")
	for _, r := range records {
		prodDelta := r.LocProdAfter - r.LocProdBefore
		testDelta := r.LocTestAfter - r.LocTestBefore
		dur := formatDuration(r.DurationSeconds)
		fmt.Fprintf(w, "%s\t%d\t%d\t$%.4f\t%+d\t%+d\t%s\n",
			r.TaskBranch, r.TokensInput, r.TokensOutput, r.TokensCostUSD,
			prodDelta, testDelta, dur)
	}
	return w.Flush()
}

// parseOutcomeRecords splits a git log output (using outcomeSep as block
// delimiter) into individual commit blocks and returns all that contain
// at least one recognized outcome trailer key.
func parseOutcomeRecords(logOutput string) []OutcomeRecord {
	var records []OutcomeRecord
	for _, block := range strings.Split(logOutput, outcomeSep+"\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		if rec := parseOneOutcomeBlock(block); rec != nil {
			records = append(records, *rec)
		}
	}
	return records
}

// parseOneOutcomeBlock parses a single commit block (refs line followed by
// trailer lines) into an OutcomeRecord. Returns nil if no trailer keys are
// recognized.
func parseOneOutcomeBlock(block string) *OutcomeRecord {
	parts := strings.SplitN(block, "\n", 2)
	if len(parts) < 2 {
		return nil
	}
	refsLine := strings.TrimSpace(parts[0])
	trailerBlock := parts[1]

	if !strings.Contains(trailerBlock, "Tokens-Input:") {
		return nil
	}

	rec := &OutcomeRecord{
		TaskBranch: extractBranchFromRefs(refsLine),
	}
	for _, line := range strings.Split(trailerBlock, "\n") {
		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "Tokens-Input":
			rec.TokensInput, _ = strconv.Atoi(val)
		case "Tokens-Output":
			rec.TokensOutput, _ = strconv.Atoi(val)
		case "Tokens-Cache-Creation":
			rec.TokensCacheCreation, _ = strconv.Atoi(val)
		case "Tokens-Cache-Read":
			rec.TokensCacheRead, _ = strconv.Atoi(val)
		case "Tokens-Cost-USD":
			rec.TokensCostUSD, _ = strconv.ParseFloat(val, 64)
		case "Loc-Prod-Before":
			rec.LocProdBefore, _ = strconv.Atoi(val)
		case "Loc-Prod-After":
			rec.LocProdAfter, _ = strconv.Atoi(val)
		case "Loc-Test-Before":
			rec.LocTestBefore, _ = strconv.Atoi(val)
		case "Loc-Test-After":
			rec.LocTestAfter, _ = strconv.Atoi(val)
		case "Duration-Seconds":
			rec.DurationSeconds, _ = strconv.Atoi(val)
		}
	}
	return rec
}

// extractBranchFromRefs returns the first local branch name from a %D
// refs string (e.g. "HEAD -> task/main-abc, origin/task/main-abc").
func extractBranchFromRefs(refs string) string {
	if idx := strings.Index(refs, " -> "); idx >= 0 {
		rest := refs[idx+4:]
		if i := strings.Index(rest, ","); i >= 0 {
			return strings.TrimSpace(rest[:i])
		}
		return strings.TrimSpace(rest)
	}
	parts := strings.SplitN(refs, ",", 2)
	return strings.TrimSpace(parts[0])
}

// formatDuration converts seconds to a human-readable "Xm Ys" string.
func formatDuration(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
}
