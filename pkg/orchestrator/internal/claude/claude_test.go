// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

// stubRepoReader implements gitops.RepoReader for tests. Only CurrentBranch
// is wired; other methods panic if called unexpectedly.
type stubRepoReader struct {
	gitops.ShellGitOps // provides default implementations
	branch             string
	branchErr          error
}

func (s *stubRepoReader) CurrentBranch(dir string) (string, error) {
	return s.branch, s.branchErr
}

// ---------------------------------------------------------------------------
// ParseClaudeTokens
// ---------------------------------------------------------------------------

func TestParseClaudeTokens_ValidResult(t *testing.T) {
	output := []byte(`{"type":"system","message":"ready"}
{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
{"type":"result","total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":20}}
`)
	r := ParseClaudeTokens(output)
	if r.InputTokens != 130 { // 100+10+20
		t.Errorf("expected InputTokens=130, got %d", r.InputTokens)
	}
	if r.OutputTokens != 50 {
		t.Errorf("expected OutputTokens=50, got %d", r.OutputTokens)
	}
	if r.CacheCreationTokens != 10 {
		t.Errorf("expected CacheCreationTokens=10, got %d", r.CacheCreationTokens)
	}
	if r.CacheReadTokens != 20 {
		t.Errorf("expected CacheReadTokens=20, got %d", r.CacheReadTokens)
	}
	if r.CostUSD != 0.05 {
		t.Errorf("expected CostUSD=0.05, got %f", r.CostUSD)
	}
}

func TestParseClaudeTokens_NoResult(t *testing.T) {
	output := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
`)
	r := ParseClaudeTokens(output)
	if r.InputTokens != 0 || r.OutputTokens != 0 {
		t.Errorf("expected zero tokens for no result event, got in=%d out=%d", r.InputTokens, r.OutputTokens)
	}
}

func TestParseClaudeTokens_Empty(t *testing.T) {
	r := ParseClaudeTokens(nil)
	if r.InputTokens != 0 {
		t.Errorf("expected zero tokens for empty input, got %d", r.InputTokens)
	}
}

func TestParseClaudeTokens_MalformedJSON(t *testing.T) {
	r := ParseClaudeTokens([]byte("not json at all\n"))
	if r.InputTokens != 0 {
		t.Errorf("expected zero tokens for malformed input, got %d", r.InputTokens)
	}
}

// ---------------------------------------------------------------------------
// ExtractTextFromStreamJSON
// ---------------------------------------------------------------------------

func TestExtractTextFromStreamJSON_ValidStream(t *testing.T) {
	output := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello "}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"world"}]}}
`)
	got := ExtractTextFromStreamJSON(output)
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestExtractTextFromStreamJSON_PlainText(t *testing.T) {
	output := []byte("plain text not json\n")
	got := ExtractTextFromStreamJSON(output)
	if got != "plain text not json\n" {
		t.Errorf("expected plain text passthrough, got %q", got)
	}
}

