// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	ctx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
)

// ---------------------------------------------------------------------------
// Dependency injection: wire the parent package's logf and loadAnalysisDoc
// into the internal/context package at init time.
// ---------------------------------------------------------------------------

func init() {
	ctx.Log = logf
	ctx.LoadAnalysisDocFn = func(dir string) any {
		return loadAnalysisDoc(dir)
	}
}

// ---------------------------------------------------------------------------
// Type aliases — re-export all types from internal/context so that
// existing code in the parent package (and external callers) can
// reference them without changing imports.
// ---------------------------------------------------------------------------

// PhaseContext: per-phase context overrides.
type PhaseContext = ctx.PhaseContext

// ProjectContext: top-level container.
type ProjectContext = ctx.ProjectContext

// SourceFile: single source file for inclusion in the project context.
type SourceFile = ctx.SourceFile

// ContextConfig: project configuration fields needed for context assembly.
type ContextConfig = ctx.ContextConfig

// Vision types.
type VisionDoc = ctx.VisionDoc
type RelatedProject = ctx.RelatedProject

// Architecture types.
type ArchitectureDoc = ctx.ArchitectureDoc
type ArchOverview = ctx.ArchOverview
type ArchInterface = ctx.ArchInterface
type ArchComponent = ctx.ArchComponent
type ArchDecision = ctx.ArchDecision
type ArchTech = ctx.ArchTech
type ArchPathRole = ctx.ArchPathRole
type ArchImplStatus = ctx.ArchImplStatus
type ArchReference = ctx.ArchReference
type ArchFigure = ctx.ArchFigure
type ArchDependencyRule = ctx.ArchDependencyRule
type ArchSharedProtocol = ctx.ArchSharedProtocol
type ArchComponentDependency = ctx.ArchComponentDependency

// Specifications types.
type SpecificationsDoc = ctx.SpecificationsDoc
type SpecRelease = ctx.SpecRelease
type SpecIndex = ctx.SpecIndex
type TestSuiteRef = ctx.TestSuiteRef
type PRDUseCaseMap = ctx.PRDUseCaseMap

// Roadmap types.
type RoadmapDoc = ctx.RoadmapDoc
type RoadmapRelease = ctx.RoadmapRelease
type RoadmapUseCase = ctx.RoadmapUseCase

// Specs collection types.
type SpecsCollection = ctx.SpecsCollection

// PRD types.
type PRDDoc = ctx.PRDDoc
type PRDRequirementGroup = ctx.PRDRequirementGroup
type PRDPackageContract = ctx.PRDPackageContract
type PRDExport = ctx.PRDExport
type PRDDependsOn = ctx.PRDDependsOn
type PRDStructRef = ctx.PRDStructRef

// Use case types.
type UseCaseDoc = ctx.UseCaseDoc
type UCInteractionStep = ctx.UCInteractionStep

// OOD types.
type OODPackageContractRef = ctx.OODPackageContractRef

// Test suite types.
type TestSuiteDoc = ctx.TestSuiteDoc
type TestCase = ctx.TestCase

// Engineering types.
type EngineeringDoc = ctx.EngineeringDoc
type DocSection = ctx.DocSection

// Go style types.
type GoStyleDoc = ctx.GoStyleDoc
type GoStylePattern = ctx.GoStylePattern

// Constitution types.
type ConstitutionArticle = ctx.ConstitutionArticle
type ConstitutionSection = ctx.ConstitutionSection

// Design constitution types.
type DesignDoc = ctx.DesignDoc
type DesignStandards = ctx.DesignStandards
type DesignFormatting = ctx.DesignFormatting
type DesignDocType = ctx.DesignDocType

// Execution constitution types.
type ExecutionDoc = ctx.ExecutionDoc
type ExecCodingStandards = ctx.ExecCodingStandards
type ExecNamingConventions = ctx.ExecNamingConventions
type ExecTraceability = ctx.ExecTraceability
type ExecSessionCompletion = ctx.ExecSessionCompletion
type ExecTechnology = ctx.ExecTechnology
type ExecGitConventions = ctx.ExecGitConventions

// Planning constitution types.
type PlanningDoc = ctx.PlanningDoc
type PlanningIssueStructure = ctx.PlanningIssueStructure
type PlanningFieldDef = ctx.PlanningFieldDef
type PlanningDocIssues = ctx.PlanningDocIssues
type PlanningDeliverableType = ctx.PlanningDeliverableType
type PlanningCodeIssues = ctx.PlanningCodeIssues

// Testing constitution types.
type TestingDoc = ctx.TestingDoc

// Semantic model types.
type SemanticModelDoc = ctx.SemanticModelDoc

// Issue format types.
type IssueFormatDoc = ctx.IssueFormatDoc
type IssueFormatSchema = ctx.IssueFormatSchema
type IssueFormatRule = ctx.IssueFormatRule
type IssueFormatField = ctx.IssueFormatField

