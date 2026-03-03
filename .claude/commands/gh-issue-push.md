Create a GitHub issue in the current repository.

## Input

$ARGUMENTS

## Steps

1. Detect the repo: run `gh repo view --json nameWithOwner -q .nameWithOwner` and use the result as `<owner>/<repo>` for all `gh` commands.
2. Determine type from the input: keywords like "bug", "fix", "broken", "crash" → bug; otherwise → enhancement.
3. Draft a concise title and well-structured body.
   - **Bug**: problem, expected vs actual behavior, reproduction steps if provided.
   - **Enhancement**: description and acceptance criteria.
4. Create the issue:
   ```
   gh issue create --repo <owner>/<repo> --title "<title>" --body "<body>" --label "<bug|enhancement>"
   ```
5. Report the issue URL.
