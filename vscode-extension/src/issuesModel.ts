// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R4
// uc: rel02.0-uc004-issue-tracker-view

import { execFile } from "child_process";

/** Runs gh CLI and returns stdout. */
function execGh(args: string[], cwd: string): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile("gh", args, { cwd }, (error, stdout) => {
      if (error) {
        reject(error);
        return;
      }
      resolve(stdout);
    });
  });
}

// ---- Exported types ----

/** Issue status derived from GitHub state and labels. */
export type IssueStatus = "open" | "in_progress" | "closed";

/** A comment on a GitHub issue. */
export interface GitHubComment {
  id: string;
  author: string;
  body: string;
  createdAt: string;
}

/** A GitHub issue with derived fields for the issue browser. */
export interface GitHubIssue {
  number: number;
  title: string;
  state: string;
  status: IssueStatus;
  priority: number;
  issueType: string;
  labels: string[];
  body: string;
  createdAt: string;
  updatedAt: string;
  closedAt: string | null;
  comments: GitHubComment[];
}

/** Token usage extracted from a comment matching "tokens: <number>" or a full JSON record. */
export interface InvocationRecord {
  tokens: number;
  issueNumber: number;
  createdAt: string;
  caller?: string;
  startedAt?: string;
  durationS?: number;
  inputTokens?: number;
  outputTokens?: number;
  cacheCreationTokens?: number;
  cacheReadTokens?: number;
  costUSD?: number;
  locBefore?: { production: number; test: number };
  locAfter?: { production: number; test: number };
  diff?: { files: number; insertions: number; deletions: number };
}

// ---- IssuesStore ----

/**
 * In-memory store of GitHub issues fetched via the gh CLI.
 * Call refresh() to update the cache; sync accessors read from cache.
 */
export class IssuesStore {
  private issues: GitHubIssue[] = [];
  private root: string;

  constructor(workspaceRoot: string) {
    this.root = workspaceRoot;
  }

  /** Fetches issues from GitHub and updates the cache. */
  async refresh(): Promise<void> {
    try {
      const stdout = await execGh([
        "issue", "list",
        "--state", "all",
        "--json", "number,title,state,labels,body,comments,createdAt,updatedAt,closedAt",
        "--limit", "200",
      ], this.root);
      const raw = JSON.parse(stdout) as RawGitHubIssue[];
      this.issues = raw.map(parseGitHubIssue);
    } catch {
      // gh CLI unavailable or unauthenticated — keep existing cache.
    }
  }

  /** Returns all cached issues. */
  listIssues(): GitHubIssue[] {
    return this.issues;
  }

  /** Returns cached issues filtered by derived status. */
  listByStatus(status: IssueStatus): GitHubIssue[] {
    return this.issues.filter((i) => i.status === status);
  }

  /** Returns counts of open and in-progress issues. */
  getStats(): { open: number; inProgress: number } {
    return {
      open: this.issues.filter((i) => i.status === "open").length,
      inProgress: this.issues.filter((i) => i.status === "in_progress").length,
    };
  }

  /** Extracts InvocationRecords from all comments across all cached issues. */
  listInvocationRecords(): InvocationRecord[] {
    const records: InvocationRecord[] = [];
    for (const issue of this.issues) {
      for (const comment of issue.comments) {
        const record = extractInvocationRecord(comment, issue.number);
        if (record) {
          records.push(record);
        }
      }
    }
    return records;
  }

  /** Extracts InvocationRecords for a specific issue. */
  getInvocationRecords(issueNumber: number): InvocationRecord[] {
    const issue = this.issues.find((i) => i.number === issueNumber);
    if (!issue) {
      return [];
    }
    const records: InvocationRecord[] = [];
    for (const comment of issue.comments) {
      const record = extractInvocationRecord(comment, issue.number);
      if (record) {
        records.push(record);
      }
    }
    return records;
  }
}

// ---- Raw types from gh CLI ----

interface RawGitHubIssue {
  number: number;
  title: string;
  state: string;
  labels: { name: string }[];
  body: string;
  comments: RawGitHubComment[];
  createdAt: string;
  updatedAt: string;
  closedAt: string | null;
}

interface RawGitHubComment {
  id: string;
  author: { login: string } | null;
  body: string;
  createdAt: string;
}

// ---- Parsing helpers ----

/** Pattern matching "tokens: <number>" in comment text. */
export const TOKENS_PATTERN = /^tokens:\s*(\d+)$/;

