// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// --- StitchReport YAML serialization ---

func TestStitchReport_SerializesAllFields(t *testing.T) {
	report := StitchReport{
		TaskID:    "mage-abc.1",
		TaskTitle: "Add widget feature",
		Status:    "success",
		Branch:    "task/main-mage-abc.1",
		Diff:      historyDiff{Files: 3, Insertions: 120, Deletions: 15},
		Files: []FileChange{
			{Path: "pkg/widget/widget.go", Status: "A", Insertions: 80, Deletions: 0},
			{Path: "pkg/widget/widget_test.go", Status: "A", Insertions: 40, Deletions: 0},
			{Path: "go.mod", Status: "M", Insertions: 0, Deletions: 15},
		},
		LOCBefore: LocSnapshot{Production: 500, Test: 200},
		LOCAfter:  LocSnapshot{Production: 580, Test: 240},
	}

	data, err := yaml.Marshal(&report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Round-trip: unmarshal back and verify fields.
	var got StitchReport
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.TaskID != report.TaskID {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, report.TaskID)
	}
	if got.TaskTitle != report.TaskTitle {
		t.Errorf("TaskTitle: got %q, want %q", got.TaskTitle, report.TaskTitle)
	}
	if got.Status != report.Status {
		t.Errorf("Status: got %q, want %q", got.Status, report.Status)
	}
	if got.Branch != report.Branch {
		t.Errorf("Branch: got %q, want %q", got.Branch, report.Branch)
	}
	if got.Diff.Files != 3 || got.Diff.Insertions != 120 || got.Diff.Deletions != 15 {
		t.Errorf("Diff: got %+v, want {3 120 15}", got.Diff)
	}
	if len(got.Files) != 3 {
		t.Fatalf("Files: got %d entries, want 3", len(got.Files))
	}
	if got.Files[0].Path != "pkg/widget/widget.go" || got.Files[0].Status != "A" {
		t.Errorf("Files[0]: got %+v", got.Files[0])
	}
	if got.Files[2].Status != "M" || got.Files[2].Deletions != 15 {
		t.Errorf("Files[2]: got %+v", got.Files[2])
	}
	if got.LOCBefore.Production != 500 || got.LOCAfter.Production != 580 {
		t.Errorf("LOC: before=%+v after=%+v", got.LOCBefore, got.LOCAfter)
	}
}

// --- parseNameStatus and parseNumstat ---

func TestParseNameStatus_AddedModifiedDeleted(t *testing.T) {
	nsOutput := "A\tpkg/new.go\nM\tpkg/existing.go\nD\tpkg/removed.go\n"
	numOutput := "50\t0\tpkg/new.go\n10\t5\tpkg/existing.go\n0\t30\tpkg/removed.go\n"

	numMap := parseNumstat(numOutput)
	files := parseNameStatus(nsOutput, numMap)

	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}

	tests := []struct {
		path   string
		status string
		ins    int
		del    int
	}{
		{"pkg/new.go", "A", 50, 0},
		{"pkg/existing.go", "M", 10, 5},
		{"pkg/removed.go", "D", 0, 30},
	}
	for i, tt := range tests {
		fc := files[i]
		if fc.Path != tt.path {
			t.Errorf("[%d] Path: got %q, want %q", i, fc.Path, tt.path)
		}
		if fc.Status != tt.status {
			t.Errorf("[%d] Status: got %q, want %q", i, fc.Status, tt.status)
		}
		if fc.Insertions != tt.ins {
			t.Errorf("[%d] Insertions: got %d, want %d", i, fc.Insertions, tt.ins)
		}
		if fc.Deletions != tt.del {
			t.Errorf("[%d] Deletions: got %d, want %d", i, fc.Deletions, tt.del)
		}
	}
}

func TestParseNameStatus_Renamed(t *testing.T) {
	nsOutput := "R100\told/path.go\tnew/path.go\n"
	numOutput := "5\t3\tnew/path.go\n"

	numMap := parseNumstat(numOutput)
	files := parseNameStatus(nsOutput, numMap)

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path != "new/path.go" {
		t.Errorf("Path: got %q, want %q", files[0].Path, "new/path.go")
	}
	if files[0].Status != "R" {
		t.Errorf("Status: got %q, want %q", files[0].Status, "R")
	}
	if files[0].Insertions != 5 || files[0].Deletions != 3 {
		t.Errorf("Counts: got ins=%d del=%d, want ins=5 del=3", files[0].Insertions, files[0].Deletions)
	}
}