// Shared field types.
type Phase = ctx.Phase
type Risk = ctx.Risk
type ContextIssue = ctx.ContextIssue
type NamedDoc = ctx.NamedDoc

// Release filter types.
type releaseFilter = ctx.ReleaseFilter

// ---------------------------------------------------------------------------
// Constants re-exported from internal/context.
// ---------------------------------------------------------------------------

const defaultMeasureContext = ctx.DefaultMeasureContext
const defaultStitchContext = ctx.DefaultStitchContext
const defaultMaxContextBytes = ctx.DefaultMaxContextBytes

// ---------------------------------------------------------------------------
// Variable re-exports from internal/context.
// ---------------------------------------------------------------------------

var standardContextPatterns = ctx.StandardContextPatterns
var typedDocPaths = ctx.TypedDocPaths

// ---------------------------------------------------------------------------
// Function delegates — unexported wrappers that preserve the original
// call signatures used throughout the parent package.
// ---------------------------------------------------------------------------

func loadPhaseContext(path string) (*PhaseContext, error) {
	return ctx.LoadPhaseContext(path)
}

func buildProjectContext(existingIssuesJSON string, project ProjectConfig, phaseCtx *PhaseContext) (*ProjectContext, error) {
	return ctx.BuildProjectContext(existingIssuesJSON, ContextConfig{
		ContextInclude: project.ContextInclude,
		ContextExclude: project.ContextExclude,
		ContextSources: project.ContextSources,
		Releases:       project.Releases,
		Release:        project.Release,
		GoSourceDirs:   project.GoSourceDirs,
	}, phaseCtx, dirCobbler)
}

func selectNextPendingUseCase(cfg ProjectConfig) (*UseCaseDoc, error) {
	return ctx.SelectNextPendingUseCase(ContextConfig{
		Releases: cfg.Releases,
		Release:  cfg.Release,
	})
}

func loadYAML[T any](path string) *T {
	return ctx.LoadYAML[T](path)
}

func loadNamedDoc(path string) *NamedDoc {
	return ctx.LoadNamedDoc(path)
}

func parseIssuesJSON(jsonStr string) []ContextIssue {
	return ctx.ParseIssuesJSON(jsonStr)
}

func numberLines(content string) string {
	return ctx.NumberLines(content)
}

func loadSourceFiles(dirs []string) []SourceFile {
	return ctx.LoadSourceFiles(dirs)
}

func parseContextSources(text string) []string {
	return ctx.ParseContextSources(text)
}

func resolveContextSources(sources string) []string {
	return ctx.ResolveContextSources(sources)
}

func resolveFileSet(text string) map[string]bool {
	return ctx.ResolveFileSet(text)
}

func classifyContextFile(path string) string {
	return ctx.ClassifyContextFile(path)
}

func ensureTypedDocs(files []string) []string {
	return ctx.EnsureTypedDocs(files)
}

func resolveStandardFiles() []string {
	return ctx.ResolveStandardFiles()
}

func newReleaseFilter(releases []string, release string) releaseFilter {
	return ctx.NewReleaseFilter(releases, release)
}

func extractFileRelease(path string) string {
	return ctx.ExtractFileRelease(path)
}

func fileMatchesRelease(path string, rf releaseFilter) bool {
	return ctx.FileMatchesRelease(path, rf)
}

func prdIDsFromUseCases(useCases []*UseCaseDoc) map[string]bool {
	return ctx.PRDIDsFromUseCases(useCases)
}

func ucStatusDone(status string) bool {
	return ctx.UCStatusDone(status)
}

func parseTouchpointPackages(touchpoints []map[string]string) []string {
	return ctx.ParseTouchpointPackages(touchpoints)
}

func loadContextFileInto(c *ProjectContext, path string, rf releaseFilter) {
	ctx.LoadContextFileInto(c, path, rf)
}

func stripParenthetical(s string) string {
	return ctx.StripParenthetical(s)
}

func sourceFileMatchesAny(sf SourceFile, suffixes []string) bool {
	return ctx.SourceFileMatchesAny(sf, suffixes)
}

func filterSourceFiles(sources []SourceFile, requiredPaths []string) []SourceFile {
	return ctx.FilterSourceFiles(sources, requiredPaths)
}

func applyContextBudget(c *ProjectContext, budget int, requiredPaths []string) {
	ctx.ApplyContextBudget(c, budget, requiredPaths)
}

func summarizeGoHeaders(content string) string {
	return ctx.SummarizeGoHeaders(content)
}

func summarizeCustom(command, filePath, fullContent string) string {
	return ctx.SummarizeCustom(command, filePath, fullContent)
}

func loadOODPromptContext() ([]OODPackageContractRef, []ArchSharedProtocol) {
	return ctx.LoadOODPromptContext()
}

// safeCountLines stays in prompt_files.go (not delegated here).
