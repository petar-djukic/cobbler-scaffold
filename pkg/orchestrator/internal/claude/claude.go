// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package claude implements Claude invocation, token parsing, history
// persistence, and progress reporting. The parent orchestrator package
// provides thin receiver-method wrappers around these functions.
package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
	claudetypes "github.com/schlunsen/claude-agent-sdk-go/types"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Injected dependencies
// ---------------------------------------------------------------------------

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// SdkQueryFunc is the function signature for claudesdk.Query.
type SdkQueryFunc func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error)

// Package-level variables set by the parent package at init time.
var (
	Log Logger = func(string, ...any) {}

	BinGit    string
	BinClaude string
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ClaudeResult holds token usage from a Claude invocation.
// InputTokens is the total input (non-cached + cache creation + cache read).
// CacheCreationTokens and CacheReadTokens break down how the input was served.
// RawOutput contains the full stream-json output from Claude for history.
// NumTurns is populated from ProgressWriter in CLI mode and from the
// SDK ResultMessage in SDK mode. DurationAPIMs and SessionID are SDK-only.
type ClaudeResult struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	CostUSD             float64
	NumTurns            int    // CLI: from ProgressWriter; SDK: from ResultMessage
	DurationAPIMs       int    // SDK mode only; API-side latency in milliseconds
	SessionID           string // SDK mode only; Claude session identifier
	RawOutput           []byte
}

// LocSnapshot holds a point-in-time LOC count.
type LocSnapshot struct {
	Production int `json:"production"`
	Test       int `json:"test"`
}

// InvocationRecord is the JSON blob recorded as a GitHub issue comment after
// every Claude invocation.
type InvocationRecord struct {
	Caller    string       `json:"caller"`
	StartedAt string       `json:"started_at"`
	DurationS int          `json:"duration_s"`
	Tokens    ClaudeTokens `json:"tokens"`
	LOCBefore LocSnapshot  `json:"loc_before"`
	LOCAfter  LocSnapshot  `json:"loc_after"`
	Diff      DiffRecord   `json:"diff"`
	NumTurns  int          `json:"num_turns,omitempty"`
}

// ClaudeTokens holds token counts for an invocation record.
type ClaudeTokens struct {
	Input         int     `json:"input"`
	Output        int     `json:"output"`
	CacheCreation int     `json:"cache_creation"`
	CacheRead     int     `json:"cache_read"`
	CostUSD       float64 `json:"cost_usd"`
}

// DiffRecord holds file-level diff statistics.
type DiffRecord struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

// HistoryStats is the YAML-serializable stats file saved alongside prompt
// and log artifacts in the history directory.
type HistoryStats struct {
	Caller        string       `yaml:"caller"`
	Generation    string       `yaml:"generation,omitempty"`
	TaskID        string       `yaml:"task_id,omitempty"`
	TaskTitle     string       `yaml:"task_title,omitempty"`
	Status        string       `yaml:"status,omitempty"`
	Error         string       `yaml:"error,omitempty"`
	StartedAt     string       `yaml:"started_at"`
	Duration      string       `yaml:"duration"`
	DurationS     int          `yaml:"duration_s"`
	Tokens        HistoryTokens `yaml:"tokens"`
	CostUSD       float64      `yaml:"cost_usd"`
	NumTurns      int          `yaml:"num_turns,omitempty"`
	DurationAPIMs int          `yaml:"duration_api_ms,omitempty"`
	SessionID     string       `yaml:"session_id,omitempty"`
	LOCBefore     LocSnapshot  `yaml:"loc_before"`
	LOCAfter      LocSnapshot  `yaml:"loc_after"`
	Diff          HistoryDiff  `yaml:"diff"`
}

// HistoryTokens holds token counts in the history stats YAML.
type HistoryTokens struct {
	Input         int `yaml:"input"`
	Output        int `yaml:"output"`
	CacheCreation int `yaml:"cache_creation"`
	CacheRead     int `yaml:"cache_read"`
}

// HistoryDiff holds diff statistics in the history stats YAML.
type HistoryDiff struct {
	Files      int `yaml:"files"`
	Insertions int `yaml:"insertions"`
	Deletions  int `yaml:"deletions"`
}