func TestParseNameStatus_EmptyInput(t *testing.T) {
	files := parseNameStatus("", nil)
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestParseNumstat_BinaryFile(t *testing.T) {
	output := "-\t-\timage.png\n10\t2\tREADME.md\n"
	m := parseNumstat(output)

	if entry, ok := m["image.png"]; !ok {
		t.Error("missing entry for image.png")
	} else if entry.ins != 0 || entry.del != 0 {
		t.Errorf("image.png: got ins=%d del=%d, want 0 0", entry.ins, entry.del)
	}

	if entry, ok := m["README.md"]; !ok {
		t.Error("missing entry for README.md")
	} else if entry.ins != 10 || entry.del != 2 {
		t.Errorf("README.md: got ins=%d del=%d, want 10 2", entry.ins, entry.del)
	}
}

func TestParseNumstat_EmptyInput(t *testing.T) {
	m := parseNumstat("")
	if len(m) != 0 {
		t.Errorf("got %d entries, want 0", len(m))
	}
}

// --- saveHistoryReport ---

func TestSaveHistoryReport_WritesFile(t *testing.T) {
	dir := t.TempDir()
	o := &Orchestrator{
		cfg: Config{
			Cobbler: CobblerConfig{HistoryDir: dir},
		},
	}

	report := StitchReport{
		TaskID:    "test-001",
		TaskTitle: "Test task",
		Status:    "success",
		Branch:    "task/main-test-001",
		Diff:      historyDiff{Files: 1, Insertions: 10, Deletions: 2},
		Files: []FileChange{
			{Path: "pkg/foo.go", Status: "M", Insertions: 10, Deletions: 2},
		},
		LOCBefore: LocSnapshot{Production: 100, Test: 50},
		LOCAfter:  LocSnapshot{Production: 108, Test: 50},
	}

	ts := "2026-02-24-10-00-00"
	o.saveHistoryReport(ts, report)

	path := filepath.Join(dir, ts+"-stitch-report.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}

	var got StitchReport
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if got.TaskID != "test-001" {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, "test-001")
	}
	if len(got.Files) != 1 || got.Files[0].Path != "pkg/foo.go" {
		t.Errorf("Files: got %+v", got.Files)
	}
}

// --- extractTextFromStreamJSON ---

func TestExtractTextFromStreamJSON_SingleAssistantMessage(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"system","message":"ready"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Here is the output:\n\n` + "```yaml\\n- index: 0\\n  title: Task one\\n```" + `"}]}}
{"type":"result","usage":{"input_tokens":100,"output_tokens":50}}
`)
	text := extractTextFromStreamJSON(raw)
	if text == "" {
		t.Fatal("expected non-empty text")
	}
	if !contains(text, "Task one") {
		t.Errorf("text missing expected content, got: %s", text)
	}
}

func TestExtractTextFromStreamJSON_NoAssistantMessages(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"system","message":"ready"}
{"type":"result","usage":{"input_tokens":100,"output_tokens":0}}
`)
	text := extractTextFromStreamJSON(raw)
	if text != "" {
		t.Errorf("expected empty text, got: %s", text)
	}
}

func TestExtractTextFromStreamJSON_MultipleTextBlocks(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"first "},{"type":"text","text":"second"}]}}
`)
	text := extractTextFromStreamJSON(raw)
	if text != "first second" {
		t.Errorf("expected 'first second', got: %q", text)
	}
}

// --- extractYAMLBlock ---

func TestExtractYAMLBlock_ValidFencedBlock(t *testing.T) {
	t.Parallel()
	text := "Here is the YAML:\n\n```yaml\n- index: 0\n  title: Task one\n  dependency: -1\n```\n\nDone."
	yaml, err := extractYAMLBlock(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "- index: 0\n  title: Task one\n  dependency: -1"
	if string(yaml) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(yaml), expected)
	}
}

func TestExtractYAMLBlock_YmlFence(t *testing.T) {
	t.Parallel()
	text := "```yml\n- index: 0\n```\n"
	yaml, err := extractYAMLBlock(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(yaml) != "- index: 0" {
		t.Errorf("got: %q", string(yaml))
	}
}

func TestExtractYAMLBlock_NoFencedBlock(t *testing.T) {
	t.Parallel()
	text := "Here is some text with no YAML block."
	_, err := extractYAMLBlock(text)
	if err == nil {
		t.Error("expected error for missing YAML block")
	}
}

func TestExtractYAMLBlock_UnclosedBlock(t *testing.T) {
	t.Parallel()
	text := "```yaml\n- index: 0\n  title: Task one"
	_, err := extractYAMLBlock(text)
	if err == nil {
		t.Error("expected error for unclosed YAML block")
	}
}

