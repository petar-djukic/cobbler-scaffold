// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package claude

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	claudetypes "github.com/schlunsen/claude-agent-sdk-go/types"
)

// ---------------------------------------------------------------------------
// Runner interface
// ---------------------------------------------------------------------------

// Runner executes Claude in a specific mode and returns token usage. The
// three implementations — CLIRunner, PodmanRunner, SDKRunner — encapsulate
// the mode-specific command construction and output handling. Shared
// concerns (credential refresh, timeout, workDir resolution) live in the
// caller (RunClaude).
type Runner interface {
	Run(ctx context.Context, prompt, workDir string, silence bool, extraArgs ...string) (ClaudeResult, error)
}

// NewRunner returns a Runner for the given mode. It reads EffectiveMode
// from deps and constructs the appropriate implementation.
func NewRunner(deps RunClaudeDeps) Runner {
	switch deps.EffectiveMode {
	case "sdk":
		return &SDKRunner{
			queryFn:       deps.SdkQueryFn,
			claudeTimeout: deps.ClaudeTimeout,
		}
	case "cli":
		return &CLIRunner{
			execRunner: execRunner{
				buildCmd: func(ctx context.Context, workDir string, extraArgs ...string) *exec.Cmd {
					return BuildDirectCmd(ctx, workDir, deps.ClaudeArgs, extraArgs...)
				},
				idleTimeoutS: deps.IdleTimeoutS,
			},
		}
	default:
		return &PodmanRunner{
			execRunner: execRunner{
				buildCmd:     deps.BuildPodmanCmdFn,
				idleTimeoutS: deps.IdleTimeoutS,
			},
		}
	}
}

// ---------------------------------------------------------------------------
// execRunner — shared exec-based logic for CLI and Podman modes
// ---------------------------------------------------------------------------

// execRunner holds the shared execution logic for modes that run Claude as
// an external process (CLI and Podman). It manages stdout capture, progress
// logging, idle watchdog, and token parsing.
type execRunner struct {
	buildCmd     func(ctx context.Context, workDir string, extra ...string) *exec.Cmd
	idleTimeoutS int
}

func (r *execRunner) run(ctx context.Context, prompt, workDir string, silence bool, extraArgs ...string) (ClaudeResult, error) {
	// Derive a cancellable child context so the idle watchdog can cancel
	// without masking the parent's DeadlineExceeded.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := r.buildCmd(ctx, workDir, extraArgs...)
	cmd.Stdin = strings.NewReader(prompt)

	var idleAt atomic.Int64
	idleAt.Store(time.Now().UnixNano())

	var stdoutBuf bytes.Buffer
	var outputWriter io.Writer
	var pw *ProgressWriter
	if silence {
		pw = NewProgressWriter(&stdoutBuf, time.Now())
		outputWriter = pw
	} else {
		outputWriter = io.MultiWriter(os.Stdout, &stdoutBuf)
		cmd.Stderr = os.Stderr
	}
	cmd.Stdout = &IdleTrackingWriter{W: outputWriter, LastWrite: &idleAt}

	idleDur := time.Duration(r.idleTimeoutS) * time.Second
	if idleDur > 0 {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					last := time.Unix(0, idleAt.Load())
					if time.Since(last) >= idleDur {
						Log("runClaude: idle watchdog triggered after %s with no output — cancelling session",
							time.Since(last).Round(time.Second))
						cancel()
						return
					}
				}
			}
		}()
	}

	start := time.Now()
	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		elapsed := time.Since(start).Round(time.Second)
		last := time.Unix(0, idleAt.Load())
		idleElapsed := time.Since(last).Round(time.Second)
		if idleDur > 0 && idleElapsed >= idleDur {
			Log("runClaude: idle timeout after %s with no output (session ran %s)", idleElapsed, elapsed)
			return ClaudeResult{}, fmt.Errorf("claude idle timeout: no output for %s", idleElapsed)
		}
		deadline, _ := ctx.Deadline()
		timeout := time.Until(deadline) + elapsed // reconstruct original timeout
		Log("runClaude: killed after %s (max time %s exceeded)", elapsed, timeout.Round(time.Second))
		return ClaudeResult{}, fmt.Errorf("claude max time exceeded (%s)", timeout.Round(time.Second))
	}

	rawOutput := stdoutBuf.Bytes()
	result := ParseClaudeTokens(rawOutput)
	if pw != nil {
		result.NumTurns = pw.Turn
	}
	result.RawOutput = make([]byte, len(rawOutput))
	copy(result.RawOutput, rawOutput)
	Log("runClaude: finished in %s in=%d (cache_create=%d cache_read=%d) out=%d cost=$%.4f (err=%v)",
		time.Since(start).Round(time.Second), result.InputTokens,
		result.CacheCreationTokens, result.CacheReadTokens,
		result.OutputTokens, result.CostUSD, err)
	return result, err
}

// ---------------------------------------------------------------------------
// CLIRunner
// ---------------------------------------------------------------------------

