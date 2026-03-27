// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestLoadConfig_HierarchicalYAML(t *testing.T) {
	yaml := `
project:
  module_path: github.com/org/repo
  binary_name: myapp
  binary_dir: build
  main_package: github.com/org/repo/cmd/myapp
  go_source_dirs: [cmd/, pkg/]
generation:
  prefix: gen-
  cycles: 5
cobbler:
  dir: .work/
  max_measure_issues: 3
claude:
  max_time_sec: 600
`
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Project.ModulePath != "github.com/org/repo" {
		t.Errorf("ModulePath: got %q, want %q", cfg.Project.ModulePath, "github.com/org/repo")
	}
	if cfg.Project.BinaryName != "myapp" {
		t.Errorf("BinaryName: got %q, want %q", cfg.Project.BinaryName, "myapp")
	}
	if cfg.Project.BinaryDir != "build" {
		t.Errorf("BinaryDir: got %q, want %q", cfg.Project.BinaryDir, "build")
	}
	if cfg.Generation.Prefix != "gen-" {
		t.Errorf("Generation.Prefix: got %q, want %q", cfg.Generation.Prefix, "gen-")
	}
	if cfg.Generation.Cycles != 5 {
		t.Errorf("Generation.Cycles: got %d, want 5", cfg.Generation.Cycles)
	}
	if cfg.Cobbler.Dir != ".work/" {
		t.Errorf("Cobbler.Dir: got %q, want %q", cfg.Cobbler.Dir, ".work/")
	}
	if cfg.Cobbler.MaxMeasureIssues != 3 {
		t.Errorf("Cobbler.MaxMeasureIssues: got %d, want 3", cfg.Cobbler.MaxMeasureIssues)
	}
	if cfg.Claude.MaxTimeSec != 600 {
		t.Errorf("Claude.MaxTimeSec: got %d, want 600", cfg.Claude.MaxTimeSec)
	}
}

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	// Minimal config — no explicit defaults.
	f := writeTemp(t, "project:\n  module_path: github.com/x/y\n")
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Project.BinaryDir != "bin" {
		t.Errorf("BinaryDir default: got %q, want \"bin\"", cfg.Project.BinaryDir)
	}
	if cfg.Generation.Prefix != "generation-" {
		t.Errorf("Generation.Prefix default: got %q, want \"generation-\"", cfg.Generation.Prefix)
	}
	if cfg.Cobbler.Dir != ".cobbler/" {
		t.Errorf("Cobbler.Dir default: got %q, want \".cobbler/\"", cfg.Cobbler.Dir)
	}
	if cfg.Cobbler.MaxStitchIssuesPerCycle != 10 {
		t.Errorf("MaxStitchIssuesPerCycle default: got %d, want 10", cfg.Cobbler.MaxStitchIssuesPerCycle)
	}
	if cfg.Cobbler.MaxMeasureIssues != 1 {
		t.Errorf("MaxMeasureIssues default: got %d, want 1", cfg.Cobbler.MaxMeasureIssues)
	}
	if cfg.Cobbler.EstimatedLinesMin != 250 {
		t.Errorf("EstimatedLinesMin default: got %d, want 250", cfg.Cobbler.EstimatedLinesMin)
	}
	if cfg.Cobbler.EstimatedLinesMax != 350 {
		t.Errorf("EstimatedLinesMax default: got %d, want 350", cfg.Cobbler.EstimatedLinesMax)
	}
	if cfg.Claude.SecretsDir != ".secrets" {
		t.Errorf("Claude.SecretsDir default: got %q, want \".secrets\"", cfg.Claude.SecretsDir)
	}
	if cfg.Claude.DefaultTokenFile != "claude.json" {
		t.Errorf("Claude.DefaultTokenFile default: got %q, want \"claude.json\"", cfg.Claude.DefaultTokenFile)
	}
	if cfg.Claude.MaxTimeSec != 300 {
		t.Errorf("Claude.MaxTimeSec default: got %d, want 300", cfg.Claude.MaxTimeSec)
	}
	if len(cfg.Claude.Args) == 0 {
		t.Error("Claude.Args default: expected non-empty default args")
	}
	if cfg.Cobbler.HistoryDir != "history" {
		t.Errorf("Cobbler.HistoryDir default: got %q, want \"history\"", cfg.Cobbler.HistoryDir)
	}
	if cfg.Cobbler.MaxConsecutiveZeroLOCCycles != 3 {
		t.Errorf("MaxConsecutiveZeroLOCCycles default: got %d, want 3", cfg.Cobbler.MaxConsecutiveZeroLOCCycles)
	}
}

