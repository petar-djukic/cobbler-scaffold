// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"maps"
	"slices"
	"strings"
	"text/template"
	"time"
)

// GeneratorRun executes N cycles of Measure + Stitch within the current generation.
// If cycles > 0 it overrides configuration.yaml's generation.cycles for this run only.
// cycles == 0 means use the configured value (or unlimited if that is also 0).
func (o *Orchestrator) GeneratorRun(cycles int) error {
	currentBranch, err := gitCurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if cycles > 0 {
		o.cfg.Generation.Cycles = cycles
	}
	o.cfg.Generation.Branch = currentBranch
	setGeneration(currentBranch)
	defer clearGeneration()
	return o.RunCycles("run")
}

// GeneratorResume recovers from an interrupted generator:run and continues.
// Reads generation branch from Config.GenerationBranch or auto-detects.
func (o *Orchestrator) GeneratorResume() error {
	branch := o.cfg.Generation.Branch
	if branch == "" {
		resolved, err := o.resolveBranch("")
		if err != nil {
			return fmt.Errorf("resolving generation branch: %w", err)
		}
		branch = resolved
	}

	if !strings.HasPrefix(branch, o.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch: %s\nSet generation.branch in configuration.yaml", branch)
	}
	if !gitBranchExists(branch, ".") {
		return fmt.Errorf("branch does not exist: %s", branch)
	}

	setGeneration(branch)
	defer clearGeneration()

	logf("resume: target branch=%s", branch)

	// Commit or stash uncommitted work, then switch to the generation branch.
	if err := saveAndSwitchBranch(branch); err != nil {
		return fmt.Errorf("switching to %s: %w", branch, err)
	}

	// Pre-flight cleanup.
	logf("resume: pre-flight cleanup")
	wtBase := worktreeBasePath()

	logf("resume: pruning worktrees")
	if err := gitWorktreePrune("."); err != nil {
		logf("resume: warning: worktree prune: %v", err)
	}

	logf("resume: removing worktree directory %s", wtBase)
	if err := os.RemoveAll(wtBase); err != nil {
		logf("resume: warning: removing worktree directory %s: %v", wtBase, err)
	}

	logf("resume: recovering stale tasks")
	ghRepo, err := detectGitHubRepo(".", o.cfg)
	if err != nil {
		logf("resume: warning: detectGitHubRepo: %v", err)
	}
	if err := o.recoverStaleTasks(branch, wtBase, ghRepo, branch); err != nil {
		logf("resume: recoverStaleTasks warning: %v", err)
	}

	o.cfg.Generation.Branch = branch

	// Drain existing ready issues before starting measure+stitch cycles.
	logf("resume: draining existing ready issues")
	if _, err := o.RunStitch(); err != nil {
		logf("resume: drain stitch warning: %v", err)
	}

	return o.RunCycles("resume")
}

