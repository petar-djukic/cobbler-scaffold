// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package release

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// TagParams holds the configuration needed by the Tag function.
type TagParams struct {
	BaseBranch   string
	DocTagPrefix string
	VersionFile  string
}

// Tag creates a documentation-only release tag (v0.YYYYMMDD.N) for the current
// state of the repository. The revision number increments for each tag created
// on the same date. Optionally updates the version file if configured.
func Tag(p TagParams) error {
	// Ensure we're on the configured base branch for doc tags.
	current, err := GitReader.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if current != p.BaseBranch {
		return fmt.Errorf("tag must be run from %s branch (currently on %s)", p.BaseBranch, current)
	}

	// Get today's date in YYYYMMDD format.
	today := time.Now().Format("20060102")

	// Find the next revision for today.
	revision := NextDocRevision(p.DocTagPrefix, today)

	// Create the tag name.
	tag := fmt.Sprintf("%s%s.%d", p.DocTagPrefix, today, revision)

	Log("tag: creating documentation release %s", tag)

	// Create the git tag.
	if err := GitTags.Tag(tag, "."); err != nil {
		return fmt.Errorf("creating tag %s: %w", tag, err)
	}

	// Update the version constant in the version file if configured.
	if p.VersionFile != "" {
		Log("tag: writing version %s to %s", tag, p.VersionFile)
		if err := WriteVersionConst(p.VersionFile, tag); err != nil {
			return fmt.Errorf("tag %s created but version file update failed: %w", tag, err)
		}
		_ = GitCommitter.StageAll(".") // best-effort; commit below handles empty index
		if err := GitCommitter.Commit(fmt.Sprintf("Set version to %s", tag), "."); err != nil {
			Log("tag: version commit warning: %v", err)
		}
	}

	Log("tag: done — created %s", tag)
	return nil
}

// NextDocRevision returns the next revision number for <prefix>DATE.* tags.
// Returns 0 if no tags exist for the given date, otherwise returns the
// highest existing revision + 1.
func NextDocRevision(prefix, date string) int {
	pattern := fmt.Sprintf("%s%s.*", prefix, date)
	tags := GitTags.ListTags(pattern, ".")
	if len(tags) == 0 {
		return 0
	}

	// Extract revision numbers from tags like v0.20260219.0, v0.20260219.1, etc.
	// Find the highest revision.
	revPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + regexp.QuoteMeta(date) + `\.(\d+)$`)
	maxRev := -1
	for _, t := range tags {
		matches := revPattern.FindStringSubmatch(t)
		if len(matches) == 2 {
			rev, err := strconv.Atoi(matches[1])
			if err == nil && rev > maxRev {
				maxRev = rev
			}
		}
	}

	if maxRev == -1 {
		return 0
	}
	return maxRev + 1
}
