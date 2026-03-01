// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  TOKENS_PATTERN,
  extractInvocationRecord,
  tryParseJsonRecord,
  deriveStatus,
  derivePriority,
  deriveIssueType,
  parseGitHubIssue,
  IssuesStore,
  GitHubComment,
} from "../issuesModel";

// ---- Mock child_process for IssuesStore tests ----

const { mockExecFile } = vi.hoisted(() => ({
  mockExecFile: vi.fn(),
}));
vi.mock("child_process", () => ({
  execFile: mockExecFile,
}));

// ---- Fixture data matching the gh CLI JSON format ----

const FIXTURE_ISSUES = [
  {
    number: 1,
    title: "Fix the bug",
    state: "OPEN",
    labels: [{ name: "bug" }, { name: "priority:1" }, { name: "code" }],
    body: "Fix a critical bug",
    comments: [],
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    closedAt: null,
  },
  {
    number: 2,
    title: "Add feature",
    state: "OPEN",
    labels: [{ name: "enhancement" }, { name: "code" }, { name: "rel02.0" }],
    body: "Add a new feature",
    comments: [],
    createdAt: "2026-01-02T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    closedAt: null,
  },
  {
    number: 3,
    title: "Closed task",
    state: "CLOSED",
    labels: [],
    body: "A completed task",
    comments: [
      {
        id: "IC_1",
        author: { login: "tester" },
        body: "tokens: 35000",
        createdAt: "2026-01-04T00:00:00Z",
      },
    ],
    createdAt: "2026-01-03T00:00:00Z",
    updatedAt: "2026-01-04T00:00:00Z",
    closedAt: "2026-01-04T00:00:00Z",
  },
  {
    number: 4,
    title: "In progress work",
    state: "OPEN",
    labels: [{ name: "in_progress" }, { name: "priority:1" }, { name: "code" }],
    body: "Currently active",
    comments: [
      {
        id: "IC_2",
        author: { login: "tester" },
        body: "started work",
        createdAt: "2026-01-05T00:00:00Z",
      },
      {
        id: "IC_3",
        author: { login: "tester" },
        body: "tokens: 12000",
        createdAt: "2026-01-05T01:00:00Z",
      },
    ],
    createdAt: "2026-01-05T00:00:00Z",
    updatedAt: "2026-01-05T00:00:00Z",
    closedAt: null,
  },
  {
    number: 5,
    title: "Stitch task",
    state: "CLOSED",
    labels: [{ name: "priority:3" }],
    body: "A stitch task with JSON record",
    comments: [
      {
        id: "IC_4",
        author: { login: "orchestrator" },
        body: '{"caller":"stitch","started_at":"2026-01-06T00:10:00Z","duration_s":180,"tokens":{"input":45000,"output":5000,"cache_creation":1000,"cache_read":2000,"cost_usd":0.15},"loc_before":{"production":500,"test":100},"loc_after":{"production":520,"test":110},"diff":{"files":3,"insertions":25,"deletions":5}}',
        createdAt: "2026-01-06T00:13:00Z",
      },
    ],
    createdAt: "2026-01-06T00:00:00Z",
    updatedAt: "2026-01-06T01:00:00Z",
    closedAt: "2026-01-06T01:00:00Z",
  },
];

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

// ---- deriveStatus ----

describe("deriveStatus", () => {
  it("returns 'closed' for CLOSED state", () => {
    expect(deriveStatus("CLOSED", [])).toBe("closed");
  });

  it("returns 'in_progress' for OPEN state with in_progress label", () => {
    expect(deriveStatus("OPEN", ["in_progress"])).toBe("in_progress");
  });

  it("returns 'open' for OPEN state without in_progress label", () => {
    expect(deriveStatus("OPEN", ["bug"])).toBe("open");
  });

  it("returns 'open' for OPEN state with empty labels", () => {
    expect(deriveStatus("OPEN", [])).toBe("open");
  });
});

// ---- derivePriority ----