// --- parseClaudeTokens ---

func TestParseClaudeTokens_ValidResult(t *testing.T) {
	t.Parallel()
	output := []byte(`{"type":"system","message":"ready"}
{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
{"type":"result","total_cost_usd":0.0325,"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":200,"cache_read_input_tokens":300}}
`)
	got := parseClaudeTokens(output)
	if got.InputTokens != 1500 {
		t.Errorf("InputTokens = %d, want 1500 (1000+200+300)", got.InputTokens)
	}
	if got.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", got.OutputTokens)
	}
	if got.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens = %d, want 200", got.CacheCreationTokens)
	}
	if got.CacheReadTokens != 300 {
		t.Errorf("CacheReadTokens = %d, want 300", got.CacheReadTokens)
	}
	if got.CostUSD != 0.0325 {
		t.Errorf("CostUSD = %f, want 0.0325", got.CostUSD)
	}
}

func TestParseClaudeTokens_NoResultEvent(t *testing.T) {
	t.Parallel()
	output := []byte(`{"type":"system","message":"ready"}
{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
`)
	got := parseClaudeTokens(output)
	if got.InputTokens != 0 || got.OutputTokens != 0 {
		t.Errorf("expected zero tokens for missing result, got in=%d out=%d", got.InputTokens, got.OutputTokens)
	}
}

func TestParseClaudeTokens_EmptyInput(t *testing.T) {
	t.Parallel()
	got := parseClaudeTokens([]byte(""))
	if got.InputTokens != 0 {
		t.Errorf("expected zero tokens for empty input, got %d", got.InputTokens)
	}
}

// --- toolSummary ---

func TestToolSummary_FilePath(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"file_path":"pkg/orchestrator/generator.go","content":"hello"}`)
	got := toolSummary(input)
	if got != "pkg/orchestrator/generator.go" {
		t.Errorf("toolSummary(file_path) = %q, want %q", got, "pkg/orchestrator/generator.go")
	}
}

func TestToolSummary_Command(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"command":"go test ./..."}`)
	got := toolSummary(input)
	if got != "go test ./..." {
		t.Errorf("toolSummary(command) = %q, want %q", got, "go test ./...")
	}
}

func TestToolSummary_LongCommandTruncated(t *testing.T) {
	t.Parallel()
	longCmd := strings.Repeat("x", 100)
	input := json.RawMessage(`{"command":"` + longCmd + `"}`)
	got := toolSummary(input)
	if len(got) != 83 { // 80 + "..."
		t.Errorf("toolSummary(long command) len = %d, want 83", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("toolSummary(long command) should end with '...', got %q", got)
	}
}

func TestToolSummary_Pattern(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"pattern":"**/*.go"}`)
	got := toolSummary(input)
	if got != "**/*.go" {
		t.Errorf("toolSummary(pattern) = %q, want %q", got, "**/*.go")
	}
}

func TestToolSummary_EmptyInput(t *testing.T) {
	t.Parallel()
	got := toolSummary(json.RawMessage(""))
	if got != "" {
		t.Errorf("toolSummary(empty) = %q, want empty", got)
	}
}

func TestToolSummary_NoKnownFields(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"unknown_field":"value"}`)
	got := toolSummary(input)
	if got != "" {
		t.Errorf("toolSummary(unknown) = %q, want empty", got)
	}
}

func TestToolSummary_PriorityOrder(t *testing.T) {
	t.Parallel()
	// file_path should take priority over command
	input := json.RawMessage(`{"file_path":"foo.go","command":"go build"}`)
	got := toolSummary(input)
	if got != "foo.go" {
		t.Errorf("toolSummary(priority) = %q, want %q", got, "foo.go")
	}
}

