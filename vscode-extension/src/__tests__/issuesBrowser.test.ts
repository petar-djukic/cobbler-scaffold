// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

import { describe, it, expect } from "vitest";
import * as path from "path";
import { BeadsStore } from "../beadsModel";
import { IssueBrowserProvider, priorityIcon } from "../issuesBrowser";

const FIXTURES = path.resolve(__dirname, "..", "__fixtures__");

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
  function createProvider(): IssueBrowserProvider {
    const store = new BeadsStore(FIXTURES);
    return new IssueBrowserProvider(store);
  }

  // ---- getChildren (root) ----

  describe("getChildren (root)", () => {
    it("returns three status group items", () => {
      const provider = createProvider();
      const root = provider.getChildren();
      expect(root).toHaveLength(3);
      expect(root.every((item) => item.kind === "statusGroup")).toBe(true);
    });

    it("groups appear in order: in_progress, open, closed", () => {
      const provider = createProvider();
      const root = provider.getChildren();
      const statuses = root.map((item) => {
        if (item.kind === "statusGroup") {
          return item.status;
        }
        return "";
      });
      expect(statuses).toEqual(["in_progress", "open", "closed"]);
    });

    it("each group has correct count from fixture data", () => {
      const provider = createProvider();
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
    it("returns issues sorted by priority within group", () => {
      const provider = createProvider();
      const openGroup = {
        kind: "statusGroup" as const,
        status: "open" as const,
        label: "Open",
        count: 2,
      };
      const children = provider.getChildren(openGroup);
      expect(children).toHaveLength(2);
      // P1 (test-001) should come before P2 (test-002)
      expect(children[0].kind).toBe("issue");
      expect(children[1].kind).toBe("issue");
      if (children[0].kind === "issue" && children[1].kind === "issue") {
        expect(children[0].issue.priority).toBeLessThanOrEqual(
          children[1].issue.priority
        );
        expect(children[0].issue.id).toBe("test-001");
        expect(children[1].issue.id).toBe("test-002");
      }
    });

    it("returns empty array for issue items (no expansion)", () => {
      const provider = createProvider();
      // Force ensureBuilt by calling getChildren first
      provider.getChildren();
      const store = new BeadsStore(FIXTURES);
      store.ensureBuilt();
      const issue = store.getIssue("test-001")!;
      const children = provider.getChildren({ kind: "issue", issue });
      expect(children).toEqual([]);
    });
  });

  // ---- getTreeItem ----

  describe("getTreeItem (statusGroup)", () => {
    it("returns TreeItem with label including count", () => {
      const provider = createProvider();
      const group = {
        kind: "statusGroup" as const,
        status: "open" as const,
        label: "Open",
        count: 2,
      };
      const ti = provider.getTreeItem(group);
      expect(ti.label).toBe("Open (2)");
    });

    it("returns Collapsed state when count > 0", () => {
      const provider = createProvider();
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

    it("returns None state when count is 0", () => {
      const provider = createProvider();
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
    it("returns TreeItem with 'id: title' label", () => {
      const provider = createProvider();
      const store = new BeadsStore(FIXTURES);
      store.ensureBuilt();
      const issue = store.getIssue("test-001")!;
      const ti = provider.getTreeItem({ kind: "issue", issue });
      expect(ti.label).toBe("test-001: Fix the bug");
    });

    it("has description with priority, type, and labels", () => {
      const provider = createProvider();
      const store = new BeadsStore(FIXTURES);
      store.ensureBuilt();
      const issue = store.getIssue("test-001")!;
      const ti = provider.getTreeItem({ kind: "issue", issue });
      expect(ti.description).toBe("P1 | bug | code");
    });

    it("has tooltip with full details", () => {
      const provider = createProvider();
      const store = new BeadsStore(FIXTURES);
      store.ensureBuilt();
      const issue = store.getIssue("test-002")!;
      const ti = provider.getTreeItem({ kind: "issue", issue });
      expect(ti.tooltip).toContain("test-002: Add feature");
      expect(ti.tooltip).toContain("Status: open");
      expect(ti.tooltip).toContain("Priority: 2");
      expect(ti.tooltip).toContain("Depends on: test-001");
    });

    it("has priority-based icon", () => {
      const provider = createProvider();
      const store = new BeadsStore(FIXTURES);
      store.ensureBuilt();
      const issue = store.getIssue("test-001")!;
      const ti = provider.getTreeItem({ kind: "issue", issue });
      expect((ti.iconPath as { id: string }).id).toBe("arrow-up");
    });
  });
});