describe("derivePriority", () => {
  it("returns 1 for priority:1 label", () => {
    expect(derivePriority(["bug", "priority:1"])).toBe(1);
  });

  it("returns 3 for priority:3 label", () => {
    expect(derivePriority(["priority:3"])).toBe(3);
  });

  it("returns 2 as default when no priority label", () => {
    expect(derivePriority(["bug", "enhancement"])).toBe(2);
  });

  it("returns 2 for empty labels", () => {
    expect(derivePriority([])).toBe(2);
  });
});

// ---- deriveIssueType ----

describe("deriveIssueType", () => {
  it("returns 'bug' for bug label", () => {
    expect(deriveIssueType(["bug"])).toBe("bug");
  });

  it("returns 'enhancement' for enhancement label", () => {
    expect(deriveIssueType(["enhancement"])).toBe("enhancement");
  });

  it("returns 'documentation' for documentation label", () => {
    expect(deriveIssueType(["documentation"])).toBe("documentation");
  });

  it("returns 'task' as default", () => {
    expect(deriveIssueType(["code"])).toBe("task");
  });

  it("returns 'task' for empty labels", () => {
    expect(deriveIssueType([])).toBe("task");
  });
});

// ---- parseGitHubIssue ----

describe("parseGitHubIssue", () => {
  it("parses a complete issue from gh JSON", () => {
    const issue = parseGitHubIssue(FIXTURE_ISSUES[0] as never);
    expect(issue.number).toBe(1);
    expect(issue.title).toBe("Fix the bug");
    expect(issue.state).toBe("OPEN");
    expect(issue.status).toBe("open");
    expect(issue.priority).toBe(1);
    expect(issue.issueType).toBe("bug");
    expect(issue.labels).toEqual(["bug", "priority:1", "code"]);
  });

  it("derives in_progress status from label", () => {
    const issue = parseGitHubIssue(FIXTURE_ISSUES[3] as never);
    expect(issue.status).toBe("in_progress");
  });

  it("derives closed status from CLOSED state", () => {
    const issue = parseGitHubIssue(FIXTURE_ISSUES[2] as never);
    expect(issue.status).toBe("closed");
  });

  it("parses comments with author login", () => {
    const issue = parseGitHubIssue(FIXTURE_ISSUES[3] as never);
    expect(issue.comments).toHaveLength(2);
    expect(issue.comments[0].author).toBe("tester");
    expect(issue.comments[0].body).toBe("started work");
  });

  it("handles null author gracefully", () => {
    const raw = {
      ...FIXTURE_ISSUES[0],
      comments: [
        { id: "IC_x", author: null, body: "test", createdAt: "" },
      ],
    };
    const issue = parseGitHubIssue(raw as never);
    expect(issue.comments[0].author).toBe("");
  });
});

// ---- extractInvocationRecord ----

describe("extractInvocationRecord", () => {
  const comment: GitHubComment = {
    id: "IC_1",
    author: "tester",
    body: "tokens: 35000",
    createdAt: "2026-01-01T00:00:00Z",
  };

  it("returns InvocationRecord for matching comment", () => {
    const record = extractInvocationRecord(comment, 1);
    expect(record).toBeDefined();
    expect(record!.tokens).toBe(35000);
    expect(record!.issueNumber).toBe(1);
    expect(record!.createdAt).toBe("2026-01-01T00:00:00Z");
  });

  it("returns undefined for non-matching comment", () => {
    const result = extractInvocationRecord({ ...comment, body: "started work" }, 1);
    expect(result).toBeUndefined();
  });

  it("returns undefined for empty text", () => {
    const result = extractInvocationRecord({ ...comment, body: "" }, 1);
    expect(result).toBeUndefined();
  });
});

// ---- tryParseJsonRecord ----

