<!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

# Command: Do Work

Pick **one** of the two workflows below depending on the deliverable type. Use **Documentation workflow** for docs under `docs/`, **Code workflow** for implementation under `pkg/`, `internal/`, `cmd/`.

## Task Priority

When selecting from available issues, **prefer documentation issues over code issues**. Documentation establishes the design before implementation begins. Complete PRDs, use cases, and architecture updates before moving to code tasks.

## How to Choose

Run `bd ready` and look at the issue description:

| Deliverable      | Workflow                                                  | Indicators                                                                                                                                                                                          |
|------------------|-----------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Documentation** | [Documentation Workflow](#documentation-workflow)         | Output path under `docs/` (PRD, use case, test suite, ARCHITECTURE, engineering guideline, SPECIFICATIONS); has "Required sections", "Format rule", or doc format name                            |
| **Code**          | [Code Workflow](#code-workflow)                           | Output under `pkg/`, `internal/`, `cmd/`; has Requirements, Design Decisions, tests or observable behaviour in Acceptance Criteria                                                                |

---

## Documentation Workflow

Use this workflow when the deliverable is **YAML documentation** under `docs/`: PRDs, use cases, test suites, ARCHITECTURE, engineering guidelines, SPECIFICATIONS.

The issue `deliverable_type` field will be `documentation` and will specify a `format_rule` (e.g., prd-format, use-case-format, architecture-format).

Read VISION.yaml and ARCHITECTURE.yaml for context. For PRDs scan existing `docs/specs/product-requirements/`; for use cases `docs/specs/use-cases/`; for test suites (release-level YAML specs) `docs/specs/test-suites/`; for generated Go tests `tests/`.

## 1. Select a documentation task

1. Run `bd ready` to see available work
2. **Pick a documentation issue**: one whose description specifies `deliverable_type: documentation` and an output path under `docs/` (e.g., `docs/specs/product-requirements/prd*.yaml`, `docs/specs/use-cases/rel*-uc*-*.yaml`, `docs/ARCHITECTURE.yaml`)
3. Run `bd update <issue-id> --status in_progress` to claim it

## 2. Before writing

1. **Read the issue** and note:
   - **Output path** (exact file, e.g., `docs/specs/product-requirements/prd-feature.yaml`)
   - **Format rule** (e.g., prd-format, use-case-format, architecture-format)
   - **Required fields** or sections from the format rule
   - **Scope or content hints** (Problem, Goals, requirements, non-goals, etc.)

2. **Read the format rule** from `docs/constitutions/design.yaml` (document_types section) and follow its structure

3. If the doc references or extends existing content (e.g., ARCHITECTURE, another PRD), read the relevant sections so the new doc is consistent

## 3. Write the doc

1. Produce the deliverable at the **exact output path** given in the issue
2. Include all **required fields** from the format rule (see design.yaml document_types)
3. Follow **documentation standards** from design.yaml (concise, active voice, no forbidden terms)
4. For diagrams: define Mermaid inline in markdown using fenced code blocks. Do not create separate image files
5. Verify the issue **Acceptance Criteria**

## 4. After writing

1. **Check completeness** against the issue Acceptance Criteria and the format rule checklist
2. **Run `mage analyze`** to validate the documentation:
   - No orphaned PRDs (all PRDs referenced by use cases)
   - No releases without test suites (all releases in road-map.yaml have a test-rel-*.yaml)
   - No broken references (touchpoints reference valid PRDs)
   - All use cases in roadmap

   Fix any issues before proceeding.

3. **Calculate metrics**: tokens used; run `mage stats` for LOC and doc word counts

4. **Log metrics and close**:

   ```bash
   bd comments add <issue-id> "tokens: <count>"
   bd close <issue-id>
   ```

5. **Commit** changes and `.beads/issues.jsonl`:

   ```bash
   git add -A
   git commit -m "Add <doc name> (<output path>)

   Stats:
     Lines of code (Go, production): <prod_loc> (+<delta>)
     Lines of code (Go, tests):      <test_loc> (+<delta>)
     Words (documentation):          <doc_words> (+<delta>)"
   ```

6. If you found follow-up work, file it in Beads

## 5. After completing an epic (documentation)

When you close the **last issue in an epic** (all child tasks complete):

1. **Review all docs** created or modified during the epic for consistency
2. **Verify epic-level acceptance criteria** (from the epic issue description)
3. **Evaluate use case completion**:
   - Identify which use case(s) this epic contributes to
   - Review success criteria in `docs/specs/use-cases/`
   - If all criteria are met, update road-map.yaml to mark the use case status as "done"
   - If not complete, note what remains and ensure follow-up tasks exist
4. **File follow-up issues** for any gaps discovered
5. **Complete PR workflow for GitHub issue** (if applicable):
   - If the epic title starts with `GH-<number>:`, execute `/git-issue-pop` Phase 5 in full:
     close the beads epic, push the feature branch, open a PR against `main`
     with `Closes #<number>` in the body, merge the PR, delete the feature branch,
     return to `main`, and verify the GitHub issue is closed.

6. **Summarize epic completion**: run `mage stats` and report what was built and use case status

---

## Code Workflow

Use this workflow when the deliverable is **implementation**: packages, internal logic, cmd, workers, tests.

Follow the **code-prd-architecture-linking** rule: code must correspond to existing PRDs and architecture; commits must mention PRDs; add PRD references in code where appropriate (e.g., top of file).

Read VISION.yaml and ARCHITECTURE.yaml for context.

## 1. Select a code task

1. Run `bd ready` to see available work
2. **Pick a code issue**: one whose description specifies `deliverable_type: code` and an implementation deliverable under `pkg/`, `internal/`, `cmd/`; has Requirements and Design Decisions for code; Acceptance Criteria like tests or observable behaviour
3. Run `bd update <issue-id> --status in_progress` to claim it

## 2. Before implementing

1. **Identify related PRDs and docs** from the issue (deliverable path, component, requirements). See `docs/specs/product-requirements/prd*.yaml` and `docs/ARCHITECTURE.yaml`

2. **Read** the relevant sections so behaviour, data shapes, and contracts are clear

3. Read the issue description (Requirements, Design Decisions, Acceptance Criteria) in full

4. **Read existing code** that you will modify or extend:
   - **NEVER propose changes to code you haven't read first**
   - Read files in the target component or package (`pkg/`, `internal/`, `cmd/`)
   - Understand existing patterns, conventions, and interfaces
   - Identify where your changes will fit into the existing structure
   - Check for related test files and understand the testing approach

## 3. Implement

1. Implement according to the issue **Requirements and Design Decisions** and the **related PRDs/architecture**
2. Verify the **Acceptance Criteria** are met (tests, behaviour, observability if specified)
3. Write tests if the issue or PRD specifies them
4. Where appropriate (e.g., package doc or top of file), add a short comment listing **implemented PRDs** (see code-prd-architecture-linking rule)

## 4. After implementation

1. **Run any tests** to verify your work

2. **Calculate metrics**: tokens used; run `mage stats` for LOC deltas

3. **Log metrics and close**:

   ```bash
   bd comments add <issue-id> "tokens: <count>"
   bd close <issue-id>
   ```

4. **Commit** changes and `.beads/issues.jsonl`. **Commit message must mention which PRDs are implemented**:

   ```bash
   git add -A
   git commit -m "Implement X (prd-feature-name, prd-component-name)

   - Description of changes

   Stats:
     Lines of code (Go, production): <prod_loc> (+<delta>)
     Lines of code (Go, tests):      <test_loc> (+<delta>)
     Words (documentation):          <doc_words> (+<delta>)"
   ```

5. If you discovered new work or issues, file them in Beads

## 5. After completing an epic (code)

When you close the **last issue in an epic** (all child tasks complete), perform a **thorough code inspection**:

1. **Read all files** that were created or modified during the epic
2. **Check for inconsistencies**:
   - Naming conventions across files and packages
   - Error handling patterns
   - Code duplication or missed abstractions
   - Test coverage gaps
3. **Verify epic-level acceptance criteria** (from the epic issue description)
4. **Run full test suite** and any integration tests
5. **File follow-up issues** for any technical debt, refactoring, or improvements discovered
6. **Check for doc updates needed**: if implementation revealed design changes or clarifications, **ask the user** before updating architecture or PRD docs
7. **Evaluate use case completion**:
   - Identify which use case(s) this epic contributes to
   - Review success criteria in `docs/specs/use-cases/`
   - If all criteria are met, update road-map.yaml to mark the use case status as "done"
   - If not complete, note what remains and ensure follow-up tasks exist
8. **Complete PR workflow for GitHub issue** (if applicable):
   - If the epic title starts with `GH-<number>:`, execute `/git-issue-pop` Phase 5 in full:
     close the beads epic, push the feature branch, open a PR against `main`
     with `Closes #<number>` in the body, merge the PR, delete the feature branch,
     return to `main`, and verify the GitHub issue is closed.

9. **Summarize epic completion**: run `mage stats` and report:
   - What was built (components, features)
   - Total metrics (tokens, LOC across all child issues)
   - Any deviations from original design
   - Follow-up work filed
   - Use case status (done or remaining work)

---

## Important Notes

- Never edit `.beads/` by hand; use `bd` only
- Always commit `.beads/issues.jsonl` along with your changes
- Track token usage for every issue closed
- **Code workflow**: link code to docs → identify PRDs/architecture → implement to fit → commit with PRD refs → optional PRD list in file/package comments
- **Documentation workflow**: follow format rules from design.yaml → verify completeness → commit with deliverable path
- **Update road-map.yaml** when use cases are completed
- Always run `mage stats` and include full Stats block in commit messages (not condensed format)

## Branch Discipline for GH- Epics

When working on an issue that belongs to a `GH-<number>` epic (created by `/git-issue-pop`):

1. **Verify you are on the correct feature branch** before starting work:
   ```bash
   git branch --show-current  # should show gh-<number>-<slug>
   ```
   If you are on `main`, switch to the feature branch first.

2. **All commits go to the feature branch**, not `main`. Push regularly:
   ```bash
   git push
   ```

3. **When you close the last issue in a GH- epic**, execute `/git-issue-pop` Phase 5 in full:
   - Close the beads epic with `bd epic close-eligible`
   - Push the feature branch, open a PR against `main` with `Closes #<number>` in the body
   - Merge the PR, delete the feature branch, return to `main`, and verify the GitHub issue is closed