func TestSaveHistoryReport_NoOpWhenHistoryDirEmpty(t *testing.T) {
	o := &Orchestrator{
		cfg: Config{
			Cobbler: CobblerConfig{HistoryDir: ""},
		},
	}

	// Should not panic or create any files.
	o.saveHistoryReport("2026-02-24-10-00-00", StitchReport{
		TaskID: "test-noop",
		Status: "success",
	})
}

// --- historyDir ---

func TestHistoryDir_Empty(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{HistoryDir: ""}}}
	if got := o.historyDir(); got != "" {
		t.Errorf("historyDir() = %q, want empty", got)
	}
}

func TestHistoryDir_Absolute(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{
		Dir:        ".cobbler/",
		HistoryDir: "/tmp/history",
	}}}
	if got := o.historyDir(); got != "/tmp/history" {
		t.Errorf("historyDir() = %q, want %q", got, "/tmp/history")
	}
}

func TestHistoryDir_Relative(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{
		Dir:        ".cobbler/",
		HistoryDir: "history",
	}}}
	want := filepath.Join(".cobbler/", "history")
	if got := o.historyDir(); got != want {
		t.Errorf("historyDir() = %q, want %q", got, want)
	}
}

// --- worktreeBasePath ---

func TestWorktreeBasePath(t *testing.T) {
	t.Parallel()
	got := worktreeBasePath()
	if got == "" {
		t.Fatal("worktreeBasePath() returned empty string")
	}
	if !strings.HasSuffix(got, "-worktrees") {
		t.Errorf("worktreeBasePath() = %q, want suffix '-worktrees'", got)
	}
}

// TestWorktreeBasePath_FromWorktree verifies that worktreeBasePath returns the
// same value whether called from the main repo root or from a git worktree of
// the same repository (prd003 R3.16, rel01.0-uc010).
func TestWorktreeBasePath_FromWorktree(t *testing.T) {
	// Requires git on PATH; skip gracefully if not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}

	mainDir := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	// Initialize a git repo with an initial commit so worktree add works.
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run(mainDir, "init", "-b", "main")
	run(mainDir, "commit", "--allow-empty", "-m", "init")

	// Add a worktree on a new branch.
	wtDir := filepath.Join(mainDir, "wt")
	run(mainDir, "worktree", "add", wtDir, "-b", "feature-test")

	// Compute expected: derived from the main repo name.
	expected := filepath.Join(os.TempDir(), filepath.Base(mainDir)+"-worktrees")

	// Call from main repo.
	os.Chdir(mainDir)
	fromMain := worktreeBasePath()

	// Call from inside the worktree.
	os.Chdir(wtDir)
	fromWorktree := worktreeBasePath()

	if fromMain != expected {
		t.Errorf("from main: got %q, want %q", fromMain, expected)
	}
	if fromWorktree != expected {
		t.Errorf("from worktree: got %q, want %q", fromWorktree, expected)
	}
	if fromMain != fromWorktree {
		t.Errorf("paths differ: main=%q worktree=%q", fromMain, fromWorktree)
	}
}

// TestWorktreeBasePath_FallbackOutsideGit verifies the graceful fallback when
// git rev-parse --git-common-dir fails (not a git repository).
func TestWorktreeBasePath_FallbackOutsideGit(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	got := worktreeBasePath()
	if got == "" {
		t.Fatal("worktreeBasePath() returned empty string in fallback")
	}
	if !strings.HasSuffix(got, "-worktrees") {
		t.Errorf("fallback: got %q, want suffix '-worktrees'", got)
	}
	expected := filepath.Join(os.TempDir(), filepath.Base(dir)+"-worktrees")
	if got != expected {
		t.Errorf("fallback: got %q, want %q", got, expected)
	}
}

// --- saveHistoryStats ---

func TestSaveHistoryStats_WritesFile(t *testing.T) {
	dir := t.TempDir()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{
		Dir:        dir + "/",
		HistoryDir: "hist",
	}}}

	stats := HistoryStats{
		Caller: "test",
		TaskID: "task-001",
		Status: "success",
	}
	o.saveHistoryStats("2026-02-26-10-00-00", "stitch", stats)

	path := filepath.Join(dir, "hist", "2026-02-26-10-00-00-stitch-stats.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	if !strings.Contains(string(data), "task-001") {
		t.Errorf("file content should contain task ID, got: %s", data)
	}
}

