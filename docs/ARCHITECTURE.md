# Mage Claude Orchestrator Architecture

## System Overview

Mage Claude Orchestrator is a Go library that automates AI-driven code generation through a two-phase loop: measure (propose tasks) and stitch (execute tasks in isolated worktrees). Consuming projects import the library, configure it with project-specific paths and templates, and expose its methods as Mage targets.

The system operates as build tooling, not a standalone application. An `Orchestrator` struct holds a `Config` and provides methods that Mage calls as targets. These methods coordinate four subsystems: git branch management, Claude invocation (via container or direct binary), issue tracking (beads), and metrics collection.

|  |
|:--:|

```plantuml
@startuml
!theme plain
skinparam backgroundColor white

package "Consuming Project" {
  [Magefile] <<mage targets>>
}

package "orchestrator" {
  [Orchestrator] <<main struct>>
  [Generator] <<lifecycle>>
  [Cobbler] <<measure + stitch>>
  [Commands] <<git, beads, go wrappers>>
  [Docker] <<container runtime>>
  [Stats] <<metrics>>
}

package "External Tools" {
  [Git]
  [Claude Code]
  [Beads (bd)]
  [Go Toolchain]
  [Podman / Docker]
}

[Magefile] --> [Orchestrator]
[Orchestrator] --> [Generator]
[Orchestrator] --> [Cobbler]
[Orchestrator] --> [Stats]
[Generator] --> [Commands]
[Cobbler] --> [Commands]
[Cobbler] --> [Docker]
[Commands] --> [Git]
[Commands] --> [Beads (bd)]
[Commands] --> [Go Toolchain]
[Docker] --> [Podman / Docker]
[Docker] --> [Claude Code]
[Cobbler] ..> [Claude Code] : direct fallback

@enduml
```

|Figure 1 System context showing orchestrator components and external tools |

### Generation Lifecycle

Generations are the primary unit of work. A generation starts from a tagged main state, creates a branch, runs measure-stitch cycles, and merges the result back to main.

States: `created` (branch exists, sources reset) -> `running` (cycles in progress) -> `finished` (tagged, ready to merge) -> `merged` (on main, branch deleted). An alternative terminal state is `abandoned` (generation was never merged).

The generation branch name follows the pattern `{GenPrefix}{timestamp}`, where the timestamp is formatted as `2006-01-02-15-04-05`. Tags mark lifecycle events: `{branch}-start`, `{branch}-finished`, `{branch}-merged`, `{branch}-abandoned`.

### Cobbler Workflow

The cobbler workflow has two phases that run in sequence within each cycle.

**Measure** reads the project state (documentation, existing issues) and invokes Claude with a prompt template. Claude proposes tasks as a JSON array. We import these tasks into beads with dependency wiring.

**Stitch** picks ready tasks from beads one at a time. For each task: create a git worktree on a task branch, invoke Claude with the task description, merge the task branch back, record metrics, close the task. Task branches use the pattern `task/{baseBranch}-{issueID}`.

### Task Isolation

Each stitch task runs in a separate git worktree. This prevents concurrent tasks from interfering with each other and keeps the generation branch clean. The worktree lives in a temp directory (`$TMPDIR/{repoName}-worktrees/{issueID}`). After Claude completes, we merge the task branch into the generation branch and remove the worktree.

Recovery handles interrupted runs. On resume, we scan for stale task branches (worktrees that were not cleaned up), remove them, reset their issues to ready, and continue.

## Main Interface

### Orchestrator and Config

The Orchestrator is the entry point. Consuming projects create a Config, pass it to `New()`, and call methods on the returned Orchestrator.

```go
type Orchestrator struct {
    cfg Config
}

func New(cfg Config) *Orchestrator
```

Config holds project-specific settings. The orchestrator applies defaults to zero-value fields.

Table 1 Config Fields

| Field | Type | Default | Purpose |
|-------|------|---------|---------|
| ModulePath | string | (required) | Go module path |
| BinaryName | string | (required) | Compiled binary name |
| BinaryDir | string | "bin" | Output directory for binaries |
| MainPackage | string | | Path to main.go entry point |
| GoSourceDirs | []string | | Directories containing Go source |
| VersionFile | string | | Path to version.go |
| GenPrefix | string | "generation-" | Prefix for generation branches |
| BeadsDir | string | ".beads/" | Beads database directory |
| CobblerDir | string | ".cobbler/" | Scratch directory |
| MagefilesDir | string | "magefiles" | Directory skipped when deleting Go files |
| ContainerImage | string | | Docker/Podman image name |
| ContainerTag | string | "latest" | Image tag |
| DockerfileDir | string | "magefiles" | Directory containing Dockerfile.claude |
| SecretsDir | string | ".secrets" | Directory for token files |
| DefaultTokenFile | string | "claude.json" | Credential filename |
| ContainerCredDst | string | "/home/crumbs/.claude/.credentials.json" | Container credential path |
| SpecGlobs | map[string]string | | Glob patterns for word-count stats |
| SeedFiles | map[string]string | | Template files seeded during reset |
| MeasurePrompt | string | (embedded) | Custom measure prompt template |
| StitchPrompt | string | (embedded) | Custom stitch prompt template |