func TestExtractTextFromStreamJSON_Empty(t *testing.T) {
	got := ExtractTextFromStreamJSON(nil)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractTextFromStreamJSON_NonAssistantTypes(t *testing.T) {
	output := []byte(`{"type":"system","message":{"content":[]}}
{"type":"user","message":{"content":[]}}
`)
	got := ExtractTextFromStreamJSON(output)
	if got != "" {
		t.Errorf("expected empty for non-assistant types, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// ExtractYAMLBlock
// ---------------------------------------------------------------------------

func TestExtractYAMLBlock_Found(t *testing.T) {
	text := "Some text\n```yaml\nkey: value\n```\nMore text"
	got, err := ExtractYAMLBlock(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "key: value" {
		t.Errorf("expected 'key: value', got %q", string(got))
	}
}

func TestExtractYAMLBlock_YmlMarker(t *testing.T) {
	text := "```yml\nfoo: bar\n```"
	got, err := ExtractYAMLBlock(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "foo: bar" {
		t.Errorf("expected 'foo: bar', got %q", string(got))
	}
}

func TestExtractYAMLBlock_NotFound(t *testing.T) {
	_, err := ExtractYAMLBlock("no yaml block here")
	if err == nil {
		t.Fatal("expected error when no YAML block found")
	}
}

func TestExtractYAMLBlock_Unclosed(t *testing.T) {
	text := "```yaml\nkey: value\n"
	_, err := ExtractYAMLBlock(text)
	if err == nil {
		t.Fatal("expected error for unclosed block")
	}
}

// ---------------------------------------------------------------------------
// ToolSummary
// ---------------------------------------------------------------------------

func TestToolSummary_FilePath(t *testing.T) {
	input := json.RawMessage(`{"file_path":"pkg/foo/bar.go","content":"stuff"}`)
	got := ToolSummary(input)
	if got != "pkg/foo/bar.go" {
		t.Errorf("expected 'pkg/foo/bar.go', got %q", got)
	}
}

func TestToolSummary_Command(t *testing.T) {
	input := json.RawMessage(`{"command":"echo hello"}`)
	got := ToolSummary(input)
	if got != "echo hello" {
		t.Errorf("expected 'echo hello', got %q", got)
	}
}

func TestToolSummary_LongCommand(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	input := json.RawMessage(fmt.Sprintf(`{"command":"%s"}`, long))
	got := ToolSummary(input)
	if len(got) > 84 { // 80 + "..."
		t.Errorf("expected truncated command, got length %d", len(got))
	}
}

func TestToolSummary_Pattern(t *testing.T) {
	input := json.RawMessage(`{"pattern":"**/*.go"}`)
	got := ToolSummary(input)
	if got != "**/*.go" {
		t.Errorf("expected '**/*.go', got %q", got)
	}
}

func TestToolSummary_Empty(t *testing.T) {
	got := ToolSummary(nil)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestToolSummary_NoKnownKeys(t *testing.T) {
	input := json.RawMessage(`{"foo":"bar"}`)
	got := ToolSummary(input)
	if got != "" {
		t.Errorf("expected empty for unknown keys, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// IntFromUsage
// ---------------------------------------------------------------------------

func TestIntFromUsage_Float64(t *testing.T) {
	usage := map[string]interface{}{"input_tokens": float64(100)}
	if got := IntFromUsage(usage, "input_tokens"); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
}

func TestIntFromUsage_Int(t *testing.T) {
	usage := map[string]interface{}{"input_tokens": 42}
	if got := IntFromUsage(usage, "input_tokens"); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestIntFromUsage_Missing(t *testing.T) {
	usage := map[string]interface{}{"output_tokens": float64(50)}
	if got := IntFromUsage(usage, "input_tokens"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestIntFromUsage_NilMap(t *testing.T) {
	if got := IntFromUsage(nil, "anything"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestIntFromUsage_StringValue(t *testing.T) {
	usage := map[string]interface{}{"input_tokens": "not a number"}
	if got := IntFromUsage(usage, "input_tokens"); got != 0 {
		t.Errorf("expected 0 for string value, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// FormatOutcomeTrailers
// ---------------------------------------------------------------------------

func TestFormatOutcomeTrailers(t *testing.T) {
	rec := InvocationRecord{
		DurationS: 120,
		Tokens: ClaudeTokens{
			Input:         1000,
			Output:        500,
			CacheCreation: 200,
			CacheRead:     300,
			CostUSD:       0.05,
		},
		LOCBefore: LocSnapshot{Production: 100, Test: 50},
		LOCAfter:  LocSnapshot{Production: 120, Test: 55},
	}
	trailers := FormatOutcomeTrailers(rec)
	if len(trailers) != 10 {
		t.Fatalf("expected 10 trailers, got %d", len(trailers))
	}
	if trailers[0] != "Tokens-Input: 1000" {
		t.Errorf("unexpected first trailer: %q", trailers[0])
	}
}

// ---------------------------------------------------------------------------
// HistoryDir
// ---------------------------------------------------------------------------

func TestHistoryDir_Empty(t *testing.T) {
	if got := HistoryDir("/cobbler", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHistoryDir_Absolute(t *testing.T) {
	if got := HistoryDir("/cobbler", "/abs/path"); got != "/abs/path" {
		t.Errorf("expected /abs/path, got %q", got)
	}
}

func TestHistoryDir_Relative(t *testing.T) {
	got := HistoryDir("/cobbler", "history")
	if got != "/cobbler/history" {
		t.Errorf("expected /cobbler/history, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// SaveHistoryStats
// ---------------------------------------------------------------------------

func TestSaveHistoryStats_WritesFile(t *testing.T) {
	dir := t.TempDir()
	stats := HistoryStats{
		Caller:    "measure",
		StartedAt: "2026-01-01T00:00:00Z",
		Duration:  "10s",
		DurationS: 10,
	}
	SaveHistoryStats(dir, "20260101T000000", "measure", stats)

	path := filepath.Join(dir, "20260101T000000-measure-stats.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected stats file to exist: %v", err)
	}
}

func TestSaveHistoryStats_EmptyDir(t *testing.T) {
	// Should be a no-op, not panic.
	SaveHistoryStats("", "ts", "phase", HistoryStats{})
}

// ---------------------------------------------------------------------------
// SaveHistoryPrompt
// ---------------------------------------------------------------------------

func TestSaveHistoryPrompt_WritesFile(t *testing.T) {
	dir := t.TempDir()
	SaveHistoryPrompt(dir, "20260101T000000", "measure", "test prompt")

	path := filepath.Join(dir, "20260101T000000-measure-prompt.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected prompt file to exist: %v", err)
	}
	if string(data) != "test prompt" {
		t.Errorf("expected 'test prompt', got %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// SaveHistoryLog
// ---------------------------------------------------------------------------

func TestSaveHistoryLog_WritesFile(t *testing.T) {
	dir := t.TempDir()
	SaveHistoryLog(dir, "20260101T000000", "stitch", []byte("raw output"))

	path := filepath.Join(dir, "20260101T000000-stitch-log.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
	if string(data) != "raw output" {
		t.Errorf("expected 'raw output', got %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// SaveHistoryReport
// ---------------------------------------------------------------------------

func TestSaveHistoryReport_WritesFile(t *testing.T) {
	dir := t.TempDir()
	report := StitchReport{
		TaskID:    "42",
		TaskTitle: "Test task",
		Status:    "success",
	}
	SaveHistoryReport(dir, "20260101T000000", report)

	path := filepath.Join(dir, "20260101T000000-stitch-report.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected report file to exist: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ProgressWriter
// ---------------------------------------------------------------------------

func TestNewProgressWriter(t *testing.T) {
	var buf bytes.Buffer
	start := time.Now()
	pw := NewProgressWriter(&buf, start)
	if pw.Buf != &buf {
		t.Error("expected Buf to match")
	}
	if pw.Turn != 0 {
		t.Errorf("expected Turn=0, got %d", pw.Turn)
	}
}

func TestProgressWriter_Write(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf, time.Now())

	data := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}` + "\n")
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}
	if pw.Turn != 1 {
		t.Errorf("expected Turn=1 after assistant message, got %d", pw.Turn)
	}
	if !pw.GotFirst {
		t.Error("expected GotFirst to be true")
	}
}

func TestProgressWriter_MultipleTurns(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf, time.Now())

	for i := 0; i < 3; i++ {
		line := `{"type":"assistant","message":{"content":[{"type":"text","text":"turn"}]}}` + "\n"
		pw.Write([]byte(line))
	}
	if pw.Turn != 3 {
		t.Errorf("expected Turn=3, got %d", pw.Turn)
	}
}

// ---------------------------------------------------------------------------
// CaptureLOCAt
// ---------------------------------------------------------------------------

func TestCaptureLOCAt_EmptyDir(t *testing.T) {
	called := false
	snap := CaptureLOCAt("", func() LocSnapshot {
		called = true
		return LocSnapshot{Production: 42}
	})
	if !called {
		t.Error("expected captureFn to be called")
	}
	if snap.Production != 42 {
		t.Errorf("expected Production=42, got %d", snap.Production)
	}
}

func TestCaptureLOCAt_WithDir(t *testing.T) {
	dir := t.TempDir()
	// CaptureLOCAt should chdir into dir and restore afterward.
	// We verify by checking that it returns the captureFn result
	// and that the working directory is restored.
	origCwd, _ := os.Getwd()
	snap := CaptureLOCAt(dir, func() LocSnapshot {
		return LocSnapshot{Production: 100, Test: 50}
	})
	afterCwd, _ := os.Getwd()
	if snap.Production != 100 {
		t.Errorf("expected Production=100, got %d", snap.Production)
	}
	if origCwd != afterCwd {
		t.Errorf("expected cwd restored to %q, got %q", origCwd, afterCwd)
	}
}

// ---------------------------------------------------------------------------
// HasOpenIssues
// ---------------------------------------------------------------------------

func TestHasOpenIssues_HasIssues(t *testing.T) {
	deps := HasOpenIssuesDeps{
		DetectGitHubRepoFn:      func(root string) (string, error) { return "owner/repo", nil },
		GitReader:               &stubRepoReader{branch: "gen-1"},
		ListOpenCobblerIssuesFn: func(repo, branch string) (int, error) { return 3, nil },
	}
	got, err := HasOpenIssues(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true when issues exist")
	}
}

func TestHasOpenIssues_NoIssues(t *testing.T) {
	deps := HasOpenIssuesDeps{
		DetectGitHubRepoFn:      func(root string) (string, error) { return "owner/repo", nil },
		GitReader:               &stubRepoReader{branch: "gen-1"},
		ListOpenCobblerIssuesFn: func(repo, branch string) (int, error) { return 0, nil },
	}
	got, err := HasOpenIssues(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false when no issues")
	}
}

func TestHasOpenIssues_DetectRepoError(t *testing.T) {
	deps := HasOpenIssuesDeps{
		DetectGitHubRepoFn: func(root string) (string, error) { return "", fmt.Errorf("no repo") },
	}
	_, err := HasOpenIssues(deps)
	if err == nil {
		t.Fatal("expected error when repo detection fails")
	}
}

// ---------------------------------------------------------------------------
// HistoryClean
// ---------------------------------------------------------------------------

func TestHistoryClean_RemovesDir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "history")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "file.yaml"), []byte("data"), 0o644)

	if err := HistoryClean(subDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Error("expected directory to be removed")
	}
}

func TestHistoryClean_EmptyDir(t *testing.T) {
	if err := HistoryClean(""); err != nil {
		t.Errorf("expected no error for empty dir, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CobblerReset
// ---------------------------------------------------------------------------

func TestCobblerReset_RemovesDir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, ".cobbler")
	os.MkdirAll(subDir, 0o755)

	if err := CobblerReset(subDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Error("expected directory to be removed")
	}
}

// ---------------------------------------------------------------------------
// EnsureCredentials
// ---------------------------------------------------------------------------

func TestEnsureCredentials_Exists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "token.json"), []byte("{}"), 0o644)

	err := EnsureCredentials(dir, "token.json", func() error {
		t.Error("extractFn should not be called when file exists")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureCredentials_ExtractSucceeds(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")

	err := EnsureCredentials(dir, "token.json", func() error {
		return os.WriteFile(tokenPath, []byte("{}"), 0o644)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureCredentials_ExtractFails(t *testing.T) {
	dir := t.TempDir()
	err := EnsureCredentials(dir, "token.json", func() error {
		return fmt.Errorf("extraction failed")
	})
	if err == nil {
		t.Fatal("expected error when extraction fails and file doesn't exist")
	}
}