func TestSaveHistoryStats_NoOpWhenEmpty(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{HistoryDir: ""}}}
	// Should not panic.
	o.saveHistoryStats("ts", "phase", HistoryStats{})
}

// --- saveHistoryPrompt ---

func TestSaveHistoryPrompt_WritesFile(t *testing.T) {
	dir := t.TempDir()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{
		Dir:        dir + "/",
		HistoryDir: "hist",
	}}}

	o.saveHistoryPrompt("2026-02-26-10-00-00", "measure", "prompt content here")

	path := filepath.Join(dir, "hist", "2026-02-26-10-00-00-measure-prompt.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	if string(data) != "prompt content here" {
		t.Errorf("file content = %q, want %q", string(data), "prompt content here")
	}
}

func TestSaveHistoryPrompt_NoOpWhenEmpty(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{HistoryDir: ""}}}
	o.saveHistoryPrompt("ts", "phase", "prompt")
}

// --- saveHistoryLog ---

func TestSaveHistoryLog_WritesFile(t *testing.T) {
	dir := t.TempDir()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{
		Dir:        dir + "/",
		HistoryDir: "hist",
	}}}

	logData := []byte(`{"type":"assistant","message":"hello"}`)
	o.saveHistoryLog("2026-02-26-10-00-00", "stitch", logData)

	path := filepath.Join(dir, "hist", "2026-02-26-10-00-00-stitch-log.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	if string(data) != string(logData) {
		t.Errorf("file content mismatch")
	}
}

func TestSaveHistoryLog_NoOpWhenEmpty(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{HistoryDir: ""}}}
	o.saveHistoryLog("ts", "phase", []byte("data"))
}

// --- buildPodmanCmd ---

func TestBuildPodmanCmd_ContainsWorkdirMount(t *testing.T) {
	t.Parallel()
	o := New(Config{})
	cmd := o.buildPodmanCmd(context.TODO(),"/work/mydir")

	args := cmd.Args
	// args[0] is the binary; remaining are the podman args
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/work/mydir:/work/mydir") {
		t.Errorf("buildPodmanCmd args missing workdir volume mount; args=%v", args)
	}
	if !strings.Contains(joined, "-w /work/mydir") {
		t.Errorf("buildPodmanCmd args missing -w workdir flag; args=%v", args)
	}
}

func TestBuildPodmanCmd_ContainsImageAndClaude(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	cfg.Podman.Image = "my-custom-image:latest"
	o := New(cfg)
	cmd := o.buildPodmanCmd(context.TODO(),"/work")

	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "my-custom-image:latest") {
		t.Errorf("buildPodmanCmd args missing image; args=%v", cmd.Args)
	}
	if !strings.Contains(joined, binClaude) {
		t.Errorf("buildPodmanCmd args missing claude binary %q; args=%v", binClaude, cmd.Args)
	}
}

func TestBuildPodmanCmd_ExtraArgsAppended(t *testing.T) {
	t.Parallel()
	o := New(Config{})
	cmd := o.buildPodmanCmd(context.TODO(),"/work", "--verbose", "--no-color")

	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--verbose") {
		t.Errorf("buildPodmanCmd missing extra arg --verbose; args=%v", cmd.Args)
	}
	if !strings.Contains(joined, "--no-color") {
		t.Errorf("buildPodmanCmd missing extra arg --no-color; args=%v", cmd.Args)
	}
}

// --- saveHistory* best-effort behavior ---

func TestSaveHistoryReport_EmptyHistoryDir_NoOp(t *testing.T) {
	// When HistoryDir is empty saveHistoryReport must return without
	// panicking. No files should be created.
	o := New(Config{})
	o.cfg.Cobbler.HistoryDir = "" // override default
	o.saveHistoryReport("20260101T120000", StitchReport{TaskID: "t1", Status: "success"})
	// success: did not panic
}

