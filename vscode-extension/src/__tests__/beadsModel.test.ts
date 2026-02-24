// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

import { describe, it, expect } from "vitest";
import * as path from "path";
import {
  parseIssue,
  parseStatus,
  parseDependencies,
  parseComments,
  extractInvocationRecord,
  TOKENS_PATTERN,
  BeadsStore,
  BeadsComment,
} from "../beadsModel";

const FIXTURES = path.resolve(__dirname, "..", "__fixtures__");

// ---- TOKENS_PATTERN ----

describe("TOKENS_PATTERN", () => {
  it("matches 'tokens: 35000' and captures the number", () => {
    const match = "tokens: 35000".match(TOKENS_PATTERN);
    expect(match).not.toBeNull();
    expect(match![1]).toBe("35000");
  });

  it("matches 'tokens: 0'", () => {
    const match = "tokens: 0".match(TOKENS_PATTERN);
    expect(match).not.toBeNull();
    expect(match![1]).toBe("0");
  });

  it("rejects 'tokens: abc'", () => {
    expect("tokens: abc".match(TOKENS_PATTERN)).toBeNull();
  });

  it("rejects text not at start", () => {
    expect("total tokens: 5000".match(TOKENS_PATTERN)).toBeNull();
  });

  it("rejects trailing content", () => {
    expect("tokens: 5000 extra".match(TOKENS_PATTERN)).toBeNull();
  });
});

// ---- extractInvocationRecord ----

describe("extractInvocationRecord", () => {
  const comment: BeadsComment = {
    id: 1,
    issue_id: "test-001",
    author: "tester",
    text: "tokens: 35000",
    created_at: "2026-01-01T00:00:00Z",
  };

  it("returns InvocationRecord for matching comment", () => {
    const record = extractInvocationRecord(comment);
    expect(record).toBeDefined();
    expect(record!.tokens).toBe(35000);
    expect(record!.comment).toBe(comment);
  });

  it("returns undefined for non-matching comment", () => {
    const result = extractInvocationRecord({ ...comment, text: "started work" });
    expect(result).toBeUndefined();
  });

  it("returns undefined for empty text", () => {
    const result = extractInvocationRecord({ ...comment, text: "" });
    expect(result).toBeUndefined();
  });
});

// ---- parseStatus ----

describe("parseStatus", () => {
  it("returns 'open' for 'open'", () => {
    expect(parseStatus("open")).toBe("open");
  });

  it("returns 'in_progress' for 'in_progress'", () => {
    expect(parseStatus("in_progress")).toBe("in_progress");
  });

  it("returns 'closed' for 'closed'", () => {
    expect(parseStatus("closed")).toBe("closed");
  });

  it("returns 'open' for unknown string", () => {
    expect(parseStatus("unknown")).toBe("open");
  });

  it("returns 'open' for null", () => {
    expect(parseStatus(null)).toBe("open");
  });

  it("returns 'open' for undefined", () => {
    expect(parseStatus(undefined)).toBe("open");
  });
});

// ---- parseComments ----

describe("parseComments", () => {
  it("returns empty array for non-array input", () => {
    expect(parseComments(null)).toEqual([]);
    expect(parseComments(undefined)).toEqual([]);
    expect(parseComments("string")).toEqual([]);
  });

  it("parses array of comment objects", () => {
    const raw = [
      {
        id: 1,
        issue_id: "test-001",
        author: "tester",
        text: "hello",
        created_at: "2026-01-01T00:00:00Z",
      },
    ];
    const result = parseComments(raw);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe(1);
    expect(result[0].text).toBe("hello");
  });

  it("skips non-object entries", () => {
    const raw = ["not an object", { id: 1, text: "valid" }];
    const result = parseComments(raw);
    expect(result).toHaveLength(1);
  });
});

// ---- parseDependencies ----

describe("parseDependencies", () => {
  it("returns empty array for non-array input", () => {
    expect(parseDependencies(null)).toEqual([]);
    expect(parseDependencies(undefined)).toEqual([]);
  });

  it("parses array of dependency objects", () => {
    const raw = [
      {
        issue_id: "test-002",
        depends_on_id: "test-001",
        type: "blocks",
        created_at: "2026-01-01T00:00:00Z",
        created_by: "tester",
        metadata: "{}",
      },
    ];
    const result = parseDependencies(raw);
    expect(result).toHaveLength(1);
    expect(result[0].depends_on_id).toBe("test-001");
    expect(result[0].type).toBe("blocks");
  });

  it("defaults missing metadata to '{}'", () => {
    const raw = [{ issue_id: "test-001", depends_on_id: "test-002" }];
    const result = parseDependencies(raw);
    expect(result[0].metadata).toBe("{}");
  });
});

