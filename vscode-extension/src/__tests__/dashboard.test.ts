// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

import { describe, it, expect, vi, beforeEach } from "vitest";
import { InvocationRecord, IssuesStore } from "../issuesModel";
import {
  aggregateMetrics,
  formatDuration,
  renderDashboardHtml,
} from "../dashboard";

// ---- Mock child_process for IssuesStore tests ----

const { mockExecFile } = vi.hoisted(() => ({
  mockExecFile: vi.fn(),
}));
vi.mock("child_process", () => ({
  execFile: mockExecFile,
}));

// ---- Fixture data in gh CLI JSON format ----

const FIXTURE_ISSUES = [
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
    labels: [{ name: "in_progress" }],
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
    labels: [],
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

// ---- formatDuration ----

describe("formatDuration", () => {
  it("formats seconds only", () => {
    expect(formatDuration(45)).toBe("45s");
  });

  it("formats minutes and seconds", () => {
    expect(formatDuration(125)).toBe("2m 5s");
  });

  it("formats exact minutes", () => {
    expect(formatDuration(120)).toBe("2m");
  });

  it("formats hours and minutes", () => {
    expect(formatDuration(3720)).toBe("1h 2m");
  });

  it("formats exact hours", () => {
    expect(formatDuration(3600)).toBe("1h");
  });
});

// ---- aggregateMetrics ----

describe("aggregateMetrics", () => {
  it("returns zeroes for empty records", () => {
    const m = aggregateMetrics([]);
    expect(m.invocationCount).toBe(0);
    expect(m.totalTokens).toBe(0);
    expect(m.totalInputTokens).toBe(0);
    expect(m.totalOutputTokens).toBe(0);
    expect(m.totalDurationS).toBe(0);
    expect(m.totalFiles).toBe(0);
  });

  it("aggregates simple token-only records", () => {
    const records: InvocationRecord[] = [
      { tokens: 5000, issueNumber: 1, createdAt: "" },
      { tokens: 3000, issueNumber: 1, createdAt: "" },
    ];
    const m = aggregateMetrics(records);
    expect(m.invocationCount).toBe(2);
    expect(m.totalTokens).toBe(8000);
    expect(m.totalInputTokens).toBe(0);
    expect(m.totalOutputTokens).toBe(0);
  });

  it("aggregates rich JSON records", () => {
    const records: InvocationRecord[] = [
      {
        tokens: 50000,
        issueNumber: 1,
        createdAt: "",
        caller: "stitch",
        startedAt: "2026-01-01T00:00:00Z",
        durationS: 120,
        inputTokens: 45000,
        outputTokens: 5000,
        costUSD: 0.15,
        locBefore: { production: 500, test: 100 },
        locAfter: { production: 520, test: 110 },
        diff: { files: 3, insertions: 25, deletions: 5 },
      },
      {
        tokens: 30000,
        issueNumber: 2,
        createdAt: "",
        caller: "measure",
        startedAt: "2026-01-02T00:00:00Z",
        durationS: 60,
        inputTokens: 28000,
        outputTokens: 2000,
        costUSD: 0.08,
        diff: { files: 0, insertions: 0, deletions: 0 },
      },
    ];
    const m = aggregateMetrics(records);
    expect(m.totalTokens).toBe(80000);
    expect(m.totalInputTokens).toBe(73000);
    expect(m.totalOutputTokens).toBe(7000);
    expect(m.totalDurationS).toBe(180);
    expect(m.totalCostUSD).toBeCloseTo(0.23);
    expect(m.totalFiles).toBe(3);
    expect(m.totalInsertions).toBe(25);
    expect(m.totalDeletions).toBe(5);
  });
});

// ---- renderDashboardHtml ----

describe("renderDashboardHtml", () => {
  it("renders empty state message when no records", () => {
    const html = renderDashboardHtml(aggregateMetrics([]));
    expect(html).toContain("No invocation records found");
    expect(html).toContain("Metrics Dashboard");
  });

  it("renders summary table with token data", () => {
    const html = renderDashboardHtml(
      aggregateMetrics([{ tokens: 5000, issueNumber: 1, createdAt: "2026-01-01T00:00:00Z" }])
    );
    expect(html).toContain("Summary");
    expect(html).toContain("5,000");
    expect(html).toContain("Per-Invocation Details");
  });

  it("renders rich data with input/output breakdown", () => {
    const records: InvocationRecord[] = [
      {
        tokens: 50000,
        issueNumber: 1,
        createdAt: "",
        caller: "stitch",
        durationS: 180,
        inputTokens: 45000,
        outputTokens: 5000,
        costUSD: 0.15,
        diff: { files: 3, insertions: 25, deletions: 5 },
      },
    ];
    const html = renderDashboardHtml(aggregateMetrics(records));
    expect(html).toContain("Input tokens");
    expect(html).toContain("Output tokens");
    expect(html).toContain("45,000");
    expect(html).toContain("5,000");
    expect(html).toContain("$0.15");
    expect(html).toContain("3m");
    expect(html).toContain("Files changed");
    expect(html).toContain("+25");
    expect(html).toContain("-5");
  });

  it("escapes HTML in issue number field", () => {
    const records: InvocationRecord[] = [
      { tokens: 100, issueNumber: 42, createdAt: "" },
    ];
    const html = renderDashboardHtml(aggregateMetrics(records));
    expect(html).toContain("42");
  });
});

// ---- Integration with IssuesStore ----

describe("dashboard integration with IssuesStore", () => {
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

  it("aggregates records from fixture data including JSON format", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const records = store.listInvocationRecords();
    expect(records).toHaveLength(3);
    const total = records.reduce((sum, r) => sum + r.tokens, 0);
    expect(total).toBe(97000);
  });

  it("parses full JSON InvocationRecord fields from fixture", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const records = store.getInvocationRecords(5);
    expect(records).toHaveLength(1);
    const rec = records[0];
    expect(rec.caller).toBe("stitch");
    expect(rec.startedAt).toBe("2026-01-06T00:10:00Z");
    expect(rec.durationS).toBe(180);
    expect(rec.inputTokens).toBe(45000);
    expect(rec.outputTokens).toBe(5000);
    expect(rec.cacheCreationTokens).toBe(1000);
    expect(rec.cacheReadTokens).toBe(2000);
    expect(rec.costUSD).toBe(0.15);
    expect(rec.locBefore).toEqual({ production: 500, test: 100 });
    expect(rec.locAfter).toEqual({ production: 520, test: 110 });
    expect(rec.diff).toEqual({ files: 3, insertions: 25, deletions: 5 });
  });

  it("renders dashboard HTML from fixture data", async () => {
    setupMockGh(FIXTURE_ISSUES);
    const store = new IssuesStore("/test/root");
    await store.refresh();
    const records = store.listInvocationRecords();
    const metrics = aggregateMetrics(records);
    const html = renderDashboardHtml(metrics);
    expect(html).toContain("Metrics Dashboard");
    expect(html).toContain("3"); // 3 invocations
    expect(html).toContain("stitch");
  });
});
