// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package claude

import (
	"context"
	"os/exec"
	"testing"
	"time"

	claudetypes "github.com/schlunsen/claude-agent-sdk-go/types"
)

// ---------------------------------------------------------------------------
// NewRunner factory
// ---------------------------------------------------------------------------

func TestNewRunner_SDK(t *testing.T) {
	deps := RunClaudeDeps{
		EffectiveMode: "sdk",
		ClaudeTimeout: 5 * time.Minute,
		SdkQueryFn: func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error) {
			return nil, nil
		},
	}
	r := NewRunner(deps)
	if _, ok := r.(*SDKRunner); !ok {
		t.Errorf("expected *SDKRunner, got %T", r)
	}
}

func TestNewRunner_CLI(t *testing.T) {
	deps := RunClaudeDeps{
		EffectiveMode: "cli",
		ClaudeArgs:    []string{"--verbose"},
		IdleTimeoutS:  60,
	}
	r := NewRunner(deps)
	if _, ok := r.(*CLIRunner); !ok {
		t.Errorf("expected *CLIRunner, got %T", r)
	}
}

func TestNewRunner_Podman(t *testing.T) {
	deps := RunClaudeDeps{
		EffectiveMode: "podman",
		BuildPodmanCmdFn: func(ctx context.Context, workDir string, extra ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "echo")
		},
		IdleTimeoutS: 120,
	}
	r := NewRunner(deps)
	if _, ok := r.(*PodmanRunner); !ok {
		t.Errorf("expected *PodmanRunner, got %T", r)
	}
}

func TestNewRunner_DefaultIsPodman(t *testing.T) {
	deps := RunClaudeDeps{
		EffectiveMode: "",
		BuildPodmanCmdFn: func(ctx context.Context, workDir string, extra ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "echo")
		},
	}
	r := NewRunner(deps)
	if _, ok := r.(*PodmanRunner); !ok {
		t.Errorf("expected *PodmanRunner for empty mode, got %T", r)
	}
}

// ---------------------------------------------------------------------------
// Runner interface compliance
// ---------------------------------------------------------------------------

func TestCLIRunner_ImplementsRunner(t *testing.T) {
	var _ Runner = (*CLIRunner)(nil)
}

func TestPodmanRunner_ImplementsRunner(t *testing.T) {
	var _ Runner = (*PodmanRunner)(nil)
}

func TestSDKRunner_ImplementsRunner(t *testing.T) {
	var _ Runner = (*SDKRunner)(nil)
}

// ---------------------------------------------------------------------------
// SDKRunner.Run
// ---------------------------------------------------------------------------

func TestSDKRunner_Run_Success(t *testing.T) {
	cost := 0.0042
	r := &SDKRunner{
		claudeTimeout: 5 * time.Minute,
		queryFn: func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error) {
			ch := make(chan claudetypes.Message, 2)
			ch <- &claudetypes.AssistantMessage{
				Content: []claudetypes.ContentBlock{
					&claudetypes.TextBlock{Text: "hello"},
				},
			}
			ch <- &claudetypes.ResultMessage{
				TotalCostUSD: &cost,
				Usage:        map[string]interface{}{"input_tokens": float64(10), "output_tokens": float64(20)},
			}
			close(ch)
			return ch, nil
		},
	}

	res, err := r.Run(context.Background(), "prompt", t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.InputTokens != 10 {
		t.Errorf("expected InputTokens=10, got %d", res.InputTokens)
	}
	if res.OutputTokens != 20 {
		t.Errorf("expected OutputTokens=20, got %d", res.OutputTokens)
	}
	if res.CostUSD != 0.0042 {
		t.Errorf("expected CostUSD=0.0042, got %f", res.CostUSD)
	}
	if string(res.RawOutput) != "hello" {
		t.Errorf("expected RawOutput='hello', got %q", string(res.RawOutput))
	}
}

func TestSDKRunner_Run_QueryError(t *testing.T) {
	r := &SDKRunner{
		claudeTimeout: 5 * time.Minute,
		queryFn: func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error) {
			return nil, context.DeadlineExceeded
		},
	}

	_, err := r.Run(context.Background(), "prompt", t.TempDir(), true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSDKRunner_Run_NoResult(t *testing.T) {
	r := &SDKRunner{
		claudeTimeout: 5 * time.Minute,
		queryFn: func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error) {
			ch := make(chan claudetypes.Message)
			close(ch)
			return ch, nil
		},
	}

	_, err := r.Run(context.Background(), "prompt", t.TempDir(), true)
	if err == nil {
		t.Fatal("expected error for no result")
	}
}

func TestSDKRunner_Run_IsError(t *testing.T) {
	r := &SDKRunner{
		claudeTimeout: 5 * time.Minute,
		queryFn: func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error) {
			ch := make(chan claudetypes.Message, 1)
			ch <- &claudetypes.ResultMessage{IsError: true}
			close(ch)
			return ch, nil
		},
	}

	_, err := r.Run(context.Background(), "prompt", t.TempDir(), true)
	if err == nil {
		t.Fatal("expected error for IsError result")
	}
}

func TestSDKRunner_Run_MaxTurnsParsed(t *testing.T) {
	var capturedOpts *claudetypes.ClaudeAgentOptions
	cost := 0.0
	r := &SDKRunner{
		claudeTimeout: 5 * time.Minute,
		queryFn: func(ctx context.Context, prompt string, opts *claudetypes.ClaudeAgentOptions) (<-chan claudetypes.Message, error) {
			capturedOpts = opts
			ch := make(chan claudetypes.Message, 1)
			ch <- &claudetypes.ResultMessage{TotalCostUSD: &cost, Usage: map[string]interface{}{"input_tokens": float64(1), "output_tokens": float64(1)}}
			close(ch)
			return ch, nil
		},
	}

	_, err := r.Run(context.Background(), "prompt", t.TempDir(), true, "--max-turns", "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOpts == nil {
		t.Fatal("opts not captured")
	}
	if capturedOpts.MaxTurns == nil || *capturedOpts.MaxTurns != 7 {
		t.Errorf("expected MaxTurns=7, got %v", capturedOpts.MaxTurns)
	}
}
