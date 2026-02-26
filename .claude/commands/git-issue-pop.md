<!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

Pop a GitHub issue from the current repository, decompose it into a local beads epic with sub-issues on a feature branch, and open a PR when the epic is complete.

## Input

$ARGUMENTS

If arguments contain an issue number (e.g. `42` or `#42`), use that issue. If arguments contain a URL, extract the issue number. If no number is given, list open issues and ask the user to pick one.

## Phase 0 -- Detect Repository

1. Run `gh repo view --json nameWithOwner -q .nameWithOwner` and use the result as `<owner>/<repo>` for all `gh` commands below.

## Phase 1 -- Fetch the GitHub Issue

1. Fetch the issue:
   ```
   gh issue view <number> --repo <owner>/<repo> --json number,title,body,labels,state
   ```
2. If the issue is not open, stop and report its state.
3. Display the issue title, body, and labels to the user.

## Phase 2 -- Gather Project Context

Same as `/make-work`:

1. Read VISION.yaml, ARCHITECTURE.yaml, road-map.yaml, and `docs/constitutions/design.yaml`.
2. Read READMEs for product requirements and use cases relevant to the issue.
3. Run `bd list` to see existing epics and issues.
4. Run `mage analyze` to identify spec issues.
5. Run `mage stats` for current LOC and documentation metrics.
6. Summarize the current project state.

## Phase 3 -- Propose Epic and Issues

Using the GitHub issue as the work item, propose:

1. A beads epic whose title matches the GitHub issue title, prefixed with the GitHub issue number (e.g. `GH-42: Feature title`).
2. Sub-issues that decompose the GitHub issue into actionable work, following the same crumb-format rules as `/make-work`:
   - Type: documentation or code
   - Required Reading: mandatory list of files the agent must read
   - Files to Create/Modify: explicit file list
   - Structure: Requirements, Design Decisions (optional), Acceptance Criteria
   - Code task sizing: 300-700 lines of production code, no more than 5 files
   - No more than 10 sub-issues

3. Present the proposed breakdown to the user for approval. Do not create anything until the user agrees.

## Phase 4 -- Create Branch and Work Items

After user approval:

1. Ensure `main` is clean and up to date:
   ```
   git checkout main
   git stash --include-untracked  # if needed
   ```

2. Create a feature branch from main:
   ```
   git checkout -b gh-<number>-<slug>
   ```
   Where `<slug>` is a short kebab-case summary of the issue title (e.g. `gh-42-add-scaffold-validation`).

3. Create the epic:
   ```
   bd create "GH-<number>: <title>" --type epic --description "<GitHub issue body + link>"
   ```
   Include in the epic description:
   - The full GitHub issue body
   - A reference line: `GitHub: <owner>/<repo>#<number>`

4. Create sub-issues under the epic:
   ```
   bd create "<sub-issue title>" --parent <epic-id> --type <documentation|code> --description "<structured description>"
   ```

5. Sync and commit on the feature branch:
   ```
   bd sync
   git add -A
   git commit -m "Pop GH-<number>: <title> into local epic"
   ```

6. Push the branch:
   ```
   git push -u origin gh-<number>-<slug>
   ```

All subsequent `/do-work` happens on this branch. Before starting work, verify you are on the correct branch:
```
git branch --show-current  # should show gh-<number>-<slug>
```

## Phase 5 -- Open a Pull Request

When ALL sub-issues in the epic are closed (check with `bd epic close-eligible`):

1. Close the beads epic:
   ```
   bd epic close-eligible
   ```

2. Final commit on the feature branch:
   ```
   bd sync
   git add -A
   git commit -m "Complete GH-<number>: <title>"
   git push
   ```

3. Open a pull request against `main`:
   ```bash
   gh pr create --repo <owner>/<repo> \
     --base main \
     --head gh-<number>-<slug> \
     --title "GH-<number>: <title>" \
     --body "$(cat <<'EOF'
   ## Summary

   <2-3 sentence summary of what this epic delivered>

   ## Changes

   <bulleted list of sub-issues completed and what each produced>

   ## Stats

   <output of mage stats with deltas from start of epic>

   ## Test plan

   - [ ] `mage analyze` passes
   - [ ] All tests pass
   - [ ] Documentation reviewed for consistency

   Closes #<number>
   EOF
   )"
   ```

   The `Closes #<number>` line auto-closes the GitHub issue when the PR merges.

4. Merge the pull request and delete the remote feature branch:

   ```bash
   gh pr merge --repo <owner>/<repo> --merge --delete-branch
   ```

5. Return to main and pull the merged changes:

   ```bash
   git checkout main
   git pull origin main
   ```

6. Delete the local feature branch (now merged):

   ```bash
   git branch -d gh-<number>-<slug>
   ```

7. Verify the GitHub issue was closed by the merge:

   ```bash
   gh issue view <number> --repo <owner>/<repo> --json state -q .state
   ```

   If the issue is still open, close it explicitly:

   ```bash
   gh issue close <number> --repo <owner>/<repo> --comment "Completed via PR #<pr-number>"
   ```

8. Report the PR URL and confirm the issue is closed.

9. If the issue is recurring (see Phase 6), execute Phase 6 now.

**Note:** Phase 5 may happen in a later session. When running `/do-work` and completing the last issue in an epic that has a `GH-` prefix in its title, check if the epic is close-eligible and execute Phase 5 automatically.

## Phase 6 -- Re-create Recurring Issues

A GitHub issue is recurring if its title starts with "Recurring:" or its body contains a "## Recurrence" section. After Phase 5 closes a recurring issue, re-create it so the next run can pick it up.

1. Detect recurrence: check whether the original issue title starts with `Recurring:` or the body contains `## Recurrence`.

2. If recurring, create a new issue with the same title, labels, and body as the original, except update the "Previous Runs" or "Previous Audits" section to append a line referencing the just-closed issue:
   ```
   - #<number> (<date>): <one-line summary of what this run produced>. PR #<pr-number>.
   ```

3. Create the new issue:
   ```bash
   gh issue create --repo <owner>/<repo> \
     --title "<same title>" \
     --label "<same labels, comma-separated>" \
     --body "<updated body>"
   ```

4. Report the new issue URL so the user knows the recurring issue is ready for the next run.
