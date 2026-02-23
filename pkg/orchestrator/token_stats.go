// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

// FileTokenStat holds size information for a single file that
// contributes to an assembled Claude prompt.
type FileTokenStat struct {
	Category string `json:"category"`
	Path     string `json:"path"`
	Bytes    int    `json:"bytes"`
}

// tokenCountModel is the default model identifier for the Anthropic
// Token Counting API. All Claude 3.5+ models share the same tokenizer.
const tokenCountModel = "claude-sonnet-4-20250514"

// TokenStats enumerates all files that buildProjectContext would load,
// displays their sizes grouped by category, and optionally calls the
// Anthropic Token Counting API for exact prompt token counts. Set
// ANTHROPIC_API_KEY to enable API counting.
func (o *Orchestrator) TokenStats() error {
	files := o.enumerateContextFiles()

	sort.Slice(files, func(i, j int) bool {
		if files[i].Category != files[j].Category {
			return files[i].Category < files[j].Category
		}
		return files[i].Path < files[j].Path
	})

	// Print per-file table.
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "CATEGORY\tFILE\tBYTES")
	fmt.Fprintln(w, "--------\t----\t-----")

	totalBytes := 0
	catBytes := map[string]int{}
	catCount := map[string]int{}
	for _, f := range files {
		fmt.Fprintf(w, "%s\t%s\t%d\n", f.Category, f.Path, f.Bytes)
		totalBytes += f.Bytes
		catBytes[f.Category] += f.Bytes
		catCount[f.Category]++
	}
	fmt.Fprintln(w, "\t\t")

	cats := sortedKeys(catBytes)
	for _, c := range cats {
		fmt.Fprintf(w, "%s\t%d files\t%d\n", c, catCount[c], catBytes[c])
	}
	fmt.Fprintln(w, "\t\t")
	fmt.Fprintf(w, "TOTAL\t%d files\t%d\n", len(files), totalBytes)
	w.Flush()

	// Build measure prompt for token counting.
	prompt, err := o.buildMeasurePrompt("", "[]", 1, "/dev/null")
	if err != nil {
		return fmt.Errorf("building measure prompt: %w", err)
	}
	fmt.Printf("\nAssembled measure prompt: %d bytes\n", len(prompt))

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Printf("Estimated tokens (bytes/4): ~%d\n", len(prompt)/4)
		fmt.Fprintf(os.Stderr, "\nSet ANTHROPIC_API_KEY for exact token counts via the Anthropic Token Counting API.\n")
		return nil
	}

	logf("token_stats: counting tokens via API (model=%s)", tokenCountModel)
	tokens, err := countTokensViaAPI(apiKey, tokenCountModel, prompt)
	if err != nil {
		return fmt.Errorf("token counting API: %w", err)
	}
	fmt.Printf("Measure prompt tokens: %d (model: %s)\n", tokens, tokenCountModel)
	return nil
}

// enumerateContextFiles lists all files that buildProjectContext loads,
// grouped by category. The returned slice is unsorted; callers should
// sort as needed.
func (o *Orchestrator) enumerateContextFiles() []FileTokenStat {
	var files []FileTokenStat

	addFile := func(path, category string) {
		info, err := os.Stat(path)
		if err != nil {
			return
		}
		files = append(files, FileTokenStat{
			Category: category,
			Path:     path,
			Bytes:    int(info.Size()),
		})
	}

	addGlob := func(pattern, category string) {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			addFile(m, category)
		}
	}

	// Top-level docs loaded by buildProjectContext.
	addFile("docs/VISION.yaml", "docs")
	addFile("docs/ARCHITECTURE.yaml", "docs")
	addFile("docs/SPECIFICATIONS.yaml", "docs")
	addFile("docs/road-map.yaml", "docs")

	// Specs collection.
	addGlob("docs/specs/product-requirements/prd*.yaml", "specs")
	addGlob("docs/specs/use-cases/rel*.yaml", "specs")
	addGlob("docs/specs/test-suites/test-rel*.yaml", "specs")
	addFile("docs/specs/dependency-map.yaml", "specs")
	addFile("docs/specs/sources.yaml", "specs")

	// Engineering guidelines.
	addGlob("docs/engineering/eng*.yaml", "engineering")

	// Constitutions loaded into project context.
	addFile("docs/constitutions/design.yaml", "constitutions")
	addFile("docs/constitutions/planning.yaml", "constitutions")
	addFile("docs/constitutions/execution.yaml", "constitutions")
	addFile("docs/constitutions/go-style.yaml", "constitutions")

	// Extra docs: any YAML in docs/ not covered by knownDocFiles.
	extras := loadExtraDocs("docs/")
	for _, doc := range extras {
		path := filepath.Join("docs", doc.Name+".yaml")
		addFile(path, "docs")
	}

	// Source code from configured directories.
	for _, dir := range o.cfg.Project.GoSourceDirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			files = append(files, FileTokenStat{
				Category: "source",
				Path:     path,
				Bytes:    int(info.Size()),
			})
			return nil
		})
	}

	// Prompt templates.
	addFile("docs/prompts/measure.yaml", "prompts")
	addFile("docs/prompts/stitch.yaml", "prompts")

	return files
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// countTokensViaAPI calls the Anthropic Token Counting API and returns
// the input token count for the given content.
func countTokensViaAPI(apiKey, model, content string) (int, error) {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type request struct {
		Model    string    `json:"model"`
		Messages []message `json:"messages"`
	}

	body, err := json.Marshal(request{
		Model:    model,
		Messages: []message{{Role: "user", Content: content}},
	})
	if err != nil {
		return 0, fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequest("POST",
		"https://api.anthropic.com/v1/messages/count_tokens",
		bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "token-counting-2024-11-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("parsing response: %w", err)
	}

	return result.InputTokens, nil
}