// RunCycles runs stitch→measure cycles until no open issues remain.
// Each cycle stitches up to MaxStitchIssuesPerCycle tasks, then measures
// up to MaxMeasureIssues new issues. The loop continues while open issues
// exist. MaxStitchIssues caps total stitch iterations across all cycles
// (0 = unlimited). Cycles caps the number of stitch+measure rounds
// (0 = unlimited). MaxConsecutiveZeroLOCCycles stops the loop when stitch
// produces zero LOC change for N consecutive cycles (default 3), preventing
// runaway refinement loops on fully-implemented specs.
func (o *Orchestrator) RunCycles(label string) error {
	maxZeroLOC := o.cfg.Cobbler.MaxConsecutiveZeroLOCCycles
	logf("generator %s: starting (stitchTotal=%d stitchPerCycle=%d measure=%d safetyCycles=%d maxZeroLOC=%d)",
		label, o.cfg.Cobbler.MaxStitchIssues, o.cfg.Cobbler.MaxStitchIssuesPerCycle, o.cfg.Cobbler.MaxMeasureIssues, o.cfg.Generation.Cycles, maxZeroLOC)

	totalStitched := 0
	consecutiveZeroLOC := 0
	for cycle := 1; ; cycle++ {
		if o.cfg.Generation.Cycles > 0 && cycle > o.cfg.Generation.Cycles {
			logf("generator %s: reached max cycles (%d), stopping", label, o.cfg.Generation.Cycles)
			break
		}

		// Determine how many tasks this cycle can stitch.
		perCycle := o.cfg.Cobbler.MaxStitchIssuesPerCycle
		if o.cfg.Cobbler.MaxStitchIssues > 0 {
			remaining := o.cfg.Cobbler.MaxStitchIssues - totalStitched
			if remaining <= 0 {
				logf("generator %s: reached total stitch limit (%d), stopping", label, o.cfg.Cobbler.MaxStitchIssues)
				break
			}
			if perCycle == 0 || remaining < perCycle {
				perCycle = remaining
			}
		}

		// Refresh analysis before each cycle so stitch sees current state.
		o.RunPreCycleAnalysis()

		// Capture LOC before stitch to detect zero-change cycles.
		locBefore := o.captureLOC()
		logf("generator %s: cycle %d — stitch (limit=%d, stitched so far=%d)", label, cycle, perCycle, totalStitched)
		n, err := o.RunStitchN(perCycle)
		totalStitched += n
		if err != nil {
			return fmt.Errorf("cycle %d stitch: %w", cycle, err)
		}
		locAfter := o.captureLOC()
		locDelta := (locAfter.Production - locBefore.Production) + (locAfter.Test - locBefore.Test)
		logf("generator %s: cycle %d — LOC delta=%d (prod %d→%d, test %d→%d)",
			label, cycle, locDelta, locBefore.Production, locAfter.Production, locBefore.Test, locAfter.Test)

		// Track consecutive zero-LOC cycles as a refinement-loop guard.
		if locDelta == 0 {
			consecutiveZeroLOC++
			logf("generator %s: cycle %d — zero LOC change (%d consecutive)", label, cycle, consecutiveZeroLOC)
			if maxZeroLOC > 0 && consecutiveZeroLOC >= maxZeroLOC {
				logf("generator %s: %d consecutive zero-LOC cycles reached limit (%d); spec likely complete — stopping",
					label, consecutiveZeroLOC, maxZeroLOC)
				break
			}
		} else {
			consecutiveZeroLOC = 0
		}

		// Check if the current release is complete and auto-advance if so.
		if advanced, ver := o.checkAutoAdvanceRelease(); advanced {
			logf("generator %s: cycle %d — auto-advanced release %s", label, cycle, ver)
		}

		logf("generator %s: cycle %d — measure", label, cycle)
		if err := o.RunMeasure(); err != nil {
			return fmt.Errorf("cycle %d measure: %w", cycle, err)
		}

		open, err := o.hasOpenIssues()
		if err != nil {
			logf("generator %s: hasOpenIssues error (assuming open): %v", label, err)
		}
		if !open && err == nil {
			logf("generator %s: no open issues remain, stopping after %d cycle(s)", label, cycle)
			break
		}
		logf("generator %s: open issues remain, continuing to cycle %d", label, cycle+1)
	}

	logf("generator %s: complete (total stitched=%d)", label, totalStitched)
	return nil
}

// checkAutoAdvanceRelease detects when the current release's use cases are all
// done and auto-advances by calling ReleaseUpdate (which marks UCs as
// "implemented" in road-map.yaml and removes the release from
// configuration.yaml). Changes are committed on the current branch. Returns
// (true, version) if a release was advanced, (false, "") otherwise.
func (o *Orchestrator) checkAutoAdvanceRelease() (bool, string) {
	rm := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return false, ""
	}

	// Find the first release that is not yet done/implemented.
	var target *RoadmapRelease
	for i := range rm.Releases {
		rel := &rm.Releases[i]
		if !ucStatusDone(rel.Status) {
			target = rel
			break
		}
	}
	if target == nil {
		return false, ""
	}

	// Check if all use cases in this release are done.
	if len(target.UseCases) == 0 {
		return false, ""
	}
	for _, uc := range target.UseCases {
		if !ucStatusDone(uc.Status) {
			return false, ""
		}
	}

	// All UCs done — auto-advance this release.
	logf("checkAutoAdvanceRelease: release %s has all use cases done, advancing", target.Version)
	if err := o.ReleaseUpdate(target.Version); err != nil {
		logf("checkAutoAdvanceRelease: ReleaseUpdate(%s) failed: %v", target.Version, err)
		return false, ""
	}

	// Commit the changes on the current branch.
	_ = gitStageAll(".")
	msg := fmt.Sprintf("Auto-advance release %s: all use cases complete\n\nMarked use cases as implemented in road-map.yaml.\nRemoved %s from project.releases in configuration.yaml.", target.Version, target.Version)
	if err := gitCommit(msg, "."); err != nil {
		logf("checkAutoAdvanceRelease: commit failed: %v", err)
	}

	// Reload config so subsequent measure sees updated releases.
	if cfg, err := LoadConfig(DefaultConfigFile); err == nil {
		o.cfg = cfg
	}

	return true, target.Version
}

