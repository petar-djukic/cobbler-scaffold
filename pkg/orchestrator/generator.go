// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

// GeneratorRun executes N cycles of Measure + Stitch within the current generation.
// Reads cycles and max-issues from Config.
func (o *Orchestrator) GeneratorRun() error {
	currentBranch, err := gitCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	o.cfg.GenerationBranch = currentBranch
	setGeneration(currentBranch)
	defer clearGeneration()
	return o.RunCycles("run")
}

// GeneratorResume recovers from an interrupted generator:run and continues.
// Reads generation branch from Config.GenerationBranch or auto-detects.
func (o *Orchestrator) GeneratorResume() error {
	branch := o.cfg.GenerationBranch
	if branch == "" {
		resolved, err := o.resolveBranch("")
		if err != nil {
			return fmt.Errorf("resolving generation branch: %w", err)
		}
		branch = resolved
	}

	if !strings.HasPrefix(branch, o.cfg.GenPrefix) {
		return fmt.Errorf("not a generation branch: %s\nSet generation_branch in configuration.yaml", branch)
	}
	if !gitBranchExists(branch) {
		return fmt.Errorf("branch does not exist: %s", branch)
	}

	setGeneration(branch)
	defer clearGeneration()

	logf("resume: target branch=%s", branch)

	// Commit uncommitted work on the current branch before switching.
	if err := gitStageAll(); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := gitCommit(fmt.Sprintf("WIP: save state before resuming on %s", branch)); err != nil {
		_ = gitUnstageAll()
	}

	// Switch to the generation branch.
	if err := ensureOnBranch(branch); err != nil {
		return fmt.Errorf("switching to %s: %w", branch, err)
	}

	// Pre-flight cleanup.
	logf("resume: pre-flight cleanup")
	wtBase := worktreeBasePath()

	logf("resume: pruning worktrees")
	_ = gitWorktreePrune()

	if _, err := os.Stat(wtBase); err == nil {
		logf("resume: removing worktree directory %s", wtBase)
		os.RemoveAll(wtBase)
	}

	logf("resume: recovering stale tasks")
	if err := o.recoverStaleTasks(branch, wtBase); err != nil {
		logf("resume: recoverStaleTasks warning: %v", err)
	}

	logf("resume: resetting cobbler scratch")
	o.CobblerReset()

	o.cfg.GenerationBranch = branch

	// Drain existing ready issues before starting measure+stitch cycles.
	logf("resume: draining existing ready issues")
	if err := o.RunStitch(); err != nil {
		logf("resume: drain stitch warning: %v", err)
	}

	return o.RunCycles("resume")
}

// RunCycles runs measure+stitch cycles until no open issues remain.
// If Config.Cycles > 0, it acts as a safety limit (max cycles before forced stop).
// If Config.Cycles == 0, cycles run until all issues are closed.
func (o *Orchestrator) RunCycles(label string) error {
	logf("generator %s: starting (max issues per stitch=%d, safety limit=%d cycles)",
		label, o.cfg.MaxIssues, o.cfg.Cycles)

	for cycle := 1; ; cycle++ {
		if o.cfg.Cycles > 0 && cycle > o.cfg.Cycles {
			logf("generator %s: reached max cycles (%d), stopping", label, o.cfg.Cycles)
			break
		}

		logf("generator %s: cycle %d — measure", label, cycle)
		if err := o.RunMeasure(); err != nil {
			return fmt.Errorf("cycle %d measure: %w", cycle, err)
		}

		logf("generator %s: cycle %d — stitch", label, cycle)
		if err := o.RunStitch(); err != nil {
			return fmt.Errorf("cycle %d stitch: %w", cycle, err)
		}

		if !o.hasOpenIssues() {
			logf("generator %s: no open issues remain, stopping after %d cycle(s)", label, cycle)
			break
		}
		logf("generator %s: open issues remain, continuing to cycle %d", label, cycle+1)
	}

	logf("generator %s: complete", label)
	return nil
}

