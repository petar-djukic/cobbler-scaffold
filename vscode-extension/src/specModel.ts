// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// prd: prd006-vscode-extension R8
// uc: rel02.0-uc006-specification-browser

import * as fs from "fs";
import * as path from "path";
import { execFileSync } from "child_process";
import * as yaml from "js-yaml";

// ---- Exported types ----

/** A use case parsed from docs/specs/use-cases/*.yaml. */
export interface UseCase {
  id: string;
  title: string;
  summary: string;
  touchpoints: Touchpoint[];
  filePath: string;
}

/** A touchpoint linking a use case to PRD requirements. */
export interface Touchpoint {
  /** Touchpoint key, e.g. "T1". */
  key: string;
  /** Full description text. */
  description: string;
  /** PRD id referenced, if parseable. */
  prdId: string | undefined;
  /** Specific requirement IDs referenced (e.g. ["R1.1", "R1.2"]). */
  requirementIds: string[];
}

/** A product requirements document parsed from docs/specs/product-requirements/*.yaml. */
export interface Prd {
  id: string;
  title: string;
  requirements: Record<string, PrdRequirement>;
  filePath: string;
}

/** A single requirement group within a PRD. */
export interface PrdRequirement {
  title: string;
  items: string[];
}

/** A test suite parsed from docs/specs/test-suites/*.yaml. */
export interface TestSuite {
  id: string;
  title: string;
  release: string;
  traces: string[];
  filePath: string;
}

/** A source file reference where a PRD ID appears. */
export interface SourceRef {
  file: string;
  line: number;
}

// ---- SpecGraph ----

/**
 * In-memory graph of specification artifacts. Parses use cases, PRDs, and
 * test suites from docs/specs/ and indexes them for navigation. Caches the
 * result and invalidates when the caller signals a file change.
 */
export class SpecGraph {
  private useCases = new Map<string, UseCase>();
  private prds = new Map<string, Prd>();
  private testSuites = new Map<string, TestSuite>();
  private sourceRefCache = new Map<string, SourceRef[]>();
  private built = false;
  private root: string;

  constructor(workspaceRoot: string) {
    this.root = workspaceRoot;
  }

  /** Builds the graph if not already built. Idempotent until invalidate(). */
  async ensureBuilt(): Promise<void> {
    if (this.built) {
      return;
    }
    this.parseUseCases();
    this.parsePrds();
    this.parseTestSuites();
    this.built = true;
  }

  /** Clears all cached data. The next ensureBuilt() call will re-parse. */
  invalidate(): void {
    this.useCases.clear();
    this.prds.clear();
    this.testSuites.clear();
    this.sourceRefCache.clear();
    this.built = false;
  }

  getUseCase(id: string): UseCase | undefined {
    return this.useCases.get(id);
  }

  getPrd(id: string): Prd | undefined {
    return this.prds.get(id);
  }

  listUseCases(): UseCase[] {
    return Array.from(this.useCases.values());
  }

  listPrds(): Prd[] {
    return Array.from(this.prds.values());
  }

  listTestSuites(): TestSuite[] {
    return Array.from(this.testSuites.values());
  }

  /**
   * Returns source files under pkg/ and magefiles/ that reference the given
   * PRD ID string. Results are cached per prdId until invalidate().
   */
  getSourceFiles(prdId: string): SourceRef[] {
    const cached = this.sourceRefCache.get(prdId);
    if (cached !== undefined) {
      return cached;
    }
    const refs = grepForPrdId(this.root, prdId);
    this.sourceRefCache.set(prdId, refs);
    return refs;
  }

  // ---- Internal parsing ----

  private parseUseCases(): void {
    const dir = path.join(this.root, "docs", "specs", "use-cases");
    for (const file of listYamlFiles(dir)) {
      const filePath = path.join(dir, file);
      const doc = loadYaml(filePath);
      if (!doc || typeof doc !== "object" || !("id" in doc)) {
        continue;
      }
      const raw = doc as Record<string, unknown>;
      const uc: UseCase = {
        id: String(raw.id ?? ""),
        title: String(raw.title ?? ""),
        summary: String(raw.summary ?? ""),
        touchpoints: parseTouchpoints(raw.touchpoints),
        filePath,
      };
      if (uc.id) {
        this.useCases.set(uc.id, uc);
      }
    }
  }

  private parsePrds(): void {
    const dir = path.join(this.root, "docs", "specs", "product-requirements");
    for (const file of listYamlFiles(dir)) {
      const filePath = path.join(dir, file);
      const doc = loadYaml(filePath);
      if (!doc || typeof doc !== "object" || !("id" in doc)) {
        continue;
      }
      const raw = doc as Record<string, unknown>;
      const prd: Prd = {
        id: String(raw.id ?? ""),
        title: String(raw.title ?? ""),
        requirements: parseRequirements(raw.requirements),
        filePath,
      };
      if (prd.id) {
        this.prds.set(prd.id, prd);
      }
    }
  }

  private parseTestSuites(): void {
    const dir = path.join(this.root, "docs", "specs", "test-suites");
    for (const file of listYamlFiles(dir)) {
      const filePath = path.join(dir, file);
      const doc = loadYaml(filePath);
      if (!doc || typeof doc !== "object" || !("id" in doc)) {
        continue;
      }
      const raw = doc as Record<string, unknown>;
      const ts: TestSuite = {
        id: String(raw.id ?? ""),
        title: String(raw.title ?? ""),
        release: String(raw.release ?? ""),
        traces: Array.isArray(raw.traces)
          ? raw.traces.map((t: unknown) => String(t))
          : [],
        filePath,
      };
      if (ts.id) {
        this.testSuites.set(ts.id, ts);
      }
    }
  }
}

