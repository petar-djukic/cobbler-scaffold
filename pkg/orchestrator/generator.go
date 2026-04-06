// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"os/exec"

	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/claude"
	ictx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
	gh "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/github"
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/gitops"
)

// NOTE: generate.Log and generate.BinGit are wired in the Orchestrator
// constructor (New) instead of an init function, eliminating the
// package-level dependency on logf.

// Generator provides the generation lifecycle: start, run, resume, stop,
// reset, list, and switch operations.
type Generator struct {
	cfg          Config
	logf         func(string, ...any)
	git          gitops.GitOps
	tracker      gh.WorkTracker
	claudeRunner *ClaudeRunner
	analyzer     *Analyzer
	measure      *Measure
	stitch       *Stitch
	releaser     *Releaser
	// Orchestrator state callbacks.
	setGeneration   func(string)
	clearGeneration func()
}

// NewGenerator creates a Generator with explicit dependencies.
func NewGenerator(o *Orchestrator) *Generator {
	return &Generator{
		cfg:             o.cfg,
		logf:            o.logf,
		git:             o.git,
		tracker:         o.tracker,
		claudeRunner:    o.ClaudeRunner,
		analyzer:        o.Analyzer,
		measure:         o.Measure,
		stitch:          o.Stitch,
		releaser:        o.Releaser,
		setGeneration:   o.setGeneration,
		clearGeneration: o.clearGeneration,
	}
}

// ---------------------------------------------------------------------------
// Type aliases for backward compatibility
// ---------------------------------------------------------------------------

// validationResult holds the outcome of measure output validation.
type validationResult = generate.ValidationResult

// issueDescription is the subset of fields parsed from an issue description.
type issueDescription = generate.IssueDescription

// issueDescFile holds a file path from an issue description.
type issueDescFile = generate.IssueDescFile

// issueDescItem holds an ID+text pair from an issue description.
type issueDescItem = generate.IssueDescItem

// stitchTask holds the state for a single stitch work item.
type stitchTask = generate.StitchTask

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

// errTaskReset is returned by doOneTask when a task fails but the stitch
// loop should continue to the next task.
var errTaskReset = generate.ErrTaskReset

// ---------------------------------------------------------------------------
// Constants re-exported from internal/generate.
// ---------------------------------------------------------------------------

// baseBranchFile is the name of the file that records which branch a
// generation was started from, stored inside the cobbler directory.
const baseBranchFile = generate.BaseBranchFile

// repoRootFile records the main repository root path so GeneratorStop
// can find the main repo when running inside a worktree.
const repoRootFile = generate.RepoRootFile

// tagSuffixes lists the lifecycle tag suffixes in order.
var tagSuffixes = generate.TagSuffixes

// srdRefPattern matches SRD requirement references in task requirement text.
var srdRefPattern = generate.SRDRefPattern

// ---------------------------------------------------------------------------
// gitDeps builds the GitDeps struct for the generate package.
// ---------------------------------------------------------------------------

func (g *Generator) genGitDeps() generate.GitDeps {
	return generate.GitDeps{
		RepoReader:    g.git,
		BranchManager: g.git,
		CommitWriter:  g.git,
	}
}

// stitchGitDeps builds the StitchGitDeps struct for stitch operations.
func (g *Generator) stitchGitDeps() generate.StitchGitDeps {
	return generate.StitchGitDeps{
		RepoReader:      g.git,
		BranchManager:   g.git,
		WorktreeManager: g.git,
	}
}

// stitchIssueDeps builds the StitchIssueDeps struct for stitch operations.
func (g *Generator) stitchIssueDeps(repo, generation string) generate.StitchIssueDeps {
	return generate.StitchIssueDeps{
		ListOpenCobblerIssues: func(r, gen string) ([]generate.StitchIssue, error) {
			issues, err := g.tracker.ListOpenCobblerIssues(r, gen)
			if err != nil {
				return nil, err
			}
			result := make([]generate.StitchIssue, len(issues))
			for i, iss := range issues {
				labels := make([]string, len(iss.Labels))
				copy(labels, iss.Labels)
				result[i] = generate.StitchIssue{
					Number:      iss.Number,
					Title:       iss.Title,
					Description: iss.Description,
					State:       iss.State,
					Labels:      labels,
				}
			}
			return result, nil
		},
		PickReadyIssue: func(r, gen string) (generate.StitchIssue, error) {
			iss, err := g.tracker.PickReadyIssue(r, gen)
			if err != nil {
				return generate.StitchIssue{}, err
			}
			return generate.StitchIssue{
				Number:      iss.Number,
				Title:       iss.Title,
				Description: iss.Description,
				State:       iss.State,
				Labels:      iss.Labels,
			}, nil
		},
		RemoveInProgressLabel: func(r string, num int) error {
			return g.tracker.RemoveIssueLabel(r, num, gh.LabelInProgress)
		},
		HasLabel: func(iss generate.StitchIssue, label string) bool {
			for _, l := range iss.Labels {
				if l == label {
					return true
				}
			}
			return false
		},
		LabelInProgress: gh.LabelInProgress,
	}
}

// ---------------------------------------------------------------------------
// Function delegates — unexported wrappers that preserve the original
// call signatures used throughout the parent package.
// ---------------------------------------------------------------------------

func resolveStopTarget(callerBranch, genBranch, recordedBase string) string {
	return generate.ResolveStopTarget(callerBranch, genBranch, recordedBase)
}

func generationName(tag string) string {
	return generate.GenerationName(tag)
}

func (g *Generator) saveCurrentWork() error {
	return generate.SaveWork("save state before leaving worktree", g.genGitDeps())
}

func (g *Generator) saveAndSwitchBranch(target string) error {
	return generate.SaveAndSwitchBranch(target, g.genGitDeps())
}

func (g *Generator) ensureOnBranch(branch string) error {
	return generate.EnsureOnBranch(branch, g.genGitDeps())
}

func removeEmptyDirs(root string) {
	generate.RemoveEmptyDirs(root)
}

func appendToGitignore(dir, entry string) error {
	return generate.AppendToGitignore(dir, entry)
}

func truncateSHA(sha string) string {
	return generate.TruncateSHA(sha)
}

func measureReleasesConstraint(releases []string, release string) string {
	return generate.MeasureReleasesConstraint(releases, release)
}

func filterImplementedReleases(releases []string) []string {
	return generate.FilterImplementedReleases(releases)
}

func filterImplementedRelease(release string) string {
	return generate.FilterImplementedRelease(release)
}

func validateMeasureOutput(issues []proposedIssue, maxReqs, maxWeight int, subItemCounts map[string]map[string]int, reqStates map[string]map[string]generate.RequirementState) validationResult {
	// Convert proposedIssue (from internal/github) to generate.ProposedIssue.
	genIssues := make([]generate.ProposedIssue, len(issues))
	for i, iss := range issues {
		genIssues[i] = generate.ProposedIssue{
			Index:       iss.Index,
			Title:       iss.Title,
			Description: iss.Description,
			Dependency:  iss.Dependency,
		}
	}
	return generate.ValidateMeasureOutput(genIssues, maxReqs, maxWeight, subItemCounts, reqStates)
}

func splitOverweightTasks(issues []proposedIssue, maxWeight int, subItemCounts map[string]map[string]int, reqStates map[string]map[string]generate.RequirementState) []proposedIssue {
	genIssues := make([]generate.ProposedIssue, len(issues))
	for i, iss := range issues {
		genIssues[i] = generate.ProposedIssue{
			Index:       iss.Index,
			Title:       iss.Title,
			Description: iss.Description,
			Dependency:  iss.Dependency,
		}
	}
	split := generate.SplitOverweightTasks(genIssues, maxWeight, subItemCounts, reqStates)
	result := make([]proposedIssue, len(split))
	for i, s := range split {
		result[i] = proposedIssue{
			Index:       s.Index,
			Title:       s.Title,
			Description: s.Description,
			Dependency:  s.Dependency,
		}
	}
	return result
}

