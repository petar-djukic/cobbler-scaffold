// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

import { describe, it, expect, vi, beforeEach } from "vitest";
import { IssuesStore } from "../issuesModel";
import { IssueBrowserProvider, priorityIcon } from "../issuesBrowser";

// ---- Mock child_process for IssuesStore ----

const { mockExecFile } = vi.hoisted(() => ({
  mockExecFile: vi.fn(),
}));
vi.mock("child_process", () => ({
  execFile: mockExecFile,
}));

// ---- Fixture data in gh CLI JSON format ----

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
    comments: [],
    createdAt: "2026-01-05T00:00:00Z",
    updatedAt: "2026-01-05T00:00:00Z",
    closedAt: null,
  },
  {
    number: 5,
    title: "Stitch task",
    state: "CLOSED",
    labels: [{ name: "priority:3" }],
    body: "A stitch task",
    comments: [],
    createdAt: "2026-01-06T00:00:00Z",
    updatedAt: "2026-01-06T01:00:00Z",
    closedAt: "2026-01-06T01:00:00Z",
  },
];

function setupMockGh(): void {
  mockExecFile.mockImplementation(
    (_cmd: string, _args: string[], _opts: unknown, callback: Function) => {
      callback(null, JSON.stringify(FIXTURE_ISSUES), "");
    }
  );
}

async function createProvider(): Promise<IssueBrowserProvider> {
  setupMockGh();
  const store = new IssuesStore("/test/root");
  await store.refresh();
  return new IssueBrowserProvider(store);
}

// ---- priorityIcon ----

describe("priorityIcon", () => {
  it("returns 'arrow-up' for priority 1", () => {
    expect(priorityIcon(1)).toBe("arrow-up");
  });

  it("returns 'dash' for priority 2", () => {
    expect(priorityIcon(2)).toBe("dash");
  });

  it("returns 'arrow-down' for priority 3", () => {
    expect(priorityIcon(3)).toBe("arrow-down");
  });

  it("returns 'dash' for unknown priority", () => {
    expect(priorityIcon(99)).toBe("dash");
  });
});

// ---- IssueBrowserProvider ----

describe("IssueBrowserProvider", () => {
  beforeEach(() => {
    mockExecFile.mockReset();
  });

  // ---- getChildren (root) ----

  describe("getChildren (root)", () => {
    it("returns three status group items", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      expect(root).toHaveLength(3);
      expect(root.every((item) => item.kind === "statusGroup")).toBe(true);
    });

    it("groups appear in order: in_progress, open, closed", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const statuses = root.map((item) => {
        if (item.kind === "statusGroup") {
          return item.status;
        }
        return "";
      });
      expect(statuses).toEqual(["in_progress", "open", "closed"]);
    });

    it("each group has correct count from fixture data", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const counts = root.map((item) => {
        if (item.kind === "statusGroup") {
          return { status: item.status, count: item.count };
        }
        return { status: "", count: 0 };
      });
      expect(counts).toEqual([
        { status: "in_progress", count: 1 },
        { status: "open", count: 2 },
        { status: "closed", count: 2 },
      ]);
    });
  });

  // ---- getChildren (status group) ----

  describe("getChildren (status group)", () => {
    it("returns issues sorted by priority within group", async () => {
      const provider = await createProvider();
      const openGroup = {
        kind: "statusGroup" as const,
        status: "open" as const,
        label: "Open",
        count: 2,
      };
      const children = provider.getChildren(openGroup);
      expect(children).toHaveLength(2);
      expect(children[0].kind).toBe("issue");
      expect(children[1].kind).toBe("issue");
      if (children[0].kind === "issue" && children[1].kind === "issue") {
        expect(children[0].issue.priority).toBeLessThanOrEqual(
          children[1].issue.priority
        );
        expect(children[0].issue.number).toBe(1);
        expect(children[1].issue.number).toBe(2);
      }
    });

    it("returns empty array for issue items (no expansion)", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const openGroup = root.find(
        (item) => item.kind === "statusGroup" && item.status === "open"
      )!;
      const children = provider.getChildren(openGroup);
      if (children[0].kind === "issue") {
        const grandchildren = provider.getChildren(children[0]);
        expect(grandchildren).toEqual([]);
      }
    });
  });

  // ---- getTreeItem ----

  describe("getTreeItem (statusGroup)", () => {
    it("returns TreeItem with label including count", async () => {
      const provider = await createProvider();
      const group = {
        kind: "statusGroup" as const,
        status: "open" as const,
        label: "Open",
        count: 2,
      };
      const ti = provider.getTreeItem(group);
      expect(ti.label).toBe("Open (2)");
    });

    it("returns Collapsed state when count > 0", async () => {
      const provider = await createProvider();
      const group = {
        kind: "statusGroup" as const,
        status: "open" as const,
        label: "Open",
        count: 2,
      };
      const ti = provider.getTreeItem(group);
      // TreeItemCollapsibleState.Collapsed = 1
      expect(ti.collapsibleState).toBe(1);
    });

    it("returns None state when count is 0", async () => {
      const provider = await createProvider();
      const group = {
        kind: "statusGroup" as const,
        status: "open" as const,
        label: "Open",
        count: 0,
      };
      const ti = provider.getTreeItem(group);
      // TreeItemCollapsibleState.None = 0
      expect(ti.collapsibleState).toBe(0);
    });
  });

  describe("getTreeItem (issue)", () => {
    it("returns TreeItem with '#number: title' label", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const openGroup = root.find(
        (item) => item.kind === "statusGroup" && item.status === "open"
      )!;
      const children = provider.getChildren(openGroup);
      const ti = provider.getTreeItem(children[0]);
      expect(ti.label).toBe("#1: Fix the bug");
    });

    it("has description with priority, type, and labels", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const openGroup = root.find(
        (item) => item.kind === "statusGroup" && item.status === "open"
      )!;
      const children = provider.getChildren(openGroup);
      const ti = provider.getTreeItem(children[0]);
      expect(ti.description).toBe("P1 | bug | bug, priority:1, code");
    });

    it("has tooltip with full details", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const openGroup = root.find(
        (item) => item.kind === "statusGroup" && item.status === "open"
      )!;
      const children = provider.getChildren(openGroup);
      const ti = provider.getTreeItem(children[1]);
      expect(ti.tooltip).toContain("#2: Add feature");
      expect(ti.tooltip).toContain("Status: open");
      expect(ti.tooltip).toContain("Priority: 2");
    });

    it("has contextValue 'githubIssue'", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const openGroup = root.find(
        (item) => item.kind === "statusGroup" && item.status === "open"
      )!;
      const children = provider.getChildren(openGroup);
      const ti = provider.getTreeItem(children[0]);
      expect(ti.contextValue).toBe("githubIssue");
    });

    it("has priority-based icon", async () => {
      const provider = await createProvider();
      const root = provider.getChildren();
      const openGroup = root.find(
        (item) => item.kind === "statusGroup" && item.status === "open"
      )!;
      const children = provider.getChildren(openGroup);
      const ti = provider.getTreeItem(children[0]);
      expect((ti.iconPath as { id: string }).id).toBe("arrow-up");
    });
  });
});
