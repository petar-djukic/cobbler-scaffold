<!-- Copyright (c) 2026 Petar Djukic. All rights reserved. SPDX-License-Identifier: MIT -->

# Prompt Template Conventions

## Introduction

We use Go text/template for both the measure and stitch prompts. Default templates are embedded in the binary via `//go:embed`. Consuming projects can override them through Config fields. This guideline documents the template data contracts, customization patterns, and conventions for writing effective prompts.

## Embedded Defaults

The orchestrator embeds two templates from the `prompts/` directory.

Table 1 Embedded Prompt Templates

| Template | File | Data Type | Purpose |
|----------|------|-----------|---------|
| Measure | prompts/measure.tmpl | MeasurePromptData | Propose new tasks from project state |
| Stitch | prompts/stitch.tmpl | StitchPromptData | Execute a single task |

We embed these files using `//go:embed` directives in measure.go and stitch.go. The embedded strings serve as defaults when Config.MeasurePrompt or Config.StitchPrompt is empty.

## Template Data Contracts

### MeasurePromptData

Table 2 MeasurePromptData Fields

| Field | Type | Source |
|-------|------|--------|
| ExistingIssues | string (JSON) | bd list output |
| Limit | int | CobblerConfig.MaxIssues |
| OutputPath | string | Computed file path in CobblerDir |
| UserInput | string | CobblerConfig.UserPrompt |

The measure template receives a JSON string of existing issues. We render it inline in the prompt so Claude sees the full issue tracker state. The Limit field tells Claude how many tasks to propose. The OutputPath is where Claude writes its JSON response.

### StitchPromptData

Table 3 StitchPromptData Fields

| Field | Type | Source |
|-------|------|--------|
| Title | string | Task title from beads |
| ID | string | Task ID from beads |
| IssueType | string | Task type from beads (default "task") |
| Description | string | Task description from beads |

The stitch template receives the task details. Claude uses these to understand what to implement and includes the task ID in commit messages.

## Customization

Consuming projects override templates by setting Config.MeasurePrompt or Config.StitchPrompt to a non-empty string. The string must be a valid Go text/template that uses the corresponding data type.

```go
cfg := orchestrator.Config{
    MeasurePrompt: myCustomMeasureTemplate,
    StitchPrompt:  myCustomStitchTemplate,
}
```

When writing custom templates, we reference the data fields using `{{.FieldName}}` syntax. Conditional sections use `{{- if .UserInput}}...{{- end}}`.

## Conventions for Prompt Authors

We follow these conventions when writing or modifying prompt templates.

We instruct Claude to read project documentation (VISION, ARCHITECTURE, PRDs) before acting. This ensures generated code aligns with the project specifications.

We include structured output instructions. The measure template specifies a JSON format for proposed tasks. The stitch template instructs Claude to commit with the task ID.

We avoid prescribing specific implementation details in the prompt. The specifications (PRDs, use cases) carry the requirements. The prompt points Claude to those documents.

We use the UserInput field for session-specific context that does not belong in the template itself.