func expandedRequirementCount(reqs []issueDescItem, subItemCounts map[string]map[string]int) int {
	return generate.ExpandedRequirementCount(reqs, subItemCounts)
}

func loadSRDSubItemCounts() map[string]map[string]int {
	return generate.LoadSRDSubItemCounts()
}

func loadRequirementStates(cobblerDir string) map[string]map[string]generate.RequirementState {
	return generate.LoadRequirementStates(cobblerDir)
}

// hasUnresolvedRequirements returns true if any R-item in requirements.yaml
// has status "ready" (not yet implemented or skipped). Used to prevent the
// generator from stopping prematurely when the GitHub API reports no open
// issues but work remains (GH-1475).
func (g *Generator) hasUnresolvedRequirements() bool {
	states := generate.LoadRequirementStates(g.cfg.Cobbler.Dir)
	if states == nil {
		return false
	}
	for _, srdReqs := range states {
		for _, st := range srdReqs {
			if st.Status == "ready" {
				return true
			}
		}
	}
	return false
}

func warnOversizedGroups(maxReqs int) {
	generate.WarnOversizedGroups(maxReqs)
}

func appendMeasureLog(cobblerDir string, newIssues []proposedIssue) {
	// Convert proposedIssue to generate.ProposedIssue.
	genIssues := make([]generate.ProposedIssue, len(newIssues))
	for i, iss := range newIssues {
		genIssues[i] = generate.ProposedIssue{
			Index:       iss.Index,
			Title:       iss.Title,
			Description: iss.Description,
			Dependency:  iss.Dependency,
		}
	}
	generate.AppendMeasureLog(cobblerDir, genIssues)
}

func taskBranchName(baseBranch, issueID string) string {
	return generate.TaskBranchName(baseBranch, issueID)
}

func taskBranchPattern(baseBranch string) string {
	return generate.TaskBranchPattern(baseBranch)
}

func parseRequiredReading(description string) []string {
	return generate.ParseRequiredReading(description)
}

func scopeSourceDirs(configDirs []string, description string) []string {
	return generate.ScopeSourceDirs(configDirs, description)
}

func validateIssueDescription(desc string) error {
	return generate.ValidateIssueDescription(desc)
}

func (g *Generator) recoverStaleBranches(baseBranch, worktreeBase, repo string) bool {
	return generate.RecoverStaleBranches(baseBranch, worktreeBase, repo, g.stitchGitDeps(), g.stitchIssueDeps(repo, ""))
}

func (g *Generator) resetOrphanedIssues(baseBranch, repo, generation string) bool {
	return generate.ResetOrphanedIssues(baseBranch, repo, generation, g.stitchGitDeps(), g.stitchIssueDeps(repo, generation))
}

func (g *Generator) pickTask(baseBranch, worktreeBase, repo, generation string) (stitchTask, error) {
	return generate.PickTask(baseBranch, worktreeBase, repo, generation, g.stitchIssueDeps(repo, generation))
}

func (g *Generator) createWorktree(task stitchTask) error {
	return generate.CreateWorktree(task, g.stitchGitDeps())
}

func commitWorktreeChanges(task stitchTask) error {
	return generate.CommitWorktreeChanges(task)
}

func cleanGoBinaries(dir string) {
	generate.CleanGoBinaries(dir)
}

func (g *Generator) mergeBranch(branchName, baseBranch, repoRoot string) error {
	return generate.MergeBranch(branchName, baseBranch, repoRoot, g.stitchGitDeps())
}

func (g *Generator) cleanupWorktree(task stitchTask) bool {
	return generate.CleanupWorktree(task, g.stitchGitDeps())
}

// ---------------------------------------------------------------------------
// Orchestrator receiver methods
// ---------------------------------------------------------------------------

// GeneratorRun executes N cycles of Measure + Stitch within the current generation.
// If cycles > 0 it overrides configuration.yaml's generation.cycles for this run only.
// cycles == 0 means use the configured value (or unlimited if that is also 0).
func (g *Generator) GeneratorRun(cycles int) error {
	// If not on a generation branch, try to enter the worktree created by
	// GeneratorStart (GH-1608).
	if _, err := g.enterGenerationWorktree(); err != nil {
		return err
	}

	currentBranch, err := g.git.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	if cycles > 0 {
		g.cfg.Generation.Cycles = cycles
	}
	g.cfg.Generation.Branch = currentBranch
	g.setGeneration(currentBranch)
	defer g.clearGeneration()
	return g.RunCycles("run")
}

// GeneratorResume recovers from an interrupted generator:run and continues.
// Reads generation branch from Config.GenerationBranch or auto-detects.
func (g *Generator) GeneratorResume() error {
	// If not on a generation branch, try to enter the worktree (GH-1608).
	if _, err := g.enterGenerationWorktree(); err != nil {
		return err
	}

	branch := g.cfg.Generation.Branch
	if branch == "" {
		resolved, err := g.resolveBranch("")
		if err != nil {
			return fmt.Errorf("resolving generation branch: %w", err)
		}
		branch = resolved
	}

	if !strings.HasPrefix(branch, g.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch: %s\nSet generation.branch in configuration.yaml", branch)
	}
	if !g.git.BranchExists(branch, ".") {
		return fmt.Errorf("branch does not exist: %s", branch)
	}

	g.setGeneration(branch)
	defer g.clearGeneration()

	g.logf("resume: target branch=%s", branch)

	// Commit or stash uncommitted work, then switch to the generation branch.
	if err := g.saveAndSwitchBranch(branch); err != nil {
		return fmt.Errorf("switching to %s: %w", branch, err)
	}

	// Pre-flight cleanup.
	g.logf("resume: pre-flight cleanup")
	wtBase := claude.WorktreeBasePath()

	g.logf("resume: pruning worktrees")
	if err := g.git.WorktreePrune("."); err != nil {
		g.logf("resume: warning: worktree prune: %v", err)
	}

	g.logf("resume: removing worktree directory %s", wtBase)
	if err := os.RemoveAll(wtBase); err != nil {
		g.logf("resume: warning: removing worktree directory %s: %v", wtBase, err)
	}

	g.logf("resume: recovering stale tasks")
	ghRepo, err := g.tracker.DetectGitHubRepo(".")
	if err != nil {
		g.logf("resume: warning: detectGitHubRepo: %v", err)
	}
	if err := g.stitch.recoverStaleTasks(branch, wtBase, ghRepo, branch); err != nil {
		g.logf("resume: recoverStaleTasks warning: %v", err)
	}

	g.cfg.Generation.Branch = branch

	// Drain existing ready issues before starting measure+stitch cycles.
	g.logf("resume: draining existing ready issues")
	if _, err := g.stitch.RunStitch(); err != nil {
		g.logf("resume: drain stitch warning: %v", err)
	}

	return g.RunCycles("resume")
}