// GeneratorStart begins a new generation trail.
// Records the current branch as the base branch, tags it, creates a generation
// branch, deletes Go files, reinitializes the Go module, and commits the clean
// state. Any clean branch is a valid starting point (prd002 R2.1).
func (o *Orchestrator) GeneratorStart() error {
	baseBranch, err := gitCurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Reject dirty worktrees — a generation must start from a clean state.
	if gitHasChanges(".") {
		return fmt.Errorf("worktree has uncommitted changes on %s; commit or stash before starting a generation", baseBranch)
	}

	// Garbage-collect issues from generations whose branch no longer exists.
	// This catches leaks from crashed tests or prior runs without cleanup.
	if ghRepo, err := detectGitHubRepo(".", o.cfg); err == nil && ghRepo != "" {
		gcStaleGenerationIssues(ghRepo, o.cfg.Generation.Prefix)
	}

	suffix := os.Getenv("COBBLER_GEN_NAME")
	if suffix == "" {
		suffix = o.cfg.Generation.Name
	}
	if suffix == "" {
		suffix = time.Now().Format("2006-01-02-15-04-05")
	}
	genName := o.cfg.Generation.Prefix + suffix
	startTag := genName + "-start"

	setGeneration(genName)
	defer clearGeneration()

	logf("generator:start: beginning (base branch: %s)", baseBranch)

	// Tag the current base branch state before the generation begins.
	logf("generator:start: tagging current state as %s", startTag)
	if err := gitTag(startTag, "."); err != nil {
		return fmt.Errorf("tagging base branch: %w", err)
	}

	// Create and switch to generation branch.
	logf("generator:start: creating branch")
	if err := gitCheckoutNew(genName, "."); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Record branch point so intermediate commits can be squashed.
	branchSHA, err := gitRevParseHEAD(".")
	if err != nil {
		return fmt.Errorf("getting branch HEAD: %w", err)
	}

	// Record the base branch so GeneratorStop knows where to merge back
	// (prd002 R2.8).
	if err := o.writeBaseBranch(baseBranch); err != nil {
		return fmt.Errorf("recording base branch: %w", err)
	}

	// Ensure bin/ is ignored on the generation branch so compiled binaries
	// are never staged by git add -A (GH-469).
	if err := appendToGitignore(".", o.cfg.Project.BinaryDir+"/"); err != nil {
		logf("generator:start: warning: could not update .gitignore: %v", err)
	}

	// Reset Go sources and reinitialize module unless preserve_sources is set.
	// Library repos (e.g. cobbler-scaffold itself) set preserve_sources: true so
	// generator:start does not destroy the library code. See prd002 R10.1.
	if o.cfg.Generation.PreserveSources {
		logf("generator:start: preserve_sources=true, skipping Go source reset")
	} else {
		logf("generator:start: resetting Go sources")
		if err := o.resetGoSources(genName); err != nil {
			return fmt.Errorf("resetting Go sources: %w", err)
		}
	}

	// Squash intermediate commits into one clean commit.
	logf("generator:start: squashing into single commit")
	if err := gitResetSoft(branchSHA, "."); err != nil {
		return fmt.Errorf("squashing start commits: %w", err)
	}
	_ = gitStageAll(".")
	var msg string
	if o.cfg.Generation.PreserveSources {
		msg = fmt.Sprintf("Start generation: %s\n\nBase branch: %s. Sources preserved (preserve_sources=true).\nTagged previous state as %s.", genName, baseBranch, genName)
	} else {
		msg = fmt.Sprintf("Start generation: %s\n\nBase branch: %s. Delete Go files, reinitialize module.\nTagged previous state as %s.", genName, baseBranch, genName)
	}
	// Use allow-empty because a specs-only repo may have no Go files
	// to delete, leaving no changes to commit after source reset.
	if err := gitCommitAllowEmpty(msg, "."); err != nil {
		return fmt.Errorf("committing clean state: %w", err)
	}

	logf("generator:start: done, run mage generator:run to begin building")
	return nil
}

// baseBranchFile is the name of the file that records which branch a
// generation was started from, stored inside the cobbler directory.
const baseBranchFile = "base-branch"

// writeBaseBranch writes the base branch name to .cobbler/base-branch.
func (o *Orchestrator) writeBaseBranch(branch string) error {
	dir := o.cfg.Cobbler.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, baseBranchFile), []byte(branch+"\n"), 0o644)
}

