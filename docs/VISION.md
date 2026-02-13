# Mage Claude Orchestrator Vision

## Executive Summary

Mage Claude Orchestrator is a Go library that automates multi-step AI-driven code generation workflows. We integrate with Magefile build automation to orchestrate Claude Code sessions that analyze projects, propose tasks, execute them in isolated git worktrees, and merge results back to the main branch. We are not an IDE plugin, a general-purpose agent framework, or a workflow engine. We are build tooling that turns a specification-driven codebase into a software factory.

## The Problem

AI coding assistants work well for single-file edits and short conversations, but struggle with sustained, multi-task development sessions. A developer who wants Claude to implement ten related features faces a manual loop: describe each task, monitor execution, review changes, commit, and repeat. The tasks often depend on each other, meaning the developer must sequence them correctly and recover from failures mid-session.

Git adds another layer of complexity. Running Claude directly on a working branch risks polluting the commit history with half-finished work. Developers resort to manual worktree management or feature branches, adding overhead that compounds with each task.

Existing solutions approach this piecemeal. Some tools wrap Claude in a script loop but ignore git isolation. Others manage branches but lack task proposal intelligence. None combine project analysis, task decomposition, isolated execution, and automated merging into a single workflow that a consuming project can invoke through its build system.

## What This Does

We solve this with a two-phase orchestration loop that we call the cobbler workflow, after the fairy tale where elves make shoes overnight.

The **measure** phase analyzes the project state. We read existing documentation (VISION, ARCHITECTURE, PRDs, use cases), query the issue tracker for current work items, and invoke Claude to propose new tasks. Claude writes a structured JSON file with task titles, descriptions, and dependency indices. We import these tasks into the beads issue tracker, wiring up dependencies between them.

The **stitch** phase executes ready tasks. For each task, we create a git worktree on a dedicated task branch, build a prompt from the task description, invoke Claude in the worktree, merge the task branch back to the generation branch, record metrics (tokens used, lines of code changed, git diff stats), and close the task in the issue tracker.

The **generation** lifecycle wraps multiple measure-stitch cycles. A generation starts by tagging the current main state, creating a generation branch, and resetting Go sources to a clean state. Cycles of measure and stitch build up the codebase on the generation branch. When the generation is complete, we merge it back to main, tag the result, and clean up.

Consuming projects import the orchestrator as a Go library and expose its methods as Mage targets. A project configures its module path, source directories, seed files, and prompt templates. The orchestrator handles everything else: git branch management, worktree isolation, Claude invocation (via container or direct binary), issue tracking, and metrics collection.

## Why We Build This

We build this because AI-assisted development at scale requires tooling that understands the development lifecycle, not just the editor. The orchestrator sits at the intersection of build automation (Mage), version control (git), issue tracking (beads), and AI execution (Claude), connecting them into a repeatable workflow.

We already use this orchestrator to build Crumbs, our work-item storage system. The orchestrator proposes tasks based on Crumbs' PRDs and use cases, executes them, and tracks progress. This self-reinforcing loop validates the orchestrator's design with real usage.

Table 1 Relationship to Other Projects

| Project | Role |
|---------|------|
| Crumbs | Consuming project; provides the Cupboard CLI and storage library |
| Beads | Issue tracker used by the orchestrator to track tasks |
| Mage | Build system; orchestrator methods are exposed as Mage targets |

## What Success Looks Like

We measure success along three dimensions.

### Automation Efficiency

A single `mage generator:run --cycles N` command produces working code from specifications. Each cycle proposes tasks aligned with the roadmap and executes them without human intervention. Recovery from interrupted runs (`generator:resume`) restores state and continues without data loss.

### Code Quality

Generated code conforms to project PRDs and architecture. Commits reference the tasks and PRDs they implement. Metrics (LOC, tokens, diff stats) are recorded on every task for auditability.

### Developer Experience

Consuming projects integrate the orchestrator by creating a Config struct and calling `New()`. The orchestrator handles git, containers, and issue tracking internally. Customization is available through prompt templates, seed files, and configuration fields. The developer's interaction surface is Mage targets, not orchestrator internals.

## Implementation Phases

Table 2 Implementation Phases

| Phase | Focus | Deliverables |
|-------|-------|--------------|
| 01.0 | Core orchestrator | Config, Orchestrator struct, logging, flag parsing |
| 01.1 | Cobbler workflows | Measure and stitch phases, prompt templates, Claude invocation |
| 02.0 | Generation lifecycle | Start, run, resume, stop, reset, list, switch |
| 02.1 | Container execution | Podman/Docker detection, image building, container-based Claude execution |
| 03.0 | Metrics and tracking | Stats collection, invocation records, LOC snapshots, beads integration |

## Risks and Mitigations

Table 3 Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Claude produces invalid code that fails to compile | Stitch task fails, blocking generation progress | Medium | Recovery mechanism resets failed tasks to ready; next cycle retries |
| Git merge conflicts between task branches | Task merge fails, requiring manual intervention | Low | Worktree isolation limits conflict surface; tasks are small and focused |
| Container runtime unavailable on developer machine | Cannot run Claude in isolation | Low | Fallback to direct claude binary; --no-container flag |
| Beads issue tracker corruption | Task state lost | Low | Beads syncs to JSONL on every write; git tracks all changes |

## What This Is NOT

We are not building an agent framework. We do not define agent capabilities, tool use policies, or multi-agent communication. Claude is a black box that receives a prompt and produces code.

We are not building a CI/CD pipeline. We run locally on developer machines. Deployment, testing infrastructure, and release automation are out of scope.

We are not building a project management tool. We use beads for issue tracking but do not provide dashboards, sprint planning, or team coordination.

We are not building a code review system. The orchestrator merges code automatically. Review happens before generation (through PRDs and architecture docs) and after (through human inspection of the merged result).

We are not building a general-purpose build system. We extend Mage with AI orchestration targets. The consuming project defines its own build targets for compilation, testing, and packaging.