// RunCycles runs stitch→measure cycles until no open issues remain.
// Each cycle stitches up to MaxStitchIssuesPerCycle tasks, then measures
// up to MaxMeasureIssues new issues. The loop continues while open issues
// exist. MaxStitchIssues caps total stitch iterations across all cycles
// (0 = unlimited). Cycles caps the number of stitch+measure rounds
// (0 = unlimited). MaxConsecutiveZeroLOCCycles stops the loop when stitch
// produces zero LOC change for N consecutive cycles (default 3), preventing
// runaway refinement loops on fully-implemented specs.
func (g *Generator) RunCycles(label string) error {
	maxZeroLOC := g.cfg.Cobbler.MaxConsecutiveZeroLOCCycles
	g.logf("generator %s: starting (stitchTotal=%d stitchPerCycle=%d measure=%d safetyCycles=%d maxZeroLOC=%d)",
		label, g.cfg.Cobbler.MaxStitchIssues, g.cfg.Cobbler.MaxStitchIssuesPerCycle, g.cfg.Cobbler.MaxMeasureIssues, g.cfg.Generation.Cycles, maxZeroLOC)

	totalStitched := 0
	consecutiveZeroLOC := 0
	for cycle := 1; ; cycle++ {
		if g.cfg.Generation.Cycles > 0 && cycle > g.cfg.Generation.Cycles {
			g.logf("generator %s: reached max cycles (%d), stopping", label, g.cfg.Generation.Cycles)
			break
		}

		// Determine how many tasks this cycle can stitch.
		perCycle := g.cfg.Cobbler.MaxStitchIssuesPerCycle
		if g.cfg.Cobbler.MaxStitchIssues > 0 {
			remaining := g.cfg.Cobbler.MaxStitchIssues - totalStitched
			if remaining <= 0 {
				g.logf("generator %s: reached total stitch limit (%d), stopping", label, g.cfg.Cobbler.MaxStitchIssues)
				break
			}
			if perCycle == 0 || remaining < perCycle {
				perCycle = remaining
			}
		}

		// Refresh analysis before each cycle so stitch sees current state.
		g.analyzer.RunPreCycleAnalysis()

		// Capture LOC before stitch to detect zero-change cycles.
		locBefore := g.claudeRunner.captureLOC()
		g.logf("generator %s: cycle %d — stitch (limit=%d, stitched so far=%d)", label, cycle, perCycle, totalStitched)
		n, err := g.stitch.RunStitchN(perCycle)
		totalStitched += n
		if err != nil {
			return fmt.Errorf("cycle %d stitch: %w", cycle, err)
		}
		locAfter := g.claudeRunner.captureLOC()
		locDelta := (locAfter.Production - locBefore.Production) + (locAfter.Test - locBefore.Test)
		g.logf("generator %s: cycle %d — LOC delta=%d (prod %d→%d, test %d→%d)",
			label, cycle, locDelta, locBefore.Production, locAfter.Production, locBefore.Test, locAfter.Test)

		// Track consecutive zero-LOC cycles as a refinement-loop guard.
		if locDelta == 0 {
			consecutiveZeroLOC++
			g.logf("generator %s: cycle %d — zero LOC change (%d consecutive)", label, cycle, consecutiveZeroLOC)
			if maxZeroLOC > 0 && consecutiveZeroLOC >= maxZeroLOC {
				g.logf("generator %s: %d consecutive zero-LOC cycles reached limit (%d); spec likely complete — stopping",
					label, consecutiveZeroLOC, maxZeroLOC)
				break
			}
		} else {
			consecutiveZeroLOC = 0
		}

		// Validate UC implementation after stitch so measure sees which UCs
		// are done and skips their requirements (GH-1361). Only marks UCs
		// whose tests exist and pass — unlike the old blanket marking.
		if totalStitched > 0 {
			g.validateAndMarkUCs()
		}

		// Check if the current release is complete and auto-advance if so.
		if advanced, ver := g.checkAutoAdvanceRelease(); advanced {
			g.logf("generator %s: cycle %d — auto-advanced release %s", label, cycle, ver)
		}

		// If the only remaining open issues are skipped, stop the generation
		// immediately rather than wasting cycles (GH-1699).
		if onlySkipped, skippedErr := g.claudeRunner.hasOnlySkippedIssues(); skippedErr == nil && onlySkipped {
			g.logf("generator %s: cycle %d — only skipped tasks remain, stopping", label, cycle)
			break
		}

		// Skip measure if open issues remain — stitch should drain them first (GH-1352).
		if openBefore, err := g.claudeRunner.hasOpenIssues(); err == nil && openBefore {
			g.logf("generator %s: cycle %d — skipping measure, open issues remain", label, cycle)
		} else {
			g.logf("generator %s: cycle %d — measure", label, cycle)
			if err := g.measure.RunMeasure(); err != nil {
				return fmt.Errorf("cycle %d measure: %w", cycle, err)
			}
		}

		open, err := g.claudeRunner.hasOpenIssues()
		if err != nil {
			g.logf("generator %s: hasOpenIssues error (assuming open): %v", label, err)
		}
		if !open && err == nil {
			// Before stopping, check if unresolved requirements remain.
			// The GitHub API may report no open issues due to a race
			// condition (stale cache after closing the last task). If
			// requirements are still "ready", run measure to create new
			// tasks instead of stopping prematurely (GH-1475).
			if g.hasUnresolvedRequirements() {
				g.logf("generator %s: cycle %d — no open issues but unresolved requirements remain, running measure",
					label, cycle)
				if err := g.measure.RunMeasure(); err != nil {
					return fmt.Errorf("cycle %d measure (fallback): %w", cycle, err)
				}
			} else {
				g.logf("generator %s: no open issues remain, stopping after %d cycle(s)", label, cycle)
				break
			}
		}
		g.logf("generator %s: open issues remain, continuing to cycle %d", label, cycle+1)
	}

	g.logf("generator %s: complete (total stitched=%d)", label, totalStitched)
	return nil
}

// checkAutoAdvanceRelease detects when the current release's use cases are all
// done and auto-advances by calling ReleaseUpdate (which marks UCs as
// "implemented" in road-map.yaml and removes the release from
// configuration.yaml). Changes are committed on the current branch. Returns
// (true, version) if a release was advanced, (false, "") otherwise.
func (g *Generator) checkAutoAdvanceRelease() (bool, string) {
	rm := ictx.LoadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return false, ""
	}

	// Find the first release that is not yet done/implemented.
	var target *RoadmapRelease
	for i := range rm.Releases {
		rel := &rm.Releases[i]
		if !ictx.UCStatusDone(rel.Status) {
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
		if !ictx.UCStatusDone(uc.Status) {
			return false, ""
		}
	}

	// All UCs are done, but check that all SRD requirements from this
	// release are also complete. UC touchpoints may not cite every R-group
	// in a SRD, leaving orphaned ready requirements that become invisible
	// once the release is filtered from the measure prompt (GH-1952).
	if g.releaseHasReadyRequirements(target.UseCases) {
		g.logf("checkAutoAdvanceRelease: release %s has all UCs done but ready requirements remain, not advancing", target.Version)
		return false, ""
	}

	// All UCs done and all requirements complete — auto-advance.
	g.logf("checkAutoAdvanceRelease: release %s has all use cases done, advancing", target.Version)
	if err := g.releaser.ReleaseUpdate(target.Version); err != nil {
		g.logf("checkAutoAdvanceRelease: ReleaseUpdate(%s) failed: %v", target.Version, err)
		return false, ""
	}

	// Commit the changes on the current branch.
	_ = g.git.StageAll(".")
	msg := fmt.Sprintf("Auto-advance release %s: all use cases complete\n\nMarked use cases as implemented in road-map.yaml.\nRemoved %s from project.releases in configuration.yaml.", target.Version, target.Version)
	if err := g.git.Commit(msg, "."); err != nil {
		g.logf("checkAutoAdvanceRelease: commit failed: %v", err)
	}

	// Reload config so subsequent measure sees updated releases.
	if cfg, err := LoadConfig(DefaultConfigFile); err == nil {
		g.cfg = cfg
	}

	return true, target.Version
}