// ---- Helpers ----

/** Lists .yaml files in a directory. Returns empty array if the directory does not exist. */
export function listYamlFiles(dir: string): string[] {
  try {
    return fs.readdirSync(dir).filter((f) => f.endsWith(".yaml"));
  } catch (err) {
    console.warn(`listYamlFiles: failed to read ${dir}: ${err}`);
    return [];
  }
}

/** Loads and parses a YAML file. Returns undefined on error. */
export function loadYaml(filePath: string): unknown {
  try {
    const content = fs.readFileSync(filePath, "utf-8");
    return yaml.load(content);
  } catch (err) {
    console.warn(`loadYaml: failed to parse ${filePath}: ${err}`);
    return undefined;
  }
}

/**
 * Parses the touchpoints field from a use case YAML.
 *
 * Touchpoints are YAML list items with single-key maps:
 *   - T1: "Description: prd006-vscode-extension R8.1, R8.2"
 *
 * After YAML parsing each item is an object like { T1: "..." }.
 */
export function parseTouchpoints(raw: unknown): Touchpoint[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  const result: Touchpoint[] = [];
  for (const item of raw) {
    if (typeof item === "string") {
      // Format: "T1: description text"
      const tp = parseTouchpointString(item);
      if (tp) {
        result.push(tp);
      }
    } else if (typeof item === "object" && item !== null) {
      // Format: { T1: "description text" } (single-key map)
      const entries = Object.entries(item as Record<string, unknown>);
      for (const [key, value] of entries) {
        const tp = parseTouchpointFromKV(key, String(value ?? ""));
        result.push(tp);
      }
    }
  }
  return result;
}

/** Parses a touchpoint from a plain string like "T1: description: prdId R1.1". */
export function parseTouchpointString(s: string): Touchpoint | undefined {
  const colonIdx = s.indexOf(":");
  if (colonIdx < 0) {
    return undefined;
  }
  const key = s.slice(0, colonIdx).trim();
  const value = s.slice(colonIdx + 1).trim();
  return parseTouchpointFromKV(key, value);
}

/** Parses a touchpoint from a key-value pair. */
export function parseTouchpointFromKV(key: string, value: string): Touchpoint {
  // Strip surrounding quotes if present.
  let desc = value;
  if (desc.startsWith('"') && desc.endsWith('"')) {
    desc = desc.slice(1, -1);
  }

  // Try to extract PRD reference: look for prdNNN-... pattern.
  const prdMatch = desc.match(/(prd\d{3}-[\w-]+)/);
  const prdId = prdMatch ? prdMatch[1] : undefined;

  // Extract requirement IDs: R followed by digits and optional dot-digits.
  const reqMatches = desc.match(/R\d+(?:\.\d+)?/g) ?? [];

  return {
    key,
    description: desc,
    prdId,
    requirementIds: reqMatches,
  };
}

/**
 * Parses the requirements field from a PRD YAML.
 *
 * Requirements are a map of requirement groups:
 *   R1:
 *     title: Config Struct
 *     items:
 *       - R1.1: Description
 */
export function parseRequirements(
  raw: unknown
): Record<string, PrdRequirement> {
  const result: Record<string, PrdRequirement> = {};
  if (!raw || typeof raw !== "object") {
    return result;
  }
  const entries = Object.entries(raw as Record<string, unknown>);
  for (const [key, value] of entries) {
    if (!value || typeof value !== "object") {
      continue;
    }
    const group = value as Record<string, unknown>;
    result[key] = {
      title: String(group.title ?? ""),
      items: Array.isArray(group.items)
        ? group.items.map((i: unknown) => String(i))
        : [],
    };
  }
  return result;
}

/**
 * Searches pkg/ and magefiles/ for lines containing the PRD ID string.
 * Uses grep via child_process for simplicity and performance.
 */
export function grepForPrdId(root: string, prdId: string): SourceRef[] {
  const refs: SourceRef[] = [];
  const dirs = ["pkg", "magefiles"]
    .map((d) => path.join(root, d))
    .filter((d) => {
      try {
        return fs.statSync(d).isDirectory();
      } catch {
        return false;
      }
    });

  if (dirs.length === 0) {
    return refs;
  }

  try {
    // grep -rn returns lines like "file:line:content"
    const output = execFileSync(
      "grep", ["-rn", "--include=*.go", prdId, ...dirs],
      { encoding: "utf-8", cwd: root }
    );
    for (const line of output.split("\n")) {
      if (!line) {
        continue;
      }
      // Format: filepath:linenum:content
      const firstColon = line.indexOf(":");
      if (firstColon < 0) {
        continue;
      }
      const secondColon = line.indexOf(":", firstColon + 1);
      if (secondColon < 0) {
        continue;
      }
      const file = line.slice(0, firstColon);
      const lineNum = parseInt(line.slice(firstColon + 1, secondColon), 10);
      if (!isNaN(lineNum)) {
        refs.push({ file, line: lineNum });
      }
    }
  } catch {
    // grep exits 1 when no matches found; this is normal.
  }
  return refs;
}