func TestApplyDefaults_DerivesMaxWeightFromMaxRequirements(t *testing.T) {
	// When MaxRequirementsPerTask is set but MaxWeightPerTask is not,
	// applyDefaults derives MaxWeightPerTask from MaxRequirementsPerTask.
	cfg := Config{Cobbler: CobblerConfig{MaxRequirementsPerTask: 6}}
	cfg.applyDefaults()
	if cfg.Cobbler.MaxWeightPerTask != 6 {
		t.Errorf("MaxWeightPerTask = %d, want 6 (derived from MaxRequirementsPerTask)",
			cfg.Cobbler.MaxWeightPerTask)
	}
}

func TestApplyDefaults_ExplicitMaxWeightPreserved(t *testing.T) {
	// When both are explicitly set, the explicit MaxWeightPerTask is kept.
	cfg := Config{Cobbler: CobblerConfig{
		MaxRequirementsPerTask: 6,
		MaxWeightPerTask:       10,
	}}
	cfg.applyDefaults()
	if cfg.Cobbler.MaxWeightPerTask != 10 {
		t.Errorf("MaxWeightPerTask = %d, want 10 (explicit value preserved)",
			cfg.Cobbler.MaxWeightPerTask)
	}
}

func TestApplyDefaults_NoMaxRequirements_NoMaxWeight(t *testing.T) {
	// When neither is set, MaxWeightPerTask stays 0 (disabled).
	cfg := Config{}
	cfg.applyDefaults()
	if cfg.Cobbler.MaxWeightPerTask != 0 {
		t.Errorf("MaxWeightPerTask = %d, want 0 (both disabled)",
			cfg.Cobbler.MaxWeightPerTask)
	}
}

func TestLoadConfig_ConstitutionFileOverride(t *testing.T) {
	dir := t.TempDir()
	planningPath := filepath.Join(dir, "planning.yaml")
	if err := os.WriteFile(planningPath, []byte("custom planning content"), 0o644); err != nil {
		t.Fatal(err)
	}

	yaml := "cobbler:\n  planning_constitution: " + planningPath + "\n"
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Cobbler.PlanningConstitution != "custom planning content" {
		t.Errorf("PlanningConstitution: got %q, want file content", cfg.Cobbler.PlanningConstitution)
	}
}