// StitchReport is the YAML-serializable report file saved alongside stats
// and log artifacts after a successful stitch. It includes per-file diffstat
// so that downstream consumers can see exactly what changed.
type StitchReport struct {
	TaskID    string      `yaml:"task_id"`
	TaskTitle string      `yaml:"task_title"`
	Status    string      `yaml:"status"`
	Branch    string      `yaml:"branch"`
	Diff      HistoryDiff `yaml:"diff"`
	Files     []FileChange `yaml:"files"`
	LOCBefore LocSnapshot `yaml:"loc_before"`
	LOCAfter  LocSnapshot `yaml:"loc_after"`
}

// FileChange holds per-file diff information. This is a local copy to avoid
// circular imports with the parent package.
type FileChange struct {
	Path       string `yaml:"path"`
	Status     string `yaml:"status"`
	Insertions int    `yaml:"insertions"`
	Deletions  int    `yaml:"deletions"`
}

// ---------------------------------------------------------------------------
// SDK synchronisation mutexes
// ---------------------------------------------------------------------------

// sdkEnvMu serialises temporary process-env mutations in RunClaudeSDK.
var sdkEnvMu sync.Mutex

// sdkStderrMu serialises os.Stderr replacement in RunClaudeSDK.
var sdkStderrMu sync.Mutex

// ---------------------------------------------------------------------------
// RunClaudeDeps carries the orchestrator fields needed by RunClaude.
// ---------------------------------------------------------------------------

// RunClaudeDeps provides all dependencies that RunClaude and RunClaudeSDK
// need from the parent Orchestrator.
type RunClaudeDeps struct {
	EffectiveMode  string
	ClaudeTimeout  time.Duration
	Temperature    float64
	Silence        bool
	ClaudeArgs     []string
	SecretsDir     string
	TokenFile      string
	IdleTimeoutS   int
	SdkQueryFn     SdkQueryFunc

	ExtractCredentialsFn func() error
}

// ---------------------------------------------------------------------------
// Pure helper functions
// ---------------------------------------------------------------------------

// FilterSDKStderr reads lines from r and forwards them to dst, except that
// lines matching the SDK's rate-limit parse warning are replaced with a
// structured log entry. It closes r and signals done when r reaches EOF.
func FilterSDKStderr(r *os.File, dst *os.File, done chan<- struct{}) {
	defer func() {
		_ = r.Close()
		close(done)
	}()
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "Failed to parse message from CLI") &&
			strings.Contains(line, "rate_limit_event") {
			ts := time.Now().Format(time.RFC3339)
			fmt.Fprintf(dst, "[%s] claude: rate_limit\n", ts)
			continue
		}
		fmt.Fprintln(dst, line)
	}
}

// ParseClaudeTokens extracts token usage from Claude's stream-json output.
func ParseClaudeTokens(output []byte) ClaudeResult {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(lines[i], &raw); err != nil {
			continue
		}
		typeField, ok := raw["type"]
		if !ok {
			continue
		}
		var eventType string
		if json.Unmarshal(typeField, &eventType) != nil || eventType != "result" {
			continue
		}

		var result struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
			Usage        struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(lines[i], &result); err != nil {
			Log("parseClaudeTokens: unmarshal error: %v", err)
			return ClaudeResult{}
		}

		u := result.Usage
		totalInput := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens

		Log("parseClaudeTokens: in=%d (base=%d cache_create=%d cache_read=%d) out=%d cost=$%.4f",
			totalInput, u.InputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens,
			u.OutputTokens, result.TotalCostUSD)

		return ClaudeResult{
			InputTokens:         totalInput,
			OutputTokens:        u.OutputTokens,
			CacheCreationTokens: u.CacheCreationInputTokens,
			CacheReadTokens:     u.CacheReadInputTokens,
			CostUSD:             result.TotalCostUSD,
		}
	}
	return ClaudeResult{}
}

// ExtractTextFromStreamJSON concatenates all text blocks from assistant
// messages in Claude's stream-json output. If no line parses as JSON at all
// (e.g. SDK mode stores plain text in RawOutput) the raw bytes are returned
// unchanged so downstream YAML extraction still works.
func ExtractTextFromStreamJSON(rawOutput []byte) string {
	var sb strings.Builder
	anyJSON := false
	for _, line := range bytes.Split(rawOutput, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var msg struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &msg) != nil {
			continue
		}
		anyJSON = true
		if msg.Type != "assistant" {
			continue
		}
		for _, block := range msg.Message.Content {
			if block.Type == "text" {
				sb.WriteString(block.Text)
			}
		}
	}
	if !anyJSON {
		return string(rawOutput)
	}
	return sb.String()
}

