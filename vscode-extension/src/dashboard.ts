// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R5
// prd: prd005-metrics-collection R1

import * as vscode from "vscode";
import { BeadsStore, InvocationRecord } from "./beadsModel";

/** Aggregated metrics computed from all InvocationRecords. */
export interface AggregateMetrics {
  totalTokens: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCacheCreationTokens: number;
  totalCacheReadTokens: number;
  totalCostUSD: number;
  totalDurationS: number;
  totalFiles: number;
  totalInsertions: number;
  totalDeletions: number;
  invocationCount: number;
  records: InvocationRecord[];
}

/** Computes aggregate metrics from a list of InvocationRecords. */
export function aggregateMetrics(records: InvocationRecord[]): AggregateMetrics {
  const result: AggregateMetrics = {
    totalTokens: 0,
    totalInputTokens: 0,
    totalOutputTokens: 0,
    totalCacheCreationTokens: 0,
    totalCacheReadTokens: 0,
    totalCostUSD: 0,
    totalDurationS: 0,
    totalFiles: 0,
    totalInsertions: 0,
    totalDeletions: 0,
    invocationCount: records.length,
    records,
  };

  for (const rec of records) {
    result.totalTokens += rec.tokens;
    result.totalInputTokens += rec.inputTokens ?? 0;
    result.totalOutputTokens += rec.outputTokens ?? 0;
    result.totalCacheCreationTokens += rec.cacheCreationTokens ?? 0;
    result.totalCacheReadTokens += rec.cacheReadTokens ?? 0;
    result.totalCostUSD += rec.costUSD ?? 0;
    result.totalDurationS += rec.durationS ?? 0;
    if (rec.diff) {
      result.totalFiles += rec.diff.files;
      result.totalInsertions += rec.diff.insertions;
      result.totalDeletions += rec.diff.deletions;
    }
  }

  return result;
}

/** Formats a duration in seconds to a human-readable string. */
export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) {
    return s > 0 ? `${m}m ${s}s` : `${m}m`;
  }
  const h = Math.floor(m / 60);
  const rm = m % 60;
  return rm > 0 ? `${h}h ${rm}m` : `${h}h`;
}

/** Formats a number with thousands separators. */
function fmtNum(n: number): string {
  return n.toLocaleString("en-US");
}

