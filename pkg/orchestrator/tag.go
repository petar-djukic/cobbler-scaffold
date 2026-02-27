// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Tag creates a documentation-only release tag (v0.YYYYMMDD.N) for the current
// state of the repository, builds the container image with that tag, and tags
// the image as :latest. The revision number increments for each tag created on
// the same date. Optionally updates the version file if configured.
//
// Tag convention:
//   - v0.* = documentation-only releases on main (manual)
//   - v1.* = Claude-generated code (created by GeneratorStop)
//
// Exposed as a mage target (e.g., mage tag).
func (o *Orchestrator) Tag() error {
	// Ensure we're on the configured base branch for doc tags.
	current, err := gitCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if current != o.cfg.Cobbler.BaseBranch {
		return fmt.Errorf("tag must be run from %s branch (currently on %s)", o.cfg.Cobbler.BaseBranch, current)
	}

	// Get today's date in YYYYMMDD format.
	today := time.Now().Format("20060102")

	// Find the next revision for today.
	revision := nextDocRevision(o.cfg.Cobbler.DocTagPrefix, today)

	// Create the tag name.
	tag := fmt.Sprintf("%s%s.%d", o.cfg.Cobbler.DocTagPrefix, today, revision)

	logf("tag: creating documentation release %s", tag)

	// Create the git tag.
	if err := gitTag(tag); err != nil {
		return fmt.Errorf("creating tag %s: %w", tag, err)
	}

	// Update the version constant in the version file if configured.
	if o.cfg.Project.VersionFile != "" {
		logf("tag: writing version %s to %s", tag, o.cfg.Project.VersionFile)
		if err := writeVersionConst(o.cfg.Project.VersionFile, tag); err != nil {
			logf("tag: version file warning: %v", err)
		} else {
			_ = gitStageAll() // best-effort; commit below handles empty index
			if err := gitCommit(fmt.Sprintf("Set version to %s", tag)); err != nil {
				logf("tag: version commit warning: %v", err)
			}
		}
	}

	// Build the container image with the new tag.
	logf("tag: building container image")
	if err := o.BuildImage(); err != nil {
		return fmt.Errorf("building image: %w", err)
	}

	logf("tag: done — created %s and built container image", tag)
	return nil
}

// nextDocRevision returns the next revision number for <prefix>DATE.* tags.
// Returns 0 if no tags exist for the given date, otherwise returns the
// highest existing revision + 1.
func nextDocRevision(prefix, date string) int {
	pattern := fmt.Sprintf("%s%s.*", prefix, date)
	tags := gitListTags(pattern)
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