// ExtractYAMLBlock finds the first ```yaml fenced code block in text
// and returns its content. Returns an error if no YAML block is found.
func ExtractYAMLBlock(text string) ([]byte, error) {
	markers := []string{"```yaml\n", "```yml\n", "```yaml\r\n", "```yml\r\n"}
	start := -1
	markerLen := 0
	for _, m := range markers {
		idx := strings.Index(text, m)
		if idx >= 0 && (start < 0 || idx < start) {
			start = idx
			markerLen = len(m)
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("no ```yaml fenced code block found in Claude output")
	}

	content := text[start+markerLen:]
	end := strings.Index(content, "\n```")
	if end < 0 {
		end = strings.Index(content, "```")
	}
	if end < 0 {
		return nil, fmt.Errorf("unclosed ```yaml fenced code block")
	}

	return []byte(strings.TrimSpace(content[:end])), nil
}

// ToolSummary extracts a concise context string from tool input JSON
// (file_path, command, pattern, etc.).
func ToolSummary(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(input, &fields) != nil {
		return ""
	}
	for _, key := range []string{"file_path", "path", "pattern", "command"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var val string
		if json.Unmarshal(raw, &val) != nil {
			continue
		}
		if key == "command" && len(val) > 80 {
			val = val[:80] + "..."
		}
		return val
	}
	return ""
}

// IntFromUsage extracts an integer from the ResultMessage usage map.
func IntFromUsage(usage map[string]interface{}, key string) int {
	if usage == nil {
		return 0
	}
	v, ok := usage[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// FormatOutcomeTrailers returns the set of git trailer strings for rec.
func FormatOutcomeTrailers(rec InvocationRecord) []string {
	return []string{
		fmt.Sprintf("Tokens-Input: %d", rec.Tokens.Input),
		fmt.Sprintf("Tokens-Output: %d", rec.Tokens.Output),
		fmt.Sprintf("Tokens-Cache-Creation: %d", rec.Tokens.CacheCreation),
		fmt.Sprintf("Tokens-Cache-Read: %d", rec.Tokens.CacheRead),
		fmt.Sprintf("Tokens-Cost-USD: %.4f", rec.Tokens.CostUSD),
		fmt.Sprintf("Loc-Prod-Before: %d", rec.LOCBefore.Production),
		fmt.Sprintf("Loc-Prod-After: %d", rec.LOCAfter.Production),
		fmt.Sprintf("Loc-Test-Before: %d", rec.LOCBefore.Test),
		fmt.Sprintf("Loc-Test-After: %d", rec.LOCAfter.Test),
		fmt.Sprintf("Duration-Seconds: %d", rec.DurationS),
	}
}

// AppendOutcomeTrailers amends the last commit in the given git worktree
// directory with outcome trailers from rec.
func AppendOutcomeTrailers(worktreeDir string, rec InvocationRecord) error {
	args := []string{"-C", worktreeDir, "commit", "--amend", "--no-edit"}
	for _, t := range FormatOutcomeTrailers(rec) {
		args = append(args, "--trailer", t)
	}
	cmd := exec.Command(BinGit, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit --amend: %w\n%s", err, out)
	}
	return nil
}

// WorktreeBasePath returns the directory used for stitch worktrees.
func WorktreeBasePath() string {
	out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
	if err == nil {
		gitDir := filepath.Clean(strings.TrimSpace(string(out)))
		if !filepath.IsAbs(gitDir) {
			cwd, _ := os.Getwd()
			gitDir = filepath.Join(cwd, gitDir)
		}
		repoRoot := filepath.Dir(gitDir)
		return filepath.Join(os.TempDir(), filepath.Base(repoRoot)+"-worktrees")
	}
	repoRoot, _ := os.Getwd()
	return filepath.Join(os.TempDir(), filepath.Base(repoRoot)+"-worktrees")
}

// ---------------------------------------------------------------------------
// History persistence
// ---------------------------------------------------------------------------

// HistoryDir returns the resolved history directory path.
func HistoryDir(cobblerDir, historyDir string) string {
	if historyDir == "" || filepath.IsAbs(historyDir) {
		return historyDir
	}
	return filepath.Join(cobblerDir, historyDir)
}

// SaveHistoryReport writes a stitch report YAML file to the history directory.
func SaveHistoryReport(dir, ts string, report StitchReport) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Log("saveHistoryReport: mkdir %s: %v", dir, err)
		return
	}

	data, err := yaml.Marshal(&report)
	if err != nil {
		Log("saveHistoryReport: marshal: %v", err)
		return
	}

	path := filepath.Join(dir, ts+"-stitch-report.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		Log("saveHistoryReport: write %s: %v", path, err)
		return
	}
	Log("saveHistoryReport: saved %s", path)
}

// SaveHistoryStats writes a stats YAML file to the history directory.
func SaveHistoryStats(dir, ts, phase string, stats HistoryStats) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Log("saveHistoryStats: mkdir %s: %v", dir, err)
		return
	}

	data, err := yaml.Marshal(&stats)
	if err != nil {
		Log("saveHistoryStats: marshal: %v", err)
		return
	}

	path := filepath.Join(dir, ts+"-"+phase+"-stats.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		Log("saveHistoryStats: write %s: %v", path, err)
		return
	}
	Log("saveHistoryStats: saved %s", path)
}

