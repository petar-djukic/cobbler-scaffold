<!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

# Command: Test Clone

Clone a repository into an isolated directory, install the orchestrator's mage infrastructure, and run the test suite. Monitor failures, fix code, and re-run until all tests pass. Delete the directory when done.

## Arguments

$ARGUMENTS is the Git repository URL or local path to clone. If a second argument is provided, it is the branch or ref to check out after cloning.

Examples:
- `/test-clone https://github.com/org/project`
- `/test-clone /path/to/local/repo`
- `/test-clone https://github.com/org/project feature-branch`

## Workflow

### 1. Parse arguments

Extract the repository URL (first argument) and optional branch (second argument) from `$ARGUMENTS`. If no arguments are provided, ask the user for the repository URL.

### 2. Create isolated workspace

```bash
WORK_DIR=$(mktemp -d -t test-clone-XXXXXX)
echo "Test workspace: $WORK_DIR"
```

### 3. Clone the repository

```bash
git clone <repo-url> "$WORK_DIR/repo"
cd "$WORK_DIR/repo"
```

If a branch was specified, check it out:
```bash
git checkout <branch>
```

### 4. Strip git history and initialize fresh repository

Remove the cloned git history and create a clean single-commit repo. This ensures the orchestrator's git-based operations (generation branches, tags, worktrees) start from a known state without interference from the upstream history.

```bash
rm -rf .git
git init
git add -A
git commit -m "Initial commit from test-clone"
```

### 5. Install mage and orchestrator actions

Ensure the mage build tool is available:

```bash
which mage || go install github.com/magefile/mage@latest
```

The cloned repository should have a `magefiles/` directory that imports the orchestrator library (`github.com/mesh-intelligence/mage-claude-orchestrator/pkg/orchestrator`). Point the dependency at the local development checkout so the tests exercise the current orchestrator code:

```bash
ORCHESTRATOR_DIR="$(pwd)"  # save for later — this is the orchestrator repo root
cd "$WORK_DIR/repo"
```

If the repo's `magefiles/go.mod` (or root `go.mod`) references the orchestrator module, add a replace directive:

```bash
# In whichever go.mod imports the orchestrator
go mod edit -replace github.com/mesh-intelligence/mage-claude-orchestrator=<path-to-local-orchestrator-checkout>
go mod tidy
```

Verify mage can discover targets:

```bash
mage -l
```

If the repository does not have magefiles, report this to the user and stop — the test-clone skill requires a project that uses the orchestrator's mage targets.

### 6. Run initial commit and verify build

Before running the full test suite, ensure the project compiles:

```bash
mage build
```

If this fails, fix the build errors first before proceeding to tests.

### 7. Run the test suite

Execute the test targets in order. Track which targets pass and which fail.

```bash
mage lint
mage test:unit
mage test:integration
mage test:all
```

If the project has a `test-plan.yaml` at the root, read it and use it as the authoritative list of test cases. Run each test case from the plan.

If the project has a `mage test:cobbler` or `mage test:generator` target (integration tests requiring Claude), skip them by default. Only run them if the user explicitly requested Claude-dependent tests.

### 8. Fix failures and re-run

For each failing test:

1. Read the error output carefully
2. Identify the root cause (compilation error, logic bug, missing dependency, config issue)
3. Fix the code in `$WORK_DIR/repo`
4. Commit the fix: `git add -A && git commit -m "Fix: <description>"`
5. Re-run the failing test to verify the fix
6. Continue to the next failure

Repeat the full test suite after fixing individual failures to catch regressions. The loop continues until ALL test targets pass.

Keep a running log of fixes applied:

| Fix # | Test | Root cause | Files changed |
|-------|------|------------|---------------|
| 1     | ...  | ...        | ...           |

### 9. Report results

After all tests pass, summarize:

1. Total tests run and passed
2. Number of fixes applied
3. Table of fixes (from step 8)
4. Final `mage stats` output if available

### 10. Clean up

Delete the isolated workspace:

```bash
rm -rf "$WORK_DIR"
echo "Test workspace cleaned up"
```

Report to the user that the test run is complete and the workspace has been deleted.

## Error handling

- If the clone fails, report the error and stop
- If mage cannot be installed, report the error and stop
- If the repo has no magefiles, report this and stop
- If a fix introduces new failures, revert the fix and try a different approach
- If you cannot fix a test after 3 attempts, log it as unfixable, skip it, and continue with other tests. Report all unfixable tests in the final summary.
- If you need to make changes to the orchestrator library (not just the cloned project), note this in the summary but do NOT modify the orchestrator source. The fix should go into the cloned project only.

## Important

- All work happens inside `$WORK_DIR`. Do not modify files outside the workspace.
- The orchestrator source (this repository) is read-only during the test run.
- Do not push any changes. Everything is local.
- The workspace is ephemeral — it will be deleted at the end.
