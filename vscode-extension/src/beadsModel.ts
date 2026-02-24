// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R4
// uc: rel02.0-uc004-issue-tracker-view

import * as fs from "fs";
import * as path from "path";

// ---- Exported types ----

/** A comment on a beads issue. */
export interface BeadsComment {
  id: number;
  issue_id: string;
  author: string;
  text: string;
  created_at: string;
}

/** A dependency relationship between beads issues. */
export interface BeadsDependency {
  issue_id: string;
  depends_on_id: string;
  type: string;
  created_at: string;
  created_by: string;
  metadata: string;
}

/** Token usage extracted from a comment matching "tokens: <number>" or a full JSON InvocationRecord. */
export interface InvocationRecord {
  tokens: number;
  comment: BeadsComment;
  /** "measure" or "stitch" when parsed from full JSON. */
  caller?: string;
  /** RFC3339 timestamp when parsed from full JSON. */
  startedAt?: string;
  /** Duration in seconds when parsed from full JSON. */
  durationS?: number;
  /** Input token count when parsed from full JSON. */
  inputTokens?: number;
  /** Output token count when parsed from full JSON. */
  outputTokens?: number;
  /** Cache creation tokens when parsed from full JSON. */
  cacheCreationTokens?: number;
  /** Cache read tokens when parsed from full JSON. */
  cacheReadTokens?: number;
  /** Cost in USD when parsed from full JSON. */
  costUSD?: number;
  /** LOC snapshot before invocation when parsed from full JSON. */
  locBefore?: { production: number; test: number };
  /** LOC snapshot after invocation when parsed from full JSON. */
  locAfter?: { production: number; test: number };
  /** Diff statistics when parsed from full JSON. */
  diff?: { files: number; insertions: number; deletions: number };
}

/** Issue status as it appears in the JSONL. */
export type IssueStatus = "open" | "in_progress" | "closed";

/** A beads issue parsed from .beads/issues.jsonl. */
export interface BeadsIssue {
  id: string;
  title: string;
  description: string;
  status: IssueStatus;
  priority: number;
  issue_type: string;
  owner: string;
  created_at: string;
  created_by: string;
  updated_at: string;
  closed_at: string | null;
  close_reason: string | null;
  labels: string[];
  dependencies: BeadsDependency[];
  comments: BeadsComment[];
}

// ---- BeadsStore ----

/**
 * In-memory store of beads issues parsed from .beads/issues.jsonl.
 * Follows the SpecGraph pattern: lazy load via ensureBuilt(), clear
 * via invalidate(), accessors return cached data.
 */
export class BeadsStore {
  private issues = new Map<string, BeadsIssue>();
  private built = false;
  private root: string;

  constructor(workspaceRoot: string) {
    this.root = workspaceRoot;
  }

  /** Builds the store if not already built. Idempotent until invalidate(). */
  ensureBuilt(): void {
    if (this.built) {
      return;
    }
    this.parseIssuesJsonl();
    this.built = true;
  }

  /** Clears all cached data. The next ensureBuilt() call will re-parse. */
  invalidate(): void {
    this.issues.clear();
    this.built = false;
  }

  /** Returns a single issue by id, or undefined. */
  getIssue(id: string): BeadsIssue | undefined {
    return this.issues.get(id);
  }

  /** Returns all issues. */
  listIssues(): BeadsIssue[] {
    return Array.from(this.issues.values());
  }

  /** Returns issues filtered by status. */
  listByStatus(status: IssueStatus): BeadsIssue[] {
    return this.listIssues().filter((i) => i.status === status);
  }

  /** Extracts InvocationRecords from all comments across all issues. */
  listInvocationRecords(): InvocationRecord[] {
    const records: InvocationRecord[] = [];
    for (const issue of this.issues.values()) {
      for (const comment of issue.comments) {
        const record = extractInvocationRecord(comment);
        if (record) {
          records.push(record);
        }
      }
    }
    return records;
  }

  /** Extracts InvocationRecords for a specific issue. */
  getInvocationRecords(issueId: string): InvocationRecord[] {
    const issue = this.issues.get(issueId);
    if (!issue) {
      return [];
    }
    const records: InvocationRecord[] = [];
    for (const comment of issue.comments) {
      const record = extractInvocationRecord(comment);
      if (record) {
        records.push(record);
      }
    }
    return records;
  }

