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
	// Prefix is the prefix for generation branch names (default "generation-").
	Prefix string `yaml:"prefix"`

	// Cycles is the maximum number of measure+stitch cycles per run
	// (default 0, meaning run until all issues are closed).
	Cycles int `yaml:"cycles"`

	// Branch selects a specific generation branch to work on.
	// If empty, the orchestrator auto-detects from existing branches.
	Branch string `yaml:"branch"`

	// CleanupDirs lists directories to remove after generation stop or reset.
	// Empty by default.
	CleanupDirs []string `yaml:"cleanup_dirs"`
}

// CobblerConfig holds settings for the measure and stitch workflows.
type CobblerConfig struct {
	// Dir is the cobbler scratch directory (default ".cobbler/").
	Dir string `yaml:"dir"`

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

	// PlanningConstitution is a file path to a custom planning constitution YAML.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	PlanningConstitution string `yaml:"planning_constitution"`

	// ExecutionConstitution is a file path to a custom execution constitution YAML.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	ExecutionConstitution string `yaml:"execution_constitution"`

	// DesignConstitution is a file path to a custom design constitution YAML.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	DesignConstitution string `yaml:"design_constitution"`

	// GoStyleConstitution is a file path to a custom Go style constitution YAML.
	// During LoadConfig the file is read and its content stored here.
	// If empty, the embedded default is used.
	GoStyleConstitution string `yaml:"go_style_constitution"`

	// EstimatedLinesMin is the minimum estimated lines per task (default 250).
	// Passed to the measure prompt template as LinesMin.
	EstimatedLinesMin int `yaml:"estimated_lines_min"`

	// EstimatedLinesMax is the maximum estimated lines per task (default 350).
	// Passed to the measure prompt template as LinesMax.
	EstimatedLinesMax int `yaml:"estimated_lines_max"`

	// HistoryDir is the directory for saving measure artifacts (prompt,
	// issues YAML, stream-json log) per iteration. Default "history".
	HistoryDir string `yaml:"history_dir"`
}

// PodmanConfig holds settings for the podman container runtime.
type PodmanConfig struct {
	// Image is the container image for Claude execution (default "claude-cli").
	// Claude runs inside a podman container for isolation.
	Image string `yaml:"image"`

	// Args are additional arguments passed to podman run before the image name.
	Args []string `yaml:"args"`
}

// ClaudeConfig holds settings for the Claude CLI.
type ClaudeConfig struct {
	// Args are the CLI arguments for automated Claude execution.
	// If empty, defaults to the standard automated flags.
	Args []string `yaml:"args"`

	// SilenceAgent suppresses Claude stdout when true (default true).
	SilenceAgent *bool `yaml:"silence_agent"`

	// SecretsDir is the directory containing token files (default ".secrets").
	SecretsDir string `yaml:"secrets_dir"`

	// DefaultTokenFile is the default credential filename (default "claude.json").
	DefaultTokenFile string `yaml:"default_token_file"`

	// TokenFile overrides the credential filename in SecretsDir.
	// If empty, DefaultTokenFile is used.
	TokenFile string `yaml:"token_file"`

	// MaxTimeSec is the maximum duration in seconds for a single Claude
	// invocation (default 300, i.e. 5 minutes). If the time expires, the
	// process is killed and the task is returned to beads.
	MaxTimeSec int `yaml:"max_time_sec"`
}

// ClaudeTimeout returns the max time as a time.Duration.
func (c *ClaudeConfig) ClaudeTimeout() time.Duration {
	return time.Duration(c.MaxTimeSec) * time.Second
}

// Config holds all orchestrator settings. Consuming repos either
// construct a Config in Go code and pass it to New(), or place a
// configuration.yaml at the repository root and call NewFromFile().
type Config struct {
	Project    ProjectConfig    `yaml:"project"`
	Generation GenerationConfig `yaml:"generation"`
	Cobbler    CobblerConfig    `yaml:"cobbler"`
	Podman     PodmanConfig     `yaml:"podman"`
	Claude     ClaudeConfig     `yaml:"claude"`
}

// DefaultConfigFile is the conventional configuration filename.
const DefaultConfigFile = "configuration.yaml"