describe("tryParseJsonRecord", () => {
  it("parses full JSON InvocationRecord from comment", () => {
    const comment: GitHubComment = {
      id: "IC_4",
      author: "orchestrator",
      body: '{"caller":"stitch","started_at":"2026-01-06T00:10:00Z","duration_s":180,"tokens":{"input":45000,"output":5000,"cache_creation":1000,"cache_read":2000,"cost_usd":0.15},"loc_before":{"production":500,"test":100},"loc_after":{"production":520,"test":110},"diff":{"files":3,"insertions":25,"deletions":5}}',
      createdAt: "2026-01-06T00:13:00Z",
    };
    const record = tryParseJsonRecord(comment, 5);
    expect(record).toBeDefined();
    expect(record!.tokens).toBe(50000);
    expect(record!.caller).toBe("stitch");
    expect(record!.startedAt).toBe("2026-01-06T00:10:00Z");
    expect(record!.durationS).toBe(180);
    expect(record!.inputTokens).toBe(45000);
    expect(record!.outputTokens).toBe(5000);
    expect(record!.cacheCreationTokens).toBe(1000);
    expect(record!.cacheReadTokens).toBe(2000);
    expect(record!.costUSD).toBe(0.15);
    expect(record!.locBefore).toEqual({ production: 500, test: 100 });
    expect(record!.locAfter).toEqual({ production: 520, test: 110 });
    expect(record!.diff).toEqual({ files: 3, insertions: 25, deletions: 5 });
    expect(record!.issueNumber).toBe(5);
  });

  it("returns undefined for non-JSON comment", () => {
    const comment: GitHubComment = {
      id: "IC_x",
      author: "test",
      body: "tokens: 5000",
      createdAt: "",
    };
    expect(tryParseJsonRecord(comment, 1)).toBeUndefined();
  });

  it("returns undefined for JSON without tokens object", () => {
    const comment: GitHubComment = {
      id: "IC_x",
      author: "test",
      body: '{"caller":"stitch"}',
      createdAt: "",
    };
    expect(tryParseJsonRecord(comment, 1)).toBeUndefined();
  });
});

// ---- IssuesStore ----

describe("IssuesStore", () => {
  beforeEach(() => {
    mockExecFile.mockReset();
  });

  function setupMockGh(data: unknown[]): void {
    mockExecFile.mockImplementation(
      (_cmd: string, _args: string[], _opts: unknown, callback: Function) => {
        callback(null, JSON.stringify(data), "");
      }
    );
  }

  it("refresh populates issues from gh CLI output", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    expect(store.listIssues()).toHaveLength(5);
  });

  it("listByStatus returns only open issues", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const open = store.listByStatus("open");
    expect(open).toHaveLength(2);
    for (const issue of open) {
      expect(issue.status).toBe("open");
    }
  });

  it("listByStatus returns only closed issues", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const closed = store.listByStatus("closed");
    expect(closed).toHaveLength(2);
    const numbers = closed.map((i) => i.number).sort();
    expect(numbers).toEqual([3, 5]);
  });

  it("listByStatus returns only in_progress issues", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const inProgress = store.listByStatus("in_progress");
    expect(inProgress).toHaveLength(1);
    expect(inProgress[0].number).toBe(4);
  });

  it("getStats returns correct counts", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const stats = store.getStats();
    expect(stats.open).toBe(2);
    expect(stats.inProgress).toBe(1);
  });

  it("listInvocationRecords returns records from all matching comments", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const records = store.listInvocationRecords();
    expect(records).toHaveLength(3);
    const tokens = records.map((r) => r.tokens).sort((a, b) => a - b);
    expect(tokens).toEqual([12000, 35000, 50000]);
  });

  it("getInvocationRecords returns records for a specific issue", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const records = store.getInvocationRecords(3);
    expect(records).toHaveLength(1);
    expect(records[0].tokens).toBe(35000);
  });

  it("getInvocationRecords returns empty array for unknown issue", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    expect(store.getInvocationRecords(999)).toEqual([]);
  });

  it("returns empty store when gh fails", async () => {
    mockExecFile.mockImplementation(
      (_cmd: string, _args: string[], _opts: unknown, callback: Function) => {
        callback(new Error("gh not found"), "", "");
      }
    );
    const store = new IssuesStore("/test/root");
    await store.refresh();
    expect(store.listIssues()).toEqual([]);
  });

  it("keeps existing cache when gh fails after initial load", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    expect(store.listIssues()).toHaveLength(5);

    mockExecFile.mockImplementation(
      (_cmd: string, _args: string[], _opts: unknown, callback: Function) => {
        callback(new Error("network error"), "", "");
      }
    );
    await store.refresh();
    expect(store.listIssues()).toHaveLength(5);
  });
});