// CLIRunner executes Claude by running the claude binary directly on the
// host. It embeds execRunner for shared process management.
type CLIRunner struct {
	execRunner
}

// Run executes Claude via the host CLI binary.
func (r *CLIRunner) Run(ctx context.Context, prompt, workDir string, silence bool, extraArgs ...string) (ClaudeResult, error) {
	return r.execRunner.run(ctx, prompt, workDir, silence, extraArgs...)
}

// ---------------------------------------------------------------------------
// PodmanRunner
// ---------------------------------------------------------------------------

// PodmanRunner executes Claude inside a podman container. It embeds
// execRunner for shared process management.
type PodmanRunner struct {
	execRunner
}

// Run executes Claude via a podman container.
func (r *PodmanRunner) Run(ctx context.Context, prompt, workDir string, silence bool, extraArgs ...string) (ClaudeResult, error) {
	return r.execRunner.run(ctx, prompt, workDir, silence, extraArgs...)
}

// ---------------------------------------------------------------------------
// SDKRunner
// ---------------------------------------------------------------------------

// SDKRunner executes Claude via the Go Agent SDK. It streams messages from
// the SDK channel, collecting text output and token usage from the
// ResultMessage.
type SDKRunner struct {
	queryFn       SdkQueryFunc
	claudeTimeout time.Duration
}

// Run executes Claude via the Go Agent SDK.
func (r *SDKRunner) Run(ctx context.Context, prompt, workDir string, silence bool, extraArgs ...string) (ClaudeResult, error) {
	opts := claudetypes.NewClaudeAgentOptions()
	opts.CWD = &workDir
	opts.DangerouslySkipPermissions = true
	opts.AllowDangerouslySkipPermissions = true

	opts = opts.WithSystemPromptPreset(claudetypes.SystemPromptPreset{
		Type:   "preset",
		Preset: "claude_code",
	})

	for i := 0; i+1 < len(extraArgs); i++ {
		if extraArgs[i] == "--max-turns" {
			if n, err := strconv.Atoi(extraArgs[i+1]); err == nil {
				opts = opts.WithMaxTurns(n)
			}
			i++
		}
	}

	start := time.Now()

	sdkStderrMu.Lock()
	origStderr := os.Stderr
	pr, pw, pipeErr := os.Pipe()
	if pipeErr == nil {
		os.Stderr = pw
	}
	stderrDone := make(chan struct{})
	if pipeErr == nil {
		go FilterSDKStderr(pr, origStderr, stderrDone)
	}
	Log("runClaude: SDK query workDir=%q (timeout=%s)", workDir, r.claudeTimeout)

	defer func() {
		if pipeErr == nil {
			pw.Close()
			<-stderrDone
			os.Stderr = origStderr
		}
		sdkStderrMu.Unlock()
	}()

	sdkEnvMu.Lock()
	oldVal, hadVal := os.LookupEnv("CLAUDECODE")
	_ = os.Unsetenv("CLAUDECODE")

	msgChan, err := r.queryFn(ctx, prompt, opts)

	if hadVal {
		_ = os.Setenv("CLAUDECODE", oldVal)
	}
	sdkEnvMu.Unlock()

	if err != nil {
		return ClaudeResult{}, fmt.Errorf("claude SDK query: %w", err)
	}

	var result ClaudeResult
	var textBuf strings.Builder
	var gotResult bool

	for msg := range msgChan {
		switch m := msg.(type) {
		case *claudetypes.AssistantMessage:
			for _, block := range m.Content {
				switch b := block.(type) {
				case *claudetypes.TextBlock:
					if !silence {
						fmt.Print(b.Text)
					}
					textBuf.WriteString(b.Text)
				}
			}
		case *claudetypes.ResultMessage:
			gotResult = true
			if m.TotalCostUSD != nil {
				result.CostUSD = *m.TotalCostUSD
			}
			result.InputTokens = IntFromUsage(m.Usage, "input_tokens")
			result.OutputTokens = IntFromUsage(m.Usage, "output_tokens")
			result.CacheCreationTokens = IntFromUsage(m.Usage, "cache_creation_input_tokens")
			result.CacheReadTokens = IntFromUsage(m.Usage, "cache_read_input_tokens")
			result.NumTurns = m.NumTurns
			result.DurationAPIMs = m.DurationAPIMs
			result.SessionID = m.SessionID
			if m.IsError {
				return result, fmt.Errorf("claude SDK session returned error result")
			}
		}
	}

	if !gotResult {
		return ClaudeResult{}, fmt.Errorf("claude SDK session produced no result (subprocess may have exited early)")
	}

	result.RawOutput = []byte(textBuf.String())
	Log("runClaude: SDK finished in %s in=%d (cache_create=%d cache_read=%d) out=%d cost=$%.4f",
		time.Since(start).Round(time.Second),
		result.InputTokens, result.CacheCreationTokens, result.CacheReadTokens,
		result.OutputTokens, result.CostUSD)
	return result, nil
}
