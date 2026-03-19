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
	"github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/generate"
)

// ---------------------------------------------------------------------------
// Dependency injection: wire the parent package's logf and binary paths
// into the internal/generate package at init time.
// ---------------------------------------------------------------------------

func init() {
	generate.Log = logf
	generate.BinGit = binGit
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

// prdRefPattern matches PRD requirement references in task requirement text.
var prdRefPattern = generate.PRDRefPattern

// ---------------------------------------------------------------------------
// gitDeps builds the GitDeps struct for the generate package.
// ---------------------------------------------------------------------------

func genGitDeps() generate.GitDeps {
	return generate.GitDeps{
		RepoReader:    defaultGitOps,
		BranchManager: defaultGitOps,
		CommitWriter:  defaultGitOps,
	}
}

// stitchGitDeps builds the StitchGitDeps struct for stitch operations.
func stitchGitDeps() generate.StitchGitDeps {
	return generate.StitchGitDeps{
		RepoReader:      defaultGitOps,
		BranchManager:   defaultGitOps,
		WorktreeManager: defaultGitOps,
	}
}

// stitchIssueDeps builds the StitchIssueDeps struct for stitch operations.
func stitchIssueDeps(repo, generation string) generate.StitchIssueDeps {
	return generate.StitchIssueDeps{
		ListOpenCobblerIssues: func(r, g string) ([]generate.StitchIssue, error) {
			issues, err := listOpenCobblerIssues(r, g)
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
		PickReadyIssue: func(r, g string) (generate.StitchIssue, error) {
			iss, err := pickReadyIssue(r, g)
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
			return removeInProgressLabel(r, num)
		},
		HasLabel: func(iss generate.StitchIssue, label string) bool {
			for _, l := range iss.Labels {
				if l == label {
					return true
				}
			}
			return false
		},
		LabelInProgress: cobblerLabelInProgress,
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

func saveAndSwitchBranch(target string) error {
	return generate.SaveAndSwitchBranch(target, genGitDeps())
}

func ensureOnBranch(branch string) error {
	return generate.EnsureOnBranch(branch, genGitDeps())
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

func validateMeasureOutput(issues []proposedIssue, maxReqs int, subItemCounts map[string]map[string]int, reqStates map[string]map[string]generate.RequirementState) validationResult {
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
	return generate.ValidateMeasureOutput(genIssues, maxReqs, subItemCounts, reqStates)
}

func expandedRequirementCount(reqs []issueDescItem, subItemCounts map[string]map[string]int) int {
	return generate.ExpandedRequirementCount(reqs, subItemCounts)
}

func loadPRDSubItemCounts() map[string]map[string]int {
	return generate.LoadPRDSubItemCounts()
}

func loadRequirementStates(cobblerDir string) map[string]map[string]generate.RequirementState {
	return generate.LoadRequirementStates(cobblerDir)
}

// hasUnresolvedRequirements returns true if any R-item in requirements.yaml
// has status "ready" (not yet implemented or skipped). Used to prevent the
// generator from stopping prematurely when the GitHub API reports no open
// issues but work remains (GH-1475).
func (o *Orchestrator) hasUnresolvedRequirements() bool {
	states := generate.LoadRequirementStates(o.cfg.Cobbler.Dir)
	if states == nil {
		return false
	}
	for _, prdReqs := range states {
		for _, st := range prdReqs {
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

func recoverStaleBranches(baseBranch, worktreeBase, repo string) bool {
	return generate.RecoverStaleBranches(baseBranch, worktreeBase, repo, stitchGitDeps(), stitchIssueDeps(repo, ""))
}

func resetOrphanedIssues(baseBranch, repo, generation string) bool {
	return generate.ResetOrphanedIssues(baseBranch, repo, generation, stitchGitDeps(), stitchIssueDeps(repo, generation))
}

func pickTask(baseBranch, worktreeBase, repo, generation string) (stitchTask, error) {
	return generate.PickTask(baseBranch, worktreeBase, repo, generation, stitchIssueDeps(repo, generation))
}

func createWorktree(task stitchTask) error {
	return generate.CreateWorktree(task, stitchGitDeps())
}

func commitWorktreeChanges(task stitchTask) error {
	return generate.CommitWorktreeChanges(task)
}

func cleanGoBinaries(dir string) {
	generate.CleanGoBinaries(dir)
}

func mergeBranch(branchName, baseBranch, repoRoot string) error {
	return generate.MergeBranch(branchName, baseBranch, repoRoot, stitchGitDeps())
}

func cleanupWorktree(task stitchTask) bool {
	return generate.CleanupWorktree(task, stitchGitDeps())
}

// ---------------------------------------------------------------------------
// Orchestrator receiver methods
// ---------------------------------------------------------------------------

// GeneratorRun executes N cycles of Measure + Stitch within the current generation.
// If cycles > 0 it overrides configuration.yaml's generation.cycles for this run only.
// cycles == 0 means use the configured value (or unlimited if that is also 0).
func (o *Orchestrator) GeneratorRun(cycles int) error {
	// If not on a generation branch, try to enter the worktree created by
	// GeneratorStart (GH-1608).
	if _, err := enterGenerationWorktree(); err != nil {
		return err
	}

	currentBranch, err := defaultGitOps.CurrentBranch(".")
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
	// If not on a generation branch, try to enter the worktree (GH-1608).
	if _, err := enterGenerationWorktree(); err != nil {
		return err
	}

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
	if !defaultGitOps.BranchExists(branch, ".") {
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
	wtBase := claude.WorktreeBasePath()

	logf("resume: pruning worktrees")
	if err := defaultGitOps.WorktreePrune("."); err != nil {
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

		// Validate UC implementation after stitch so measure sees which UCs
		// are done and skips their requirements (GH-1361). Only marks UCs
		// whose tests exist and pass — unlike the old blanket marking.
		if totalStitched > 0 {
			o.validateAndMarkUCs()
		}

		// Check if the current release is complete and auto-advance if so.
		if advanced, ver := o.checkAutoAdvanceRelease(); advanced {
			logf("generator %s: cycle %d — auto-advanced release %s", label, cycle, ver)
		}

		// Skip measure if open issues remain — stitch should drain them first (GH-1352).
		if openBefore, err := o.hasOpenIssues(); err == nil && openBefore {
			logf("generator %s: cycle %d — skipping measure, open issues remain", label, cycle)
		} else {
			logf("generator %s: cycle %d — measure", label, cycle)
			if err := o.RunMeasure(); err != nil {
				return fmt.Errorf("cycle %d measure: %w", cycle, err)
			}
		}

		open, err := o.hasOpenIssues()
		if err != nil {
			logf("generator %s: hasOpenIssues error (assuming open): %v", label, err)
		}
		if !open && err == nil {
			// Before stopping, check if unresolved requirements remain.
			// The GitHub API may report no open issues due to a race
			// condition (stale cache after closing the last task). If
			// requirements are still "ready", run measure to create new
			// tasks instead of stopping prematurely (GH-1475).
			if o.hasUnresolvedRequirements() {
				logf("generator %s: cycle %d — no open issues but unresolved requirements remain, running measure",
					label, cycle)
				if err := o.RunMeasure(); err != nil {
					return fmt.Errorf("cycle %d measure (fallback): %w", cycle, err)
				}
			} else {
				logf("generator %s: no open issues remain, stopping after %d cycle(s)", label, cycle)
				break
			}
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
	_ = defaultGitOps.StageAll(".")
	msg := fmt.Sprintf("Auto-advance release %s: all use cases complete\n\nMarked use cases as implemented in road-map.yaml.\nRemoved %s from project.releases in configuration.yaml.", target.Version, target.Version)
	if err := defaultGitOps.Commit(msg, "."); err != nil {
		logf("checkAutoAdvanceRelease: commit failed: %v", err)
	}

	// Reload config so subsequent measure sees updated releases.
	if cfg, err := LoadConfig(DefaultConfigFile); err == nil {
		o.cfg = cfg
	}

	return true, target.Version
}

// validateAndMarkUCs finds the first non-implemented release in road-map.yaml
// and validates each UC individually. A UC is marked "implemented" only when
// its test files exist and pass. Replaces the former markCompletedReleaseUCs /
// markActiveReleaseUCsDone pair that blindly marked all UCs when no open issues
// remained (GH-1361).
func (o *Orchestrator) validateAndMarkUCs() {
	rm := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return
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
	if target == nil || len(target.UseCases) == 0 {
		return
	}

	var marked []string
	for _, uc := range target.UseCases {
		if ucStatusDone(uc.Status) {
			continue
		}

		// Load UC touchpoints to extract PRD citations.
		ucTouchpoints := loadUCTouchpoints(uc.ID)
		if len(ucTouchpoints) == 0 {
			logf("validateAndMarkUCs: UC %s — no touchpoints found, skipping", uc.ID)
			continue
		}

		// Requirement-based completion is the primary gate (GH-1378).
		complete, remaining := generate.UCRequirementsComplete(o.cfg.Cobbler.Dir, ucTouchpoints)
		if !complete {
			logf("validateAndMarkUCs: UC %s — %d requirements still pending: %v", uc.ID, len(remaining), remaining)
			continue
		}

		// Test-based validation is a secondary signal (log-only).
		if !validateUCImplemented(uc.ID) {
			logf("validateAndMarkUCs: UC %s — requirements complete but tests missing/failing (non-blocking)", uc.ID)
		}

		logf("validateAndMarkUCs: UC %s validated — marking as implemented", uc.ID)
		if err := updateRoadmapSingleUCStatus(target.Version, uc.ID, "implemented"); err != nil {
			logf("validateAndMarkUCs: failed to mark %s: %v", uc.ID, err)
			continue
		}
		marked = append(marked, uc.ID)
	}

	if len(marked) == 0 {
		return
	}

	_ = defaultGitOps.StageAll(".")
	msg := fmt.Sprintf("Mark validated UCs as implemented in release %s\n\nUCs with complete requirements: %s",
		target.Version, strings.Join(marked, ", "))
	if err := defaultGitOps.Commit(msg, "."); err != nil {
		logf("validateAndMarkUCs: commit failed: %v", err)
	}
}

// loadUCTouchpoints loads a use case file and returns its touchpoints as
// flat strings (e.g. "T1: Config struct: ... prd001-core R1"). Returns nil
// if the UC file is not found or cannot be parsed.
func loadUCTouchpoints(ucID string) []string {
	path := filepath.Join("docs/specs/use-cases", ucID+".yaml")
	uc, err := loadUseCase(path)
	if err != nil {
		return nil
	}
	return uc.Touchpoints
}

// validateUCImplemented checks whether a use case has test files and whether
// those tests pass. Returns true only if both conditions are met.
func validateUCImplemented(ucID string) bool {
	testDir := an.TestDirForUC(ucID)
	if testDir == "" {
		return false
	}
	testCount := an.CountTestFiles(testDir)
	if testCount == 0 {
		logf("validateUCImplemented: %s — no test files in %s", ucID, testDir)
		return false
	}

	// Run the UC's tests. Use -count=1 to disable caching.
	cmd := exec.Command("go", "test", "-tags=usecase", "-count=1", "-timeout", "300s", "./"+testDir+"/...")
	out, err := cmd.CombinedOutput()
	if err != nil {
		logf("validateUCImplemented: %s — tests failed: %v\n%s", ucID, err, string(out))
		return false
	}
	logf("validateUCImplemented: %s — %d test file(s) pass", ucID, testCount)
	return true
}

// GeneratorStart begins a new generation trail.
// Records the current branch as the base branch, tags it, creates a generation
// branch, deletes Go files, reinitializes the Go module, and commits the clean
// state. Any clean branch is a valid starting point (prd002 R2.1).
func (o *Orchestrator) GeneratorStart() error {
	baseBranch, err := defaultGitOps.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Reject dirty worktrees — a generation must start from a clean state.
	if defaultGitOps.HasChanges(".") {
		return fmt.Errorf("worktree has uncommitted changes on %s; commit or stash before starting a generation", baseBranch)
	}

	// Clear history from any previous generation so the new generation
	// starts with a clean slate (GH-1356). The history directory may
	// survive across generations when it is gitignored or when
	// generator:stop was not called.
	if err := o.HistoryClean(); err != nil {
		logf("generator:start: warning clearing history: %v", err)
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
	if err := defaultGitOps.Tag(startTag, "."); err != nil {
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
	logf("generator:start: creating worktree at %s", worktreeDir)

	if err := defaultGitOps.CreateBranch(genName, "."); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}
	cmd := defaultGitOps.WorktreeAdd(worktreeDir, genName, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Clean up the branch if worktree creation fails.
		_ = defaultGitOps.DeleteBranch(genName, ".")
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Switch into the worktree so all subsequent operations (source reset,
	// squash, etc.) run there rather than in the main repo.
	if err := os.Chdir(worktreeDir); err != nil {
		return fmt.Errorf("switching to worktree: %w", err)
	}

	// Record branch point so intermediate commits can be squashed.
	branchSHA, err := defaultGitOps.RevParseHEAD(".")
	if err != nil {
		return fmt.Errorf("getting branch HEAD: %w", err)
	}

	// Record the base branch so GeneratorStop knows where to merge back
	// (prd002 R2.8).
	if err := o.writeBaseBranch(baseBranch); err != nil {
		return fmt.Errorf("recording base branch: %w", err)
	}

	// Record the main repo root so GeneratorStop can find it from the
	// worktree (GH-1608).
	if err := o.writeRepoRoot(repoRoot); err != nil {
		return fmt.Errorf("recording repo root: %w", err)
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
		// Reset roadmap statuses so measure does not skip releases whose
		// code was just deleted (GH-1368).
		if err := o.resetImplementedReleases(); err != nil {
			logf("generator:start: warning resetting releases: %v", err)
		}
	}

	// Generate requirements state file from PRD R-items (GH-1378).
	prdDir := "docs/specs/product-requirements"
	if _, err := generate.GenerateRequirementsFile(prdDir, o.cfg.Cobbler.Dir, o.cfg.Generation.PreserveSources); err != nil {
		logf("generator:start: warning generating requirements file: %v", err)
	}

	// Squash intermediate commits into one clean commit.
	logf("generator:start: squashing into single commit")
	if err := defaultGitOps.ResetSoft(branchSHA, "."); err != nil {
		return fmt.Errorf("squashing start commits: %w", err)
	}
	_ = defaultGitOps.StageAll(".")
	var msg string
	if o.cfg.Generation.PreserveSources {
		msg = fmt.Sprintf("Start generation: %s\n\nBase branch: %s. Sources preserved (preserve_sources=true).\nTagged previous state as %s.", genName, baseBranch, genName)
	} else {
		msg = fmt.Sprintf("Start generation: %s\n\nBase branch: %s. Delete Go files, reinitialize module.\nTagged previous state as %s.", genName, baseBranch, genName)
	}
	// Use allow-empty because a specs-only repo may have no Go files
	// to delete, leaving no changes to commit after source reset.
	if err := defaultGitOps.CommitAllowEmpty(msg, "."); err != nil {
		return fmt.Errorf("committing clean state: %w", err)
	}

	logf("generator:start: done, worktree is at %s", worktreeDir)
	logf("generator:start: run mage generator:run to begin building")
	return nil
}

// writeBaseBranch writes the base branch name to .cobbler/base-branch.
func (o *Orchestrator) writeBaseBranch(branch string) error {
	dir := o.cfg.Cobbler.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, baseBranchFile), []byte(branch+"\n"), 0o644)
}

// writeRepoRoot writes the main repository root path to .cobbler/repo-root.
// GeneratorStop reads this to locate the main repo when running inside a worktree.
func (o *Orchestrator) writeRepoRoot(root string) error {
	dir := o.cfg.Cobbler.Dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, repoRootFile), []byte(root+"\n"), 0o644)
}

// readRepoRoot reads the main repository root from .cobbler/repo-root.
// Returns "" if the file does not exist (pre-worktree generations).
func (o *Orchestrator) readRepoRoot() string {
	data, err := os.ReadFile(filepath.Join(o.cfg.Cobbler.Dir, repoRootFile))
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
func enterGenerationWorktree() (string, error) {
	// If we're already on a generation branch, no need to search.
	if branch, err := defaultGitOps.CurrentBranch("."); err == nil {
		if strings.HasPrefix(branch, "generation-") {
			return "", nil
		}
	}

	wtPath := findGenerationWorktree("generation-")
	if wtPath == "" {
		return "", nil
	}
	logf("auto-entering generation worktree at %s", wtPath)
	if err := os.Chdir(wtPath); err != nil {
		return "", fmt.Errorf("switching to generation worktree %s: %w", wtPath, err)
	}
	return wtPath, nil
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

// GeneratorStop completes a generation trail and merges it into the base branch.
// Reads the base branch from .cobbler/base-branch (falls back to "main").
// Uses Config.GenerationBranch, current branch, or auto-detects.
//
// When running inside a worktree created by GeneratorStart (GH-1608), the
// function tags the generation in the worktree, switches to the main repo for
// the merge, and removes the worktree afterward.
func (o *Orchestrator) GeneratorStop() error {
	// If invoked from the main repo, try to enter the worktree (GH-1608).
	if _, err := enterGenerationWorktree(); err != nil {
		return err
	}

	branch := o.cfg.Generation.Branch
	if branch != "" {
		if !defaultGitOps.BranchExists(branch, ".") {
			return fmt.Errorf("branch does not exist: %s", branch)
		}
	} else {
		current, err := defaultGitOps.CurrentBranch(".")
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

	// Detect whether we are inside a worktree created by GeneratorStart.
	repoRoot := o.readRepoRoot()
	var worktreeDir string
	if repoRoot != "" {
		var err error
		worktreeDir, err = filepath.Abs(".")
		if err != nil {
			return fmt.Errorf("resolving worktree directory: %w", err)
		}
		logf("generator:stop: running in worktree %s (main repo: %s)", worktreeDir, repoRoot)
	}

	// Capture the caller's branch before switching to the generation branch.
	callerBranch, err := defaultGitOps.CurrentBranch(".")
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Switch to the generation branch and tag its final state.
	if err := ensureOnBranch(branch); err != nil {
		return fmt.Errorf("switching to generation branch: %w", err)
	}

	// Determine the merge target (GH-523).
	recordedBase := o.readBaseBranch()
	baseBranch := recordedBase
	if repoRoot == "" {
		// Legacy path (no worktree): respect caller branch override.
		baseBranch = resolveStopTarget(callerBranch, branch, recordedBase)
		if baseBranch != recordedBase {
			logf("generator:stop: caller was on %s; using it as merge target instead of recorded base %s", callerBranch, recordedBase)
		}
	}

	// Commit any uncommitted history files (orchestrator logs, late stats)
	// so they are captured in the finished tag for post-hoc analysis (GH-1452).
	if hdir := o.historyDir(); hdir != "" {
		if err := defaultGitOps.StageDir(hdir, "."); err == nil && defaultGitOps.HasChanges(".") {
			logf("generator:stop: committing history files before tagging")
			_ = defaultGitOps.Commit("Commit history files before generator:stop tag", ".")
		}
	}

	logf("generator:stop: tagging as %s", finishedTag)
	if err := defaultGitOps.Tag(finishedTag, "."); err != nil {
		return fmt.Errorf("tagging generation: %w", err)
	}

	// If running in a worktree, switch to the main repo for the merge.
	if repoRoot != "" {
		logf("generator:stop: switching to main repo at %s", repoRoot)
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("switching to main repo: %w", err)
		}
	} else {
		// Legacy path: switch to the base branch in the current repo.
		logf("generator:stop: switching to %s", baseBranch)
		if err := defaultGitOps.Checkout(baseBranch, "."); err != nil {
			return fmt.Errorf("checking out %s: %w", baseBranch, err)
		}
	}

	// Clean up untracked history files on the base branch so they don't
	// persist across branches (GH-1356). The history is preserved in the
	// finished tag above (GH-1452).
	if err := o.HistoryClean(); err != nil {
		logf("generator:stop: warning clearing history: %v", err)
	}

	// Remove the worktree before merging so the generation branch is not
	// locked by the worktree checkout. mergeGeneration deletes the branch
	// after a successful merge; git refuses to delete a branch that is
	// checked out in any worktree (GH-1608).
	if worktreeDir != "" {
		logf("generator:stop: removing worktree %s", worktreeDir)
		if err := defaultGitOps.WorktreeRemove(worktreeDir, "."); err != nil {
			logf("generator:stop: worktree remove warning: %v", err)
		}
		_ = defaultGitOps.WorktreePrune(".")
	}

	if err := o.mergeGeneration(branch, baseBranch); err != nil {
		return err
	}

	// Close any open cobbler-gen issues for this generation.
	if ghRepo, err := detectGitHubRepo(".", o.cfg); err == nil && ghRepo != "" {
		if err := closeGenerationIssues(ghRepo, branch); err != nil {
			logf("generator:stop: close issues warning: %v", err)
		}
	}

	// Reset all implemented releases back to spec_complete (GH-1021).
	if err := o.resetImplementedReleases(); err != nil {
		logf("generator:stop: reset releases warning: %v", err)
	}

	o.cleanupDirs()

	logf("generator:stop: done, work is on %s", baseBranch)
	return nil
}

// mergeGeneration resets Go sources, commits the clean state, merges the
// generation branch into the base branch, tags the result, resets the base
// branch to specs-only, and deletes the generation branch.
func (o *Orchestrator) mergeGeneration(branch, baseBranch string) error {
	if o.cfg.Generation.PreserveSources {
		logf("generator:stop: preserve_sources=true, skipping pre-merge Go source reset on %s", baseBranch)
	} else {
		logf("generator:stop: resetting Go sources on %s", baseBranch)
		_ = o.resetGoSources(branch) // best-effort; merge will overwrite these files
	}

	_ = defaultGitOps.StageAll(".") // best-effort; commit below handles empty index
	var prepareMsg string
	if o.cfg.Generation.PreserveSources {
		prepareMsg = fmt.Sprintf("Prepare %s for generation merge (preserve_sources)\n\nSources preserved. Merging %s.", baseBranch, branch)
	} else {
		prepareMsg = fmt.Sprintf("Prepare %s for generation merge: delete Go code\n\nDocumentation preserved for merge. Code will be replaced by %s.", baseBranch, branch)
	}
	if err := defaultGitOps.CommitAllowEmpty(prepareMsg, "."); err != nil {
		return fmt.Errorf("committing prepare step: %w", err)
	}

	logf("generator:stop: merging into %s", baseBranch)
	cmd := defaultGitOps.MergeCmd(branch, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("merging %s: %w", branch, err)
	}

	// Restore Go files from earlier generations.
	startTag := branch + "-start"
	if err := o.restoreFromStartTag(startTag); err != nil {
		logf("generator:stop: restore warning: %v", err)
	}

	mergedTag := branch + "-merged"
	logf("generator:stop: tagging %s as %s", baseBranch, mergedTag)
	if err := defaultGitOps.Tag(mergedTag, "."); err != nil {
		return fmt.Errorf("tagging merge: %w", err)
	}

	// Reset base branch to specs-only.
	if o.cfg.Generation.PreserveSources {
		logf("generator:stop: preserve_sources=true, skipping post-tag source reset on %s", baseBranch)
	} else {
		logf("generator:stop: resetting %s to specs-only", baseBranch)
		o.cleanGoSources()
	}
	if err := o.HistoryClean(); err != nil {
		logf("generator:stop: warning cleaning history: %v", err)
	}
	_ = defaultGitOps.StageAll(".")
	cleanupMsg := fmt.Sprintf("Reset %s to specs-only after v1 tag\n\nGenerated code preserved at version tags. Branch restored to documentation-only state.", baseBranch)
	_ = defaultGitOps.Commit(cleanupMsg, ".") // best-effort; may be empty if nothing changed

	logf("generator:stop: deleting branch %s", branch)
	_ = defaultGitOps.ForceDeleteBranch(branch, ".") // force-delete: safe -d fails after specs-only reset
	return nil
}

// resetImplementedReleases loads road-map.yaml, finds all releases with
// status "implemented", and calls ReleaseClear for each to reset them to
// "spec_complete" and repopulate configuration.yaml (GH-1021).
// Also reverts individual UC statuses that were marked "implemented" by
// validateAndMarkUCs even when the release itself is not yet implemented
// (GH-1469).
func (o *Orchestrator) resetImplementedReleases() error {
	rm := loadYAML[RoadmapDoc]("docs/road-map.yaml")
	if rm == nil {
		return nil
	}
	var cleared []string
	var revertedUCs []string
	for _, rel := range rm.Releases {
		if strings.EqualFold(rel.Status, "implemented") {
			if err := o.ReleaseClear(rel.Version); err != nil {
				logf("resetImplementedReleases: ReleaseClear(%s) failed: %v", rel.Version, err)
				continue
			}
			cleared = append(cleared, rel.Version)
			continue
		}
		// Revert individual UC statuses even when the release itself is
		// not yet implemented (GH-1469). validateAndMarkUCs promotes UCs
		// one at a time during the run; generator:stop must undo them.
		for _, uc := range rel.UseCases {
			if ucStatusDone(uc.Status) {
				if err := updateRoadmapSingleUCStatus(rel.Version, uc.ID, "spec_complete"); err != nil {
					logf("resetImplementedReleases: revert UC %s in %s failed: %v", uc.ID, rel.Version, err)
					continue
				}
				revertedUCs = append(revertedUCs, uc.ID)
			}
		}
	}
	if len(cleared) == 0 && len(revertedUCs) == 0 {
		return nil
	}
	_ = defaultGitOps.StageAll(".")
	var parts []string
	if len(cleared) > 0 {
		parts = append(parts, fmt.Sprintf("Releases: %s", strings.Join(cleared, ", ")))
	}
	if len(revertedUCs) > 0 {
		parts = append(parts, fmt.Sprintf("UCs: %s", strings.Join(revertedUCs, ", ")))
	}
	msg := fmt.Sprintf("Reset statuses to spec_complete after generator:stop\n\n%s", strings.Join(parts, "\n"))
	if err := defaultGitOps.Commit(msg, "."); err != nil {
		return fmt.Errorf("commit release reset: %w", err)
	}
	logf("resetImplementedReleases: cleared %d release(s), reverted %d UC(s)", len(cleared), len(revertedUCs))
	return nil
}

// restoreFromStartTag restores Go source files that existed on main at the
// given start tag but are missing after the merge.
func (o *Orchestrator) restoreFromStartTag(startTag string) error {
	startFiles, err := defaultGitOps.LsTreeFiles(startTag, ".")
	if err != nil {
		return fmt.Errorf("listing files at %s: %w", startTag, err)
	}

	var restored []string
	for _, path := range startFiles {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		if strings.HasPrefix(path, o.cfg.Project.MagefilesDir+"/") {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}

		content, err := defaultGitOps.ShowFileContent(startTag, path, ".")
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
	_ = defaultGitOps.StageAll(".")
	msg := fmt.Sprintf("Restore %d file(s) from earlier generations\n\nFiles restored from %s:\n%s",
		len(restored), startTag, strings.Join(restored, "\n"))
	if err := defaultGitOps.Commit(msg, "."); err != nil {
		return fmt.Errorf("committing restored files: %w", err)
	}
	return nil
}

// listGenerationBranches returns all generation-* branch names.
func (o *Orchestrator) listGenerationBranches() []string {
	return defaultGitOps.ListBranches(o.cfg.Generation.Prefix+"*", ".")
}

// cleanupUnmergedTags renames tags for generations that were never
// merged into a single -abandoned tag.
func (o *Orchestrator) cleanupUnmergedTags() {
	tags := defaultGitOps.ListTags(o.cfg.Generation.Prefix+"*", ".")
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
				_ = defaultGitOps.RenameTag(t, abTag, ".") // best-effort; tag may not exist
			}
		} else {
			logf("generator:reset: removing tag %s", t)
			_ = defaultGitOps.DeleteTag(t, ".") // best-effort cleanup
		}
	}
}

// resolveBranch determines which branch to work on.
func (o *Orchestrator) resolveBranch(explicit string) (string, error) {
	if explicit != "" {
		if !defaultGitOps.BranchExists(explicit, ".") {
			return "", fmt.Errorf("branch does not exist: %s", explicit)
		}
		return explicit, nil
	}

	branches := o.listGenerationBranches()
	switch len(branches) {
	case 0:
		return defaultGitOps.CurrentBranch(".")
	case 1:
		return branches[0], nil
	default:
		slices.Sort(branches)
		return "", fmt.Errorf("multiple generation branches exist (%s); set generation.branch in configuration.yaml", strings.Join(branches, ", "))
	}
}

// GeneratorList shows active branches and past generations.
func (o *Orchestrator) GeneratorList() error {
	branches := o.listGenerationBranches()
	tags := defaultGitOps.ListTags(o.cfg.Generation.Prefix+"*", ".")
	current, _ := defaultGitOps.CurrentBranch(".")

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
func (o *Orchestrator) GeneratorSwitch() error {
	target := o.cfg.Generation.Branch
	baseBranch := o.cfg.Cobbler.BaseBranch
	if target == "" {
		return fmt.Errorf("set generation.branch in configuration.yaml\nAvailable branches: %s, %s", strings.Join(o.listGenerationBranches(), ", "), baseBranch)
	}

	if target != baseBranch && !strings.HasPrefix(target, o.cfg.Generation.Prefix) {
		return fmt.Errorf("not a generation branch or %s: %s", baseBranch, target)
	}
	if !defaultGitOps.BranchExists(target, ".") {
		return fmt.Errorf("branch does not exist: %s", target)
	}

	current, err := defaultGitOps.CurrentBranch(".")
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

	// If a generation worktree exists, remove it first and switch to
	// the main repo (GH-1608).
	repoRoot := o.readRepoRoot()
	if repoRoot != "" {
		worktreeDir, _ := filepath.Abs(".")
		logf("generator:reset: removing generation worktree %s", worktreeDir)
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("switching to main repo: %w", err)
		}
		_ = defaultGitOps.WorktreeRemove(worktreeDir, ".")
		_ = defaultGitOps.WorktreePrune(".")
		// worktree path cleaned up via worktree remove (GH-1608)
	} else {
		// Check if there's a generation worktree to remove.
		if wtPath := findGenerationWorktree(o.cfg.Generation.Prefix); wtPath != "" {
			logf("generator:reset: removing generation worktree %s", wtPath)
			_ = defaultGitOps.WorktreeRemove(wtPath, ".")
			_ = defaultGitOps.WorktreePrune(".")
		}
	}

	baseBranch := o.cfg.Cobbler.BaseBranch
	if err := ensureOnBranch(baseBranch); err != nil {
		return fmt.Errorf("switching to %s: %w", baseBranch, err)
	}

	wtBase := claude.WorktreeBasePath()
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
		if err := closeGenerationIssues(ghRepo, baseBranch); err != nil {
			logf("generator:reset: close issues warning for %s: %v", baseBranch, err)
		}
	}

	if err := defaultGitOps.WorktreePrune("."); err != nil {
		logf("generator:reset: warning: worktree prune: %v", err)
	}

	if _, err := os.Stat(wtBase); err == nil {
		logf("generator:reset: removing worktree directory %s", wtBase)
		if err := os.RemoveAll(wtBase); err != nil {
			logf("generator:reset: warning: removing worktree dir: %v", err)
		}
	}

	if len(genBranches) > 0 {
		// Prune again to ensure worktree registrations are fully cleaned up
		// before deleting branches (GH-1608). Without this, git may refuse
		// to delete a branch it still thinks is checked out in a worktree.
		_ = defaultGitOps.WorktreePrune(".")
		logf("generator:reset: removing %d generation branch(es)", len(genBranches))
		for _, gb := range genBranches {
			logf("generator:reset: deleting branch %s", gb)
			_ = defaultGitOps.ForceDeleteBranch(gb, ".")
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
	_ = defaultGitOps.StageAll(".")                                                  // best-effort; commit below handles empty index
	_ = defaultGitOps.CommitAllowEmpty("Generator reset: return to clean state", ".") // best-effort; reset is complete regardless

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

// cleanGoSources removes all Go files, empty source directories, the
// binary directory, go.sum, and require/replace blocks from go.mod.
// After this call the working tree contains only specs and a minimal
// go.mod (module + go version) (GH-1468).
func (o *Orchestrator) cleanGoSources() {
	o.deleteGoFiles(".")
	for _, dir := range o.cfg.Project.GoSourceDirs {
		removeEmptyDirs(dir)
	}
	os.RemoveAll(o.cfg.Project.BinaryDir + "/")
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

// cleanupDirs removes all directories listed in Config.CleanupDirs.
func (o *Orchestrator) cleanupDirs() {
	for _, dir := range o.cfg.Generation.CleanupDirs {
		logf("cleanupDirs: removing %s", dir)
		os.RemoveAll(dir)
	}
}

// GeneratorInit writes a default configuration.yaml if one does not exist.
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