// releaseHasReadyRequirements returns true if any requirement from SRDs
// referenced by the release's use case touchpoints is still "ready" in
// requirements.yaml. This checks ALL requirements in each referenced SRD,
// not just the R-groups cited by touchpoints, to catch orphaned R-items
// that no UC touchpoint covers (GH-1952).
func (g *Generator) releaseHasReadyRequirements(useCases []RoadmapUseCase) bool {
	states := generate.LoadRequirementStates(g.cfg.Cobbler.Dir)
	if states == nil {
		return false
	}

	// Collect all SRD stems referenced by this release's UC touchpoints.
	// TouchpointSRDRefRe matches "srdNNN-name R1, R2" (with R-groups).
	// BareSRDRefRe matches "srdNNN-name" even without R-groups, catching
	// touchpoints like "(srd096-users)" that omit R-group refs (GH-1960).
	srdStems := make(map[string]bool)
	for _, uc := range useCases {
		touchpoints := loadUCTouchpoints(uc.ID)
		for _, tp := range touchpoints {
			matches := generate.TouchpointSRDRefRe.FindAllStringSubmatch(tp, -1)
			for _, m := range matches {
				srdStems[m[1]] = true
			}
			// Also match bare SRD references without R-groups.
			bareMatches := generate.BareSRDRefRe.FindAllString(tp, -1)
			for _, stem := range bareMatches {
				srdStems[stem] = true
			}
		}
	}

	// Check if any requirement in those SRDs is still ready.
	for stem := range srdStems {
		for srdKey, srdReqs := range states {
			if srdKey != stem && !strings.HasPrefix(srdKey, stem+"-") {
				continue
			}
			for id, st := range srdReqs {
				if st.Status == "ready" {
					g.logf("releaseHasReadyRequirements: %s %s is still ready", srdKey, id)
					return true
				}
			}
		}
	}
	return false
}

// validateAndMarkUCs finds the first non-implemented release in road-map.yaml
// and validates each UC individually. A UC is marked "implemented" only when
// its test files exist and pass. Replaces the former markCompletedReleaseUCs /
// markActiveReleaseUCsDone pair that blindly marked all UCs when no open issues
// remained (GH-1361).
func (g *Generator) validateAndMarkUCs() {
	rm := ictx.LoadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return
	}

	// Find the first release that is not yet done/implemented.
	var target *RoadmapRelease
	for i := range rm.Releases {
		rel := &rm.Releases[i]
		if !ictx.UCStatusDone(rel.Status) {
			target = rel
			break
		}
	}
	if target == nil || len(target.UseCases) == 0 {
		return
	}

	var marked []string
	for _, uc := range target.UseCases {
		if ictx.UCStatusDone(uc.Status) {
			continue
		}

		// Load UC touchpoints to extract SRD citations.
		ucTouchpoints := loadUCTouchpoints(uc.ID)
		if len(ucTouchpoints) == 0 {
			g.logf("validateAndMarkUCs: UC %s — no touchpoints found, skipping", uc.ID)
			continue
		}

		// Requirement-based completion is the primary gate (GH-1378).
		complete, remaining := generate.UCRequirementsComplete(g.cfg.Cobbler.Dir, ucTouchpoints)
		if !complete {
			g.logf("validateAndMarkUCs: UC %s — %d requirements still pending: %v", uc.ID, len(remaining), remaining)
			continue
		}

		// Test-based validation is a secondary signal (log-only).
		if !g.validateUCImplemented(uc.ID) {
			g.logf("validateAndMarkUCs: UC %s — requirements complete but tests missing/failing (non-blocking)", uc.ID)
		}

		g.logf("validateAndMarkUCs: UC %s validated — marking as implemented", uc.ID)
		if err := updateRoadmapSingleUCStatus(target.Version, uc.ID, "implemented"); err != nil {
			g.logf("validateAndMarkUCs: failed to mark %s: %v", uc.ID, err)
			continue
		}
		marked = append(marked, uc.ID)
	}

	if len(marked) == 0 {
		return
	}

	_ = g.git.StageAll(".")
	msg := fmt.Sprintf("Mark validated UCs as implemented in release %s\n\nUCs with complete requirements: %s",
		target.Version, strings.Join(marked, ", "))
	if err := g.git.Commit(msg, "."); err != nil {
		g.logf("validateAndMarkUCs: commit failed: %v", err)
	}
}

// loadUCTouchpoints loads a use case file and returns its touchpoints as
// flat strings (e.g. "T1: Config struct: ... srd001-core R1"). Returns nil
// if the UC file is not found or cannot be parsed.
func loadUCTouchpoints(ucID string) []string {
	path := filepath.Join("docs/specs/use-cases", ucID+".yaml")
	uc, err := an.LoadUseCase(path)
	if err != nil {
		return nil
	}
	return uc.Touchpoints
}

// validateUCImplemented checks whether a use case has test files and whether
// those tests pass. Returns true only if both conditions are met.
func (g *Generator) validateUCImplemented(ucID string) bool {
	testDir := an.TestDirForUC(ucID)
	if testDir == "" {
		return false
	}
	testCount := an.CountTestFiles(testDir)
	if testCount == 0 {
		g.logf("validateUCImplemented: %s — no test files in %s", ucID, testDir)
		return false
	}

	// Run the UC's tests. Use -count=1 to disable caching.
	cmd := exec.Command("go", "test", "-tags=usecase", "-count=1", "-timeout", "300s", "./"+testDir+"/...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		g.logf("validateUCImplemented: %s — tests failed: %v\n%s", ucID, err, string(out))
		return false
	}
	g.logf("validateUCImplemented: %s — %d test file(s) pass", ucID, testCount)
	return true
}

