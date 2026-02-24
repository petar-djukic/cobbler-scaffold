// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

import { describe, it, expect } from "vitest";
import * as path from "path";
import { BeadsStore, InvocationRecord, BeadsComment } from "../beadsModel";
import {
  aggregateMetrics,
  formatDuration,
  renderDashboardHtml,
} from "../dashboard";

const FIXTURES = path.resolve(__dirname, "..", "__fixtures__");

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
    const comment: BeadsComment = {
      id: 1,
      issue_id: "t-1",
      author: "x",
      text: "tokens: 5000",
      created_at: "",
    };
    const records: InvocationRecord[] = [
      { tokens: 5000, comment },
      { tokens: 3000, comment },
    ];
    const m = aggregateMetrics(records);
    expect(m.invocationCount).toBe(2);
    expect(m.totalTokens).toBe(8000);
    expect(m.totalInputTokens).toBe(0);
    expect(m.totalOutputTokens).toBe(0);
  });

  it("aggregates rich JSON records", () => {
    const comment: BeadsComment = {
      id: 1,
      issue_id: "t-1",
      author: "x",
      text: "{}",
      created_at: "",
    };
    const records: InvocationRecord[] = [
      {
        tokens: 50000,
        comment,
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
        comment,
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
    const comment: BeadsComment = {
      id: 1,
      issue_id: "t-1",
      author: "x",
      text: "tokens: 5000",
      created_at: "2026-01-01T00:00:00Z",
    };
    const html = renderDashboardHtml(
      aggregateMetrics([{ tokens: 5000, comment }])
    );
    expect(html).toContain("Summary");
    expect(html).toContain("5,000");
    expect(html).toContain("Per-Invocation Details");
  });

  it("renders rich data with input/output breakdown", () => {
    const comment: BeadsComment = {
      id: 1,
      issue_id: "t-1",
      author: "x",
      text: "{}",
      created_at: "",
    };
    const records: InvocationRecord[] = [
      {
        tokens: 50000,
        comment,
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

  it("escapes HTML in caller and issue fields", () => {
    const comment: BeadsComment = {
      id: 1,
      issue_id: "<script>alert(1)</script>",
      author: "x",
      text: "tokens: 100",
      created_at: "",
    };
    const html = renderDashboardHtml(
      aggregateMetrics([{ tokens: 100, comment }])
    );
    expect(html).not.toContain("<script>");
    expect(html).toContain("&lt;script&gt;");
  });
});

// ---- Integration with BeadsStore ----

describe("dashboard integration with BeadsStore", () => {
  it("aggregates records from fixture data including JSON format", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const records = store.listInvocationRecords();
    // Fixture has: test-003 with "tokens: 35000", test-004 with "tokens: 12000",
    // test-005 with full JSON record (45000+5000=50000 tokens).
    expect(records.length).toBe(3);
    const total = records.reduce((sum, r) => sum + r.tokens, 0);
    expect(total).toBe(97000);
  });

  it("parses full JSON InvocationRecord fields from fixture", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const records = store.getInvocationRecords("test-005");
    expect(records.length).toBe(1);
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

  it("renders dashboard HTML from fixture data", () => {
    const store = new BeadsStore(FIXTURES);
    store.ensureBuilt();
    const records = store.listInvocationRecords();
    const metrics = aggregateMetrics(records);
    const html = renderDashboardHtml(metrics);
    expect(html).toContain("Metrics Dashboard");
    expect(html).toContain("3"); // 3 invocations
    expect(html).toContain("stitch");
  });
});