// GeneratorStart begins a new generation trail.
// Tags current main state, creates a generation branch, deletes Go files,
// reinitializes the Go module, and commits the clean state.
func (o *Orchestrator) GeneratorStart() error {
	if err := ensureOnBranch("main"); err != nil {
		return fmt.Errorf("switching to main: %w", err)
	}

	genName := o.cfg.GenPrefix + time.Now().Format("2006-01-02-15-04-05")
	startTag := genName + "-start"

	setGeneration(genName)
	defer clearGeneration()

	logf("generator:start: beginning")

	// Tag current main state before the generation begins.
	logf("generator:start: tagging current state as %s", startTag)
	if err := gitTag(startTag); err != nil {
		return fmt.Errorf("tagging main: %w", err)
	}

	// Create and switch to generation branch.
	logf("generator:start: creating branch")
	if err := gitCheckoutNew(genName); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Record branch point so intermediate commits can be squashed.
	branchSHA, err := gitRevParseHEAD()
	if err != nil {
		return fmt.Errorf("getting branch HEAD: %w", err)
	}

	// Reset beads database and reinitialize with generation prefix.
	if err := o.beadsResetDB(); err != nil {
		return fmt.Errorf("resetting beads: %w", err)
	}
	if err := o.beadsInitWith(genName); err != nil {
		return fmt.Errorf("initializing beads: %w", err)
	}

	// Reset Go sources and reinitialize module.
	logf("generator:start: resetting Go sources")
	if err := o.resetGoSources(genName); err != nil {
		return fmt.Errorf("resetting Go sources: %w", err)
	}

	// Squash intermediate commits (beads reset/init) into one clean commit.
	logf("generator:start: squashing into single commit")
	if err := gitResetSoft(branchSHA); err != nil {
		return fmt.Errorf("squashing start commits: %w", err)
	}
	_ = gitStageAll()
	msg := fmt.Sprintf("Start generation: %s\n\nDelete Go files, reinitialize module, initialize beads.\nTagged previous state as %s.", genName, genName)
	if err := gitCommit(msg); err != nil {
		return fmt.Errorf("committing clean state: %w", err)
	}

	logf("generator:start: done, run mage generator:run to begin building")
	return nil
}

// GeneratorStop completes a generation trail and merges it into main.
// Uses Config.GenerationBranch, current branch, or auto-detects.
func (o *Orchestrator) GeneratorStop() error {
	branch := o.cfg.GenerationBranch
	if branch != "" {
		if !gitBranchExists(branch) {
			return fmt.Errorf("branch does not exist: %s", branch)
		}
	} else {
		current, err := gitCurrentBranch()
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
		if strings.HasPrefix(current, o.cfg.GenPrefix) {
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

	if !strings.HasPrefix(branch, o.cfg.GenPrefix) {
		return fmt.Errorf("not a generation branch: %s\nSet generation_branch in configuration.yaml", branch)
	}

	setGeneration(branch)
	defer clearGeneration()

	finishedTag := branch + "-finished"

	logf("generator:stop: beginning")

	// Switch to the generation branch and tag its final state.
	if err := ensureOnBranch(branch); err != nil {
		return fmt.Errorf("switching to generation branch: %w", err)
	}
	logf("generator:stop: tagging as %s", finishedTag)
	if err := gitTag(finishedTag); err != nil {
		return fmt.Errorf("tagging generation: %w", err)
	}

	// Switch to main.
	logf("generator:stop: switching to main")
	if err := gitCheckout("main"); err != nil {
		return fmt.Errorf("checking out main: %w", err)
	}

	if err := o.mergeGenerationIntoMain(branch); err != nil {
		return err
	}

	o.cleanupDirs()

	logf("generator:stop: done, work is on main")
	return nil
}

// mergeGenerationIntoMain resets Go sources, commits the clean state,
// merges the generation branch, tags the result, and deletes the branch.
func (o *Orchestrator) mergeGenerationIntoMain(branch string) error {
	logf("generator:stop: resetting Go sources on main")
	_ = o.resetGoSources(branch)

	_ = gitStageAll()
	prepareMsg := fmt.Sprintf("Prepare main for generation merge: delete Go code\n\nDocumentation preserved for merge. Code will be replaced by %s.", branch)
	if err := gitCommitAllowEmpty(prepareMsg); err != nil {
		return fmt.Errorf("committing prepare step: %w", err)
	}

	logf("generator:stop: merging into main")
	cmd := gitMergeCmd(branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("merging %s: %w", branch, err)
	}

	mainTag := branch + "-merged"
	logf("generator:stop: tagging main as %s", mainTag)
	if err := gitTag(mainTag); err != nil {
		return fmt.Errorf("tagging merge: %w", err)
	}

	// Create versioned tags using v[REL].[DATE].[REVISION] convention.
	if date := o.generationDateCompact(branch); date != "" {
		revision := o.generationRevision(branch)
		codeTag := fmt.Sprintf("v0.%s.%d", date, revision)
		reqTag := fmt.Sprintf("v0.%s.%d-requirements", date, revision)

		logf("generator:stop: tagging code as %s", codeTag)
		if err := gitTag(codeTag); err != nil {
			logf("generator:stop: code tag warning: %v", err)
		}

		startTag := branch + "-start"
		logf("generator:stop: tagging requirements as %s (at %s)", reqTag, startTag)
		if err := gitTagAt(reqTag, startTag); err != nil {
			logf("generator:stop: requirements tag warning: %v", err)
		}
	}

	logf("generator:stop: deleting branch")
	_ = gitDeleteBranch(branch)
	return nil
}

// listGenerationBranches returns all generation-* branch names.
func (o *Orchestrator) listGenerationBranches() []string {
	return gitListBranches(o.cfg.GenPrefix + "*")
}

// tagSuffixes lists the lifecycle tag suffixes in order.
var tagSuffixes = []string{"-start", "-finished", "-merged", "-abandoned"}

// generationDate extracts the date portion (YYYY-MM-DD) from a
// generation branch name like "generation-2026-02-12-07-13-55".
func (o *Orchestrator) generationDate(branch string) string {
	rest := strings.TrimPrefix(branch, o.cfg.GenPrefix)
	if rest == branch {
		return ""
	}
	if len(rest) < 10 {
		return ""
	}
	return rest[:10]
}

// generationDateCompact extracts the date in YYYYMMDD format from a
// generation branch name like "generation-2026-02-12-07-13-55".
func (o *Orchestrator) generationDateCompact(branch string) string {
	date := o.generationDate(branch)
	if date == "" {
		return ""
	}
	return strings.ReplaceAll(date, "-", "")
}

// generationRevision returns the 0-indexed position of a generation
// among all generations started on the same date. Counts unique
// generation names from tags and branches matching the date.
func (o *Orchestrator) generationRevision(branch string) int {
	date := o.generationDate(branch) // "2026-02-12"
	if date == "" {
		return 0
	}

	nameSet := make(map[string]bool)
	for _, t := range gitListTags(o.cfg.GenPrefix + date + "-*") {
		nameSet[generationName(t)] = true
	}
	for _, b := range gitListBranches(o.cfg.GenPrefix + date + "-*") {
		nameSet[b] = true
	}

	var names []string
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)

	for i, n := range names {
		if n == branch {
			return i
		}
	}
	return 0
}

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
	tags := gitListTags(o.cfg.GenPrefix + "*")
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
				_ = gitRenameTag(t, abTag)
			}
		} else {
			logf("generator:reset: removing tag %s", t)
			_ = gitDeleteTag(t)
		}
	}
}

