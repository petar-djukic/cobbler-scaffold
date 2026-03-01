// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R4
// uc: rel02.0-uc004-issue-tracker-view

import * as vscode from "vscode";
import { IssuesStore, GitHubIssue, IssueStatus } from "./issuesModel";

// ---- Tree item types ----

/** Discriminated union for all node types in the issues tree. */
export type IssueTreeItem = StatusGroupItem | IssueItem;

interface StatusGroupItem {
  kind: "statusGroup";
  status: IssueStatus;
  label: string;
  count: number;
}

interface IssueItem {
  kind: "issue";
  issue: GitHubIssue;
}

// ---- Status group configuration ----

const STATUS_GROUPS: { status: IssueStatus; label: string; icon: string }[] = [
  { status: "in_progress", label: "In Progress", icon: "sync" },
  { status: "open", label: "Open", icon: "circle-outline" },
  { status: "closed", label: "Closed", icon: "check" },
];

/** Returns a codicon id for the issue priority. */
export function priorityIcon(priority: number): string {
  switch (priority) {
    case 1:
      return "arrow-up";
    case 2:
      return "dash";
    case 3:
      return "arrow-down";
    default:
      return "dash";
  }
}

// ---- Provider ----

/**
 * TreeDataProvider for the mageOrchestrator.issues view. Displays
 * GitHub issues grouped by status, sorted by priority within each group.
 */
export class IssueBrowserProvider
  implements vscode.TreeDataProvider<IssueTreeItem>
{
  private _onDidChangeTreeData = new vscode.EventEmitter<
    IssueTreeItem | undefined | void
  >();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private store: IssuesStore;

  constructor(store: IssuesStore) {
    this.store = store;
  }

  /** Fires a tree refresh. Data is updated by the IssuesStore polling cycle. */
  refresh(): void {
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: IssueTreeItem): vscode.TreeItem {
    switch (element.kind) {
      case "statusGroup":
        return this.statusGroupTreeItem(element);
      case "issue":
        return this.issueTreeItem(element);
    }
  }

  getChildren(element?: IssueTreeItem): IssueTreeItem[] {
    if (!element) {
      return this.rootChildren();
    }

    if (element.kind === "statusGroup") {
      return this.statusGroupChildren(element.status);
    }

    return [];
  }

  // ---- Root children ----

  private rootChildren(): StatusGroupItem[] {
    return STATUS_GROUPS.map(({ status, label }) => {
      const count = this.store.listByStatus(status).length;
      return { kind: "statusGroup" as const, status, label, count };
    });
  }

  // ---- Status group children ----

  private statusGroupChildren(status: IssueStatus): IssueItem[] {
    return this.store
      .listByStatus(status)
      .sort((a, b) => a.priority - b.priority)
      .map((issue): IssueItem => ({ kind: "issue", issue }));
  }

  // ---- Tree item builders ----

  private statusGroupTreeItem(item: StatusGroupItem): vscode.TreeItem {
    const ti = new vscode.TreeItem(
      `${item.label} (${item.count})`,
      item.count > 0
        ? vscode.TreeItemCollapsibleState.Collapsed
        : vscode.TreeItemCollapsibleState.None
    );
    ti.contextValue = "issueStatusGroup";
    ti.iconPath = new vscode.ThemeIcon(
      STATUS_GROUPS.find((g) => g.status === item.status)?.icon ??
        "circle-outline"
    );
    return ti;
  }

  private issueTreeItem(item: IssueItem): vscode.TreeItem {
    const issue = item.issue;
    const ti = new vscode.TreeItem(
      `#${issue.number}: ${issue.title}`,
      vscode.TreeItemCollapsibleState.None
    );
    ti.description = this.issueDescription(issue);
    ti.tooltip = this.issueTooltip(issue);
    ti.contextValue = "githubIssue";
    ti.iconPath = new vscode.ThemeIcon(priorityIcon(issue.priority));
    return ti;
  }

  private issueDescription(issue: GitHubIssue): string {
    const parts: string[] = [];
    parts.push(`P${issue.priority}`);
    parts.push(issue.issueType);
    if (issue.labels.length > 0) {
      parts.push(issue.labels.join(", "));
    }
    return parts.join(" | ");
  }

  private issueTooltip(issue: GitHubIssue): string {
    const lines: string[] = [
      `#${issue.number}: ${issue.title}`,
      `Status: ${issue.status}`,
      `Priority: ${issue.priority}`,
      `Type: ${issue.issueType}`,
    ];
    if (issue.labels.length > 0) {
      lines.push(`Labels: ${issue.labels.join(", ")}`);
    }
    return lines.join("\n");
  }
}