func TestSaveHistoryReport_WritesToDisk(t *testing.T) {
	tmp := t.TempDir()
	o := New(Config{})
	o.cfg.Cobbler.HistoryDir = tmp
	o.saveHistoryReport("20260101T120000", StitchReport{TaskID: "t1", Status: "success"})

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), "-stitch-report.yaml") {
		t.Errorf("unexpected filename: %s", entries[0].Name())
	}
}

func TestSaveHistoryStats_EmptyHistoryDir_NoOp(t *testing.T) {
	o := New(Config{})
	o.cfg.Cobbler.HistoryDir = ""
	o.saveHistoryStats("20260101T120000", "stitch", HistoryStats{})
	// success: did not panic
}

func TestSaveHistoryPrompt_EmptyHistoryDir_NoOp(t *testing.T) {
	o := New(Config{})
	o.cfg.Cobbler.HistoryDir = ""
	o.saveHistoryPrompt("20260101T120000", "stitch", "prompt text")
	// success: did not panic
}

func TestSaveHistoryLog_EmptyHistoryDir_NoOp(t *testing.T) {
	o := New(Config{})
	o.cfg.Cobbler.HistoryDir = ""
	o.saveHistoryLog("20260101T120000", "stitch", []byte("log output"))
	// success: did not panic
}

// --- captureLOC ---

func TestCaptureLOC_CountsProductionAndTestFiles(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("line 1\nline 2\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b_test.go"), []byte("test 1\ntest 2\ntest 3\n"), 0644)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	o := New(Config{})
	snap := o.captureLOC()
	if snap.Production != 2 {
		t.Errorf("Production = %d, want 2", snap.Production)
	}
	if snap.Test != 3 {
		t.Errorf("Test = %d, want 3", snap.Test)
	}
}

func TestCaptureLOC_EmptyDir_ReturnsZero(t *testing.T) {
	// Not parallel: uses os.Chdir.
	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	o := New(Config{})
	snap := o.captureLOC()
	if snap.Production != 0 || snap.Test != 0 {
		t.Errorf("empty dir: got {%d %d}, want {0 0}", snap.Production, snap.Test)
	}
}

// --- InvocationRecord JSON serialization (covers recordInvocation marshaling) ---

func TestInvocationRecord_JSONShape(t *testing.T) {
	t.Parallel()
	rec := InvocationRecord{
		Caller:    "stitch",
		StartedAt: "2026-02-27T10:00:00Z",
		DurationS: 42,
		Tokens:    claudeTokens{Input: 1500, Output: 500, CacheCreation: 200, CacheRead: 300, CostUSD: 0.0325},
		LOCBefore: LocSnapshot{Production: 100, Test: 50},
		LOCAfter:  LocSnapshot{Production: 110, Test: 55},
		Diff:      diffRecord{Files: 3, Insertions: 20, Deletions: 5},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal top-level: %v", err)
	}

	// All required fields must be present (prd005 R1.1-R1.7).
	for _, key := range []string{"caller", "started_at", "duration_s", "tokens", "loc_before", "loc_after", "diff"} {
		if _, ok := got[key]; !ok {
			t.Errorf("InvocationRecord JSON missing field %q", key)
		}
	}

	// Spot-check values.
	var caller string
	json.Unmarshal(got["caller"], &caller)
	if caller != "stitch" {
		t.Errorf("caller = %q, want stitch", caller)
	}

	var tokens claudeTokens
	json.Unmarshal(got["tokens"], &tokens)
	if tokens.Input != 1500 || tokens.Output != 500 {
		t.Errorf("tokens = %+v, want {Input:1500 Output:500}", tokens)
	}
	if tokens.CacheCreation != 200 || tokens.CacheRead != 300 {
		t.Errorf("tokens cache = {CacheCreation:%d CacheRead:%d}, want {200 300}", tokens.CacheCreation, tokens.CacheRead)
	}
}

// --- parseClaudeTokens error branch ---

func TestParseClaudeTokens_MalformedResultEvent(t *testing.T) {
	t.Parallel()
	// type="result" line present but usage field is malformed JSON.
	output := []byte(`{"type":"result","usage":"not_an_object"}`)
	got := parseClaudeTokens(output)
	// Malformed result: should return zero values gracefully.
	if got.InputTokens != 0 || got.OutputTokens != 0 {
		t.Errorf("malformed result: got in=%d out=%d, want 0 0", got.InputTokens, got.OutputTokens)
	}
}