func TestLoadConfig_MissingConstitutionFile(t *testing.T) {
	yaml := "cobbler:\n  execution_constitution: /nonexistent/path/execution.yaml\n"
	f := writeTemp(t, yaml)
	_, err := LoadConfig(f)
	if err == nil {
		t.Error("expected error for missing constitution file, got nil")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/configuration.yaml")
	if err == nil {
		t.Error("expected error for missing config file, got nil")
	}
}

func TestConfig_Silence_NilDefaultsTrue(t *testing.T) {
	cfg := Config{}
	if !cfg.Silence() {
		t.Error("Silence() with nil SilenceAgent should return true")
	}
}

func TestConfig_Silence_ExplicitFalse(t *testing.T) {
	f := false
	cfg := Config{Claude: ClaudeConfig{SilenceAgent: &f}}
	if cfg.Silence() {
		t.Error("Silence() with SilenceAgent=false should return false")
	}
}

func TestConfig_Silence_ExplicitTrue(t *testing.T) {
	tr := true
	cfg := Config{Claude: ClaudeConfig{SilenceAgent: &tr}}
	if !cfg.Silence() {
		t.Error("Silence() with SilenceAgent=true should return true")
	}
}

func TestConfig_EffectiveTokenFile_Default(t *testing.T) {
	cfg := Config{Claude: ClaudeConfig{DefaultTokenFile: "claude.json"}}
	if got := cfg.EffectiveTokenFile(); got != "claude.json" {
		t.Errorf("EffectiveTokenFile without override: got %q, want %q", got, "claude.json")
	}
}

func TestConfig_EffectiveTokenFile_Override(t *testing.T) {
	cfg := Config{Claude: ClaudeConfig{
		DefaultTokenFile: "claude.json",
		TokenFile:        "custom-token.json",
	}}
	if got := cfg.EffectiveTokenFile(); got != "custom-token.json" {
		t.Errorf("EffectiveTokenFile with override: got %q, want %q", got, "custom-token.json")
	}
}

func TestConfig_ClaudeTimeout(t *testing.T) {
	cfg := Config{Claude: ClaudeConfig{MaxTimeSec: 120}}
	want := 120 * time.Second
	if got := cfg.ClaudeTimeout(); got != want {
		t.Errorf("ClaudeTimeout: got %v, want %v", got, want)
	}
}

func TestLoadConfig_TemperatureFromYAML(t *testing.T) {
	yaml := `claude:
  temperature: 0.7
`
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Claude.Temperature != 0.7 {
		t.Errorf("Temperature: got %f, want 0.7", cfg.Claude.Temperature)
	}
}

func TestLoadConfig_TemperatureDefaultsToZero(t *testing.T) {
	f := writeTemp(t, "project:\n  module_path: example.com/x\n")
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Claude.Temperature != 0 {
		t.Errorf("Temperature default: got %f, want 0", cfg.Claude.Temperature)
	}
}

func TestLoadConfig_EnforceMeasureValidationFromYAML(t *testing.T) {
	yaml := `cobbler:
  enforce_measure_validation: true
  max_measure_retries: 3
`
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Cobbler.EnforceMeasureValidation {
		t.Error("EnforceMeasureValidation: got false, want true")
	}
	if cfg.Cobbler.MaxMeasureRetries != 3 {
		t.Errorf("MaxMeasureRetries: got %d, want 3", cfg.Cobbler.MaxMeasureRetries)
	}
}

func TestLoadConfig_MaxConsecutiveZeroLOCCyclesFromYAML(t *testing.T) {
	yaml := `cobbler:
  max_consecutive_zero_loc_cycles: 7
`
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Cobbler.MaxConsecutiveZeroLOCCycles != 7 {
		t.Errorf("MaxConsecutiveZeroLOCCycles: got %d, want 7", cfg.Cobbler.MaxConsecutiveZeroLOCCycles)
	}
}

func TestLoadConfig_EnforceMeasureValidationDefaultsFalse(t *testing.T) {
	f := writeTemp(t, "project:\n  module_path: example.com/x\n")
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Cobbler.EnforceMeasureValidation {
		t.Error("EnforceMeasureValidation default: got true, want false")
	}
	if cfg.Cobbler.MaxMeasureRetries != 0 {
		t.Errorf("MaxMeasureRetries default: got %d, want 0", cfg.Cobbler.MaxMeasureRetries)
	}
}

// --- LoadConfig: SeedFiles resolution ---

func TestLoadConfig_SeedFilesResolved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Write a seed source file.
	seedSrc := filepath.Join(dir, "template.go.tmpl")
	if err := os.WriteFile(seedSrc, []byte("package {{.ModulePath}}"), 0o644); err != nil {
		t.Fatal(err)
	}

	yaml := "project:\n  seed_files:\n    cmd/main.go: " + seedSrc + "\n"
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Project.SeedFiles["cmd/main.go"]; got != "package {{.ModulePath}}" {
		t.Errorf("SeedFiles[\"cmd/main.go\"]: got %q, want file content", got)
	}
}

func TestLoadConfig_SeedFilesMissing(t *testing.T) {
	t.Parallel()
	yaml := "project:\n  seed_files:\n    cmd/main.go: /nonexistent/template.go.tmpl\n"
	f := writeTemp(t, yaml)
	_, err := LoadConfig(f)
	if err == nil {
		t.Error("expected error for missing seed source file, got nil")
	}
}

func TestLoadConfig_MeasurePromptFromFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "measure.yaml")
	if err := os.WriteFile(promptPath, []byte("role: measure\ntask: generate issues"), 0o644); err != nil {
		t.Fatal(err)
	}

	yaml := "cobbler:\n  measure_prompt: " + promptPath + "\n"
	f := writeTemp(t, yaml)
	cfg, err := LoadConfig(f)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Cobbler.MeasurePrompt != "role: measure\ntask: generate issues" {
		t.Errorf("MeasurePrompt: got %q, want file content", cfg.Cobbler.MeasurePrompt)
	}
}

// --- WriteDefaultConfig ---

func TestWriteDefaultConfig_CreatesFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "configuration.yaml")
	if err := WriteDefaultConfig(path); err != nil {
		t.Fatalf("WriteDefaultConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written config: %v", err)
	}
	content := string(data)

	// File should start with the header comment.
	if len(content) == 0 {
		t.Fatal("WriteDefaultConfig wrote an empty file")
	}
	if content[:1] != "#" {
		t.Errorf("expected file to start with '#', got %q", content[:10])
	}

	// Re-parsing the written file should apply defaults correctly.
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig on written config: %v", err)
	}
	if cfg.Project.BinaryDir != "bin" {
		t.Errorf("BinaryDir: got %q, want \"bin\"", cfg.Project.BinaryDir)
	}
	if cfg.Claude.MaxTimeSec == 0 {
		t.Error("Claude.MaxTimeSec should be non-zero after WriteDefaultConfig")
	}
}

func TestWriteDefaultConfig_ExistingFileError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "configuration.yaml")
	// Pre-create the file.
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteDefaultConfig(path); err == nil {
		t.Error("expected error when file already exists, got nil")
	}
}