### Operations

Table 2 Orchestrator Operations

| Method | Purpose | PRD |
|--------|---------|-----|
| GeneratorStart() | Tag main, create generation branch, reset sources | prd002 |
| GeneratorRun() | Run N measure+stitch cycles | prd002 |
| GeneratorResume() | Recover and continue interrupted run | prd002 |
| GeneratorStop() | Merge generation into main, tag, clean up | prd002 |
| GeneratorReset() | Destroy all generations, return to clean main | prd002 |
| GeneratorList() | Show active and past generations | prd002 |
| GeneratorSwitch() | Switch between generation branches | prd002 |
| Measure() | Propose tasks via Claude | prd003 |
| Stitch() | Execute ready tasks in worktrees | prd003 |
| Stats() | Print LOC and documentation metrics | prd005 |
| Init() | Initialize beads | prd001 |
| FullReset() | Reset cobbler, generator, and beads | prd001 |
| CobblerReset() | Remove scratch directory | prd003 |
| BeadsInit() | Initialize beads database | prd001 |
| BeadsReset() | Reset beads database | prd001 |

### Cobbler Configuration

Measure and stitch share a CobblerConfig that controls Claude invocation behavior.

Table 3 CobblerConfig Fields

| Field | Type | Default | Purpose |
|-------|------|---------|---------|
| SilenceAgent | bool | true | Suppress Claude stdout |
| MaxIssues | int | 10 | Maximum tasks per phase |
| UserPrompt | string | "" | Additional context for Claude |
| GenerationBranch | string | "" | Explicit branch to work on |
| TokenFile | string | Config.DefaultTokenFile | Credential file in .secrets/ |
| NoContainer | bool | false | Skip container, use local claude |

### Prompt Templates

Prompts are Go text/template strings embedded from `prompts/measure.tmpl` and `prompts/stitch.tmpl`. Consuming projects can override them via Config.MeasurePrompt and Config.StitchPrompt.

Table 4 Template Data Types

| Template | Data Type | Fields |
|----------|-----------|--------|
| Measure | MeasurePromptData | ExistingIssues (JSON string), Limit (int), OutputPath (string), UserInput (string) |
| Stitch | StitchPromptData | Title, ID, IssueType, Description (all strings) |

### Metrics

Every Claude invocation records an InvocationRecord as a JSON comment on the beads issue.

Table 5 InvocationRecord Fields

| Field | Type | Purpose |
|-------|------|---------|
| Caller | string | "measure" or "stitch" |
| StartedAt | string (RFC3339) | When Claude was invoked |
| DurationS | int | Total duration in seconds |
| Tokens | {Input, Output int} | Token usage |
| LOCBefore | {Production, Test int} | Go LOC before Claude |
| LOCAfter | {Production, Test int} | Go LOC after Claude |
| Diff | {Files, Insertions, Deletions int} | Git diff stats |

## System Components

**Orchestrator (orchestrator.go)**: Entry point. Holds Config, provides New() constructor, manages logging with optional generation tagging. All other components are methods on this struct or package-level functions.

**Generator (generator.go)**: Manages the generation lifecycle. Creates generation branches, runs cycles, merges results to main, handles resume from interrupted runs. Uses git tags to mark lifecycle events. Resets Go sources and re-seeds template files on start and reset.

**Cobbler - Measure (measure.go)**: Builds the measure prompt from existing issues and project state, invokes Claude, parses the JSON output, and imports proposed issues into beads with dependency wiring. Records invocation metrics.

**Cobbler - Stitch (stitch.go)**: Picks ready tasks from beads, creates worktrees, invokes Claude, merges branches, records metrics, and closes tasks. Handles recovery of stale tasks from interrupted runs.

**Cobbler Common (cobbler.go)**: Shared configuration (CobblerConfig), flag registration, Claude invocation (runClaude), token parsing, LOC capture, invocation recording, and worktree path management.

**Commands (commands.go)**: Wrapper functions for external tools. Over 50 functions wrapping git, beads (bd), and Go CLI commands. Centralizes binary names as constants and provides structured access to command output.

**Docker (docker.go)**: Container runtime detection (podman, then docker), image building, and container-based Claude execution. Mounts the workspace and credentials into the container. Falls back to direct binary when no runtime is available.

**Flags (flags.go)**: Mage flag interception. Captures flags from os.Args before Mage processes them, enabling per-target flag parsing. Provides InitFlags(), ParseTargetFlags(), and TargetArgs().

**Stats (stats.go)**: Collects Go LOC counts (production and test) and documentation word counts. Uses the configured GoSourceDirs and SpecGlobs. Output is used for invocation records and the `mage stats` target.