// SaveHistoryPrompt writes the prompt to the history directory.
func SaveHistoryPrompt(dir, ts, phase, prompt string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Log("saveHistoryPrompt: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, ts+"-"+phase+"-prompt.yaml")
	if err := os.WriteFile(path, []byte(prompt), 0o644); err != nil {
		Log("saveHistoryPrompt: write: %v", err)
	} else {
		Log("saveHistoryPrompt: saved %s", path)
	}
}

// SaveHistoryLog writes the raw Claude output to the history directory.
func SaveHistoryLog(dir, ts, phase string, rawOutput []byte) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		Log("saveHistoryLog: mkdir %s: %v", dir, err)
		return
	}
	path := filepath.Join(dir, ts+"-"+phase+"-log.log")
	if err := os.WriteFile(path, rawOutput, 0o644); err != nil {
		Log("saveHistoryLog: write: %v", err)
	} else {
		Log("saveHistoryLog: saved %s", path)
	}
}

// ---------------------------------------------------------------------------
// ProgressWriter
// ---------------------------------------------------------------------------

// ProgressWriter wraps a bytes.Buffer, logging concise one-line summaries
// of Claude stream-json events via Log(). All bytes pass through unchanged.
type ProgressWriter struct {
	Buf       *bytes.Buffer
	Start     time.Time
	LastEvent time.Time
	Partial   []byte
	Turn      int
	GotFirst  bool
}

// NewProgressWriter creates a ProgressWriter writing to dst.
func NewProgressWriter(dst *bytes.Buffer, start time.Time) *ProgressWriter {
	return &ProgressWriter{Buf: dst, Start: start, LastEvent: start}
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	if !pw.GotFirst {
		pw.GotFirst = true
		Log("claude: [%s] first output", time.Since(pw.Start).Round(time.Second))
	}
	n, err := pw.Buf.Write(p)
	if err != nil {
		return n, err
	}
	pw.Partial = append(pw.Partial, p...)
	for {
		idx := bytes.IndexByte(pw.Partial, '\n')
		if idx < 0 {
			break
		}
		pw.LogLine(pw.Partial[:idx])
		pw.Partial = pw.Partial[idx+1:]
	}
	return n, nil
}