// readBaseBranch reads the base branch from .cobbler/base-branch on the
// current branch. Returns "main" if the file does not exist (backward
// compatibility with older generations, prd002 R5.3).
func (o *Orchestrator) readBaseBranch() string {
	data, err := os.ReadFile(filepath.Join(o.cfg.Cobbler.Dir, baseBranchFile))
	if err != nil {
		return "main"
	}
	branch := strings.TrimSpace(string(data))
	if branch == "" {
		return "main"
	}
	return branch
}

// resolveStopTarget returns the branch that generator:stop should merge into.
// callerBranch is the branch checked out when generator:stop was invoked.
// genBranch is the generation branch being stopped. recordedBase is the branch
// written by generator:start. When the caller is on a branch other than the
// generation branch and other than the recorded base, that caller branch is
// returned so that generator:stop merges into an explicit feature branch rather
// than always forcing a merge into the recorded base (GH-523).
func resolveStopTarget(callerBranch, genBranch, recordedBase string) string {
	if callerBranch != genBranch && callerBranch != recordedBase {
		return callerBranch
	}
	return recordedBase
}

// GeneratorStop completes a generation trail and merges it into the base branch.
// Reads the base branch from .cobbler/base-branch (falls back to "main").
// Uses Config.GenerationBranch, current branch, or auto-detects.
func (o *Orchestrator) GeneratorStop() error {
	branch := o.cfg.Generation.Branch
	if branch != "" {
		if !gitBranchExists(branch, ".") {
			return fmt.Errorf("branch does not exist: %s", branch)
		}
	} else {
		current, err := gitCurrentBranch(".")
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
		if strings.HasPrefix(current, o.cfg.Generation.Prefix) {
			branch = current
			logf("generator:stop: stopping current branch %s", branch)
		} else {
			resolved, err := o.resolveBranch("")
			if err != nil {
				return err
			}
			branch = resolved
		}
	}

	if !strings.HasPrefix(branch, o.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch: %s\nSet generation.branch in configuration.yaml", branch)
	}

	setGeneration(branch)
	defer clearGeneration()

	finishedTag := branch + "-finished"

	logf("generator:stop: beginning")

	// Capture the caller's branch before switching to the generation branch.
	// If the caller is on a feature branch (e.g. gh-168-...) rather than
	// the recorded base branch, prefer it as the merge target so that
	// generator:stop respects the PR workflow described in the close-out
	// procedure (GH-523).
	callerBranch, err := gitCurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Switch to the generation branch and tag its final state.
	if err := ensureOnBranch(branch); err != nil {
		return fmt.Errorf("switching to generation branch: %w", err)
	}

	// Determine the merge target (GH-523).
	recordedBase := o.readBaseBranch()
	baseBranch := resolveStopTarget(callerBranch, branch, recordedBase)
	if baseBranch != recordedBase {
		logf("generator:stop: caller was on %s; using it as merge target instead of recorded base %s", callerBranch, recordedBase)
	}

	logf("generator:stop: tagging as %s", finishedTag)
	if err := gitTag(finishedTag, "."); err != nil {
		return fmt.Errorf("tagging generation: %w", err)
	}

	// Switch to the base branch.
	logf("generator:stop: switching to %s", baseBranch)
	if err := gitCheckout(baseBranch, "."); err != nil {
		return fmt.Errorf("checking out %s: %w", baseBranch, err)
	}

	if err := o.mergeGeneration(branch, baseBranch); err != nil {
		return err
	}

	// Close any open cobbler-gen issues for this generation so they do not
	// accumulate as orphans after the branch is deleted.
	if ghRepo, err := detectGitHubRepo(".", o.cfg); err == nil && ghRepo != "" {
		if err := closeGenerationIssues(ghRepo, branch); err != nil {
			logf("generator:stop: close issues warning: %v", err)
		}
	}

	o.cleanupDirs()

	logf("generator:stop: done, work is on %s", baseBranch)
	return nil
}