// ---- parseIssue ----

describe("parseIssue", () => {
  it("returns undefined when id is empty", () => {
    expect(parseIssue({ id: "" })).toBeUndefined();
    expect(parseIssue({})).toBeUndefined();
  });

  it("parses a complete issue object", () => {
    const raw = {
      id: "test-001",
      title: "Fix bug",
      description: "desc",
      status: "open",
      priority: 1,
      issue_type: "bug",
      owner: "test@test.com",
      created_at: "2026-01-01T00:00:00Z",
      created_by: "tester",
      updated_at: "2026-01-01T00:00:00Z",
      closed_at: null,
      close_reason: null,
      labels: ["code"],
      dependencies: [],
      comments: [],
    };
    const issue = parseIssue(raw);
    expect(issue).toBeDefined();
    expect(issue!.id).toBe("test-001");
    expect(issue!.title).toBe("Fix bug");
    expect(issue!.status).toBe("open");
    expect(issue!.priority).toBe(1);
    expect(issue!.labels).toEqual(["code"]);
  });

  it("defaults missing labels to empty array", () => {
    const issue = parseIssue({ id: "test-001" });
    expect(issue!.labels).toEqual([]);
  });

  it("defaults missing dependencies to empty array", () => {
    const issue = parseIssue({ id: "test-001" });
    expect(issue!.dependencies).toEqual([]);
  });

  it("defaults missing comments to empty array", () => {
    const issue = parseIssue({ id: "test-001" });
    expect(issue!.comments).toEqual([]);
  });

  it("defaults priority to 3 when missing", () => {
    const issue = parseIssue({ id: "test-001" });
    expect(issue!.priority).toBe(3);
  });

  it("defaults status to 'open' when unknown", () => {
    const issue = parseIssue({ id: "test-001", status: "weird" });
    expect(issue!.status).toBe("open");
  });
});

// ---- BeadsStore integration ----

describe("BeadsStore", () => {
  it("ensureBuilt populates issues from fixture JSONL", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    expect(store.listIssues().length).toBe(5);
  });

  it("ensureBuilt is idempotent", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const count1 = store.listIssues().length;
    store.ensureBuilt();
    const count2 = store.listIssues().length;
    expect(count1).toBe(count2);
  });

  it("invalidate clears cached data", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    expect(store.listIssues().length).toBeGreaterThan(0);
    store.invalidate();
    expect(store.listIssues()).toEqual([]);
  });

  it("getIssue returns the correct issue", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const issue = store.getIssue("test-001");
    expect(issue).toBeDefined();
    expect(issue!.title).toBe("Fix the bug");
  });

  it("getIssue returns undefined for unknown id", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    expect(store.getIssue("nonexistent")).toBeUndefined();
  });

  it("listByStatus returns only open issues", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const open = store.listByStatus("open");
    expect(open.length).toBe(2);
    for (const issue of open) {
      expect(issue.status).toBe("open");
    }
  });

  it("listByStatus returns only closed issues", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const closed = store.listByStatus("closed");
    expect(closed.length).toBe(2);
    const ids = closed.map((i) => i.id).sort();
    expect(ids).toEqual(["test-003", "test-005"]);
  });

  it("listByStatus returns only in_progress issues", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const inProgress = store.listByStatus("in_progress");
    expect(inProgress.length).toBe(1);
    expect(inProgress[0].id).toBe("test-004");
  });

  it("listInvocationRecords returns records from all matching comments", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const records = store.listInvocationRecords();
    expect(records.length).toBe(3);
    const tokens = records.map((r) => r.tokens).sort((a, b) => a - b);
    expect(tokens).toEqual([12000, 35000, 50000]);
  });

  it("getInvocationRecords returns records for a specific issue", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const records = store.getInvocationRecords("test-003");
    expect(records.length).toBe(1);
    expect(records[0].tokens).toBe(35000);
  });

  it("getInvocationRecords returns empty array for unknown issue", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    expect(store.getInvocationRecords("nonexistent")).toEqual([]);
  });

  it("produces empty store for nonexistent root", () => {
    const store = new BeadsStore("/nonexistent/root");
    store.ensureBuilt();
    expect(store.listIssues()).toEqual([]);
  });
});
