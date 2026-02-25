// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R3
// uc: rel02.0-uc003-branch-comparison

import * as vscode from "vscode";
import { execSync, execFileSync } from "child_process";

/** Version tag pattern: v[REL].[DATE].[REVISION] (e.g., v0.20260224.1). */
export const VERSION_TAG_PATTERN = /^v\d+\.\d{8}\.\d+/;

/** Status codes returned by git diff --name-status. */
export type FileStatus = "A" | "M" | "D" | "R" | "C" | "T";

/** A single file entry from a git diff. */
export interface DiffEntry {
  status: FileStatus;
  path: string;
  /** Present for renames: the new path. */
  newPath?: string;
}

// ---- Tree item types ----

/** Discriminated union for comparison tree nodes. */
export type ComparisonItem = DirectoryNode | FileNode | InfoNode;

interface DirectoryNode {
  kind: "directory";
  dirPath: string;
  entries: DiffEntry[];
}

export interface FileNode {
  kind: "file";
  entry: DiffEntry;
  refA: string;
  refB: string;
  workspaceRoot: string;
}

interface InfoNode {
  kind: "info";
  message: string;
}

// ---- Provider ----

/**
 * TreeDataProvider for the mageOrchestrator.comparison view. Displays
 * files changed between two git refs, grouped by directory. Each file
 * is clickable to open a VS Code diff editor.
 */
export class ComparisonBrowserProvider
  implements vscode.TreeDataProvider<ComparisonItem>
{
  private _onDidChangeTreeData = new vscode.EventEmitter<
    ComparisonItem | undefined | void
  >();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private entries: DiffEntry[] = [];
  private refA = "";
  private refB = "";

  constructor(private workspaceRoot: string) {}

  /** Sets the two refs to compare and refreshes the tree. */
  setComparison(refA: string, refB: string, entries: DiffEntry[]): void {
    this.refA = refA;
    this.refB = refB;
    this.entries = entries;
    this._onDidChangeTreeData.fire();
  }

  /** Clears the comparison view. */
  clear(): void {
    this.entries = [];
    this.refA = "";
    this.refB = "";
    this._onDidChangeTreeData.fire();
  }

  refresh(): void {
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: ComparisonItem): vscode.TreeItem {
    switch (element.kind) {
      case "directory":
        return this.directoryTreeItem(element);
      case "file":
        return this.fileTreeItem(element);
      case "info":
        return this.infoTreeItem(element);
    }
  }

  getChildren(element?: ComparisonItem): ComparisonItem[] {
    if (!element) {
      return this.getRootChildren();
    }
    if (element.kind === "directory") {
      return element.entries.map(
        (e): FileNode => ({
          kind: "file",
          entry: e,
          refA: this.refA,
          refB: this.refB,
          workspaceRoot: this.workspaceRoot,
        })
      );
    }
    return [];
  }

  // ---- Root children ----

  private getRootChildren(): ComparisonItem[] {
    if (this.entries.length === 0) {
      if (this.refA && this.refB) {
        return [{ kind: "info", message: "No differences between the selected refs" }];
      }
      return [{ kind: "info", message: "Select two tags or generations to compare" }];
    }

    // Group by directory.
    const groups = groupByDirectory(this.entries);
    const dirNames = Array.from(groups.keys()).sort();

    // If only one directory, flatten to file nodes.
    if (dirNames.length === 1) {
      const entries = groups.get(dirNames[0]) ?? [];
      return entries.map(
        (e): FileNode => ({
          kind: "file",
          entry: e,
          refA: this.refA,
          refB: this.refB,
          workspaceRoot: this.workspaceRoot,
        })
      );
    }

    return dirNames.map(
      (d): DirectoryNode => ({
        kind: "directory",
        dirPath: d,
        entries: groups.get(d) ?? [],
      })
    );
  }

  // ---- Tree item builders ----

  private directoryTreeItem(node: DirectoryNode): vscode.TreeItem {
    const ti = new vscode.TreeItem(
      node.dirPath,
      vscode.TreeItemCollapsibleState.Expanded
    );
    ti.description = `${node.entries.length} file${node.entries.length === 1 ? "" : "s"}`;
    ti.iconPath = new vscode.ThemeIcon("folder");
    ti.contextValue = "comparisonDirectory";
    return ti;
  }

  private fileTreeItem(node: FileNode): vscode.TreeItem {
    const displayPath = node.entry.newPath ?? node.entry.path;
    const fileName = displayPath.split("/").pop() ?? displayPath;
    const ti = new vscode.TreeItem(
      fileName,
      vscode.TreeItemCollapsibleState.None
    );
    ti.description = statusLabel(node.entry.status);
    ti.iconPath = new vscode.ThemeIcon(statusIcon(node.entry.status));
    ti.contextValue = "comparisonFile";
    ti.tooltip = `${displayPath} (${statusLabel(node.entry.status)})`;

    // Click opens diff editor between the two refs.
    if (node.entry.status !== "D") {
      ti.command = {
        command: "mageOrchestrator.openComparisonDiff",
        title: "Open Diff",
        arguments: [node],
      };
    }

    return ti;
  }

  private infoTreeItem(node: InfoNode): vscode.TreeItem {
    const ti = new vscode.TreeItem(
      node.message,
      vscode.TreeItemCollapsibleState.None
    );
    ti.iconPath = new vscode.ThemeIcon("info");
    ti.contextValue = "comparisonInfo";
    return ti;
  }
}