// mergeGeneration resets Go sources, commits the clean state, merges the
// generation branch into the base branch, tags the result, resets the base
// branch to specs-only, and deletes the generation branch.
// When preserve_sources is true the pre-merge source reset and the
// post-tag cleanGoSources call are both skipped (prd002 R10.2).
func (o *Orchestrator) mergeGeneration(branch, baseBranch string) error {
	if o.cfg.Generation.PreserveSources {
		logf("generator:stop: preserve_sources=true, skipping pre-merge Go source reset on %s", baseBranch)
	} else {
		logf("generator:stop: resetting Go sources on %s", baseBranch)
		_ = o.resetGoSources(branch) // best-effort; merge will overwrite these files
	}

	_ = gitStageAll(".") // best-effort; commit below handles empty index
	var prepareMsg string
	if o.cfg.Generation.PreserveSources {
		prepareMsg = fmt.Sprintf("Prepare %s for generation merge (preserve_sources)\n\nSources preserved. Merging %s.", baseBranch, branch)
	} else {
		prepareMsg = fmt.Sprintf("Prepare %s for generation merge: delete Go code\n\nDocumentation preserved for merge. Code will be replaced by %s.", baseBranch, branch)
	}
	if err := gitCommitAllowEmpty(prepareMsg, "."); err != nil {
		return fmt.Errorf("committing prepare step: %w", err)
	}

	logf("generator:stop: merging into %s", baseBranch)
	cmd := gitMergeCmd(branch, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("merging %s: %w", branch, err)
	}

	// Restore Go files from earlier generations so the v1 tag captures a
	// complete snapshot (prd002 R5.9). Runs before tagging.
	startTag := branch + "-start"
	if err := o.restoreFromStartTag(startTag); err != nil {
		logf("generator:stop: restore warning: %v", err)
	}

	mergedTag := branch + "-merged"
	logf("generator:stop: tagging %s as %s", baseBranch, mergedTag)
	if err := gitTag(mergedTag, "."); err != nil {
		return fmt.Errorf("tagging merge: %w", err)
	}

	// Reset base branch to specs-only (prd002 R5.10, R5.11).
	// Version tagging is handled separately by mage tag (Tag() in tag.go).
	// Use cleanGoSources (not resetGoSources) to avoid re-seeding files like version.go.
	// Skip when preserve_sources is true — library repos keep their Go source (prd002 R10.2).
	if o.cfg.Generation.PreserveSources {
		logf("generator:stop: preserve_sources=true, skipping post-tag source reset on %s", baseBranch)
	} else {
		logf("generator:stop: resetting %s to specs-only", baseBranch)
		o.cleanGoSources()
	}
	if err := o.HistoryClean(); err != nil {
		logf("generator:stop: warning cleaning history: %v", err)
	}
	_ = gitStageAll(".")
	cleanupMsg := fmt.Sprintf("Reset %s to specs-only after v1 tag\n\nGenerated code preserved at version tags. Branch restored to documentation-only state.", baseBranch)
	_ = gitCommit(cleanupMsg, ".") // best-effort; may be empty if nothing changed

	logf("generator:stop: deleting branch")
	_ = gitDeleteBranch(branch, ".") // best-effort; branch may already be deleted
	return nil
}

// restoreFromStartTag restores Go source files that existed on main at the
// given start tag but are missing after the merge. This preserves code from
// earlier generations that would otherwise be lost during the reset+merge
// cycle. See prd002-generation-lifecycle R5.8.
func (o *Orchestrator) restoreFromStartTag(startTag string) error {
	startFiles, err := gitLsTreeFiles(startTag, ".")
	if err != nil {
		return fmt.Errorf("listing files at %s: %w", startTag, err)
	}

	var restored []string
	for _, path := range startFiles {
		// Only restore Go source files outside magefiles/.
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if strings.HasPrefix(path, o.cfg.Project.MagefilesDir+"/") {
			continue
		}

		// Skip files that already exist on disk.
		if _, err := os.Stat(path); err == nil {
			continue
		}

		content, err := gitShowFileContent(startTag, path, ".")
		if err != nil {
			logf("generator:stop: could not read %s from %s: %v", path, startTag, err)
			continue
		}

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logf("generator:stop: could not create directory %s: %v", dir, err)
			continue
		}

		if err := os.WriteFile(path, content, 0o644); err != nil {
			logf("generator:stop: could not write %s: %v", path, err)
			continue
		}
		restored = append(restored, path)
	}

	if len(restored) == 0 {
		return nil
	}

	logf("generator:stop: restored %d file(s) from earlier generations", len(restored))
	_ = gitStageAll(".")
	msg := fmt.Sprintf("Restore %d file(s) from earlier generations\n\nFiles restored from %s:\n%s",
		len(restored), startTag, strings.Join(restored, "\n"))
	if err := gitCommit(msg, "."); err != nil {
		return fmt.Errorf("committing restored files: %w", err)
	}
	return nil
}

