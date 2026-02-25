// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R1, R3, R5, R7
// uc: rel02.0-uc001-lifecycle-commands
// uc: rel02.0-uc003-branch-comparison

import * as vscode from "vscode";
import * as commands from "./commands";
import { SpecBrowserProvider } from "./specBrowser";
import { SpecGraph } from "./specModel";
import { TraceabilityProvider, viewRequirement } from "./traceability";
import { GenerationBrowserProvider } from "./generationBrowser";
import { BeadsStore } from "./beadsModel";
import { IssueBrowserProvider } from "./issuesBrowser";
import { MetricsDashboard } from "./dashboard";
import {
  ComparisonBrowserProvider,
  GitRefContentProvider,
} from "./comparisonBrowser";

/** Output channel for error and diagnostic logging. */
let outputChannel: vscode.OutputChannel;

/** Wraps a callback so exceptions are logged to outputChannel instead of lost. */
function safeCallback(label: string, fn: () => void): () => void {
  return () => {
    try {
      fn();
    } catch (err) {
      outputChannel.appendLine(`${label}: ${err}`);
    }
  };
}

/**
 * Activates the extension. Registers all commands, watchers, and
 * the output channel. Called by VS Code when configuration.yaml
 * exists in the workspace (see activationEvents in package.json).
 */