// --- formatOutcomeTrailers ---

func TestFormatOutcomeTrailers_ReturnsTenStrings(t *testing.T) {
	t.Parallel()
	rec := InvocationRecord{
		Caller:    "stitch",
		StartedAt: "2026-02-28T00:00:00Z",
		DurationS: 1234,
		Tokens:    claudeTokens{Input: 45000, Output: 12000, CacheCreation: 5000, CacheRead: 3000, CostUSD: 0.75},
		LOCBefore: LocSnapshot{Production: 441, Test: 0},
		LOCAfter:  LocSnapshot{Production: 520, Test: 45},
	}
	trailers := formatOutcomeTrailers(rec)
	if len(trailers) != 10 {
		t.Fatalf("formatOutcomeTrailers: got %d trailers, want 10; trailers=%v", len(trailers), trailers)
	}
	expected := []string{
		"Tokens-Input: 45000",
		"Tokens-Output: 12000",
		"Tokens-Cache-Creation: 5000",
		"Tokens-Cache-Read: 3000",
		"Tokens-Cost-USD: 0.7500",
		"Loc-Prod-Before: 441",
		"Loc-Prod-After: 520",
		"Loc-Test-Before: 0",
		"Loc-Test-After: 45",
		"Duration-Seconds: 1234",
	}
	for i, want := range expected {
		if trailers[i] != want {
			t.Errorf("trailer[%d]: got %q, want %q", i, trailers[i], want)
		}
	}
}

func TestFormatOutcomeTrailers_ZeroRecord(t *testing.T) {
	t.Parallel()
	trailers := formatOutcomeTrailers(InvocationRecord{})
	if len(trailers) != 10 {
		t.Fatalf("zero record: got %d trailers, want 10", len(trailers))
	}
	// Zero cost should format as 0.0000.
	if trailers[4] != "Tokens-Cost-USD: 0.0000" {
		t.Errorf("zero cost trailer: got %q, want %q", trailers[4], "Tokens-Cost-USD: 0.0000")
	}
}

// --- appendOutcomeTrailers ---

func TestAppendOutcomeTrailers_AmendsLastCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Initialize a minimal git repo.
	gitSetup := [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	}
	for _, args := range gitSetup {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git setup: %v\n%s", err, out)
		}
	}

	// Write a file and make an initial commit.
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "Initial commit"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	rec := InvocationRecord{
		DurationS: 120,
		Tokens:    claudeTokens{Input: 1000, Output: 200, CacheCreation: 50, CacheRead: 30, CostUSD: 0.05},
		LOCBefore: LocSnapshot{Production: 100, Test: 20},
		LOCAfter:  LocSnapshot{Production: 150, Test: 30},
	}
	if err := appendOutcomeTrailers(dir, rec); err != nil {
		// git commit --amend --trailer requires git >= 2.38; skip if unsupported.
		t.Skipf("appendOutcomeTrailers: %v", err)
	}

	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%(trailers)").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	trailerStr := string(out)
	for _, wantKey := range []string{
		"Tokens-Input:",
		"Tokens-Output:",
		"Tokens-Cost-USD:",
		"Duration-Seconds:",
		"Loc-Prod-Before:",
		"Loc-Prod-After:",
	} {
		if !strings.Contains(trailerStr, wantKey) {
			t.Errorf("trailer output missing key %q\ngot:\n%s", wantKey, trailerStr)
		}
	}
}

// --- newProgressWriter ---

func TestNewProgressWriter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pw := newProgressWriter(&buf, start)
	if pw == nil {
		t.Fatal("newProgressWriter returned nil")
	}
	if pw.buf != &buf {
		t.Error("buf field not set")
	}
	if pw.start != start {
		t.Errorf("start = %v, want %v", pw.start, start)
	}
	if pw.turn != 0 {
		t.Errorf("turn = %d, want 0", pw.turn)
	}
	if pw.gotFirst {
		t.Error("gotFirst should be false initially")
	}
}

// --- progressWriter.Write ---

func TestProgressWriter_Write_PassesThrough(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())
	data := []byte(`{"type":"system"}` + "\n")
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Errorf("n = %d, want %d", n, len(data))
	}
	if buf.String() != string(data) {
		t.Errorf("buffer = %q, want %q", buf.String(), string(data))
	}
}