// listGenerationBranches returns all generation-* branch names.
func (o *Orchestrator) listGenerationBranches() []string {
	return gitListBranches(o.cfg.Generation.Prefix + "*", ".")
}

// tagSuffixes lists the lifecycle tag suffixes in order.
var tagSuffixes = []string{"-start", "-finished", "-merged", "-abandoned"}

// generationName strips the lifecycle suffix from a tag to recover
// the generation name.
func generationName(tag string) string {
	for _, suffix := range tagSuffixes {
		if cut, ok := strings.CutSuffix(tag, suffix); ok {
			return cut
		}
	}
	return tag
}

// cleanupUnmergedTags renames tags for generations that were never
// merged into a single -abandoned tag.
func (o *Orchestrator) cleanupUnmergedTags() {
	tags := gitListTags(o.cfg.Generation.Prefix + "*", ".")
	if len(tags) == 0 {
		return
	}

	merged := make(map[string]bool)
	for _, t := range tags {
		if name, ok := strings.CutSuffix(t, "-merged"); ok {
			merged[name] = true
		}
	}

	marked := make(map[string]bool)
	for _, t := range tags {
		name := generationName(t)
		if merged[name] {
			continue
		}
		if !marked[name] {
			marked[name] = true
			abTag := name + "-abandoned"
			if t != abTag {
				logf("generator:reset: marking abandoned: %s -> %s", t, abTag)
				_ = gitRenameTag(t, abTag, ".") // best-effort; tag may not exist
			}
		} else {
			logf("generator:reset: removing tag %s", t)
			_ = gitDeleteTag(t, ".") // best-effort cleanup
		}
	}
}

// resolveBranch determines which branch to work on.
func (o *Orchestrator) resolveBranch(explicit string) (string, error) {
	if explicit != "" {
		if !gitBranchExists(explicit, ".") {
			return "", fmt.Errorf("branch does not exist: %s", explicit)
		}
		return explicit, nil
	}

	branches := o.listGenerationBranches()
	switch len(branches) {
	case 0:
		return gitCurrentBranch(".")
	case 1:
		return branches[0], nil
	default:
		slices.Sort(branches)
		return "", fmt.Errorf("multiple generation branches exist (%s); set generation.branch in configuration.yaml", strings.Join(branches, ", "))
	}
}

// saveAndSwitchBranch commits or stashes uncommitted changes on the
// current branch, then checks out the target branch. It tries a WIP
// commit first; if that fails and the tree is still dirty, it stashes
// changes so the checkout can succeed.
func saveAndSwitchBranch(target string) error {
	current, err := gitCurrentBranch(".")
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}

	if err := gitStageAll("."); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}

	msg := fmt.Sprintf("WIP: save state before switching to %s", target)
	if err := gitCommit(msg, "."); err != nil {
		// Commit failed (e.g. nothing to commit). Unstage and fall
		// back to stash if the tree is still dirty.
		_ = gitUnstageAll(".") // best-effort; unstage before stash fallback
		if gitHasChanges(".") {
			logf("saveAndSwitchBranch: commit failed, stashing dirty tree")
			_ = gitStash(msg, ".") // best-effort; switching branch is the priority
		}
	}

	logf("saveAndSwitchBranch: %s -> %s", current, target)
	return gitCheckout(target, ".")
}

// ensureOnBranch switches to the given branch if not already on it.
func ensureOnBranch(branch string) error {
	current, err := gitCurrentBranch(".")
	if err != nil {
		return err
	}
	if current == branch {
		return nil
	}
	logf("ensureOnBranch: switching from %s to %s", current, branch)
	return gitCheckout(branch, ".")
}

// GeneratorList shows active branches and past generations.
func (o *Orchestrator) GeneratorList() error {
	branches := o.listGenerationBranches()
	tags := gitListTags(o.cfg.Generation.Prefix + "*", ".")
	current, _ := gitCurrentBranch(".")

	nameSet := make(map[string]bool)
	branchSet := make(map[string]bool)
	for _, b := range branches {
		nameSet[b] = true
		branchSet[b] = true
	}

	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
		nameSet[generationName(t)] = true
	}

	if len(nameSet) == 0 {
		fmt.Println("No generations found.")
		return nil
	}

	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	slices.Sort(names)

	for _, name := range names {
		isActive := branchSet[name]
		isAbandoned := tagSet[name+"-abandoned"]

		marker := " "
		if name == current {
			marker = "*"
		}

		var lifecycle []string
		for _, suffix := range tagSuffixes {
			if tagSet[name+suffix] {
				lifecycle = append(lifecycle, suffix[1:])
			}
		}

		if isActive {
			if len(lifecycle) > 0 {
				fmt.Printf("%s %s  (active, tags: %s)\n", marker, name, strings.Join(lifecycle, ", "))
			} else {
				fmt.Printf("%s %s  (active)\n", marker, name)
			}
		} else if isAbandoned {
			fmt.Printf("%s %s  (abandoned)\n", marker, name)
		} else {
			fmt.Printf("%s %s  (tags: %s)\n", marker, name, strings.Join(lifecycle, ", "))
		}
	}

	return nil
}