export function activate(context: vscode.ExtensionContext): void {
  outputChannel = vscode.window.createOutputChannel("Mage Orchestrator");
  context.subscriptions.push(outputChannel);

  // Register lifecycle commands.
  context.subscriptions.push(
    vscode.commands.registerCommand("mageOrchestrator.start", () =>
      commands.generatorStart(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.run", () =>
      commands.generatorRun(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.resume", () =>
      commands.generatorResume(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.stop", () =>
      commands.generatorStop(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.reset", () =>
      commands.generatorReset(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.switch", () =>
      commands.generatorSwitch(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.cobblerMeasure", () =>
      commands.cobblerMeasure(outputChannel)
    ),
    vscode.commands.registerCommand("mageOrchestrator.cobblerStitch", () =>
      commands.cobblerStitch(outputChannel)
    )
  );

  // FileSystemWatchers for reactive view refresh.
  const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (root) {
    const beadsWatcher = vscode.workspace.createFileSystemWatcher(
      new vscode.RelativePattern(root, ".beads/**")
    );
    const gitRefsWatcher = vscode.workspace.createFileSystemWatcher(
      new vscode.RelativePattern(root, ".git/refs/**")
    );
    const configWatcher = vscode.workspace.createFileSystemWatcher(
      new vscode.RelativePattern(root, "configuration.yaml")
    );

    context.subscriptions.push(beadsWatcher, gitRefsWatcher, configWatcher);

    // Log watcher events to the output channel.
    context.subscriptions.push(
      beadsWatcher.onDidChange(() =>
        outputChannel.appendLine("beads data changed")
      ),
      gitRefsWatcher.onDidChange(() =>
        outputChannel.appendLine("git refs changed")
      ),
      configWatcher.onDidChange(() =>
        outputChannel.appendLine("configuration.yaml changed")
      )
    );

    // Generation Browser tree view (prd006 R2).
    const genBrowser = new GenerationBrowserProvider(root);
    context.subscriptions.push(
      vscode.window.registerTreeDataProvider(
        "mageOrchestrator.status",
        genBrowser
      )
    );
    context.subscriptions.push(
      gitRefsWatcher.onDidChange(safeCallback("genBrowser.refresh", () => genBrowser.refresh())),
      gitRefsWatcher.onDidCreate(safeCallback("genBrowser.refresh", () => genBrowser.refresh())),
      gitRefsWatcher.onDidDelete(safeCallback("genBrowser.refresh", () => genBrowser.refresh()))
    );

    // Branch and Tag Comparison view (prd006 R3).
    const comparisonBrowser = new ComparisonBrowserProvider(root);
    context.subscriptions.push(
      vscode.window.registerTreeDataProvider(
        "mageOrchestrator.comparison",
        comparisonBrowser
      )
    );
    context.subscriptions.push(
      vscode.workspace.registerTextDocumentContentProvider(
        "mage-git-ref",
        new GitRefContentProvider(root)
      )
    );
    context.subscriptions.push(
      vscode.commands.registerCommand("mageOrchestrator.compareTags", () =>
        commands.compareTags(outputChannel, comparisonBrowser)
      ),
      vscode.commands.registerCommand(
        "mageOrchestrator.compareGenerations",
        (genA: string, genB: string) =>
          commands.compareGenerations(
            outputChannel,
            comparisonBrowser,
            genA,
            genB
          )
      ),
      vscode.commands.registerCommand(
        "mageOrchestrator.openComparisonDiff",
        (node) => commands.openComparisonDiff(node)
      ),
      vscode.commands.registerCommand(
        "mageOrchestrator.selectForCompare",
        async (item: { generation: { name: string } }) => {
          // Store first generation; on second selection, trigger comparison.
          const name = item.generation.name;
          const stored =
            context.workspaceState.get<string>("compareGenerationA");
          if (stored && stored !== name) {
            await commands.compareGenerations(
              outputChannel,
              comparisonBrowser,
              stored,
              name
            );
            await context.workspaceState.update(
              "compareGenerationA",
              undefined
            );
          } else {
            await context.workspaceState.update("compareGenerationA", name);
            vscode.window.showInformationMessage(
              `Selected ${name} for comparison. Select a second generation to compare.`
            );
          }
        }
      )
    );

    // Specification Browser tree view (prd006 R8).
    const specBrowser = new SpecBrowserProvider(root);
    context.subscriptions.push(
      vscode.window.registerTreeDataProvider(
        "mageOrchestrator.specs",
        specBrowser
      )
    );

    const specsWatcher = vscode.workspace.createFileSystemWatcher(
      new vscode.RelativePattern(root, "docs/specs/**")
    );
    context.subscriptions.push(
      specsWatcher,
      specsWatcher.onDidChange(safeCallback("specBrowser.refresh", () => specBrowser.refresh())),
      specsWatcher.onDidCreate(safeCallback("specBrowser.refresh", () => specBrowser.refresh())),
      specsWatcher.onDidDelete(safeCallback("specBrowser.refresh", () => specBrowser.refresh()))
    );

    // Issue tracker tree view (prd006 R4).
    const beadsStore = new BeadsStore(root);
    const issueBrowser = new IssueBrowserProvider(beadsStore);
    context.subscriptions.push(
      vscode.window.registerTreeDataProvider(
        "mageOrchestrator.issues",
        issueBrowser
      )
    );
    context.subscriptions.push(
      beadsWatcher.onDidChange(safeCallback("issueBrowser.refresh", () => issueBrowser.refresh())),
      beadsWatcher.onDidCreate(safeCallback("issueBrowser.refresh", () => issueBrowser.refresh())),
      beadsWatcher.onDidDelete(safeCallback("issueBrowser.refresh", () => issueBrowser.refresh()))
    );

    // Metrics dashboard webview (prd006 R5).
    const dashboard = new MetricsDashboard(beadsStore);
    context.subscriptions.push(
      vscode.commands.registerCommand("mageOrchestrator.showDashboard", () =>
        dashboard.show()
      )
    );
    context.subscriptions.push(
      beadsWatcher.onDidChange(safeCallback("dashboard.refresh", () => dashboard.refresh())),
      beadsWatcher.onDidCreate(safeCallback("dashboard.refresh", () => dashboard.refresh())),
      beadsWatcher.onDidDelete(safeCallback("dashboard.refresh", () => dashboard.refresh()))
    );

    // Code-to-spec traceability CodeLens (prd006 R9).
    const traceGraph = new SpecGraph(root);
    context.subscriptions.push(
      vscode.languages.registerCodeLensProvider(
        { language: "go", scheme: "file" },
        new TraceabilityProvider()
      )
    );
    context.subscriptions.push(
      vscode.commands.registerCommand(
        "mageOrchestrator.viewRequirement",
        (id: string) => viewRequirement(traceGraph, id)
      )
    );
    context.subscriptions.push(
      specsWatcher.onDidChange(safeCallback("traceGraph.invalidate", () => traceGraph.invalidate())),
      specsWatcher.onDidCreate(safeCallback("traceGraph.invalidate", () => traceGraph.invalidate())),
      specsWatcher.onDidDelete(safeCallback("traceGraph.invalidate", () => traceGraph.invalidate()))
    );
  } else {
    // Fallback when no workspace root is available.
    context.subscriptions.push(
      vscode.commands.registerCommand("mageOrchestrator.showDashboard", () => {
        vscode.window.showWarningMessage(
          "Metrics dashboard requires an open workspace."
        );
      })
    );
  }

  outputChannel.appendLine("Mage Orchestrator extension activated");
}

/**
 * Deactivates the extension. All watchers, terminals, and subscriptions
 * pushed to context.subscriptions are disposed automatically by VS Code.
 */
export function deactivate(): void {
  // VS Code disposes all items in context.subscriptions automatically.
}
