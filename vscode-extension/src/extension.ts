// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R1, R7
// uc: rel02.0-uc001-lifecycle-commands

import * as vscode from "vscode";
import * as commands from "./commands";

/** Output channel for error and diagnostic logging. */
let outputChannel: vscode.OutputChannel;

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

  // Register placeholder for the dashboard command (existing stub).
  context.subscriptions.push(
    vscode.commands.registerCommand("mageOrchestrator.showDashboard", () => {
      vscode.window.showInformationMessage(
        "Mage Orchestrator dashboard — not yet implemented."
      );
    })
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

    // Log watcher events to the output channel. Tree providers will
    // subscribe to these watchers in their own modules.
    beadsWatcher.onDidChange(() =>
      outputChannel.appendLine("beads data changed")
    );
    gitRefsWatcher.onDidChange(() =>
      outputChannel.appendLine("git refs changed")
    );
    configWatcher.onDidChange(() =>
      outputChannel.appendLine("configuration.yaml changed")
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