// LogLine parses a single JSON line and logs assistant turns, tool calls,
// and the final result event.
func (pw *ProgressWriter) LogLine(line []byte) {
	if len(line) == 0 {
		return
	}
	var msg struct {
		Type    string `json:"type"`
		Message struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return
	}
	now := time.Now()
	step := now.Sub(pw.LastEvent).Round(time.Second)
	total := now.Sub(pw.Start).Round(time.Second)
	pw.LastEvent = now

	switch msg.Type {
	case "assistant":
		pw.Turn++
		snippet := ""
		for _, b := range msg.Message.Content {
			if b.Type == "text" && b.Text != "" {
				snippet = b.Text
				if len(snippet) > 120 {
					snippet = snippet[:120] + "..."
				}
				snippet = strings.ReplaceAll(snippet, "\n", " ")
				break
			}
		}
		if snippet != "" {
			Log("claude: [%s +%s] turn %d: %s", total, step, pw.Turn, snippet)
		} else {
			Log("claude: [%s +%s] turn %d", total, step, pw.Turn)
		}
		for _, b := range msg.Message.Content {
			if b.Type == "tool_use" {
				Log("claude: [%s] turn %d: tool %s %s", total, pw.Turn, b.Name, ToolSummary(b.Input))
			}
		}
	case "user":
		Log("claude: [%s +%s] tools done, waiting for LLM", total, step)
	case "rate_limit_event":
		Log("claude: [%s] rate_limit", total)
	case "system":
		Log("claude: [%s] ready", total)
	case "result":
		u := msg.Usage
		totalIn := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
		Log("claude: [%s] done: %d turn(s), in=%d (base=%d cache_create=%d cache_read=%d) out=%d cost=$%.4f",
			total, pw.Turn, totalIn, u.InputTokens, u.CacheCreationInputTokens,
			u.CacheReadInputTokens, u.OutputTokens, msg.TotalCostUSD)
	}
}

// ---------------------------------------------------------------------------
// IdleTrackingWriter
// ---------------------------------------------------------------------------

// IdleTrackingWriter wraps an io.Writer and records the last write timestamp
// in LastWrite (nanoseconds) so the idle watchdog can detect stalled sessions.
type IdleTrackingWriter struct {
	W         io.Writer
	LastWrite *atomic.Int64
}

func (t *IdleTrackingWriter) Write(p []byte) (int, error) {
	t.LastWrite.Store(time.Now().UnixNano())
	return t.W.Write(p)
}

// ---------------------------------------------------------------------------
// RunClaude
// ---------------------------------------------------------------------------

// RunClaude executes Claude and returns token usage. It performs shared
// pre-flight (logging, credential refresh, workDir resolution, timeout
// context) and delegates to a mode-specific Runner created via NewRunner.
func RunClaude(deps RunClaudeDeps, prompt, dir string, silence bool, extraClaudeArgs ...string) (ClaudeResult, error) {
	Log("runClaude: promptLen=%d dir=%q silence=%v", len(prompt), dir, silence)

	if deps.Temperature != 0 {
		Log("runClaude: warning: temperature=%.2f configured but Claude CLI does not support --temperature; parameter ignored", deps.Temperature)
	}

	// Refresh credentials before each invocation.
	if deps.ExtractCredentialsFn != nil {
		if err := deps.ExtractCredentialsFn(); err != nil {
			Log("runClaude: credential refresh warning: %v", err)
		}
	}

	workDir := dir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return ClaudeResult{}, fmt.Errorf("getting working directory: %w", err)
		}
	}

	timeout := deps.ClaudeTimeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	runner := NewRunner(deps)
	return runner.Run(ctx, prompt, workDir, silence, extraClaudeArgs...)
}

// BuildDirectCmd constructs the exec.Cmd for running the claude binary
// directly on the host. CLAUDECODE is stripped from the environment.
func BuildDirectCmd(ctx context.Context, workDir string, claudeArgs []string, extraClaudeArgs ...string) *exec.Cmd {
	args := append([]string{}, claudeArgs...)
	args = append(args, extraClaudeArgs...)
	deadline, _ := ctx.Deadline()
	Log("runClaude: exec %s %v (mode=cli timeout=%s)", BinClaude, args, deadline)
	cmd := exec.CommandContext(ctx, BinClaude, args...)
	cmd.Dir = workDir
	filtered := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered
	return cmd
}

// RunClaudeSDK delegates to SDKRunner.Run. It is kept for backward
// compatibility with callers that construct a RunClaudeDeps directly.
func RunClaudeSDK(deps RunClaudeDeps, ctx context.Context, prompt, workDir string, silence bool, extraClaudeArgs ...string) (ClaudeResult, error) {
	r := &SDKRunner{queryFn: deps.SdkQueryFn, claudeTimeout: deps.ClaudeTimeout}
	return r.Run(ctx, prompt, workDir, silence, extraClaudeArgs...)
}

// ---------------------------------------------------------------------------
// Pre-flight checks
// ---------------------------------------------------------------------------

// CheckClaudeDeps provides dependencies for CheckClaude.
type CheckClaudeDeps struct {
	EffectiveMode      string
	EnsureCredentialsFn func() error
}

