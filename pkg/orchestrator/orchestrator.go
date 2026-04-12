// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	claudesdk "github.com/schlunsen/claude-agent-sdk-go"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/build"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
	rel "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/release"
)

// Orchestrator provides Claude Code orchestration operations.
// Create one with New() and call its methods from mage targets.
type Orchestrator struct {
	cfg        Config
	sdkQueryFn claude.SdkQueryFunc
	tracker    gh.WorkTracker
	git        gitops.GitOps

	// Domain structs — composed in New().
	Builder    *Builder
	Scaffolder *Scaffolder
	Comparer   *Comparer
	VsCode     *VsCode
	Stats      *Stats
	Releaser   *Releaser
	Analyzer    *Analyzer
	ClaudeRunner *ClaudeRunner
	Measure      *Measure
	Stitch       *Stitch
	Generator    *Generator

	// Logging state — previously package-level globals.
	phaseMu           sync.RWMutex
	currentGeneration string
	currentPhase      string
	phaseStart        time.Time
	logSink           io.WriteCloser
	logSinkMu         sync.Mutex
}

// New creates an Orchestrator with the given configuration.
// It applies defaults to any zero-value Config fields.
func New(cfg Config) *Orchestrator {
	cfg.applyDefaults()

	o := &Orchestrator{
		cfg:        cfg,
		sdkQueryFn: claudesdk.Query,
		git:        &gitops.ShellGitOps{},
	}

	o.tracker = gh.NewGitHubTracker(
		gh.Deps{
			Log:          o.logf,
			GhBin:        binGh,
			BranchExists: o.git.BranchExists,
		},
		gh.RepoConfig{
			IssuesRepo: cfg.Cobbler.IssuesRepo,
			ModulePath: cfg.Project.ModulePath,
			TargetRepo: cfg.Project.TargetRepo,
		},
	)

	// Wire instance dependencies into internal packages.
	claude.Log = o.logf
	claude.BinGit = binGit
	claude.BinClaude = binClaude
	generate.Log = o.logf
	generate.BinGit = binGit
	build.Log = o.logf
	build.BinGo = binGo
	build.BinLint = binLint
	build.BinSecurity = binSecurity
	build.BinMage = binMage
	build.BinGit = binGit
	ictx.Log = o.logf
	ictx.LoadAnalysisDocFn = func(dir string) any {
		return an.LoadAnalysisDoc(dir)
	}
	rel.Log = o.logf
	rel.GitReader = o.git
	rel.GitTags = o.git
	rel.GitCommitter = o.git

	// Construct domain structs.
	o.Builder = NewBuilder(cfg)
	o.Scaffolder = NewScaffolder(o.git, o.logf)
	o.Comparer = NewComparer(o.logf, o.git)
	o.VsCode = NewVsCode(o.logf)
	o.Stats = NewStats(cfg, o.logf, o.git, o.tracker)
	o.Releaser = NewReleaser(cfg)
	o.Analyzer = NewAnalyzer(cfg, o.logf)
	o.ClaudeRunner = NewClaudeRunner(
		cfg, o.git, o.tracker, o.sdkQueryFn, o.logf,
		o.Builder.ExtractCredentials,
		o.Stats.CollectStats,
	)
	// Generator is created first (without Measure/Stitch) so that
	// Measure and Stitch constructors can reference its helper methods
	// (resolveBranch, ensureOnBranch, etc.). The domain references
	// are wired afterward.
	o.Generator = NewGenerator(o)
	o.Measure = NewMeasure(o)
	o.Stitch = NewStitch(o)
	o.Generator.measure = o.Measure
	o.Generator.stitch = o.Stitch

	return o
}

// DumpMeasurePrompt assembles and prints the measure prompt to stdout.
func (o *Orchestrator) DumpMeasurePrompt() error {
	prompt, err := o.Measure.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		return fmt.Errorf("building measure prompt: %w", err)
	}
	fmt.Print(prompt)
	return nil
}

// DumpStitchPrompt assembles and prints the stitch prompt to stdout.
// Uses a placeholder task so the template structure is visible.
func (o *Orchestrator) DumpStitchPrompt() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	prompt, err := o.Stitch.buildStitchPrompt(stitchTask{
		WorktreeDir: cwd,
		ID:          "EXAMPLE-001",
		Title:       "Example task",
		Description: "Placeholder task description for prompt preview.",
		IssueType:   "task",
	})
	if err != nil {
		return fmt.Errorf("building stitch prompt: %w", err)
	}
	fmt.Print(prompt)
	return nil
}