  // ---- Internal parsing ----

  private parseIssuesJsonl(): void {
    const filePath = path.join(this.root, ".beads", "issues.jsonl");
    let content: string;
    try {
      content = fs.readFileSync(filePath, "utf-8");
    } catch {
      return;
    }

    for (const line of content.split("\n")) {
      const trimmed = line.trim();
      if (trimmed.length === 0) {
        continue;
      }
      try {
        const raw = JSON.parse(trimmed) as Record<string, unknown>;
        const issue = parseIssue(raw);
        if (issue) {
          this.issues.set(issue.id, issue);
        }
      } catch {
        // Skip malformed JSON lines.
      }
    }
  }
}

// ---- Helpers ----

/** Pattern matching "tokens: <number>" in comment text. */
export const TOKENS_PATTERN = /^tokens:\s*(\d+)$/;

/** Extracts an InvocationRecord from a comment, or returns undefined.
 *  Tries full JSON format first, then falls back to "tokens: <number>". */
export function extractInvocationRecord(
  comment: BeadsComment
): InvocationRecord | undefined {
  // Try full JSON InvocationRecord first.
  const jsonRecord = tryParseJsonRecord(comment);
  if (jsonRecord) {
    return jsonRecord;
  }
  // Fall back to simple "tokens: <number>" format.
  const match = comment.text.match(TOKENS_PATTERN);
  if (!match) {
    return undefined;
  }
  return {
    tokens: parseInt(match[1], 10),
    comment,
  };
}

/** Attempts to parse a full JSON InvocationRecord from comment text. */
export function tryParseJsonRecord(
  comment: BeadsComment
): InvocationRecord | undefined {
  const text = comment.text.trim();
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
      comment,
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

/** Parses a raw JSON object into a BeadsIssue, or returns undefined. */
export function parseIssue(
  raw: Record<string, unknown>
): BeadsIssue | undefined {
  const id = String(raw.id ?? "");
  if (!id) {
    return undefined;
  }
  return {
    id,
    title: String(raw.title ?? ""),
    description: String(raw.description ?? ""),
    status: parseStatus(raw.status),
    priority: typeof raw.priority === "number" ? raw.priority : 3,
    issue_type: String(raw.issue_type ?? "task"),
    owner: String(raw.owner ?? ""),
    created_at: String(raw.created_at ?? ""),
    created_by: String(raw.created_by ?? ""),
    updated_at: String(raw.updated_at ?? ""),
    closed_at: raw.closed_at != null ? String(raw.closed_at) : null,
    close_reason: raw.close_reason != null ? String(raw.close_reason) : null,
    labels: Array.isArray(raw.labels)
      ? raw.labels.map((l: unknown) => String(l))
      : [],
    dependencies: parseDependencies(raw.dependencies),
    comments: parseComments(raw.comments),
  };
}

/** Normalizes a status string to IssueStatus. Unknown values default to "open". */
export function parseStatus(raw: unknown): IssueStatus {
  const s = String(raw ?? "").toLowerCase();
  if (s === "open" || s === "in_progress" || s === "closed") {
    return s;
  }
  return "open";
}

/** Parses the dependencies array from a raw issue. */
export function parseDependencies(raw: unknown): BeadsDependency[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .filter(
      (d): d is Record<string, unknown> => typeof d === "object" && d !== null
    )
    .map((d) => ({
      issue_id: String(d.issue_id ?? ""),
      depends_on_id: String(d.depends_on_id ?? ""),
      type: String(d.type ?? ""),
      created_at: String(d.created_at ?? ""),
      created_by: String(d.created_by ?? ""),
      metadata: String(d.metadata ?? "{}"),
    }));
}

/** Parses the comments array from a raw issue. */
export function parseComments(raw: unknown): BeadsComment[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .filter(
      (c): c is Record<string, unknown> => typeof c === "object" && c !== null
    )
    .map((c) => ({
      id: typeof c.id === "number" ? c.id : 0,
      issue_id: String(c.issue_id ?? ""),
      author: String(c.author ?? ""),
      text: String(c.text ?? ""),
      created_at: String(c.created_at ?? ""),
    }));
}