// GeneratorStart begins a new generation trail.
// Records the current branch as the base branch, tags it, creates a generation
// branch, deletes Go files, reinitializes the Go module, and commits the clean
// state. Any clean branch is a valid starting point (srd002 R2.1).
func (g *Generator) GeneratorStart() error {
	baseBranch, err := g.git.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Reject dirty worktrees — a generation must start from a clean state.
	if g.git.HasChanges(".") {
		return fmt.Errorf("worktree has uncommitted changes on %s; commit or stash before starting a generation", baseBranch)
	}

	// Clear history from any previous generation so the new generation
	// starts with a clean slate (GH-1356). The history directory may
	// survive across generations when it is gitignored or when
	// generator:stop was not called.
	if err := g.claudeRunner.HistoryClean(); err != nil {
		g.logf("generator:start: warning clearing history: %v", err)
	}

	// Garbage-collect issues from generations whose branch no longer exists.
	// This catches leaks from crashed tests or prior runs without cleanup.
	if ghRepo, err := g.tracker.DetectGitHubRepo("."); err == nil && ghRepo != "" {
		g.tracker.GcStaleGenerationIssues(ghRepo, g.cfg.Generation.Prefix)
	}

	suffix := os.Getenv("COBBLER_GEN_NAME")
	if suffix == "" {
		suffix = g.cfg.Generation.Name
	}
	if suffix == "" {
		suffix = time.Now().Format("2006-01-02-15-04-05")
	}
	genName := g.cfg.Generation.Prefix + suffix
	startTag := genName + "-start"

	g.setGeneration(genName)
	defer g.clearGeneration()

	g.logf("generator:start: beginning (base branch: %s)", baseBranch)

	// Tag the current base branch state before the generation begins.
	g.logf("generator:start: tagging current state as %s", startTag)
	if err := g.git.Tag(startTag, "."); err != nil {
		return fmt.Errorf("tagging base branch: %w", err)
	}

	// Resolve the main repo root before creating the worktree. All
	// subsequent operations will run inside the worktree via os.Chdir.
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving repo root: %w", err)
	}

	// Create a sibling worktree for the generation branch. The main repo
	// stays on the base branch so generator:stop does not accidentally
	// merge into the wrong place (GH-1608).
	worktreeDir := filepath.Join(filepath.Dir(repoRoot), genName)
	g.logf("generator:start: creating worktree at %s", worktreeDir)

	if err := g.git.CreateBranch(genName, "."); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}
	cmd := g.git.WorktreeAdd(worktreeDir, genName, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Clean up the branch if worktree creation fails.
		_ = g.git.DeleteBranch(genName, ".")
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Switch into the worktree so all subsequent operations (source reset,
	// squash, etc.) run there rather than in the main repo.
	if err := os.Chdir(worktreeDir); err != nil {
		return fmt.Errorf("switching to worktree: %w", err)
	}

	// Record branch point so intermediate commits can be squashed.
	branchSHA, err := g.git.RevParseHEAD(".")
	if err != nil {
		return fmt.Errorf("getting branch HEAD: %w", err)
	}

	// Record the base branch so GeneratorStop knows where to merge back
	// (srd002 R2.8).
	if err := g.writeBaseBranch(baseBranch); err != nil {
		return fmt.Errorf("recording base branch: %w", err)
	}

	// Record the main repo root so GeneratorStop can find it from the
	// worktree (GH-1608).
	if err := g.writeRepoRoot(repoRoot); err != nil {
		return fmt.Errorf("recording repo root: %w", err)
	}

	// Ensure bin/ is ignored on the generation branch so compiled binaries
	// are never staged by git add -A (GH-469).
	if err := appendToGitignore(".", g.cfg.Project.BinaryDir+"/"); err != nil {
		g.logf("generator:start: warning: could not update .gitignore: %v", err)
	}

	// Reset Go sources and reinitialize module unless preserve_sources is set.
	// Library repos (e.g. cobbler-scaffold itself) set preserve_sources: true so
	// generator:start does not destroy the library code. See srd002 R10.1.
	if g.cfg.Generation.PreserveSources {
		g.logf("generator:start: preserve_sources=true, skipping Go source reset")
	} else {
		g.logf("generator:start: resetting Go sources")
		if err := g.resetGoSources(genName); err != nil {
			return fmt.Errorf("resetting Go sources: %w", err)
		}
		// Reset roadmap statuses so measure does not skip releases whose
		// code was just deleted (GH-1368).
		if err := g.resetImplementedReleases(); err != nil {
			g.logf("generator:start: warning resetting releases: %v", err)
		}
	}

	// Generate requirements state file from SRD R-items (GH-1378).
	srdDir := "docs/specs/software-requirements"
	if _, err := generate.GenerateRequirementsFile(srdDir, g.cfg.Cobbler.Dir, g.cfg.Generation.PreserveSources); err != nil {
		g.logf("generator:start: warning generating requirements file: %v", err)
	}

	// Squash intermediate commits into one clean commit.
	g.logf("generator:start: squashing into single commit")
	if err := g.git.ResetSoft(branchSHA, "."); err != nil {
		return fmt.Errorf("squashing start commits: %w", err)
	}
	_ = g.git.StageAll(".")
	var msg string
	if g.cfg.Generation.PreserveSources {
		msg = fmt.Sprintf("Start generation: %s\n\nBase branch: %s. Sources preserved (preserve_sources=true).\nTagged previous state as %s.", genName, baseBranch, genName)
	} else {
		msg = fmt.Sprintf("Start generation: %s\n\nBase branch: %s. Delete Go files, reinitialize module.\nTagged previous state as %s.", genName, baseBranch, genName)
	}
	// Use allow-empty because a specs-only repo may have no Go files
	// to delete, leaving no changes to commit after source reset.
	if err := g.git.CommitAllowEmpty(msg, "."); err != nil {
		return fmt.Errorf("committing clean state: %w", err)
	}

	g.logf("generator:start: done, worktree is at %s", worktreeDir)
	g.logf("generator:start: run mage generator:run to begin building")
	return nil
}

// writeBaseBranch writes the base branch name to .cobbler/base-branch.
func (g *Generator) writeBaseBranch(branch string) error {
	dir := g.cfg.Cobbler.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, baseBranchFile), []byte(branch+"\n"), 0o644)
}

// writeRepoRoot writes the main repository root path to .cobbler/repo-root.
// GeneratorStop reads this to locate the main repo when running inside a worktree.
func (g *Generator) writeRepoRoot(root string) error {
	dir := g.cfg.Cobbler.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, repoRootFile), []byte(root+"\n"), 0o644)
}