/** Parses raw gh JSON into a GitHubIssue. */
export function parseGitHubIssue(raw: RawGitHubIssue): GitHubIssue {
  const labels = (raw.labels ?? []).map((l) => l.name);
  return {
    number: raw.number,
    title: raw.title ?? "",
    state: raw.state ?? "OPEN",
    status: deriveStatus(raw.state, labels),
    priority: derivePriority(labels),
    issueType: deriveIssueType(labels),
    labels,
    body: raw.body ?? "",
    createdAt: raw.createdAt ?? "",
    updatedAt: raw.updatedAt ?? "",
    closedAt: raw.closedAt ?? null,
    comments: (raw.comments ?? []).map(parseGitHubComment),
  };
}

/** Parses a raw gh JSON comment into GitHubComment. */
function parseGitHubComment(raw: RawGitHubComment): GitHubComment {
  return {
    id: raw.id ?? "",
    author: raw.author?.login ?? "",
    body: raw.body ?? "",
    createdAt: raw.createdAt ?? "",
  };
}

/** Derives IssueStatus from GitHub state and labels. */
export function deriveStatus(state: string, labels: string[]): IssueStatus {
  if (state === "CLOSED") {
    return "closed";
  }
  if (labels.includes("in_progress")) {
    return "in_progress";
  }
  return "open";
}

/** Derives priority from labels. Default is 2 (medium). */
export function derivePriority(labels: string[]): number {
  for (const label of labels) {
    const match = label.match(/^priority:(\d+)$/);
    if (match) {
      return parseInt(match[1], 10);
    }
  }
  return 2;
}

/** Derives issue type from labels. Default is "task". */
export function deriveIssueType(labels: string[]): string {
  if (labels.includes("bug")) {
    return "bug";
  }
  if (labels.includes("enhancement")) {
    return "enhancement";
  }
  if (labels.includes("documentation")) {
    return "documentation";
  }
  return "task";
}

/** Extracts an InvocationRecord from a comment, or returns undefined.
 *  Tries full JSON format first, then falls back to "tokens: <number>". */
export function extractInvocationRecord(
  comment: GitHubComment,
  issueNumber: number
): InvocationRecord | undefined {
  const jsonRecord = tryParseJsonRecord(comment, issueNumber);
  if (jsonRecord) {
    return jsonRecord;
  }
  const match = comment.body.match(TOKENS_PATTERN);
  if (!match) {
    return undefined;
  }
  return {
    tokens: parseInt(match[1], 10),
    issueNumber,
    createdAt: comment.createdAt,
  };
}

/** Attempts to parse a full JSON InvocationRecord from comment text. */
export function tryParseJsonRecord(
  comment: GitHubComment,
  issueNumber: number
): InvocationRecord | undefined {
  const text = comment.body.trim();
  if (!text.startsWith("{")) {
    return undefined;
  }
  try {
    const raw = JSON.parse(text) as Record<string, unknown>;
    const tokens = raw.tokens as Record<string, unknown> | undefined;
    if (!tokens || typeof tokens !== "object") {
      return undefined;
    }
    const input = typeof tokens.input === "number" ? tokens.input : 0;
    const output = typeof tokens.output === "number" ? tokens.output : 0;
    const cacheCreation =
      typeof tokens.cache_creation === "number" ? tokens.cache_creation : 0;
    const cacheRead =
      typeof tokens.cache_read === "number" ? tokens.cache_read : 0;
    const costUSD =
      typeof tokens.cost_usd === "number" ? tokens.cost_usd : 0;

    const record: InvocationRecord = {
      tokens: input + output,
      issueNumber,
      createdAt: comment.createdAt,
      caller: typeof raw.caller === "string" ? raw.caller : undefined,
      startedAt:
        typeof raw.started_at === "string" ? raw.started_at : undefined,
      durationS:
        typeof raw.duration_s === "number" ? raw.duration_s : undefined,
      inputTokens: input,
      outputTokens: output,
      cacheCreationTokens: cacheCreation || undefined,
      cacheReadTokens: cacheRead || undefined,
      costUSD: costUSD || undefined,
    };

    const locBefore = raw.loc_before as Record<string, unknown> | undefined;
    if (locBefore && typeof locBefore === "object") {
      record.locBefore = {
        production:
          typeof locBefore.production === "number" ? locBefore.production : 0,
        test: typeof locBefore.test === "number" ? locBefore.test : 0,
      };
    }

    const locAfter = raw.loc_after as Record<string, unknown> | undefined;
    if (locAfter && typeof locAfter === "object") {
      record.locAfter = {
        production:
          typeof locAfter.production === "number" ? locAfter.production : 0,
        test: typeof locAfter.test === "number" ? locAfter.test : 0,
      };
    }

    const diff = raw.diff as Record<string, unknown> | undefined;
    if (diff && typeof diff === "object") {
      record.diff = {
        files: typeof diff.files === "number" ? diff.files : 0,
        insertions:
          typeof diff.insertions === "number" ? diff.insertions : 0,
        deletions: typeof diff.deletions === "number" ? diff.deletions : 0,
      };
    }

    return record;
  } catch {
    return undefined;
  }
}