// ---- Git operations ----

/**
 * Lists all version tags matching the VERSION_TAG_PATTERN.
 * Returns tags sorted newest-first by tag name.
 */
export function listVersionTags(workspaceRoot: string): string[] {
  try {
    const raw = execSync("git tag --list", {
      cwd: workspaceRoot,
      encoding: "utf-8",
    });
    return raw
      .split("\n")
      .map((t) => t.trim())
      .filter((t) => VERSION_TAG_PATTERN.test(t))
      .sort()
      .reverse();
  } catch {
    return [];
  }
}

/**
 * Computes the file-level diff between two git refs.
 * Returns an array of DiffEntry objects parsed from git diff --name-status.
 */
export function computeDiff(
  workspaceRoot: string,
  refA: string,
  refB: string
): DiffEntry[] {
  try {
    const raw = execFileSync(
      "git", ["diff", "--name-status", refA, refB],
      { cwd: workspaceRoot, encoding: "utf-8" }
    );
    return parseDiffOutput(raw);
  } catch {
    return [];
  }
}

/**
 * Resolves a generation name to its best available tag ref.
 * Prefers -merged, then -finished, then -start.
 */
export function resolveGenerationRef(
  workspaceRoot: string,
  generationName: string
): string | undefined {
  const suffixes = ["merged", "finished", "start"];
  try {
    const raw = execSync("git tag --list", {
      cwd: workspaceRoot,
      encoding: "utf-8",
    });
    const tags = new Set(
      raw
        .split("\n")
        .map((t) => t.trim())
        .filter((t) => t.length > 0)
    );
    for (const suffix of suffixes) {
      const tag = `${generationName}-${suffix}`;
      if (tags.has(tag)) {
        return tag;
      }
    }
  } catch {
    // Fall through.
  }
  return undefined;
}

// ---- Parsers ----

/** Parses git diff --name-status output into DiffEntry objects. */
export function parseDiffOutput(raw: string): DiffEntry[] {
  const entries: DiffEntry[] = [];
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    if (trimmed.length === 0) {
      continue;
    }
    // Format: STATUS<tab>PATH or STATUS<tab>OLD_PATH<tab>NEW_PATH (for renames).
    const parts = trimmed.split("\t");
    if (parts.length < 2) {
      continue;
    }
    const statusChar = parts[0].charAt(0);
    if (!"AMDRTC".includes(statusChar)) {
      continue;
    }
    const statusCode = statusChar as FileStatus;
    const entry: DiffEntry = { status: statusCode, path: parts[1] };
    if (parts.length >= 3 && (statusCode === "R" || statusCode === "C")) {
      entry.newPath = parts[2];
    }
    entries.push(entry);
  }
  return entries;
}

// ---- Helpers ----

/** Groups diff entries by their parent directory. */
export function groupByDirectory(entries: DiffEntry[]): Map<string, DiffEntry[]> {
  const groups = new Map<string, DiffEntry[]>();
  for (const entry of entries) {
    const filePath = entry.newPath ?? entry.path;
    const lastSlash = filePath.lastIndexOf("/");
    const dir = lastSlash >= 0 ? filePath.substring(0, lastSlash) : ".";
    let group = groups.get(dir);
    if (!group) {
      group = [];
      groups.set(dir, group);
    }
    group.push(entry);
  }
  return groups;
}

/** Returns a human-readable label for a file status code. */
export function statusLabel(status: FileStatus): string {
  switch (status) {
    case "A":
      return "added";
    case "M":
      return "modified";
    case "D":
      return "deleted";
    case "R":
      return "renamed";
    case "C":
      return "copied";
    case "T":
      return "type changed";
    default:
      return status;
  }
}

// ---- Content provider for diff views ----

/**
 * TextDocumentContentProvider that serves file content from a git ref.
 * URIs use the mage-git-ref: scheme with a JSON query containing
 * { ref, path }. Used to display diff editors between two refs.
 */
export class GitRefContentProvider
  implements vscode.TextDocumentContentProvider
{
  constructor(private workspaceRoot: string) {}

  provideTextDocumentContent(uri: vscode.Uri): string {
    let params: { ref?: string; path?: string };
    try {
      params = JSON.parse(decodeURIComponent(uri.query));
    } catch {
      return "";
    }
    const ref = params.ref ?? "";
    const filePath = params.path ?? "";
    if (!ref || !filePath) {
      return "";
    }
    try {
      return execFileSync("git", ["show", `${ref}:${filePath}`], {
        cwd: this.workspaceRoot,
        encoding: "utf-8",
      });
    } catch {
      return "";
    }
  }
}

/** Returns a codicon name for a file status code. */
function statusIcon(status: FileStatus): string {
  switch (status) {
    case "A":
      return "diff-added";
    case "M":
      return "diff-modified";
    case "D":
      return "diff-removed";
    case "R":
      return "diff-renamed";
    case "C":
      return "diff-added";
    case "T":
      return "diff-modified";
    default:
      return "file";
  }
}