// readRepoRoot reads the main repository root from .cobbler/repo-root.
// Returns "" if the file does not exist (pre-worktree generations).
func (g *Generator) readRepoRoot() string {
	data, err := os.ReadFile(filepath.Join(g.cfg.Cobbler.Dir, repoRootFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// findGenerationWorktree searches for a git worktree on a generation branch
// (matching the given prefix) by parsing `git worktree list --porcelain`.
// Returns the worktree path or "" if none found.
func findGenerationWorktree(prefix string) string {
	out, err := exec.Command(binGit, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return ""
	}
	var currentPath string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch refs/heads/"+prefix) && currentPath != "" {
			return currentPath
		}
	}
	return ""
}

// enterGenerationWorktree checks whether the current repo has a worktree
// on a generation branch. If so, it changes the working directory to that
// worktree. Returns the worktree path (empty if none found) and any error.
func (g *Generator) enterGenerationWorktree() (string, error) {
	// If we're already on a generation branch, no need to search.
	if branch, err := g.git.CurrentBranch("."); err == nil {
		if strings.HasPrefix(branch, "generation-") {
			return "", nil
		}
	}

	wtPath := findGenerationWorktree("generation-")
	if wtPath == "" {
		return "", nil
	}
	g.logf("auto-entering generation worktree at %s", wtPath)
	if err := os.Chdir(wtPath); err != nil {
		return "", fmt.Errorf("switching to generation worktree %s: %w", wtPath, err)
	}
	return wtPath, nil
}

// readBaseBranch reads the base branch from .cobbler/base-branch on the
// current branch. Returns "main" if the file does not exist (backward
// compatibility with older generations, srd002 R5.3).
func (g *Generator) readBaseBranch() string {
	data, err := os.ReadFile(filepath.Join(g.cfg.Cobbler.Dir, baseBranchFile))
	if err != nil {
		return "main"
	}
	branch := strings.TrimSpace(string(data))
	if branch == "" {
		return "main"
	}
	return branch
}

// GeneratorStop completes a generation trail and merges it into the base branch.
// Reads the base branch from .cobbler/base-branch (falls back to "main").
// Uses Config.GenerationBranch, current branch, or auto-detects.
//
// When running inside a worktree created by GeneratorStart (GH-1608), the
// function tags the generation in the worktree, switches to the main repo for
// the merge, and removes the worktree afterward.
func (g *Generator) GeneratorStop() error {
	// If invoked from the main repo, try to enter the worktree (GH-1608).
	if _, err := g.enterGenerationWorktree(); err != nil {
		return err
	}

	branch := g.cfg.Generation.Branch
	if branch != "" {
		if !g.git.BranchExists(branch, ".") {
			return fmt.Errorf("branch does not exist: %s", branch)
		}
	} else {
		current, err := g.git.CurrentBranch(".")
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
		if strings.HasPrefix(current, g.cfg.Generation.Prefix) {
			branch = current
			g.logf("generator:stop: stopping current branch %s", branch)
		} else {
			resolved, err := g.resolveBranch("")
			if err != nil {
				return err
			}
			branch = resolved
		}
	}

	if !strings.HasPrefix(branch, g.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch: %s\nSet generation.branch in configuration.yaml", branch)
	}

	g.setGeneration(branch)
	defer g.clearGeneration()

	finishedTag := branch + "-finished"

	g.logf("generator:stop: beginning")

	// Detect whether we are inside a worktree created by GeneratorStart.
	repoRoot := g.readRepoRoot()
	var worktreeDir string
	if repoRoot != "" {
		var err error
		worktreeDir, err = filepath.Abs(".")
		if err != nil {
			return fmt.Errorf("resolving worktree directory: %w", err)
		}
		g.logf("generator:stop: running in worktree %s (main repo: %s)", worktreeDir, repoRoot)
	}

	// Capture the caller's branch before switching to the generation branch.
	callerBranch, err := g.git.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Switch to the generation branch and tag its final state.
	if err := g.ensureOnBranch(branch); err != nil {
		return fmt.Errorf("switching to generation branch: %w", err)
	}

	// Determine the merge target (GH-523).
	recordedBase := g.readBaseBranch()
	baseBranch := recordedBase
	if repoRoot == "" {
		// Legacy path (no worktree): respect caller branch override.
		baseBranch = resolveStopTarget(callerBranch, branch, recordedBase)
		if baseBranch != recordedBase {
			g.logf("generator:stop: caller was on %s; using it as merge target instead of recorded base %s", callerBranch, recordedBase)
		}
	}

	// Commit any uncommitted history files (orchestrator logs, late stats)
	// so they are captured in the finished tag for post-hoc analysis (GH-1452).
	if hdir := g.claudeRunner.historyDir(); hdir != "" {
		if err := g.git.StageDir(hdir, "."); err == nil && g.git.HasChanges(".") {
			g.logf("generator:stop: committing history files before tagging")
			_ = g.git.Commit("Commit history files before generator:stop tag", ".")
		}
	}

	g.logf("generator:stop: tagging as %s", finishedTag)
	if err := g.git.Tag(finishedTag, "."); err != nil {
		return fmt.Errorf("tagging generation: %w", err)
	}

	// Add a specs-only cleanup commit to the generation branch so that
	// merging — whether directly or via PR — leaves the base branch
	// clean. The generated code is preserved at the -finished tag
	// (GH-1876).
	if !g.cfg.Generation.PreserveSources {
		g.logf("generator:stop: adding specs-only cleanup to generation branch")
		g.cleanGoSources()
		if err := g.claudeRunner.HistoryClean(); err != nil {
			g.logf("generator:stop: warning cleaning history on generation branch: %v", err)
		}
		_ = g.git.StageAll(".")
		cleanupMsg := fmt.Sprintf("Reset %s to specs-only for merge\n\nGenerated code preserved at tag %s.",
			branch, finishedTag)
		if err := g.git.CommitAllowEmpty(cleanupMsg, "."); err != nil {
			return fmt.Errorf("committing specs-only cleanup on generation branch: %w", err)
		}
	}

	// If running in a worktree, switch to the main repo for the merge.
	if repoRoot != "" {
		g.logf("generator:stop: switching to main repo at %s", repoRoot)
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("switching to main repo: %w", err)
		}
	} else {
		// Legacy path: switch to the base branch in the current repo.
		g.logf("generator:stop: switching to %s", baseBranch)
		if err := g.git.Checkout(baseBranch, "."); err != nil {
			return fmt.Errorf("checking out %s: %w", baseBranch, err)
		}
	}

	// Clean up untracked history files on the base branch so they don't
	// persist across branches (GH-1356). The history is preserved in the
	// finished tag above (GH-1452).
	if err := g.claudeRunner.HistoryClean(); err != nil {
		g.logf("generator:stop: warning clearing history: %v", err)
	}

	// Remove the worktree before merging so the generation branch is not
	// locked by the worktree checkout. mergeGeneration deletes the branch
	// after a successful merge; git refuses to delete a branch that is
	// checked out in any worktree (GH-1608).
	if worktreeDir != "" {
		g.logf("generator:stop: removing worktree %s", worktreeDir)
		if err := g.git.WorktreeRemove(worktreeDir, "."); err != nil {
			g.logf("generator:stop: worktree remove warning: %v", err)
		}
		_ = g.git.WorktreePrune(".")
	}

	if err := g.mergeGeneration(branch, baseBranch); err != nil {
		return err
	}

	// Close any open cobbler-gen issues for this generation.
	if ghRepo, err := g.tracker.DetectGitHubRepo("."); err == nil && ghRepo != "" {
		if err := g.tracker.CloseGenerationIssues(ghRepo, branch); err != nil {
			g.logf("generator:stop: close issues warning: %v", err)
		}
	}

	// Reset all implemented releases back to spec_complete (GH-1021).
	if err := g.resetImplementedReleases(); err != nil {
		g.logf("generator:stop: reset releases warning: %v", err)
	}

	g.cleanupDirs()

	g.logf("generator:stop: done, work is on %s", baseBranch)
	return nil
}

// mergeGeneration merges the generation branch into the base branch, tags
// the result, and deletes the generation branch. The generation branch is
// expected to already be in specs-only state (cleanup committed before merge
// by GeneratorStop, GH-1876) unless PreserveSources is true.
func (g *Generator) mergeGeneration(branch, baseBranch string) error {
	g.logf("generator:stop: merging %s into %s", branch, baseBranch)
	cmd := g.git.MergeCmd(branch, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("merging %s: %w", branch, err)
	}

	// Restore Go files from earlier generations that may not be on the
	// generation branch (safety net for edge cases).
	startTag := branch + "-start"
	if err := g.restoreFromStartTag(startTag); err != nil {
		g.logf("generator:stop: restore warning: %v", err)
	}

	mergedTag := branch + "-merged"
	g.logf("generator:stop: tagging %s as %s", baseBranch, mergedTag)
	if err := g.git.Tag(mergedTag, "."); err != nil {
		return fmt.Errorf("tagging merge: %w", err)
	}

	// Clean history files that may have leaked onto the base branch.
	if err := g.claudeRunner.HistoryClean(); err != nil {
		g.logf("generator:stop: warning cleaning history: %v", err)
	}
	_ = g.git.StageAll(".")
	_ = g.git.Commit("Clean history after generation merge", ".") // best-effort

	g.logf("generator:stop: deleting branch %s", branch)
	_ = g.git.ForceDeleteBranch(branch, ".") // force-delete: safe -d fails after specs-only reset
	return nil
}

// resetImplementedReleases loads road-map.yaml, finds all releases with
// status "implemented", and calls ReleaseClear for each to reset them to
// "spec_complete" and repopulate configuration.yaml (GH-1021).
// Also reverts individual UC statuses that were marked "implemented" by
// validateAndMarkUCs even when the release itself is not yet implemented
// (GH-1469).
func (g *Generator) resetImplementedReleases() error {
	rm := ictx.LoadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return nil
	}
	var cleared []string
	var revertedUCs []string
	for _, rel := range rm.Releases {
		if strings.EqualFold(rel.Status, "implemented") {
			if err := g.releaser.ReleaseClear(rel.Version); err != nil {
				g.logf("resetImplementedReleases: ReleaseClear(%s) failed: %v", rel.Version, err)
				continue
			}
			cleared = append(cleared, rel.Version)
			continue
		}
		// Revert individual UC statuses even when the release itself is
		// not yet implemented (GH-1469). validateAndMarkUCs promotes UCs
		// one at a time during the run; generator:stop must undo them.
		for _, uc := range rel.UseCases {
			if ictx.UCStatusDone(uc.Status) {
				if err := updateRoadmapSingleUCStatus(rel.Version, uc.ID, "spec_complete"); err != nil {
					g.logf("resetImplementedReleases: revert UC %s in %s failed: %v", uc.ID, rel.Version, err)
					continue
				}
				revertedUCs = append(revertedUCs, uc.ID)
			}
		}
	}
	if len(cleared) == 0 && len(revertedUCs) == 0 {
		return nil
	}
	_ = g.git.StageAll(".")
	var parts []string
	if len(cleared) > 0 {
		parts = append(parts, fmt.Sprintf("Releases: %s", strings.Join(cleared, ", ")))
	}
	if len(revertedUCs) > 0 {
		parts = append(parts, fmt.Sprintf("UCs: %s", strings.Join(revertedUCs, ", ")))
	}
	msg := fmt.Sprintf("Reset statuses to spec_complete after generator:stop\n\n%s", strings.Join(parts, "\n"))
	if err := g.git.Commit(msg, "."); err != nil {
		return fmt.Errorf("commit release reset: %w", err)
	}
	g.logf("resetImplementedReleases: cleared %d release(s), reverted %d UC(s)", len(cleared), len(revertedUCs))
	return nil
}