**Beads (beads.go)**: Initializes and resets the beads issue tracker. Manages the beads database directory and provides helpers for beads lifecycle operations.

**Config (config.go)**: Config struct definition with SeedData template data. Provides applyDefaults() for zero-value fields.

## Design Decisions

**Decision 1: Library, not application.** We chose to build a library that consuming projects import, not a standalone CLI. This allows each project to configure the orchestrator with its own module path, source directories, and prompt templates. The Mage build system provides the CLI interface. Alternative: a standalone CLI would require configuration files and lose the type-safe Config struct.

**Decision 2: Git worktree isolation.** Each stitch task runs in a separate git worktree on its own branch. This prevents concurrent tasks from interfering and keeps the generation branch clean. Worktrees are temporary and cleaned up after merge. Alternative: running Claude directly on the generation branch risks partial commits and merge conflicts between tasks.

**Decision 3: Two-phase cobbler loop.** We separate task proposal (measure) from task execution (stitch). This allows measure to see the full project state before proposing work, and stitch to execute tasks independently. Alternative: a single-phase approach where Claude both proposes and executes loses the ability to review proposed tasks before execution.

**Decision 4: Container-first Claude execution.** We prefer running Claude in a container (podman, then docker) for isolation, falling back to direct binary when no container runtime is available. The --no-container flag overrides this behavior. Alternative: always using the direct binary loses container isolation benefits.

**Decision 5: Beads for issue tracking.** We use the beads git-backed issue tracker because it stores issues as JSONL files tracked by git. This means task state travels with the generation branch and is recoverable from any commit. Alternative: external issue trackers (GitHub Issues, Jira) require network access and do not travel with the branch.

**Decision 6: Embedded prompt templates with override.** Default prompts are embedded in the binary via `//go:embed`. Consuming projects can override them through Config.MeasurePrompt and Config.StitchPrompt. This gives zero-configuration defaults with full customizability. Alternative: external prompt files require file path management and increase deployment complexity.

**Decision 7: Recovery on resume.** Generator resume scans for stale task branches, orphaned in-progress issues, and leftover worktrees. It cleans all of them up and resets affected tasks to ready before continuing. This makes interrupted runs recoverable without manual intervention. Alternative: requiring manual cleanup after interruption is error-prone and frustrating.

## Technology Choices

Table 6 Technology Choices

| Component | Technology | Purpose |
|-----------|------------|---------|
| Language | Go 1.25 | Library and consuming projects use Go |
| Build system | Magefile (magefile/mage) | Orchestrator methods exposed as Mage targets |
| Version control | git (worktrees, tags, branches) | Isolation, lifecycle tracking, merge |
| Issue tracking | Beads (bd CLI) | Git-backed task management via JSONL |
| AI execution | Claude Code (CLI) | Code generation and task execution |
| Container runtime | Podman / Docker | Isolated Claude execution |
| Prompt templating | Go text/template | Parameterized prompts |
| Dependencies | Standard library only | No external Go dependencies |

## Project Structure

```
mage-claude-orchestrator/
  orchestrator.go     # Orchestrator struct, New(), logging
  config.go           # Config struct, SeedData, defaults
  flags.go            # Mage flag interception
  cobbler.go          # Shared cobbler config, runClaude, metrics
  measure.go          # Measure phase: prompt, Claude, import
  stitch.go           # Stitch phase: worktree, Claude, merge
  generator.go        # Generation lifecycle: start/run/resume/stop/reset
  commands.go         # Git, beads, Go command wrappers
  docker.go           # Container runtime detection and execution
  beads.go            # Beads initialization and reset
  stats.go            # LOC and documentation metrics
  go.mod              # Module definition (no external deps)
  prompts/
    measure.tmpl      # Default measure prompt template
    stitch.tmpl       # Default stitch prompt template
```

All code lives in a single `orchestrator` package. Consuming projects import it and wire it into their magefiles.

## Implementation Status

The orchestrator is implemented and in active use by the Crumbs project. All components described in this architecture are functional: generation lifecycle, measure and stitch workflows, container execution with fallback, metrics collection, and recovery from interrupted runs.

## Related Documents

Table 7 Related Documents

| Document | Purpose |
|----------|---------|
| VISION.md | What we build and why; success criteria and boundaries |
| road-map.yaml | Release schedule and use case status |
| prd001-orchestrator-core | Config, Orchestrator struct, flag parsing, initialization |
| prd002-generation-lifecycle | Generation start, run, resume, stop, reset, list, switch |
| prd003-cobbler-workflows | Measure and stitch phases, prompt templates, task execution |
| prd004-container-execution | Container runtime detection, image building, Claude in containers |
| prd005-metrics-collection | Stats, invocation records, LOC snapshots |
| engineering/eng01-generation-workflow | Generation conventions and task branch naming |
| engineering/eng02-prompt-templates | Prompt template conventions and customization |