// GeneratorSwitch commits current work and checks out another generation branch.
// Uses Config.GenerationBranch as the target.
func (o *Orchestrator) GeneratorSwitch() error {
	target := o.cfg.Generation.Branch
	baseBranch := o.cfg.Cobbler.BaseBranch
	if target == "" {
		return fmt.Errorf("set generation.branch in configuration.yaml\nAvailable branches: %s, %s", strings.Join(o.listGenerationBranches(), ", "), baseBranch)
	}

	if target != baseBranch && !strings.HasPrefix(target, o.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch or %s: %s", baseBranch, target)
	}
	if !gitBranchExists(target, ".") {
		return fmt.Errorf("branch does not exist: %s", target)
	}

	current, err := gitCurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if current == target {
		logf("generator:switch: already on %s", target)
		return nil
	}

	if err := saveAndSwitchBranch(target); err != nil {
		return fmt.Errorf("switching to %s: %w", target, err)
	}

	logf("generator:switch: now on %s", target)
	return nil
}

// GeneratorReset destroys generation branches, worktrees, and Go source directories.
func (o *Orchestrator) GeneratorReset() error {
	logf("generator:reset: beginning")

	baseBranch := o.cfg.Cobbler.BaseBranch
	if err := ensureOnBranch(baseBranch); err != nil {
		return fmt.Errorf("switching to %s: %w", baseBranch, err)
	}

	wtBase := worktreeBasePath()
	ghRepo, _ := detectGitHubRepo(".", o.cfg)
	genBranches := o.listGenerationBranches()
	if len(genBranches) > 0 {
		logf("generator:reset: removing task branches and worktrees")
		for _, gb := range genBranches {
			recoverStaleBranches(gb, wtBase, ghRepo)
		}
	}

	if ghRepo != "" {
		logf("generator:reset: closing GitHub issues")
		for _, gb := range genBranches {
			if err := closeGenerationIssues(ghRepo, gb); err != nil {
				logf("generator:reset: close issues warning for %s: %v", gb, err)
			}
		}
		// Close issues created on the base branch (handles repos where measure ran on the
		// base branch, such as test repos that run cobbler:measure without a generation branch).
		if err := closeGenerationIssues(ghRepo, baseBranch); err != nil {
			logf("generator:reset: close issues warning for %s: %v", baseBranch, err)
		}
	}

	if err := gitWorktreePrune("."); err != nil {
		logf("generator:reset: warning: worktree prune: %v", err)
	}

	if _, err := os.Stat(wtBase); err == nil {
		logf("generator:reset: removing worktree directory %s", wtBase)
		if err := os.RemoveAll(wtBase); err != nil {
			logf("generator:reset: warning: removing worktree dir: %v", err)
		}
	}

	if len(genBranches) > 0 {
		logf("generator:reset: removing %d generation branch(es)", len(genBranches))
		for _, gb := range genBranches {
			logf("generator:reset: deleting branch %s", gb)
			_ = gitForceDeleteBranch(gb, ".") // best-effort; branch may be already removed
		}
	}

	o.cleanupUnmergedTags()

	logf("generator:reset: removing Go source directories")
	for _, dir := range o.cfg.Project.GoSourceDirs {
		logf("generator:reset: removing %s", dir)
		os.RemoveAll(dir) // nolint: best-effort directory cleanup
	}
	os.RemoveAll(o.cfg.Project.BinaryDir + "/") // nolint: best-effort directory cleanup
	o.cleanupDirs()

	logf("generator:reset: seeding Go sources and reinitializing go.mod")
	if err := o.seedFiles(baseBranch); err != nil {
		return fmt.Errorf("seeding files: %w", err)
	}
	if err := o.reinitGoModule(); err != nil {
		return fmt.Errorf("reinitializing go module: %w", err)
	}

	logf("generator:reset: committing clean state")
	_ = gitStageAll(".")                                                  // best-effort; commit below handles empty index
	_ = gitCommitAllowEmpty("Generator reset: return to clean state", ".") // best-effort; reset is complete regardless

	logf("generator:reset: done, only %s branch remains", baseBranch)
	return nil
}

