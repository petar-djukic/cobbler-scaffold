// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	an "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/analysis"
	ctx "github.com/mesh-intelligence/cobbler-scaffold/pkg/orchestrator/internal/context"
)

// ---------------------------------------------------------------------------
// Dependency injection: wire the parent package's logf and loadAnalysisDoc
// into the internal/context package at init time.
// ---------------------------------------------------------------------------

func init() {
	ctx.Log = logf
	ctx.LoadAnalysisDocFn = func(dir string) any {
		return an.LoadAnalysisDoc(dir)
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
type PRDRequirementItem = ctx.PRDRequirementItem
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

// Interface constitution types.
type InterfaceDoc = ctx.InterfaceDoc

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

// buildProjectContext converts parent-package ProjectConfig to the internal
// ContextConfig before delegating to ctx.BuildProjectContext. This wrapper
// exists because the parent and internal packages define separate config structs.
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

// selectNextPendingUseCase converts parent-package ProjectConfig to the
// internal ContextConfig before delegating. Same rationale as buildProjectContext.
func selectNextPendingUseCase(cfg ProjectConfig) (*UseCaseDoc, error) {
	return ctx.SelectNextPendingUseCase(ContextConfig{
		Releases: cfg.Releases,
		Release:  cfg.Release,
	})
}