// DefaultConfig returns a Config populated with all default values.
// Project-specific fields (ModulePath, BinaryName, etc.) are left empty;
// the caller fills them in or the user edits the generated file.
func DefaultConfig() Config {
	t := true
	cfg := Config{
		Claude: ClaudeConfig{SilenceAgent: &t},
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

	header := "# Orchestrator configuration — edit fields below.\n# See docs/ARCHITECTURE.yaml for field descriptions.\n\n"
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
	if c.Claude.SilenceAgent == nil {
		return true
	}
	return *c.Claude.SilenceAgent
}

// EffectiveTokenFile returns the token file to use: TokenFile if set,
// otherwise DefaultTokenFile.
func (c *Config) EffectiveTokenFile() string {
	if c.Claude.TokenFile != "" {
		return c.Claude.TokenFile
	}
	return c.Claude.DefaultTokenFile
}

// ClaudeTimeout returns the max Claude invocation time as a Duration.
func (c *Config) ClaudeTimeout() time.Duration {
	return c.Claude.ClaudeTimeout()
}

func (c *Config) applyDefaults() {
	if c.Project.BinaryDir == "" {
		c.Project.BinaryDir = "bin"
	}
	if c.Generation.Prefix == "" {
		c.Generation.Prefix = "generation-"
	}
	if c.Cobbler.BeadsDir == "" {
		c.Cobbler.BeadsDir = ".beads/"
	}
	if c.Cobbler.Dir == "" {
		c.Cobbler.Dir = ".cobbler/"
	}
	if c.Project.MagefilesDir == "" {
		c.Project.MagefilesDir = "magefiles"
	}
	if c.Claude.SecretsDir == "" {
		c.Claude.SecretsDir = ".secrets"
	}
	if c.Claude.DefaultTokenFile == "" {
		c.Claude.DefaultTokenFile = "claude.json"
	}
	if len(c.Claude.Args) == 0 {
		c.Claude.Args = defaultClaudeArgs
	}
	if c.Cobbler.MaxStitchIssuesPerCycle == 0 {
		c.Cobbler.MaxStitchIssuesPerCycle = 10
	}
	if c.Cobbler.MaxMeasureIssues == 0 {
		c.Cobbler.MaxMeasureIssues = 1
	}
	if c.Cobbler.EstimatedLinesMin == 0 {
		c.Cobbler.EstimatedLinesMin = 250
	}
	if c.Cobbler.EstimatedLinesMax == 0 {
		c.Cobbler.EstimatedLinesMax = 350
	}
	if c.Cobbler.HistoryDir == "" {
		c.Cobbler.HistoryDir = "history"
	}
	if c.Claude.MaxTimeSec == 0 {
		c.Claude.MaxTimeSec = 300
	}
	if c.Podman.Image == "" {
		c.Podman.Image = "claude-cli"
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
	for dest, src := range cfg.Project.SeedFiles {
		if src == "" {
			continue
		}
		content, err := os.ReadFile(src)
		if err != nil {
			return Config{}, fmt.Errorf("reading seed file %s for %s: %w", src, dest, err)
		}
		cfg.Project.SeedFiles[dest] = string(content)
	}

	// Read prompt template files from disk.
	if cfg.Cobbler.MeasurePrompt != "" {
		content, err := os.ReadFile(cfg.Cobbler.MeasurePrompt)
		if err != nil {
			return Config{}, fmt.Errorf("reading measure prompt %s: %w", cfg.Cobbler.MeasurePrompt, err)
		}
		cfg.Cobbler.MeasurePrompt = string(content)
	}
	if cfg.Cobbler.StitchPrompt != "" {
		content, err := os.ReadFile(cfg.Cobbler.StitchPrompt)
		if err != nil {
			return Config{}, fmt.Errorf("reading stitch prompt %s: %w", cfg.Cobbler.StitchPrompt, err)
		}
		cfg.Cobbler.StitchPrompt = string(content)
	}

	// Read constitution files from disk.
	if cfg.Cobbler.PlanningConstitution != "" {
		content, err := os.ReadFile(cfg.Cobbler.PlanningConstitution)
		if err != nil {
			return Config{}, fmt.Errorf("reading planning constitution %s: %w", cfg.Cobbler.PlanningConstitution, err)
		}
		cfg.Cobbler.PlanningConstitution = string(content)
	}
	if cfg.Cobbler.ExecutionConstitution != "" {
		content, err := os.ReadFile(cfg.Cobbler.ExecutionConstitution)
		if err != nil {
			return Config{}, fmt.Errorf("reading execution constitution %s: %w", cfg.Cobbler.ExecutionConstitution, err)
		}
		cfg.Cobbler.ExecutionConstitution = string(content)
	}
	if cfg.Cobbler.DesignConstitution != "" {
		content, err := os.ReadFile(cfg.Cobbler.DesignConstitution)
		if err != nil {
			return Config{}, fmt.Errorf("reading design constitution %s: %w", cfg.Cobbler.DesignConstitution, err)
		}
		cfg.Cobbler.DesignConstitution = string(content)
	}
	if cfg.Cobbler.GoStyleConstitution != "" {
		content, err := os.ReadFile(cfg.Cobbler.GoStyleConstitution)
		if err != nil {
			return Config{}, fmt.Errorf("reading go style constitution %s: %w", cfg.Cobbler.GoStyleConstitution, err)
		}
		cfg.Cobbler.GoStyleConstitution = string(content)
	}

	cfg.applyDefaults()
	return cfg, nil
}
