<!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

# Generation Workflow Conventions

## Introduction

We use generations to isolate AI-driven code creation from the main branch. A generation is a named branch where measure-stitch cycles build up the codebase from specifications. This guideline documents the conventions for generation naming, task branch naming, tagging, and the workflow developers follow when using the orchestrator.

## Generation Branch Naming

We name generation branches with a prefix and timestamp: `{GenPrefix}{YYYY-MM-DD-HH-MM-SS}`. The default prefix is `generation-`. Examples: `generation-2026-02-13-09-30-00`, `generation-2026-02-14-14-15-30`.

The timestamp captures when the generation was started. We do not reuse generation names. Each `generator:start` creates a unique branch.

## Tag Conventions

We tag lifecycle events on generation branches. Tags follow the pattern `{generationName}-{suffix}`.

Table 1 Generation Tag Suffixes

| Suffix | When Created | Meaning |
|--------|--------------|---------|
| -start | GeneratorStart | Main state before generation began |
| -finished | GeneratorStop (before merge) | Final state of the generation branch |
| -merged | GeneratorStop (after merge) | Main state after successful merge |
| -abandoned | GeneratorReset | Generation was never merged |

We also create versioned tags on main after a successful merge: `v{YYYY-MM-DD}-code` (at the merged commit) and `v{YYYY-MM-DD}-requirements` (at the start tag). These enable comparing the specification state with the generated code state.

## Task Branch Naming

Each stitch task runs on a branch named `task/{baseBranch}-{issueID}`. The base branch is usually the generation branch. Examples: `task/generation-2026-02-13-09-30-00-abc123`, `task/generation-2026-02-13-09-30-00-def456`.

We use `task/` as a prefix (not `{baseBranch}/task/`) to avoid git ref conflicts when the base branch is `main`.

## Worktree Locations

Worktrees live in a temporary directory: `$TMPDIR/{repoName}-worktrees/{issueID}`. We do not create worktrees inside the repository to avoid confusing git and cluttering the project directory. Worktrees are cleaned up after each task merge.

## Developer Workflow

A typical generation session follows this sequence.

1. **Start**: `mage generator:start` creates the generation branch from main.
2. **Run**: `mage generator:run --cycles N` executes N measure+stitch cycles.
3. **Monitor**: `mage generator:list` shows active and past generations.
4. **Stop**: `mage generator:stop` merges the generation into main.

If a run is interrupted, `mage generator:resume` recovers state and continues. If a generation is no longer needed, `mage generator:reset` returns to a clean main.

## Offline Work

We work offline. All operations are local git commits. We do not push during generation. The developer pushes when they have network access. See cupboard-workflow for the full offline protocol.