// restoreFromStartTag restores Go source files that existed on main at the
// given start tag but are missing after the merge.
func (g *Generator) restoreFromStartTag(startTag string) error {
	startFiles, err := g.git.LsTreeFiles(startTag, ".")
	if err != nil {
		return fmt.Errorf("listing files at %s: %w", startTag, err)
	}

	var restored []string
	for _, path := range startFiles {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if strings.HasPrefix(path, g.cfg.Project.MagefilesDir+"/") {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}

		content, err := g.git.ShowFileContent(startTag, path, ".")
		if err != nil {
			g.logf("generator:stop: could not read %s from %s: %v", path, startTag, err)
			continue
		}

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			g.logf("generator:stop: could not create directory %s: %v", dir, err)
			continue
		}

		if err := os.WriteFile(path, content, 0o644); err != nil {
			g.logf("generator:stop: could not write %s: %v", path, err)
			continue
		}
		restored = append(restored, path)
	}

	if len(restored) == 0 {
		return nil
	}

	g.logf("generator:stop: restored %d file(s) from earlier generations", len(restored))
	_ = g.git.StageAll(".")
	msg := fmt.Sprintf("Restore %d file(s) from earlier generations\n\nFiles restored from %s:\n%s",
		len(restored), startTag, strings.Join(restored, "\n"))
	if err := g.git.Commit(msg, "."); err != nil {
		return fmt.Errorf("committing restored files: %w", err)
	}
	return nil
}

// listGenerationBranches returns all generation-* branch names.
func (g *Generator) listGenerationBranches() []string {
	return g.git.ListBranches(g.cfg.Generation.Prefix+"*", ".")
}

// cleanupUnmergedTags renames tags for generations that were never
// merged into a single -abandoned tag.
func (g *Generator) cleanupUnmergedTags() {
	tags := g.git.ListTags(g.cfg.Generation.Prefix+"*", ".")
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
				g.logf("generator:reset: marking abandoned: %s -> %s", t, abTag)
				_ = g.git.RenameTag(t, abTag, ".") // best-effort; tag may not exist
			}
		} else {
			g.logf("generator:reset: removing tag %s", t)
			_ = g.git.DeleteTag(t, ".") // best-effort cleanup
		}
	}
}

// resolveBranch determines which branch to work on.
func (g *Generator) resolveBranch(explicit string) (string, error) {
	if explicit != "" {
		if !g.git.BranchExists(explicit, ".") {
			return "", fmt.Errorf("branch does not exist: %s", explicit)
		}
		return explicit, nil
	}

	branches := g.listGenerationBranches()
	switch len(branches) {
	case 0:
		return g.git.CurrentBranch(".")
	case 1:
		return branches[0], nil
	default:
		slices.Sort(branches)
		return "", fmt.Errorf("multiple generation branches exist (%s); set generation.branch in configuration.yaml", strings.Join(branches, ", "))
	}
}

// GeneratorList shows active branches and past generations.
func (g *Generator) GeneratorList() error {
	branches := g.listGenerationBranches()
	tags := g.git.ListTags(g.cfg.Generation.Prefix+"*", ".")
	current, _ := g.git.CurrentBranch(".")

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
//
// When running inside a worktree created by GeneratorStart (GH-2043) and the
// target is the base branch, a plain checkout would fail because git does not
// allow the same branch to be checked out in two worktrees. In that case we
// save work, switch to the main repo, and remove the worktree.
func (g *Generator) GeneratorSwitch() error {
	// If invoked from the main repo, try to enter the worktree (GH-1608).
	if _, err := g.enterGenerationWorktree(); err != nil {
		return err
	}

	target := g.cfg.Generation.Branch
	baseBranch := g.cfg.Cobbler.BaseBranch
	if target == "" {
		return fmt.Errorf("set generation.branch in configuration.yaml\nAvailable branches: %s, %s", strings.Join(g.listGenerationBranches(), ", "), baseBranch)
	}

	if target != baseBranch && !strings.HasPrefix(target, g.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch or %s: %s", baseBranch, target)
	}
	if !g.git.BranchExists(target, ".") {
		return fmt.Errorf("branch does not exist: %s", target)
	}

	current, err := g.git.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if current == target {
		g.logf("generator:switch: already on %s", target)
		return nil
	}

	// When switching to the base branch from a worktree, git checkout would
	// fail because the base branch is already checked out in the main repo.
	// Save work, switch to the main repo, and remove the worktree (GH-2043).
	repoRoot := g.readRepoRoot()
	if repoRoot != "" && target == baseBranch {
		g.logf("generator:switch: saving work on %s before leaving worktree", current)
		if err := g.saveCurrentWork(); err != nil {
			return fmt.Errorf("saving work: %w", err)
		}
		worktreeDir, _ := filepath.Abs(".")
		g.logf("generator:switch: switching to main repo at %s", repoRoot)
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("switching to main repo: %w", err)
		}
		g.logf("generator:switch: removing worktree %s", worktreeDir)
		if err := g.git.WorktreeRemove(worktreeDir, "."); err != nil {
			g.logf("generator:switch: worktree remove warning: %v", err)
		}
		_ = g.git.WorktreePrune(".")
		g.logf("generator:switch: now on %s", target)
		return nil
	}

	if err := g.saveAndSwitchBranch(target); err != nil {
		return fmt.Errorf("switching to %s: %w", target, err)
	}

	g.logf("generator:switch: now on %s", target)
	return nil
}

