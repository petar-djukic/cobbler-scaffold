// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all orchestrator settings. Consuming repos either
// construct a Config in Go code and pass it to New(), or place a
// configuration.yaml at the repository root and call NewFromFile().
type Config struct {
	// ModulePath is the Go module path (e.g., "github.com/mesh-intelligence/crumbs").
	ModulePath string `yaml:"module_path"`

	// BinaryName is the name of the compiled binary (e.g., "cupboard").
	BinaryName string `yaml:"binary_name"`

	// BinaryDir is the output directory for compiled binaries (default "bin").
	BinaryDir string `yaml:"binary_dir"`

	// MainPackage is the path to the main.go entry point
	// (e.g., "cmd/cupboard/main.go").
	MainPackage string `yaml:"main_package"`

	// GoSourceDirs lists directories containing Go source files
	// (e.g., ["cmd/", "pkg/", "internal/", "tests/"]).
	GoSourceDirs []string `yaml:"go_source_dirs"`

	// VersionFile is the path to the version file
	// (e.g., "pkg/crumbs/version.go").
	VersionFile string `yaml:"version_file"`

	// GenPrefix is the prefix for generation branch names (default "generation-").
	GenPrefix string `yaml:"gen_prefix"`

	// BeadsDir is the beads database directory (default ".beads/").
	BeadsDir string `yaml:"beads_dir"`

	// CobblerDir is the cobbler scratch directory (default ".cobbler/").
	CobblerDir string `yaml:"cobbler_dir"`

	// MagefilesDir is the directory skipped when deleting Go files
	// (default "magefiles").
	MagefilesDir string `yaml:"magefiles_dir"`

	// SecretsDir is the directory containing token files (default ".secrets").
	SecretsDir string `yaml:"secrets_dir"`

	// DefaultTokenFile is the default credential filename (default "claude.json").
	DefaultTokenFile string `yaml:"default_token_file"`

	// SpecGlobs maps a label to a glob pattern for word-count stats.
	SpecGlobs map[string]string `yaml:"spec_globs"`

	// SeedFiles maps relative file paths to template source file paths.
	// During LoadConfig, each source path is read and its content replaces
	// the map value. During generator:start and generator:reset the content
	// strings are executed as Go text/template templates with SeedData.
	SeedFiles map[string]string `yaml:"seed_files"`

	// MeasurePrompt is a file path to a custom measure prompt template.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	MeasurePrompt string `yaml:"measure_prompt"`

	// StitchPrompt is a file path to a custom stitch prompt template.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	StitchPrompt string `yaml:"stitch_prompt"`

	// ClaudeArgs are the CLI arguments for automated Claude execution.
	// If empty, defaults to the standard automated flags.
	ClaudeArgs []string `yaml:"claude_args"`

	// SilenceAgent suppresses Claude stdout when true (default true).
	SilenceAgent *bool `yaml:"silence_agent"`

	// MaxIssues is the maximum number of tasks per measure or stitch phase (default 10).
	MaxIssues int `yaml:"max_issues"`

	// Cycles is the number of measure+stitch cycles per run (default 1).
	Cycles int `yaml:"cycles"`

	// UserPrompt provides additional context for the measure prompt.
	UserPrompt string `yaml:"user_prompt"`

	// GenerationBranch selects a specific generation branch to work on.
	// If empty, the orchestrator auto-detects from existing branches.
	GenerationBranch string `yaml:"generation_branch"`

	// TokenFile overrides the credential filename in SecretsDir.
	// If empty, DefaultTokenFile is used.
	TokenFile string `yaml:"token_file"`
}

// SeedData is the template data passed to SeedFiles templates.
type SeedData struct {
	Version    string
	ModulePath string
}

// Silence returns true when Claude output should be suppressed.
// Handles the nil-pointer case for the default (true).
func (c *Config) Silence() bool {
	if c.SilenceAgent == nil {
		return true
	}
	return *c.SilenceAgent
}

// EffectiveTokenFile returns the token file to use: TokenFile if set,
// otherwise DefaultTokenFile.
func (c *Config) EffectiveTokenFile() string {
	if c.TokenFile != "" {
		return c.TokenFile
	}
	return c.DefaultTokenFile
}

func (c *Config) applyDefaults() {
	if c.BinaryDir == "" {
		c.BinaryDir = "bin"
	}
	if c.GenPrefix == "" {
		c.GenPrefix = "generation-"
	}
	if c.BeadsDir == "" {
		c.BeadsDir = ".beads/"
	}
	if c.CobblerDir == "" {
		c.CobblerDir = ".cobbler/"
	}
	if c.MagefilesDir == "" {
		c.MagefilesDir = "magefiles"
	}
	if c.SecretsDir == "" {
		c.SecretsDir = ".secrets"
	}
	if c.DefaultTokenFile == "" {
		c.DefaultTokenFile = "claude.json"
	}
	if len(c.ClaudeArgs) == 0 {
		c.ClaudeArgs = defaultClaudeArgs
	}
	if c.MaxIssues == 0 {
		c.MaxIssues = 10
	}
	if c.Cycles == 0 {
		c.Cycles = 1
	}
}

// LoadConfig reads a configuration YAML file and returns a Config.
// For SeedFiles entries, the values are treated as file paths: LoadConfig
// reads each file and replaces the map value with its content.
// For MeasurePrompt and StitchPrompt, if non-empty LoadConfig reads
// the referenced file.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file: %w", err)
	}

	// Read seed file templates from disk.
	for dest, src := range cfg.SeedFiles {
		if src == "" {
			continue
		}
		content, err := os.ReadFile(src)
		if err != nil {
			return Config{}, fmt.Errorf("reading seed file %s for %s: %w", src, dest, err)
		}
		cfg.SeedFiles[dest] = string(content)
	}

	// Read prompt template files from disk.
	if cfg.MeasurePrompt != "" {
		content, err := os.ReadFile(cfg.MeasurePrompt)
		if err != nil {
			return Config{}, fmt.Errorf("reading measure prompt %s: %w", cfg.MeasurePrompt, err)
		}
		cfg.MeasurePrompt = string(content)
	}
	if cfg.StitchPrompt != "" {
		content, err := os.ReadFile(cfg.StitchPrompt)
		if err != nil {
			return Config{}, fmt.Errorf("reading stitch prompt %s: %w", cfg.StitchPrompt, err)
		}
		cfg.StitchPrompt = string(content)
	}

	cfg.applyDefaults()
	return cfg, nil
}
