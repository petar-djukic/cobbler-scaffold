// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
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

// logf prints a timestamped log line to stderr. When currentGeneration
// is set, the generation name appears right after the timestamp.
func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format(time.RFC3339)
	if currentGeneration != "" {
		fmt.Fprintf(os.Stderr, "[%s] [%s] %s\n", ts, currentGeneration, msg)
	} else {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", ts, msg)
	}
}
