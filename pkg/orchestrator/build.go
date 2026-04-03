// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/build"
)

// Builder provides build, lint, install, and clean operations.
type Builder struct {
	cfg Config
}

// NewBuilder creates a Builder with the given configuration.
func NewBuilder(cfg Config) *Builder {
	return &Builder{cfg: cfg}
}

// NOTE: build.Log and build.Bin* are wired in the Orchestrator
// constructor (New) instead of an init function.

// Build compiles the project binary. If MainPackage is empty, the
// target is skipped.
func (b *Builder) Build() error {
	return build.Build(b.buildConfig())
}

// BuildAll compiles all cmd/ sub-packages to BinaryDir when MainPackage is
// empty. It discovers every cmd/*/main.go package and builds each to
// bin/<name> using go build -o bin/<name> ./cmd/<name>/. If no cmd/
// directory exists the target is skipped. prd003 B1.1.
func (b *Builder) BuildAll() error {
	return build.BuildAll(b.buildConfig())
}

// discoverCmdPackages returns the import paths of all packages under cmd/
// that contain a main.go file, relative to root.
func discoverCmdPackages(root string) ([]string, error) {
	return build.DiscoverCmdPackages(root)
}

// Lint runs golangci-lint on the project.
func (b *Builder) Lint() error {
	return build.Lint()
}

// Install runs go install for the main package. If MainPackage
// is empty, the target is skipped.
func (b *Builder) Install() error {
	return build.Install(b.buildConfig())
}

// Clean removes the build artifact directory.
func (b *Builder) Clean() error {
	return build.Clean(b.cfg.Project.BinaryDir)
}

// ExtractCredentials reads Claude credentials from the macOS Keychain
// and writes them to SecretsDir/TokenFile.
func (b *Builder) ExtractCredentials() error {
	return build.ExtractCredentials(b.cfg.Claude.SecretsDir, b.cfg.EffectiveTokenFile())
}

// buildConfig returns a build.BuildConfig from the builder's config.
func (b *Builder) buildConfig() build.BuildConfig {
	return build.BuildConfig{
		MainPackage: b.cfg.Project.MainPackage,
		BinaryDir:   b.cfg.Project.BinaryDir,
		BinaryName:  b.cfg.Project.BinaryName,
	}
}
