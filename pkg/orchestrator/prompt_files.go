// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// contextFileEntry describes a single file included in the assembled Claude prompt.
type contextFileEntry struct {
	Source   string // "default" or "config"
	Category string
	Path     string
	Lines    int
	Bytes    int
}

// PrintContextFiles lists every file that would be appended to the Claude prompt
// with its source annotation (default/config), category, line count, and estimated
// token count (bytes/4). A totals line is printed at the end. No API call or
// podman is required.
//
// Exposed as a mage target (mage prompt:files).
func (o *Orchestrator) PrintContextFiles() error {
	entries := o.resolveContextFileEntries()

	totalLines := 0
	totalBytes := 0
	for _, e := range entries {
		totalLines += e.Lines
		totalBytes += e.Bytes
		fmt.Printf("%-9s  %-10s  %-52s  %6dL  ~%dtok\n",
			"("+e.Source+")", e.Category, e.Path, e.Lines, e.Bytes/4)
	}

	fmt.Printf("\n%d files, %dL, ~%d tokens\n", len(entries), totalLines, totalBytes/4)
	return nil
}

// resolveContextFileEntries returns the ordered list of files that
// buildProjectContext loads, annotated with their source (default/config).
// It mirrors the include/exclude logic of buildProjectContext so that
// enumerateContextFiles (stats:tokens) and PrintContextFiles (prompt:files)
// both report the correct, accurate file set.
func (o *Orchestrator) resolveContextFileEntries() []contextFileEntry {
	var entries []contextFileEntry

	// Build exclude set (empty map when ContextExclude is unset).
	excludeSet := resolveFileSet(o.cfg.Project.ContextExclude)

	// Resolve doc files: ContextInclude overrides standard patterns.
	var docFiles []string
	var docSource string
	if strings.TrimSpace(o.cfg.Project.ContextInclude) != "" {
		docFiles = resolveContextSources(o.cfg.Project.ContextInclude)
		docFiles = ensureTypedDocs(docFiles)
		docSource = "config"
	} else {
		docFiles = resolveStandardFiles()
		docSource = "default"
	}

	// Apply exclusions and collect doc entries.
	seen := make(map[string]bool, len(docFiles))
	for _, path := range docFiles {
		if excludeSet[path] {
			continue
		}
		seen[path] = true
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		entries = append(entries, contextFileEntry{
			Source:   docSource,
			Category: classifyContextFile(path),
			Path:     path,
			Lines:    safeCountLines(path),
			Bytes:    int(info.Size()),
		})
	}

	// Extra context sources from configuration.
	if strings.TrimSpace(o.cfg.Project.ContextSources) != "" {
		extras := resolveContextSources(o.cfg.Project.ContextSources)
		for _, path := range extras {
			if seen[path] || excludeSet[path] {
				continue
			}
			seen[path] = true
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			entries = append(entries, contextFileEntry{
				Source:   "config",
				Category: "extra",
				Path:     path,
				Lines:    safeCountLines(path),
				Bytes:    int(info.Size()),
			})
		}
	}

	// Source code from configured directories, filtered by ContextExclude.
	for _, dir := range o.cfg.Project.GoSourceDirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			if excludeSet[path] {
				return nil
			}
			entries = append(entries, contextFileEntry{
				Source:   "config",
				Category: "source",
				Path:     path,
				Lines:    safeCountLines(path),
				Bytes:    int(info.Size()),
			})
			return nil
		})
	}

	// Prompt templates (always included, not config-driven).
	for _, p := range []string{"docs/prompts/measure.yaml", "docs/prompts/stitch.yaml"} {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		entries = append(entries, contextFileEntry{
			Source:   "default",
			Category: "prompts",
			Path:     p,
			Lines:    safeCountLines(p),
			Bytes:    int(info.Size()),
		})
	}

	return entries
}

// safeCountLines calls countLines and discards the error, returning 0 on failure.
func safeCountLines(path string) int {
	n, _ := countLines(path)
	return n
}
