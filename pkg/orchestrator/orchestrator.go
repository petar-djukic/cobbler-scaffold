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
)

// Orchestrator provides Claude Code orchestration operations.
// Create one with New() and call its methods from mage targets.
type Orchestrator struct {
	cfg Config
}

// New creates an Orchestrator with the given configuration.
// It applies defaults to any zero-value Config fields.
func New(cfg Config) *Orchestrator {
	cfg.applyDefaults()
	return &Orchestrator{cfg: cfg}
}

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

// currentGeneration holds the active generation name. When set, logf
// includes it right after the timestamp so every log line within a
// generation is tagged automatically.
var currentGeneration string

// setGeneration sets the active generation name for log tagging.
func setGeneration(name string) { currentGeneration = name }

// clearGeneration removes the generation tag from subsequent log lines.
func clearGeneration() { currentGeneration = "" }

// currentPhase holds the active workflow phase (e.g. "measure", "stitch").
// When set, logf includes it and the elapsed time since the phase started.
var currentPhase string
var phaseStart time.Time

// setPhase sets the active workflow phase for log tagging.
func setPhase(name string) {
	currentPhase = name
	phaseStart = time.Now()
}

// clearPhase removes the phase tag from subsequent log lines.
func clearPhase() {
	currentPhase = ""
	phaseStart = time.Time{}
}

// logSink is an optional secondary destination for logf output.
// When non-nil, every logf line is written to both stderr and logSink.
var (
	logSink   io.WriteCloser
	logSinkMu sync.Mutex
)

// openLogSink opens a file at path and sets it as the logf tee destination.
// Subsequent logf calls write to both stderr and this file until closeLogSink
// is called.
func openLogSink(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("openLogSink: mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("openLogSink: %w", err)
	}
	logSinkMu.Lock()
	defer logSinkMu.Unlock()
	if logSink != nil {
		logSink.Close()
	}
	logSink = f
	return nil
}

// closeLogSink closes the current log sink and stops tee-ing logf output.
func closeLogSink() {
	logSinkMu.Lock()
	defer logSinkMu.Unlock()
	if logSink != nil {
		logSink.Close()
		logSink = nil
	}
}

// logf prints a timestamped log line to stderr. When currentGeneration
// is set, the generation name appears right after the timestamp. When
// currentPhase is set, the phase name and elapsed time since phase start
// are included.
func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format(time.RFC3339)

	var prefix string
	if currentGeneration != "" && currentPhase != "" {
		elapsed := time.Since(phaseStart).Round(time.Second)
		prefix = fmt.Sprintf("[%s] [%s] [%s +%s]", ts, currentGeneration, currentPhase, elapsed)
	} else if currentGeneration != "" {
		prefix = fmt.Sprintf("[%s] [%s]", ts, currentGeneration)
	} else if currentPhase != "" {
		elapsed := time.Since(phaseStart).Round(time.Second)
		prefix = fmt.Sprintf("[%s] [%s +%s]", ts, currentPhase, elapsed)
	} else {
		prefix = fmt.Sprintf("[%s]", ts)
	}
	line := fmt.Sprintf("%s %s\n", prefix, msg)
	fmt.Fprint(os.Stderr, line)
	logSinkMu.Lock()
	if logSink != nil {
		logSink.Write([]byte(line))
	}
	logSinkMu.Unlock()
}