// CheckClaude verifies that Claude can be invoked.
func CheckClaude(deps CheckClaudeDeps) error {
	if _, err := exec.LookPath(BinClaude); err != nil {
		return fmt.Errorf("claude not found on PATH; install the Claude CLI")
	}
	return deps.EnsureCredentialsFn()
}

// EnsureCredentials checks that the credential file exists in secretsDir.
func EnsureCredentials(secretsDir, tokenFile string, extractFn func() error) error {
	credPath := filepath.Join(secretsDir, tokenFile)
	if _, err := os.Stat(credPath); err == nil {
		return nil
	}

	Log("ensureCredentials: %s not found, attempting keychain extraction", credPath)
	if err := extractFn(); err != nil {
		Log("ensureCredentials: keychain extraction failed: %v", err)
	}

	if _, err := os.Stat(credPath); err != nil {
		return fmt.Errorf("claude credentials not found at %s; "+
			"run 'mage credentials' on the host or place a valid credential file at %s",
			credPath, credPath)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Config logging
// ---------------------------------------------------------------------------

// LogConfigDeps holds fields needed by LogConfig.
type LogConfigDeps struct {
	Silence               bool
	MaxStitchIssues       int
	MaxStitchIssuesPerCycle int
	MaxMeasureIssues      int
	GenerationBranch      string
	UserPrompt            string
}

// LogConfig prints the resolved configuration for debugging.
func LogConfig(target string, deps LogConfigDeps) {
	Log("%s config: silence=%v stitchTotal=%d stitchPerCycle=%d measure=%d generationBranch=%q",
		target, deps.Silence, deps.MaxStitchIssues, deps.MaxStitchIssuesPerCycle, deps.MaxMeasureIssues, deps.GenerationBranch)
	if deps.UserPrompt != "" {
		Log("%s config: userPrompt=%q", target, deps.UserPrompt)
	}
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// HistoryClean removes the history subdirectory.
func HistoryClean(hdir string) error {
	if hdir == "" {
		return nil
	}
	Log("historyClean: removing %s", hdir)
	if err := os.RemoveAll(hdir); err != nil {
		return fmt.Errorf("removing history dir %s: %w", hdir, err)
	}
	Log("historyClean: done")
	return nil
}

// CobblerReset removes the cobbler scratch directory.
func CobblerReset(dir string) error {
	Log("cobblerReset: removing %s", dir)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing %s: %w", dir, err)
	}
	Log("cobblerReset: done")
	return nil
}

// ---------------------------------------------------------------------------
// LOC capture
// ---------------------------------------------------------------------------

// CaptureLOCFn is the function signature for LOC capture.
type CaptureLOCFn func() LocSnapshot

// CaptureLOCAt returns Go LOC counts measured in dir. It temporarily changes
// the working directory so the capture function walks the correct tree.
func CaptureLOCAt(dir string, captureFn CaptureLOCFn) LocSnapshot {
	if dir == "" {
		return captureFn()
	}
	orig, err := os.Getwd()
	if err != nil {
		Log("captureLOCAt: getwd: %v", err)
		return LocSnapshot{}
	}
	if err := os.Chdir(dir); err != nil {
		Log("captureLOCAt: chdir to %s: %v", dir, err)
		return LocSnapshot{}
	}
	defer func() { os.Chdir(orig) }() //nolint:errcheck
	return captureFn()
}

// HasOpenIssues returns true if there are open orchestrator issues.
type HasOpenIssuesDeps struct {
	DetectGitHubRepoFn      func(repoRoot string) (string, error)
	GitReader               gitops.RepoReader
	ListOpenCobblerIssuesFn func(repo, branch string) (int, error)
}

// HasOpenIssues returns true if there are open orchestrator issues.
func HasOpenIssues(deps HasOpenIssuesDeps) (bool, error) {
	repoRoot, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("getwd: %w", err)
	}
	ghRepo, err := deps.DetectGitHubRepoFn(repoRoot)
	if err != nil {
		return false, fmt.Errorf("detectGitHubRepo: %w", err)
	}
	branch, err := deps.GitReader.CurrentBranch(".")
	if err != nil {
		return false, fmt.Errorf("gitCurrentBranch: %w", err)
	}
	count, err := deps.ListOpenCobblerIssuesFn(ghRepo, branch)
	if err != nil {
		return false, fmt.Errorf("listOpenCobblerIssues: %w", err)
	}
	return count > 0, nil
}
