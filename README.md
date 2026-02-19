# mage-claude-orchestrator

Go library for automating AI code generation via Claude Code. Consuming projects import this library through their Magefile and run generation cycles that propose tasks (measure) and execute them in isolated git worktrees (stitch). Claude runs inside a podman container for process isolation.

## Prerequisites

| Tool | Purpose |
|------|---------|
| Go 1.25+ | Build and run |
| [Mage](https://magefile.org/) | Build system; orchestrator methods are exposed as Mage targets |
| [Beads](https://github.com/mesh-intelligence/beads) (bd) | Git-backed issue tracking |
| Podman | Container runtime for Claude execution |

## Podman Setup

Claude runs inside a podman container. The orchestrator wraps every Claude invocation in `podman run` with the repository directory mounted into the container.

### 1. Install Podman

On macOS:

```bash
brew install podman
podman machine init
podman machine start
```

On Linux (Fedora/RHEL):

```bash
sudo dnf install podman
```

On Linux (Debian/Ubuntu):

```bash
sudo apt install podman
```

### 2. Prepare the Claude Image

The container image must have the `claude` CLI installed. Build or pull an image that includes it, then set `podman_image` in your configuration.yaml.

### 3. Verify

```bash
podman run --rm <your-image> claude --version
```

If this prints a version string, podman is ready.

### 4. Configure

Set `podman_image` in `configuration.yaml`:

```yaml
podman_image: "your-claude-image:latest"
```

Optional extra arguments (environment variables, additional mounts):

```yaml
podman_args:
  - "-e"
  - "ANTHROPIC_API_KEY=sk-..."
```

The orchestrator runs a pre-flight check before every measure and stitch phase. If podman is not installed or cannot start a container, it exits with instructions pointing here.

## Three-Phase Constitution Architecture

The orchestrator uses three constitutions aligned with the three workflow phases:

### 1. Design Constitution

**Phase:** Interactive design/architecting (writing VISION, ARCHITECTURE, PRDs, use cases, test suites)

**Location:** Scaffolded to consuming projects as `docs/CONSTITUTION-design.yaml`

**Contains:** Documentation standards, format schemas for all document types, traceability model

**Used by:** Human or Claude in interactive mode writing specifications

The design constitution includes:

- Articles D1-D5: Specification-first, YAML-first, test suite linkage, traceability, roadmap-driven releases
- Documentation standards: Strunk & White style, forbidden terms, figure formats
- Document types: VISION, ARCHITECTURE, PRD, use case, test suite, engineering guideline, specification
- Naming conventions and completeness checklists for each document type

### 2. Planning Constitution

**Phase:** Breaking down work (measure)

**Location:** Embedded in orchestrator binary (`pkg/orchestrator/constitutions/planning.yaml`)

**Contains:** Release priority, task sizing rules, issue structure (crumb-format), dependency ordering

**Used by:** Measure prompt to propose well-formed tasks

The planning constitution includes:

- Articles P1-P5: Release-driven priority, task sizing (300-700 LOC, ≤5 files), task limit, issue structure, dependency ordering
- Issue structure: Common fields, documentation vs code issues, example templates
- Deliverable types: ARCHITECTURE, PRD, use case, test suite, engineering guideline, specification

### 3. Execution Constitution

**Phase:** Implementing tasks (stitch)

**Location:** Embedded in orchestrator binary (`pkg/orchestrator/constitutions/execution.yaml`)

**Contains:** Go coding standards, design patterns, traceability, session completion, quality gates

**Used by:** Stitch prompt to ensure code quality and project conventions

The execution constitution includes:

- Articles E1-E5: Specification-first, traceability, no scope creep, session completion, quality gates
- Coding standards: Copyright headers, no duplication, design patterns (Strategy, Command, Facade, etc.)
- Project structure: cmd/, internal/, pkg/, tests/, magefiles/
- Error handling, concurrency, testing, naming conventions
- Session completion workflow and git conventions

When you run `mage scaffold`, the design constitution is automatically copied to the consuming project. The planning and execution constitutions are embedded in the orchestrator binary and injected into the measure (`pkg/orchestrator/prompts/measure.tmpl`) and stitch (`pkg/orchestrator/prompts/stitch.tmpl`) prompt templates respectively.

## Configuration

All options live in `configuration.yaml` at the repository root. For consuming projects, the orchestrator provides a scaffold command that detects project structure and generates configuration.yaml automatically:

```bash
mage test:scaffold /path/to/your/project
```

Alternatively, create `configuration.yaml` manually and set the project-specific fields (`module_path`, `binary_name`, `main_package`, `go_source_dirs`, `podman_image`).

### Configuration Reference

| Field | Default | Description |
|-------|---------|-------------|
| module_path | (required) | Go module path |
| binary_name | (required) | Compiled binary name |
| binary_dir | bin | Output directory for binaries |
| main_package | (required) | Path to main.go entry point |
| go_source_dirs | (required) | Directories with Go source files |
| version_file | | Path to version.go; updated by generator:stop with the version tag |
| magefiles_dir | magefiles | Directory skipped when deleting Go files |
| spec_globs | {} | Label to glob pattern map for word-count stats (e.g., `prd: "docs/specs/product-requirements/*.yaml"`) |
| seed_files | {} | Destination to template source paths; templates are rendered with Version and ModulePath during reset |
| gen_prefix | generation- | Prefix for generation branch names |
| cycles | 0 | Max measure+stitch cycles per run; 0 means run until all issues are closed |
| generation_branch | | Specific generation branch to work on; auto-detected if empty |
| cleanup_dirs | [] | Directories to remove after generation stop or reset |
| cobbler_dir | .cobbler/ | Cobbler scratch directory |
| beads_dir | .beads/ | Beads database directory |
| max_stitch_issues | 0 | Total maximum stitch iterations for an entire run; 0 means unlimited |
| max_stitch_issues_per_cycle | 10 | Maximum tasks stitch processes before calling measure again |
| max_measure_issues | 1 | Maximum new issues to create per measure pass |
| user_prompt | | Additional context for the measure prompt |
| measure_prompt | pkg/orchestrator/prompts/measure.tmpl | File path to custom measure prompt template (defaults to embedded template) |
| stitch_prompt | pkg/orchestrator/prompts/stitch.tmpl | File path to custom stitch prompt template (defaults to embedded template) |
| estimated_lines_min | 250 | Minimum estimated lines per task (passed to measure template) |
| estimated_lines_max | 350 | Maximum estimated lines per task (passed to measure template) |
| podman_image | (required) | Container image for Claude execution |
| podman_args | [] | Additional podman run arguments |
| claude_max_time_sec | 300 | Maximum seconds per Claude invocation; process killed on expiry |
| claude_args | (see below) | CLI arguments for Claude execution |
| silence_agent | true | Suppress Claude stdout |
| secrets_dir | .secrets | Directory containing token files |
| default_token_file | claude.json | Default credential filename |
| token_file | | Override credential filename |

Default claude_args: `--dangerously-skip-permissions -p --verbose --output-format stream-json`

## Quick Start

In your consuming project's Magefile:

```go
package main

import orchestrator "github.com/mesh-intelligence/mage-claude-orchestrator/pkg/orchestrator"

var o *orchestrator.Orchestrator

func init() {
    var err error
    o, err = orchestrator.NewFromFile("configuration.yaml")
    if err != nil {
        panic(err)
    }
}

func GeneratorStart() error { return o.GeneratorStart() }
func GeneratorRun() error   { return o.GeneratorRun() }
func GeneratorStop() error  { return o.GeneratorStop() }
```

Then run:

```bash
# Edit configuration.yaml: set module_path, binary_name, podman_image, etc.
mage generator:start       # Create generation branch from main
mage generator:run         # Run measure+stitch cycles
mage generator:stop        # Merge generation into main
```

If a run is interrupted, `mage generator:resume` recovers state and continues. To discard a generation, `mage generator:reset` returns to a clean main.

## Mage Targets

| Target | Description |
|--------|-------------|
| init | Initialize the project (beads issue tracker) |
| reset | Full reset: cobbler, generator, beads |
| stats | Print Go LOC and documentation word counts as JSON |
| build | Compile the project binary |
| lint | Run golangci-lint |
| install | Run go install for the main package |
| clean | Remove build artifacts |
| credentials | Extract Claude credentials from macOS Keychain |
| test:unit | Run go test on all packages |
| test:integration | Run go test in tests/ directory |
| test:all | Run unit and integration tests |
| test:scaffold | Scaffold a target repository for testing |
| test:cobbler | Full cobbler regression suite (requires Claude) |
| test:generator | Full generator lifecycle suite (requires Claude) |
| test:resume | Resume recovery test (requires Claude) |
| cobbler:measure | Assess project state and propose tasks via Claude |
| cobbler:stitch | Pick ready tasks and execute them in worktrees |
| cobbler:reset | Remove cobbler scratch directory |
| generator:start | Begin a new generation (create branch from main) |
| generator:run | Execute measure+stitch cycles within current generation |
| generator:resume | Recover from interrupted run and continue |
| generator:stop | Complete generation and merge into main |
| generator:list | Show active branches and past generations |
| generator:switch | Commit work and check out another generation branch |
| generator:reset | Destroy generation branches and return to clean main |
| beads:init | Initialize beads issue tracker |
| beads:reset | Clear beads issue history |

## Claude Code Skills

The orchestrator ships with Claude Code slash commands for interactive workflows.

| Skill | Description |
|-------|-------------|
| /bootstrap | Initialize a new project: ask clarifying questions, create epics and issues |
| /make-work | Analyze project state and propose next work based on roadmap priorities |
| /do-work | Route to /do-work-docs or /do-work-code based on the issue type |
| /do-work-docs | Documentation workflow: pick a docs issue, write the deliverable per format rules, close the issue |
| /do-work-code | Code workflow: pick a code issue, read PRDs, implement, test, close the issue |
| /test-clone | Test the orchestrator by scaffolding a target repository and running the test plan |

## License

MIT