/** Generates the HTML content for the metrics dashboard. */
export function renderDashboardHtml(metrics: AggregateMetrics): string {
  const hasRichData = metrics.records.some((r) => r.caller !== undefined);

  let tokenSummaryRows: string;
  if (hasRichData) {
    tokenSummaryRows = `
      <tr><td>Input tokens</td><td>${fmtNum(metrics.totalInputTokens)}</td></tr>
      <tr><td>Output tokens</td><td>${fmtNum(metrics.totalOutputTokens)}</td></tr>
      <tr><td>Cache creation tokens</td><td>${fmtNum(metrics.totalCacheCreationTokens)}</td></tr>
      <tr><td>Cache read tokens</td><td>${fmtNum(metrics.totalCacheReadTokens)}</td></tr>
      <tr><td>Total tokens</td><td>${fmtNum(metrics.totalTokens)}</td></tr>
      <tr><td>Estimated cost</td><td>$${metrics.totalCostUSD.toFixed(2)}</td></tr>`;
  } else {
    tokenSummaryRows = `
      <tr><td>Total tokens</td><td>${fmtNum(metrics.totalTokens)}</td></tr>`;
  }

  let durationRow = "";
  if (metrics.totalDurationS > 0) {
    durationRow = `<tr><td>Total duration</td><td>${formatDuration(metrics.totalDurationS)}</td></tr>`;
  }

  let diffRows = "";
  if (metrics.totalFiles > 0 || metrics.totalInsertions > 0 || metrics.totalDeletions > 0) {
    diffRows = `
      <tr><td>Files changed</td><td>${fmtNum(metrics.totalFiles)}</td></tr>
      <tr><td>Insertions</td><td>+${fmtNum(metrics.totalInsertions)}</td></tr>
      <tr><td>Deletions</td><td>-${fmtNum(metrics.totalDeletions)}</td></tr>`;
  }

  const invocationRows = metrics.records
    .map((rec) => {
      const caller = rec.caller ?? "—";
      const date = rec.startedAt
        ? new Date(rec.startedAt).toLocaleString()
        : rec.comment.created_at
          ? new Date(rec.comment.created_at).toLocaleString()
          : "—";
      const duration =
        rec.durationS !== undefined ? formatDuration(rec.durationS) : "—";
      const input =
        rec.inputTokens !== undefined ? fmtNum(rec.inputTokens) : "—";
      const output =
        rec.outputTokens !== undefined ? fmtNum(rec.outputTokens) : "—";
      const total = fmtNum(rec.tokens);
      const locProd =
        rec.locBefore && rec.locAfter
          ? `${fmtNum(rec.locBefore.production)} → ${fmtNum(rec.locAfter.production)}`
          : "—";
      const locTest =
        rec.locBefore && rec.locAfter
          ? `${fmtNum(rec.locBefore.test)} → ${fmtNum(rec.locAfter.test)}`
          : "—";
      const diff = rec.diff
        ? `${rec.diff.files}F +${rec.diff.insertions} -${rec.diff.deletions}`
        : "—";
      const issue = rec.comment.issue_id;

      return `<tr>
        <td>${escapeHtml(caller)}</td>
        <td>${escapeHtml(date)}</td>
        <td>${duration}</td>
        <td>${input}</td>
        <td>${output}</td>
        <td>${total}</td>
        <td>${locProd}</td>
        <td>${locTest}</td>
        <td>${diff}</td>
        <td>${escapeHtml(issue)}</td>
      </tr>`;
    })
    .join("\n");

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    body {
      font-family: var(--vscode-font-family);
      color: var(--vscode-foreground);
      background: var(--vscode-editor-background);
      padding: 16px;
      font-size: var(--vscode-font-size);
    }
    h1 { font-size: 1.4em; margin-bottom: 16px; }
    h2 { font-size: 1.1em; margin-top: 24px; margin-bottom: 8px; }
    table {
      border-collapse: collapse;
      width: 100%;
      margin-bottom: 16px;
    }
    th, td {
      text-align: left;
      padding: 4px 12px;
      border-bottom: 1px solid var(--vscode-widget-border, #333);
    }
    th {
      background: var(--vscode-editor-inactiveSelectionBackground, #2a2d2e);
      font-weight: 600;
    }
    tr:hover td {
      background: var(--vscode-list-hoverBackground, #2a2d2e);
    }
    .summary-table { max-width: 400px; }
    .empty { color: var(--vscode-descriptionForeground); font-style: italic; }
  </style>
</head>
<body>
  <h1>Metrics Dashboard</h1>

  ${
    metrics.invocationCount === 0
      ? '<p class="empty">No invocation records found. Metrics appear after generation cycles record token usage.</p>'
      : `
  <h2>Summary</h2>
  <table class="summary-table">
    <tr><td>Invocations</td><td>${metrics.invocationCount}</td></tr>
    ${tokenSummaryRows}
    ${durationRow}
    ${diffRows}
  </table>

  <h2>Per-Invocation Details</h2>
  <table>
    <thead>
      <tr>
        <th>Caller</th>
        <th>Date</th>
        <th>Duration</th>
        <th>Input</th>
        <th>Output</th>
        <th>Total</th>
        <th>LOC (prod)</th>
        <th>LOC (test)</th>
        <th>Diff</th>
        <th>Issue</th>
      </tr>
    </thead>
    <tbody>
      ${invocationRows}
    </tbody>
  </table>`
  }
</body>
</html>`;
}

/** Escapes HTML special characters. */
function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/**
 * Manages the metrics dashboard webview panel. Only one panel exists
 * at a time; calling show() when the panel is already visible brings
 * it to the foreground and refreshes its content.
 */
export class MetricsDashboard {
  private panel: vscode.WebviewPanel | undefined;
  private store: BeadsStore;

  constructor(store: BeadsStore) {
    this.store = store;
  }

  /** Shows (or reveals) the dashboard panel and refreshes its content. */
  show(): void {
    if (this.panel) {
      this.panel.reveal(vscode.ViewColumn.One);
      this.refresh();
      return;
    }

    this.panel = vscode.window.createWebviewPanel(
      "mageOrchestrator.dashboard",
      "Metrics Dashboard",
      vscode.ViewColumn.One,
      { enableScripts: false }
    );

    this.panel.onDidDispose(() => {
      this.panel = undefined;
    });

    this.refresh();
  }

  /** Refreshes the dashboard content from the current BeadsStore data. */
  refresh(): void {
    if (!this.panel) {
      return;
    }
    this.store.invalidate();
    this.store.ensureBuilt();
    const records = this.store.listInvocationRecords();
    const metrics = aggregateMetrics(records);
    this.panel.webview.html = renderDashboardHtml(metrics);
  }

  /** Disposes the panel if it exists. */
  dispose(): void {
    this.panel?.dispose();
  }
}
