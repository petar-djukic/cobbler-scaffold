// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectConfig holds settings that describe the consuming project.
type ProjectConfig struct {
	// ModulePath is the Go module path (e.g., "github.com/org/project").
	ModulePath string `yaml:"module_path"`

	// BinaryName is the name of the compiled binary.
	BinaryName string `yaml:"binary_name"`

	// BinaryDir is the output directory for compiled binaries (default "bin").
	BinaryDir string `yaml:"binary_dir"`

	// MainPackage is the path to the main.go entry point.
	MainPackage string `yaml:"main_package"`

	// GoSourceDirs lists directories containing Go source files
	// (e.g., ["cmd/", "pkg/", "internal/", "tests/"]).
	GoSourceDirs []string `yaml:"go_source_dirs"`

	// VersionFile is the path to the version file.
	VersionFile string `yaml:"version_file"`

	// MagefilesDir is the directory skipped when deleting Go files
	// (default "magefiles").
	MagefilesDir string `yaml:"magefiles_dir"`

	// SpecGlobs maps a label to a glob pattern for word-count stats.
	SpecGlobs map[string]string `yaml:"spec_globs"`

	// SeedFiles maps relative file paths to template source file paths.
	// During LoadConfig, each source path is read and its content replaces
	// the map value. During generator:start and generator:reset the content
	// strings are executed as Go text/template templates with SeedData.
	SeedFiles map[string]string `yaml:"seed_files"`
}

// GenerationConfig holds settings for the generation lifecycle.
type GenerationConfig struct {
	// GenPrefix is the prefix for generation branch names (default "generation-").
	GenPrefix string `yaml:"gen_prefix"`

	// Cycles is the maximum number of measure+stitch cycles per run
	// (default 0, meaning run until all issues are closed).
	Cycles int `yaml:"cycles"`

	// GenerationBranch selects a specific generation branch to work on.
	// If empty, the orchestrator auto-detects from existing branches.
	GenerationBranch string `yaml:"generation_branch"`

	// CleanupDirs lists directories to remove after generation stop or reset.
	// Empty by default.
	CleanupDirs []string `yaml:"cleanup_dirs"`
}

// CobblerConfig holds settings for the measure and stitch workflows.
type CobblerConfig struct {
	// CobblerDir is the cobbler scratch directory (default ".cobbler/").
	CobblerDir string `yaml:"cobbler_dir"`

	// BeadsDir is the beads database directory (default ".beads/").
	BeadsDir string `yaml:"beads_dir"`

	// MaxStitchIssues is the total maximum number of stitch iterations for
	// an entire run (default 0, meaning unlimited).
	MaxStitchIssues int `yaml:"max_stitch_issues"`

	// MaxStitchIssuesPerCycle is the maximum number of tasks stitch
	// processes before calling measure again (default 10).
	MaxStitchIssuesPerCycle int `yaml:"max_stitch_issues_per_cycle"`

	// MaxMeasureIssues is the maximum number of new issues to create per
	// measure pass (default 1).
	MaxMeasureIssues int `yaml:"max_measure_issues"`

	// UserPrompt provides additional context for the measure prompt.
	UserPrompt string `yaml:"user_prompt"`

	// MeasurePrompt is a file path to a custom measure prompt template.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	MeasurePrompt string `yaml:"measure_prompt"`

	// StitchPrompt is a file path to a custom stitch prompt template.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	StitchPrompt string `yaml:"stitch_prompt"`

	// EstimatedLinesMin is the minimum estimated lines per task (default 250).
	// Passed to the measure prompt template as LinesMin.
	EstimatedLinesMin int `yaml:"estimated_lines_min"`

	// EstimatedLinesMax is the maximum estimated lines per task (default 350).
	// Passed to the measure prompt template as LinesMax.
	EstimatedLinesMax int `yaml:"estimated_lines_max"`
}

// PodmanConfig holds settings for the podman container runtime.
type PodmanConfig struct {
	// PodmanImage is the container image for Claude execution (required).
	// Claude runs inside a podman container for isolation.
	PodmanImage string `yaml:"podman_image"`

	// PodmanArgs are additional arguments passed to podman run before the image name.
	PodmanArgs []string `yaml:"podman_args"`

	// ClaudeMaxTimeSec is the maximum duration in seconds for a single Claude
	// invocation (default 300, i.e. 5 minutes). If the time expires, the
	// process is killed and the task is returned to beads.
	ClaudeMaxTimeSec int `yaml:"claude_max_time_sec"`
}

// ClaudeTimeout returns the max time as a time.Duration.
func (p *PodmanConfig) ClaudeTimeout() time.Duration {
	return time.Duration(p.ClaudeMaxTimeSec) * time.Second
}

// ClaudeConfig holds settings for the Claude CLI.
type ClaudeConfig struct {
	// ClaudeArgs are the CLI arguments for automated Claude execution.
	// If empty, defaults to the standard automated flags.
	ClaudeArgs []string `yaml:"claude_args"`

	// SilenceAgent suppresses Claude stdout when true (default true).
	SilenceAgent *bool `yaml:"silence_agent"`

	// SecretsDir is the directory containing token files (default ".secrets").
	SecretsDir string `yaml:"secrets_dir"`

	// DefaultTokenFile is the default credential filename (default "claude.json").
	DefaultTokenFile string `yaml:"default_token_file"`

	// TokenFile overrides the credential filename in SecretsDir.
	// If empty, DefaultTokenFile is used.
	TokenFile string `yaml:"token_file"`
}

// Config holds all orchestrator settings. Consuming repos either
// construct a Config in Go code and pass it to New(), or place a
// configuration.yaml at the repository root and call NewFromFile().
type Config struct {
	ProjectConfig    `yaml:",inline"`
	GenerationConfig `yaml:",inline"`
	CobblerConfig    `yaml:",inline"`
	PodmanConfig     `yaml:",inline"`
	ClaudeConfig     `yaml:",inline"`
}

// DefaultConfigFile is the conventional configuration filename.
const DefaultConfigFile = "configuration.yaml"

// DefaultConfig returns a Config populated with all default values.
// Project-specific fields (ModulePath, BinaryName, etc.) are left empty;
// the caller fills them in or the user edits the generated file.
func DefaultConfig() Config {
	t := true
	cfg := Config{
		ClaudeConfig: ClaudeConfig{SilenceAgent: &t},
	}
	cfg.applyDefaults()
	return cfg
}

// WriteDefaultConfig writes a configuration.yaml at the given path
// with all defaults filled in. Returns an error if the file already exists.
func WriteDefaultConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	cfg := DefaultConfig()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshalling default config: %w", err)
	}

	header := "# Orchestrator configuration — edit fields below.\n# See docs/ARCHITECTURE.md for field descriptions.\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
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
	if c.MaxStitchIssuesPerCycle == 0 {
		c.MaxStitchIssuesPerCycle = 10
	}
	if c.MaxMeasureIssues == 0 {
		c.MaxMeasureIssues = 1
	}
	if c.EstimatedLinesMin == 0 {
		c.EstimatedLinesMin = 250
	}
	if c.EstimatedLinesMax == 0 {
		c.EstimatedLinesMax = 350
	}
	if c.ClaudeMaxTimeSec == 0 {
		c.ClaudeMaxTimeSec = 300
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