// Tracker returns the work tracker interface for issue operations.
func (o *Orchestrator) Tracker() gh.WorkTracker { return o.tracker }

// Git returns the git operations interface.
func (o *Orchestrator) Git() gitops.GitOps { return o.git }

// Config returns a copy of the Orchestrator's configuration.
func (o *Orchestrator) Config() Config { return o.cfg }

// NewFromFile reads configuration from a YAML file at the given path,
// applies defaults, and returns a configured Orchestrator.
func NewFromFile(path string) (*Orchestrator, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("loading config from %s: %w", path, err)
	}
	return New(cfg), nil
}

// setGeneration sets the active generation name for log tagging.
func (o *Orchestrator) setGeneration(name string) {
	o.phaseMu.Lock()
	o.currentGeneration = name
	o.phaseMu.Unlock()
}

// getGeneration returns the current generation name (thread-safe).
func (o *Orchestrator) getGeneration() string {
	o.phaseMu.RLock()
	defer o.phaseMu.RUnlock()
	return o.currentGeneration
}

// clearGeneration removes the generation tag from subsequent log lines.
func (o *Orchestrator) clearGeneration() {
	o.phaseMu.Lock()
	o.currentGeneration = ""
	o.phaseMu.Unlock()
}

// setPhase sets the active workflow phase for log tagging.
func (o *Orchestrator) setPhase(name string) {
	o.phaseMu.Lock()
	o.currentPhase = name
	o.phaseStart = time.Now()
	o.phaseMu.Unlock()
}

// clearPhase removes the phase tag from subsequent log lines.
func (o *Orchestrator) clearPhase() {
	o.phaseMu.Lock()
	o.currentPhase = ""
	o.phaseStart = time.Time{}
	o.phaseMu.Unlock()
}

// openLogSink opens a file at path and sets it as the logf tee destination.
// Subsequent logf calls write to both stderr and this file until closeLogSink
// is called.
func (o *Orchestrator) openLogSink(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("openLogSink: mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("openLogSink: %w", err)
	}
	o.logSinkMu.Lock()
	defer o.logSinkMu.Unlock()
	if o.logSink != nil {
		o.logSink.Close()
	}
	o.logSink = f
	return nil
}

// closeLogSink closes the current log sink and stops tee-ing logf output.
func (o *Orchestrator) closeLogSink() {
	o.logSinkMu.Lock()
	defer o.logSinkMu.Unlock()
	if o.logSink != nil {
		o.logSink.Close()
		o.logSink = nil
	}
}

// logf prints a timestamped log line to stderr. When currentGeneration
// is set, the generation name appears right after the timestamp. When
// currentPhase is set, the phase name and elapsed time since phase start
// are included.
func (o *Orchestrator) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format(time.RFC3339)

	o.phaseMu.RLock()
	gen := o.currentGeneration
	phase := o.currentPhase
	start := o.phaseStart
	o.phaseMu.RUnlock()

	var prefix string
	if gen != "" && phase != "" {
		elapsed := time.Since(start).Round(time.Second)
		prefix = fmt.Sprintf("[%s] [%s] [%s +%s]", ts, gen, phase, elapsed)
	} else if gen != "" {
		prefix = fmt.Sprintf("[%s] [%s]", ts, gen)
	} else if phase != "" {
		elapsed := time.Since(start).Round(time.Second)
		prefix = fmt.Sprintf("[%s] [%s +%s]", ts, phase, elapsed)
	} else {
		prefix = fmt.Sprintf("[%s]", ts)
	}
	line := fmt.Sprintf("%s %s\n", prefix, msg)
	fmt.Fprint(os.Stderr, line)
	o.logSinkMu.Lock()
	if o.logSink != nil {
		o.logSink.Write([]byte(line))
	}
	o.logSinkMu.Unlock()
}

// ValidateTaskWeights validates a proposed task's weight budget against
// MaxWeightPerTask from the config. The input string has the form
// "srd005-wc R2.5, R2.6, R3.1, R3.2". Prints the report to stdout (GH-2078).
func (o *Orchestrator) ValidateTaskWeights(input string) error {
	result := generate.ValidateTaskWeights(o.cfg.Cobbler.Dir, input, o.cfg.Cobbler.MaxWeightPerTask)
	fmt.Println(result)
	return nil
}