// resetGoSources deletes Go files, removes empty source dirs,
// clears build artifacts, seeds files, and reinitializes the Go module.
func (o *Orchestrator) resetGoSources(version string) error {
	o.deleteGoFiles(".")
	for _, dir := range o.cfg.Project.GoSourceDirs {
		removeEmptyDirs(dir)
	}
	os.RemoveAll(o.cfg.Project.BinaryDir + "/")
	if err := o.seedFiles(version); err != nil {
		return fmt.Errorf("seeding files: %w", err)
	}
	return o.reinitGoModule()
}

// cleanGoSources removes all Go files, empty source directories, and the
// binary directory without re-seeding files or reinitializing the module.
// Used for the specs-only reset after v1 tags are created.
func (o *Orchestrator) cleanGoSources() {
	o.deleteGoFiles(".")
	for _, dir := range o.cfg.Project.GoSourceDirs {
		removeEmptyDirs(dir)
	}
	os.RemoveAll(o.cfg.Project.BinaryDir + "/")
}

// seedFiles creates the configured seed files using Go templates.
func (o *Orchestrator) seedFiles(version string) error {
	data := SeedData{
		Version:    version,
		ModulePath: o.cfg.Project.ModulePath,
	}

	for _, path := range slices.Sorted(maps.Keys(o.cfg.Project.SeedFiles)) {
		tmplStr := o.cfg.Project.SeedFiles[path]
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		tmpl, err := template.New(path).Parse(tmplStr)
		if err != nil {
			return fmt.Errorf("parsing seed template for %s: %w", path, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("executing seed template for %s: %w", path, err)
		}

		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// reinitGoModule removes go.sum and go.mod, then creates a fresh module
// with a local replace directive and resolves dependencies.
func (o *Orchestrator) reinitGoModule() error {
	_ = os.Remove("go.sum") // best-effort; file may not exist
	_ = os.Remove("go.mod") // best-effort; file may not exist
	if err := o.goModInit(); err != nil {
		return fmt.Errorf("go mod init: %w", err)
	}
	if err := goModEditReplace(o.cfg.Project.ModulePath, "./"); err != nil {
		return fmt.Errorf("go mod edit -replace: %w", err)
	}
	if err := goModTidy(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	return nil
}

// deleteGoFiles removes all .go files except those in .git/ and magefiles/.
func (o *Orchestrator) deleteGoFiles(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (path == ".git" || path == o.cfg.Project.MagefilesDir) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				logf("deleteGoFiles: warning removing %s: %v", path, err)
			}
		}
		return nil
	})
}

// removeEmptyDirs removes empty directories under the given root.
func removeEmptyDirs(root string) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return
	}
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dirs[i]) // best-effort empty dir cleanup
		}
	}
}

// cleanupDirs removes all directories listed in Config.CleanupDirs.
func (o *Orchestrator) cleanupDirs() {
	for _, dir := range o.cfg.Generation.CleanupDirs {
		logf("cleanupDirs: removing %s", dir)
		os.RemoveAll(dir)
	}
}

// GeneratorInit writes a default configuration.yaml if one does not exist.
// Exposed as mage generator:init.
func GeneratorInit() error {
	logf("generator:init: writing %s", DefaultConfigFile)
	if err := WriteDefaultConfig(DefaultConfigFile); err != nil {
		return err
	}
	logf("generator:init: created %s — edit project-specific fields before running", DefaultConfigFile)
	return nil
}

// Init is a no-op placeholder kept for mage target compatibility.
func (o *Orchestrator) Init() error {
	return nil
}

// FullReset performs a full reset: cobbler and generator.
func (o *Orchestrator) FullReset() error {
	if err := o.CobblerReset(); err != nil {
		return err
	}
	return o.GeneratorReset()
}

// appendToGitignore adds entry to the .gitignore file in dir if not already
// present. Creates the file if it does not exist. Used by GeneratorStart to
// ensure build artifacts (bin/) are not committed to the generation branch
// regardless of where they are produced (GH-469).
func appendToGitignore(dir, entry string) error {
	path := filepath.Join(dir, ".gitignore")

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil // already present
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()

	prefix := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(f, "%s%s\n", prefix, entry)
	return err
}
