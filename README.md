# mage-claude-orchestrator

Go library for automating AI code generation via Claude Code. Consuming projects import this library through their Magefile and run generation cycles that propose tasks (measure) and execute them in isolated git worktrees (stitch). Claude runs inside a podman container for process isolation.

## Quick Start

### Prerequisites

Your target project must have:

- ✅ Go module initialized (`go.mod` exists)
- ✅ Git repository initialized
- ✅ On the `main` branch with a clean working tree

### Step 1: Clone the Orchestrator

```bash
git clone https://github.com/mesh-intelligence/mage-claude-orchestrator.git
cd mage-claude-orchestrator
```

### Step 2: Prepare Your Target Repository

```bash
# Navigate to your project
cd /path/to/your/project

# Initialize Go module (if not already done)
go mod init github.com/your-username/your-project

# Initialize git (if not already done)
git init
git add .
git commit -m "Initial commit"

# Ensure you're on main branch
git checkout -b main 2>/dev/null || git checkout main
```

### Step 3: Scaffold Your Project

From the **orchestrator repository**:

```bash
cd /path/to/mage-claude-orchestrator

# Absolute path
mage test:scaffold /path/to/your/project

# OR relative path (both work)
mage test:scaffold ../your-project
```

The scaffold automatically:

- Copies `orchestrator.go` to `magefiles/orchestrator.go` in your project
- Detects project structure (module path, main package, source directories)
- Generates `configuration.yaml` with detected settings
- Wires `magefiles/go.mod` with orchestrator dependency
- Copies design constitution to `docs/constitutions/design.yaml`
- Creates version template if main package detected

### Step 4: Initialize and Run

In **your target repository**:

```bash
cd /path/to/your/project

# Initialize beads issue tracker
mage init

# Configure Claude credentials (see Configuration section below)
# Edit configuration.yaml and add your Claude API key or session

# Start your first generation
mage generator:start       # Create generation branch from main
mage generator:run         # Run measure+stitch cycles
mage generator:stop        # Merge generation into main
```

If a run is interrupted, `mage generator:resume` recovers state and continues. To discard a generation, `mage generator:reset` returns to a clean main.

### Files Created by Scaffold

```text
your-project/
├── configuration.yaml          # Auto-generated config
├── docs/
│   └── constitutions/
│       ├── design.yaml         # Format rules for specs (editable)
│       ├── planning.yaml       # Measure phase rules (editable)
│       └── execution.yaml      # Stitch phase rules (editable)
└── magefiles/
    ├── orchestrator.go         # Mage targets (template from orchestrator repo)
    ├── version.go.tmpl         # Seed template (if main package detected)
    ├── go.mod                  # Separate module for build tooling
    └── go.sum
```

The `magefiles/` directory keeps build tooling dependencies separate from your project dependencies.

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

## Specification Approach

