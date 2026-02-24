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

	// ContextSources is a newline-delimited list of extra file paths and
	// glob patterns that supplement the standard document structure in the
	// measure prompt's project context. Standard files (vision, architecture,
	// specs, roadmap, PRDs, use cases, test suites, dependency-map, sources,
	// engineering) are loaded automatically by an internal algorithm.
	// ContextSources adds project-specific extras beyond that standard set.
	// Globs are expanded at runtime; duplicates are logged and removed.
	// Source code is handled separately by GoSourceDirs.
	ContextSources string `yaml:"context_sources"`

	// ContextInclude is a newline-delimited list of glob patterns. When
	// set, these patterns replace the standard document discovery
	// (resolveStandardFiles). Only matching files are loaded into the
	// project context. ContextSources still adds extras on top.
	// When empty, the default standard file discovery applies.
	ContextInclude string `yaml:"context_include"`

	// ContextExclude is a newline-delimited list of glob patterns. Files
	// matching any pattern (or under a matching directory) are excluded
	// from the project context. Applied to docs, context sources, and
	// source code. Use "." to exclude everything.
	ContextExclude string `yaml:"context_exclude"`

	// Release is the target release version (e.g., "01.0"). When set,
	// use cases and test suites are filtered to only include files whose
	// release version is <= this value. PRDs are filtered to only those
	// referenced by the included use cases. An empty value disables
	// release-based filtering and includes all files.
	Release string `yaml:"release"`

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

	// GoldenExample is a file path to a golden example issue YAML.
	// During LoadConfig the file is read and its content stored here.
	// When present, the measure prompt instructs Claude to match this
	// example's style, granularity, and naming conventions.
	GoldenExample string `yaml:"golden_example"`

	// MaxContextBytes is the maximum serialized size (in bytes) of the
	// ProjectContext injected into the stitch prompt. When the context
	// exceeds this budget, non-required source files are progressively
	// removed. Recommended value: 200000 (~50K tokens at 4 bytes/token).
	// When 0 (the default), budget enforcement is skipped.
	MaxContextBytes int `yaml:"max_context_bytes"`

	// EnforceMeasureValidation enables strict validation of measure output.
	// When true, issues that violate P9 granularity ranges or P7 file naming
	// are rejected and measure retries. When false (default), violations are
	// logged as advisory warnings and import proceeds.
	EnforceMeasureValidation bool `yaml:"enforce_measure_validation"`

	// MaxMeasureRetries is the maximum number of retry attempts per iteration
	// when EnforceMeasureValidation rejects the output. When 0 (default),
	// no retries are attempted. A value of 2-3 is recommended.
	MaxMeasureRetries int `yaml:"max_measure_retries"`

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

	// ContainerCredentialsPath is the absolute path inside the container
	// where the Claude CLI expects its credentials file.
	// Default: /home/crumbs/.claude/.credentials.json
	ContainerCredentialsPath string `yaml:"container_credentials_path"`
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
	return time.Duration(c.Claude.MaxTimeSec) * time.Second
}

// readFileInto reads the file at the path stored in *field and replaces
// the value with the file content. If *field is empty, it is a no-op.
func readFileInto(field *string) error {
	if *field == "" {
		return nil
	}
	content, err := os.ReadFile(*field)
	if err != nil {
		return fmt.Errorf("reading %s: %w", *field, err)
	}
	*field = string(content)
	return nil
}

// defaultContextSources is empty because the standard document structure
// is now loaded automatically by resolveStandardFiles. This field only
// holds project-specific extras beyond the standard set.
const defaultContextSources = ""

func (c *Config) applyDefaults() {
	if c.Project.BinaryDir == "" {
		c.Project.BinaryDir = "bin"
	}
	if c.Generation.Prefix == "" {
		c.Generation.Prefix = "generation-"
	}
	if c.Cobbler.BeadsDir == "" {
		c.Cobbler.BeadsDir = dirBeads + "/"
	}
	if c.Cobbler.Dir == "" {
		c.Cobbler.Dir = dirCobbler + "/"
	}
	if c.Project.MagefilesDir == "" {
		c.Project.MagefilesDir = dirMagefiles
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
	if c.Claude.ContainerCredentialsPath == "" {
		c.Claude.ContainerCredentialsPath = "/home/crumbs/.claude/.credentials.json"
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

	// Read prompt and constitution files from disk, replacing the path
	// with the file content.
	for _, field := range []*string{
		&cfg.Cobbler.MeasurePrompt,
		&cfg.Cobbler.StitchPrompt,
		&cfg.Cobbler.PlanningConstitution,
		&cfg.Cobbler.ExecutionConstitution,
		&cfg.Cobbler.DesignConstitution,
		&cfg.Cobbler.GoStyleConstitution,
		&cfg.Cobbler.GoldenExample,
	} {
		if err := readFileInto(field); err != nil {
			return Config{}, err
		}
	}

	cfg.applyDefaults()
	return cfg, nil
}
