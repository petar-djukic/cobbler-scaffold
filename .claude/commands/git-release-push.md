Run the full release workflow: analyze, test, tag, and push.

## Steps

1. Run `mage analyze`. If it fails, fix the problems and re-run (up to 3 attempts). Stop if still failing after 3 attempts.
2. Run `mage test:unit`. If it fails, fix the problems and re-run (up to 3 attempts). Stop if still failing after 3 attempts.
3. Run `mage test:uc`. If it fails, fix the problems and re-run (up to 3 attempts). Stop if still failing after 3 attempts.
4. Run `mage tag` to create a `v0.YYYYMMDD.N` tag. Capture the tag name from the output or via `git describe --tags --abbrev=0`.
5. Push the current branch to `origin`. If `git remote | grep -q release` succeeds, also push the branch to `release`.
6. Push tags to `origin` with `git push origin --tags`. If the `release` remote exists, also push tags to `release` with `git push release --tags`.
7. Report the tag name, branch, and which remotes received the push.