All project specifications are written in YAML, not in prose documents or external tooling such as [spec-kit](https://github.com/github/spec-kit). YAML is preferred because it is structured, machine-readable, diff-friendly, and unambiguous — properties that matter when Claude is reading and generating specifications autonomously.

### Document hierarchy

```text
docs/
├── VISION.yaml                 # Goals, personas, release definitions
├── ARCHITECTURE.yaml           # Components, interfaces, protocols, data flows
├── road-map.yaml               # Releases and which use cases each delivers
└── specs/
    ├── product-requirements/   # prd-*.yaml  — feature requirements
    └── use-cases/              # uc-NNN-slug.yaml  — concrete user flows

tests/
└── rel-NNN/                    # One directory per release
    └── rel-NNN_test.go         # Use cases as sub-tests (TestRelNNN/uc-NNN-slug)
```

### Releases and use cases

Release membership is tracked in `docs/road-map.yaml`, maintained during the design phase alongside VISION and ARCHITECTURE. Each release entry lists which use case IDs it delivers. Use cases themselves are named `uc-NNN-slug.yaml` with a stable numeric ID that does not encode the release — re-prioritizing a use case to a later release does not rename the file. `road-map.yaml` is the single source of truth for release membership.

### Use cases order development

The measure phase reads `road-map.yaml` to determine which release is next and proposes tasks that advance the use cases for that release. This keeps the generation cycle tied to the product roadmap rather than to an ad-hoc backlog.

### Testing

Tests are structured in two tiers:

- **Unit tests** live alongside the code they test (`pkg/*/`) and correspond to PRDs. A unit test exercises the behaviour specified by a PRD, and its commit message references the PRD (e.g. `prd-feature-name`).
- **Release acceptance tests** live in `tests/rel-NNN/` — one directory per release. Each file runs the use cases for that release as Go sub-tests (`t.Run("uc-NNN-slug", ...)`), providing a tracer-bullet check that the full release is working.

## Three-Phase Constitution Architecture

The orchestrator uses three constitutions aligned with the three workflow phases:

### 1. Design Constitution

**Phase:** Interactive design/architecting (writing VISION, ARCHITECTURE, PRDs, use cases, test suites)

**Location:** Scaffolded to consuming projects as `docs/constitutions/design.yaml`

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

Alternatively, create `configuration.yaml` manually and set the project-specific fields (`project.module_path`, `project.binary_name`, `project.main_package`, `project.go_source_dirs`, `podman.image`).

### Configuration Reference

Configuration is hierarchical. Top-level sections: `project`, `generation`, `cobbler`, `podman`, `claude`.

#### project

| Field | Default | Description |
|-------|---------|-------------|
| project.module_path | (required) | Go module path |
| project.binary_name | (required) | Compiled binary name |
| project.binary_dir | bin | Output directory for binaries |
| project.main_package | (required) | Path to main.go entry point |
| project.go_source_dirs | (required) | Directories with Go source files |
| project.version_file | | Path to version.go; updated by generator:stop with the version tag |
| project.magefiles_dir | magefiles | Directory skipped when deleting Go files |
| project.spec_globs | {} | Label to glob pattern map for word-count stats (e.g., `prd: "docs/specs/product-requirements/*.yaml"`) |
| project.seed_files | {} | Destination to template source paths; templates are rendered with Version and ModulePath during reset |

#### generation

| Field | Default | Description |
|-------|---------|-------------|
| generation.prefix | generation- | Prefix for generation branch names |
| generation.cycles | 0 | Max measure+stitch cycles per run; 0 means run until all issues are closed |
| generation.branch | | Specific generation branch to work on; auto-detected if empty |
| generation.cleanup_dirs | [] | Directories to remove after generation stop or reset |

#### cobbler

| Field | Default | Description |
|-------|---------|-------------|
| cobbler.dir | .cobbler/ | Cobbler scratch directory |
| cobbler.beads_dir | .beads/ | Beads database directory |
| cobbler.max_stitch_issues | 0 | Total maximum stitch iterations for an entire run; 0 means unlimited |
| cobbler.max_stitch_issues_per_cycle | 10 | Maximum tasks stitch processes before calling measure again |
| cobbler.max_measure_issues | 1 | Maximum new issues to create per measure pass |
| cobbler.user_prompt | | Additional context for the measure prompt |
| cobbler.measure_prompt | | File path to custom measure prompt template (defaults to embedded template) |
| cobbler.stitch_prompt | | File path to custom stitch prompt template (defaults to embedded template) |
| cobbler.planning_constitution | docs/constitutions/planning.yaml | File path to planning constitution; overrides embedded default |
| cobbler.execution_constitution | docs/constitutions/execution.yaml | File path to execution constitution; overrides embedded default |
| cobbler.design_constitution | docs/constitutions/design.yaml | File path to design constitution; overrides embedded default |
| cobbler.estimated_lines_min | 250 | Minimum estimated lines per task (passed to measure template) |
| cobbler.estimated_lines_max | 350 | Maximum estimated lines per task (passed to measure template) |

#### podman

| Field | Default | Description |
|-------|---------|-------------|
| podman.image | (required) | Container image for Claude execution |
| podman.args | [] | Additional podman run arguments |

#### claude

| Field | Default | Description |
|-------|---------|-------------|
| claude.args | (see below) | CLI arguments for Claude execution |
| claude.silence_agent | true | Suppress Claude stdout |
| claude.secrets_dir | .secrets | Directory containing token files |
| claude.default_token_file | claude.json | Default credential filename |
| claude.token_file | | Override credential filename |
| claude.max_time_sec | 300 | Maximum seconds per Claude invocation; process killed on expiry |

Default `claude.args`: `--dangerously-skip-permissions -p --verbose --output-format stream-json`

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
| analyze | Check cross-artifact consistency (orphaned PRDs, missing test suites, broken references) |
| tag | Create a release tag (v0.YYYYMMDD.N) and build container image |
| uninstall | Remove orchestrator-managed files (magefiles/orchestrator.go, docs/constitutions/, configuration.yaml) |
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

After `mage tag`, push the commit and tag to the remote:

```bash
git push release main && git push release main --tags
```

## Claude Code Skills

The orchestrator ships with Claude Code slash commands for interactive workflows.

| Skill | Description |
|-------|-------------|
| /bootstrap | Initialize a new project: ask clarifying questions, write VISION.yaml and ARCHITECTURE.yaml |
| /make-work | Analyze project state and propose next work based on roadmap priorities |
| /do-work | Route to /do-work-docs or /do-work-code based on the issue type |
| /do-work-docs | Documentation workflow: pick a docs issue, write the deliverable per format rules, close the issue |
| /do-work-code | Code workflow: pick a code issue, read PRDs, implement, test, close the issue |
| /test-clone | Test the orchestrator by scaffolding a target repository and running the test plan |

## License

MIT