func TestProgressWriter_Write_SetsGotFirst(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())
	if pw.gotFirst {
		t.Fatal("gotFirst should be false before first write")
	}
	pw.Write([]byte("x"))
	if !pw.gotFirst {
		t.Error("gotFirst should be true after first write")
	}
}

func TestProgressWriter_Write_AccumulatesPartial(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())
	// Write without newline — should accumulate in partial.
	pw.Write([]byte(`{"type":"sys`))
	if len(pw.partial) == 0 {
		t.Error("partial should have accumulated data")
	}
	// Complete the line.
	pw.Write([]byte("tem\"}\n"))
	// After newline, partial for that line should be consumed.
	if len(pw.partial) != 0 {
		t.Errorf("partial should be empty after newline, got %q", string(pw.partial))
	}
}

// --- progressWriter.logLine ---

func TestProgressWriter_LogLine_EmptyLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())
	// Should not panic on empty line.
	pw.logLine(nil)
	pw.logLine([]byte{})
}

func TestProgressWriter_LogLine_InvalidJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())
	// Should not panic on invalid JSON.
	pw.logLine([]byte("not json"))
	if pw.turn != 0 {
		t.Errorf("turn should remain 0 for invalid JSON, got %d", pw.turn)
	}
}

func TestProgressWriter_LogLine_AssistantTurn(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())

	msg := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello world"},
			},
		},
	}
	line, _ := json.Marshal(msg)
	pw.logLine(line)
	if pw.turn != 1 {
		t.Errorf("turn = %d, want 1 after assistant message", pw.turn)
	}
}

func TestProgressWriter_LogLine_AssistantToolUse(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())

	msg := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "tool_use", "name": "Read", "input": map[string]any{"file_path": "/tmp/test.go"}},
			},
		},
	}
	line, _ := json.Marshal(msg)
	pw.logLine(line)
	if pw.turn != 1 {
		t.Errorf("turn = %d, want 1", pw.turn)
	}
}

func TestProgressWriter_LogLine_ResultEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())
	pw.turn = 3

	msg := map[string]any{
		"type":           "result",
		"total_cost_usd": 0.1234,
		"usage": map[string]any{
			"input_tokens":                100,
			"output_tokens":               50,
			"cache_creation_input_tokens": 10,
			"cache_read_input_tokens":     5,
		},
	}
	line, _ := json.Marshal(msg)
	pw.logLine(line)
	// turn should remain unchanged for result events.
	if pw.turn != 3 {
		t.Errorf("turn = %d, want 3", pw.turn)
	}
}

func TestProgressWriter_LogLine_LongSnippetTruncated(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	pw := newProgressWriter(&buf, time.Now())

	longText := strings.Repeat("a", 200)
	msg := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": longText},
			},
		},
	}
	line, _ := json.Marshal(msg)
	pw.logLine(line)
	if pw.turn != 1 {
		t.Errorf("turn = %d, want 1", pw.turn)
	}
}

// --- logConfig ---

func TestLogConfig(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Cobbler: CobblerConfig{
			MaxStitchIssues:         10,
			MaxStitchIssuesPerCycle: 3,
			MaxMeasureIssues:        5,
			UserPrompt:              "test prompt",
		},
	}}
	// logConfig should not panic. It writes to logf (stderr) which we don't capture.
	o.logConfig("test-target")
}

func TestLogConfig_NoUserPrompt(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{
		Cobbler: CobblerConfig{
			MaxStitchIssues: 5,
		},
	}}
	// When UserPrompt is empty, the second logf call is skipped.
	o.logConfig("stitch")
}

// --- CobblerReset ---

func TestCobblerReset_RemovesDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), ".cobbler")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("data"), 0o644)

	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: dir}}}
	if err := o.CobblerReset(); err != nil {
		t.Fatalf("CobblerReset: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected cobbler dir to be removed")
	}
}

func TestCobblerReset_NonExistentDir(t *testing.T) {
	t.Parallel()
	o := &Orchestrator{cfg: Config{Cobbler: CobblerConfig{Dir: filepath.Join(t.TempDir(), "nope")}}}
	if err := o.CobblerReset(); err != nil {
		t.Fatalf("CobblerReset on nonexistent dir: %v", err)
	}
}
