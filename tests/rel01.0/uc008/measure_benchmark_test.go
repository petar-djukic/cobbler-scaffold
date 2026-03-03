//go:build benchmark

// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Performance benchmarks for the measure phase (prd003-cobbler-workflows R1, R2, R6).
// Runs the iterative single-turn measure (post GH-12) at limits 1-5 and collects
// structured timing, token, and cost data from history stats files.

package uc008_test

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator"
	"github.com/mesh-intelligence/cobbler-scaffold/tests/rel01.0/internal/testutil"
	"gopkg.in/yaml.v3"
)

var (
	orchRoot    string
	snapshotDir string
)

func TestMain(m *testing.M) {
	var err error
	orchRoot, err = testutil.FindOrchestratorRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "benchmark: resolving orchRoot: %v\n", err)
		os.Exit(1)
	}
	snapshot, cleanup, prepErr := testutil.PrepareSnapshot(orchRoot)
	if prepErr != nil {
		fmt.Fprintf(os.Stderr, "benchmark: preparing snapshot: %v\n", prepErr)
		os.Exit(1)
	}
	snapshotDir = snapshot
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

// measureIterationStats mirrors the HistoryStats YAML structure for
// fields we need from each measure stats file.
type measureIterationStats struct {
	Caller    string `yaml:"caller"`
	StartedAt string `yaml:"started_at"`
	Duration  string `yaml:"duration"`
	DurationS int    `yaml:"duration_s"`
	Tokens    struct {
		Input         int `yaml:"input"`
		Output        int `yaml:"output"`
		CacheCreation int `yaml:"cache_creation"`
		CacheRead     int `yaml:"cache_read"`
	} `yaml:"tokens"`
	CostUSD float64 `yaml:"cost_usd"`
}

// measureLimitResult summarizes a single limit run.
type measureLimitResult struct {
	Limit      int                     `yaml:"limit"`
	WallTime   string                  `yaml:"wall_time"`
	WallTimeS  int                     `yaml:"wall_time_s"`
	Issues     int                     `yaml:"issues"`
	TotalCost  float64                 `yaml:"total_cost_usd"`
	Iterations []measureIterationStats `yaml:"iterations"`
}

// measureBenchmarkSummary is the top-level YAML output.
type measureBenchmarkSummary struct {
	TestName  string               `yaml:"test_name"`
	Timestamp string               `yaml:"timestamp"`
	Model     string               `yaml:"model"`
	Strategy  string               `yaml:"strategy"`
	Results   []measureLimitResult `yaml:"results"`
}

// TestRel01_UC008_MeasureTiming runs the iterative single-turn measure at
// limits 1 through 5 and records wall-clock time, issue count, and per-iteration
// token breakdown. Each limit gets a fresh repo and GitHub Issues state.
func TestRel01_UC008_MeasureTiming(t *testing.T) {
	summary := measureBenchmarkSummary{
		TestName:  "TestRel01_UC008_MeasureTiming",
		Timestamp: time.Now().Format(time.RFC3339),
		Model:     "claude-opus-4-6",
		Strategy:  "iterative-single-turn (post GH-12, --max-turns 1)",
	}

	for limit := 1; limit <= 5; limit++ {
		limit := limit
		name := fmt.Sprintf("limit=%d", limit)
		t.Run(name, func(t *testing.T) {
			result := runMeasureAtLimit(t, limit)
			summary.Results = append(summary.Results, result)
		})
	}

	out, err := yaml.Marshal(&summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	t.Logf("\n--- Measure Benchmark Summary ---\n%s", string(out))
}

// runMeasureAtLimit sets up a fresh repo, runs measure with the given limit,
// and collects results from history stats files.
func runMeasureAtLimit(t *testing.T, limit int) measureLimitResult {
	t.Helper()
	dir := testutil.SetupRepo(t, snapshotDir)
	testutil.SetupClaude(t, dir)

	testutil.WriteConfigOverride(t, dir, func(cfg *orchestrator.Config) {
		cfg.Cobbler.MaxMeasureIssues = limit
	})

	if err := testutil.RunMage(t, dir, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := testutil.RunMage(t, dir, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	start := time.Now()
	if err := testutil.RunMage(t, dir, "cobbler:measure"); err != nil {
		t.Fatalf("cobbler:measure (limit=%d): %v", limit, err)
	}
	wallTime := time.Since(start)

	issues := testutil.CountReadyIssues(t, dir)
	iterations := collectMeasureStats(t, dir)

	var totalCost float64
	for _, iter := range iterations {
		totalCost += iter.CostUSD
	}

	result := measureLimitResult{
		Limit:      limit,
		WallTime:   wallTime.Truncate(time.Second).String(),
		WallTimeS:  int(wallTime.Seconds()),
		Issues:     issues,
		TotalCost:  totalCost,
		Iterations: iterations,
	}

	t.Logf("limit=%d: wall=%s issues=%d cost=$%.2f iterations=%d",
		limit, result.WallTime, issues, totalCost, len(iterations))

	return result
}

// collectMeasureStats reads all measure stats files from the history
// directory and returns them sorted by started_at.
func collectMeasureStats(t *testing.T, dir string) []measureIterationStats {
	t.Helper()
	files := testutil.HistoryStatsFiles(t, dir, "measure")
	if len(files) == 0 {
		t.Log("no measure stats files found in history directory")
		return nil
	}

	var stats []measureIterationStats
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Logf("read stats file %s: %v", f, err)
			continue
		}
		var s measureIterationStats
		if err := yaml.Unmarshal(data, &s); err != nil {
			t.Logf("unmarshal stats file %s: %v", f, err)
			continue
		}
		stats = append(stats, s)
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].StartedAt < stats[j].StartedAt
	})

	return stats
}