// resolveBranch determines which branch to work on.
func (o *Orchestrator) resolveBranch(explicit string) (string, error) {
	if explicit != "" {
		if !gitBranchExists(explicit) {
			return "", fmt.Errorf("branch does not exist: %s", explicit)
		}
		return explicit, nil
	}

	branches := o.listGenerationBranches()
	switch len(branches) {
	case 0:
		return gitCurrentBranch()
	case 1:
		return branches[0], nil
	default:
		sort.Strings(branches)
		return "", fmt.Errorf("multiple generation branches exist (%s); set generation_branch in configuration.yaml", strings.Join(branches, ", "))
	}
}

// ensureOnBranch switches to the given branch if not already on it.
func ensureOnBranch(branch string) error {
	current, err := gitCurrentBranch()
	if err != nil {
		return err
	}
	if current == branch {
		return nil
	}
	logf("ensureOnBranch: switching from %s to %s", current, branch)
	return gitCheckout(branch)
}

// GeneratorList shows active branches and past generations.
func (o *Orchestrator) GeneratorList() error {
	branches := o.listGenerationBranches()
	tags := gitListTags(o.cfg.GenPrefix + "*")
	current, _ := gitCurrentBranch()

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
	sort.Strings(names)

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
	target := o.cfg.GenerationBranch
	if target == "" {
		return fmt.Errorf("set generation_branch in configuration.yaml\nAvailable branches: %s, main", strings.Join(o.listGenerationBranches(), ", "))
	}

	if target != "main" && !strings.HasPrefix(target, o.cfg.GenPrefix) {
		return fmt.Errorf("not a generation branch or main: %s", target)
	}
	if !gitBranchExists(target) {
		return fmt.Errorf("branch does not exist: %s", target)
	}

	current, err := gitCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if current == target {
		logf("generator:switch: already on %s", target)
		return nil
	}

	if err := gitStageAll(); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := gitCommit(fmt.Sprintf("WIP: save state before switching to %s", target)); err != nil {
		_ = gitUnstageAll()
	}

	logf("generator:switch: %s -> %s", current, target)
	if err := gitCheckout(target); err != nil {
		return fmt.Errorf("switching to %s: %w", target, err)
	}

	logf("generator:switch: now on %s", target)
	return nil
}