// GeneratorReset destroys generation branches, worktrees, and Go source directories.
func (g *Generator) GeneratorReset() error {
	g.logf("generator:reset: beginning")

	// If a generation worktree exists, remove it first and switch to
	// the main repo (GH-1608).
	repoRoot := g.readRepoRoot()
	if repoRoot != "" {
		worktreeDir, _ := filepath.Abs(".")
		g.logf("generator:reset: removing generation worktree %s", worktreeDir)
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("switching to main repo: %w", err)
		}
		_ = g.git.WorktreeRemove(worktreeDir, ".")
		_ = g.git.WorktreePrune(".")
		// worktree path cleaned up via worktree remove (GH-1608)
	} else {
		// Check if there's a generation worktree to remove.
		if wtPath := findGenerationWorktree(g.cfg.Generation.Prefix); wtPath != "" {
			g.logf("generator:reset: removing generation worktree %s", wtPath)
			_ = g.git.WorktreeRemove(wtPath, ".")
			_ = g.git.WorktreePrune(".")
		}
	}

	baseBranch := g.cfg.Cobbler.BaseBranch
	if err := g.ensureOnBranch(baseBranch); err != nil {
		return fmt.Errorf("switching to %s: %w", baseBranch, err)
	}

	wtBase := claude.WorktreeBasePath()
	ghRepo, _ := g.tracker.DetectGitHubRepo(".")
	genBranches := g.listGenerationBranches()
	if len(genBranches) > 0 {
		g.logf("generator:reset: removing task branches and worktrees")
		for _, gb := range genBranches {
			g.recoverStaleBranches(gb, wtBase, ghRepo)
		}
	}

	if ghRepo != "" {
		g.logf("generator:reset: closing GitHub issues")
		for _, gb := range genBranches {
			if err := g.tracker.CloseGenerationIssues(ghRepo, gb); err != nil {
				g.logf("generator:reset: close issues warning for %s: %v", gb, err)
			}
		}
		if err := g.tracker.CloseGenerationIssues(ghRepo, baseBranch); err != nil {
			g.logf("generator:reset: close issues warning for %s: %v", baseBranch, err)
		}
	}

	if err := g.git.WorktreePrune("."); err != nil {
		g.logf("generator:reset: warning: worktree prune: %v", err)
	}

	if _, err := os.Stat(wtBase); err == nil {
		g.logf("generator:reset: removing worktree directory %s", wtBase)
		if err := os.RemoveAll(wtBase); err != nil {
			g.logf("generator:reset: warning: removing worktree dir: %v", err)
		}
	}

	if len(genBranches) > 0 {
		// Prune again to ensure worktree registrations are fully cleaned up
		// before deleting branches (GH-1608). Without this, git may refuse
		// to delete a branch it still thinks is checked out in a worktree.
		_ = g.git.WorktreePrune(".")
		g.logf("generator:reset: removing %d generation branch(es)", len(genBranches))
		for _, gb := range genBranches {
			g.logf("generator:reset: deleting branch %s", gb)
			_ = g.git.ForceDeleteBranch(gb, ".")
		}
	}

	g.cleanupUnmergedTags()

	g.logf("generator:reset: removing Go source directories")
	for _, dir := range g.cfg.Project.GoSourceDirs {
		g.logf("generator:reset: removing %s", dir)
		os.RemoveAll(dir) // nolint: best-effort directory cleanup
	}
	os.RemoveAll(g.cfg.Project.BinaryDir + "/") // nolint: best-effort directory cleanup
	g.cleanupDirs()

	g.logf("generator:reset: seeding Go sources and reinitializing go.mod")
	if err := g.seedFiles(baseBranch); err != nil {
		return fmt.Errorf("seeding files: %w", err)
	}
	if err := g.reinitGoModule(); err != nil {
		return fmt.Errorf("reinitializing go module: %w", err)
	}

	g.logf("generator:reset: committing clean state")
	_ = g.git.StageAll(".")                                                  // best-effort; commit below handles empty index
	_ = g.git.CommitAllowEmpty("Generator reset: return to clean state", ".") // best-effort; reset is complete regardless

	g.logf("generator:reset: done, only %s branch remains", baseBranch)
	return nil
}

// resetGoSources deletes Go files, removes empty source dirs,
// clears build artifacts, seeds files, and reinitializes the Go module.
func (g *Generator) resetGoSources(version string) error {
	g.deleteGoFiles(".")
	for _, dir := range g.cfg.Project.GoSourceDirs {
		removeEmptyDirs(dir)
	}
	os.RemoveAll(g.cfg.Project.BinaryDir + "/")
	if err := g.seedFiles(version); err != nil {
		return fmt.Errorf("seeding files: %w", err)
	}
	return g.reinitGoModule()
}

// cleanGoSources removes all Go files, empty source directories, the
// binary directory, go.sum, and require/replace blocks from go.mod.
// After this call the working tree contains only specs and a minimal
// go.mod (module + go version) (GH-1468).
func (g *Generator) cleanGoSources() {
	g.deleteGoFiles(".")
	for _, dir := range g.cfg.Project.GoSourceDirs {
		removeEmptyDirs(dir)
	}
	os.RemoveAll(g.cfg.Project.BinaryDir + "/")
	_ = os.Remove("go.sum") // best-effort; may not exist
	stripGoModRequires("go.mod")
}

// stripGoModRequires rewrites go.mod to keep only the module declaration
// and go version, removing require and replace blocks.
func stripGoModRequires(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // file absent — nothing to clean
	}
	var kept []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "require ") && strings.HasSuffix(trimmed, "(") {
			inBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, "replace ") && strings.HasSuffix(trimmed, "(") {
			inBlock = true
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
			}
			continue
		}
		// Skip single-line require/replace directives.
		if strings.HasPrefix(trimmed, "require ") || strings.HasPrefix(trimmed, "replace ") {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.Join(kept, "\n")
	// Ensure single trailing newline.
	result = strings.TrimRight(result, "\n") + "\n"
	_ = os.WriteFile(path, []byte(result), 0o644)
}

// seedFiles creates the configured seed files using Go templates.
func (g *Generator) seedFiles(version string) error {
	data := SeedData{
		Version:    version,
		ModulePath: g.cfg.Project.ModulePath,
	}

	for _, path := range slices.Sorted(maps.Keys(g.cfg.Project.SeedFiles)) {
		tmplStr := g.cfg.Project.SeedFiles[path]
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
func (g *Generator) reinitGoModule() error {
	_ = os.Remove("go.sum") // best-effort; file may not exist
	_ = os.Remove("go.mod") // best-effort; file may not exist
	if err := g.goModInit(); err != nil {
		return fmt.Errorf("go mod init: %w", err)
	}
	if err := goModEditReplace(g.cfg.Project.ModulePath, "./"); err != nil {
		return fmt.Errorf("go mod edit -replace: %w", err)
	}
	if err := goModTidy(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	return nil
}

// deleteGoFiles removes all .go files except those in .git/ and magefiles/.
func (g *Generator) deleteGoFiles(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (path == ".git" || path == g.cfg.Project.MagefilesDir) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				g.logf("deleteGoFiles: warning removing %s: %v", path, err)
			}
		}
		return nil
	})
}

// cleanupDirs removes all directories listed in Config.CleanupDirs.
func (g *Generator) cleanupDirs() {
	for _, dir := range g.cfg.Generation.CleanupDirs {
		g.logf("cleanupDirs: removing %s", dir)
		os.RemoveAll(dir)
	}
}

// GeneratorInit writes a default configuration.yaml if one does not exist.
func GeneratorInit() error {
	fmt.Fprintf(os.Stderr, "generator:init: writing %s\n", DefaultConfigFile)
	if err := WriteDefaultConfig(DefaultConfigFile); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "generator:init: created %s — edit project-specific fields before running\n", DefaultConfigFile)
	return nil
}

// Init is a no-op placeholder kept for mage target compatibility.
func (g *Generator) Init() error {
	return nil
}

// FullReset performs a full reset: cobbler and generator.
func (g *Generator) FullReset() error {
	if err := g.claudeRunner.CobblerReset(); err != nil {
		return err
	}
	return g.GeneratorReset()
}

// goModInit initializes a Go module in the current directory.
func (g *Generator) goModInit() error {
	return exec.Command(binGo, "mod", "init", g.cfg.Project.ModulePath).Run()
}

// diffNameStatus runs git diff --name-status and returns per-file entries.
func (g *Generator) diffNameStatus(ref, dir string) ([]claude.FileChange, error) {
	gfc, err := g.git.DiffNameStatus(ref, dir)
	if err != nil {
		return nil, err
	}
	files := make([]claude.FileChange, len(gfc))
	for i, fc := range gfc {
		files[i] = claude.FileChange{
			Path:       fc.Path,
			Status:     fc.Status,
			Insertions: fc.Insertions,
			Deletions:  fc.Deletions,
		}
	}
	return files, nil
}