// GeneratorReset destroys generation branches, worktrees, and Go source directories.
func (o *Orchestrator) GeneratorReset() error {
	logf("generator:reset: beginning")

	if err := ensureOnBranch("main"); err != nil {
		return fmt.Errorf("switching to main: %w", err)
	}

	wtBase := worktreeBasePath()
	genBranches := o.listGenerationBranches()
	if len(genBranches) > 0 {
		logf("generator:reset: removing task branches and worktrees")
		for _, gb := range genBranches {
			recoverStaleBranches(gb, wtBase)
		}
	}

	_ = gitWorktreePrune()

	if _, err := os.Stat(wtBase); err == nil {
		logf("generator:reset: removing worktree directory %s", wtBase)
		os.RemoveAll(wtBase)
	}

	if len(genBranches) > 0 {
		logf("generator:reset: removing %d generation branch(es)", len(genBranches))
		for _, gb := range genBranches {
			logf("generator:reset: deleting branch %s", gb)
			_ = gitForceDeleteBranch(gb)
		}
	}

	o.cleanupUnmergedTags()

	logf("generator:reset: removing Go source directories")
	for _, dir := range o.cfg.GoSourceDirs {
		logf("generator:reset: removing %s", dir)
		os.RemoveAll(dir)
	}
	os.RemoveAll(o.cfg.BinaryDir + "/")
	o.cleanupDirs()

	logf("generator:reset: seeding Go sources and reinitializing go.mod")
	if err := o.seedFiles("main"); err != nil {
		return fmt.Errorf("seeding files: %w", err)
	}
	if err := o.reinitGoModule(); err != nil {
		return fmt.Errorf("reinitializing go module: %w", err)
	}

	logf("generator:reset: committing clean state")
	_ = gitStageAll()
	_ = gitCommit("Generator reset: return to clean state")

	logf("generator:reset: done, only main branch remains")
	return nil
}

// resetGoSources deletes Go files, removes empty source dirs,
// clears build artifacts, seeds files, and reinitializes the Go module.
func (o *Orchestrator) resetGoSources(version string) error {
	o.deleteGoFiles(".")
	for _, dir := range o.cfg.GoSourceDirs {
		removeEmptyDirs(dir)
	}
	os.RemoveAll(o.cfg.BinaryDir + "/")
	if err := o.seedFiles(version); err != nil {
		return fmt.Errorf("seeding files: %w", err)
	}
	return o.reinitGoModule()
}

// seedFiles creates the configured seed files using Go templates.
func (o *Orchestrator) seedFiles(version string) error {
	data := SeedData{
		Version:    version,
		ModulePath: o.cfg.ModulePath,
	}

	for path, tmplStr := range o.cfg.SeedFiles {
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
	os.Remove("go.sum")
	os.Remove("go.mod")
	if err := o.goModInit(); err != nil {
		return fmt.Errorf("go mod init: %w", err)
	}
	if err := goModEditReplace(o.cfg.ModulePath, "./"); err != nil {
		return fmt.Errorf("go mod edit -replace: %w", err)
	}
	if err := goModTidy(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}
	return nil
}

// deleteGoFiles removes all .go files except those in .git/ and magefiles/.
func (o *Orchestrator) deleteGoFiles(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && (path == ".git" || path == o.cfg.MagefilesDir) {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			os.Remove(path)
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
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err == nil && len(entries) == 0 {
			os.Remove(dirs[i])
		}
	}
}

// cleanupDirs removes all directories listed in Config.CleanupDirs.
func (o *Orchestrator) cleanupDirs() {
	for _, dir := range o.cfg.CleanupDirs {
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

// Init initializes the project (beads).
func (o *Orchestrator) Init() error {
	return o.BeadsInit()
}

// FullReset performs a full reset: cobbler, generator, beads.
func (o *Orchestrator) FullReset() error {
	if err := o.CobblerReset(); err != nil {
		return err
	}
	if err := o.GeneratorReset(); err != nil {
		return err
	}
	return o.BeadsReset()
}
