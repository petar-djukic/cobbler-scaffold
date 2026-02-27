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
	"sort"

	"gopkg.in/yaml.v3"
)

// FileTokenStat holds size information for a single file that
// contributes to an assembled Claude prompt.
type FileTokenStat struct {
	Category string `yaml:"category"`
	Path     string `yaml:"path"`
	Bytes    int    `yaml:"bytes"`
}

// tokenCountModel is the default model identifier for the Anthropic
// Token Counting API. All Claude 3.5+ models share the same tokenizer.
const tokenCountModel = "claude-sonnet-4-20250514"

// tokenStatsReport is the top-level YAML output for stats:tokens.
type tokenStatsReport struct {
	Files      []FileTokenStat    `yaml:"files"`
	Categories []categorySummary  `yaml:"categories"`
	Total      totalSummary       `yaml:"total"`
	Prompt     promptTokenSummary `yaml:"prompt"`
}

type categorySummary struct {
	Category string `yaml:"category"`
	Files    int    `yaml:"files"`
	Bytes    int    `yaml:"bytes"`
}

type totalSummary struct {
	Files int `yaml:"files"`
	Bytes int `yaml:"bytes"`
}

type promptTokenSummary struct {
	Bytes           int    `yaml:"bytes"`
	EstimatedTokens int    `yaml:"estimated_tokens"`
	ExactTokens     int    `yaml:"exact_tokens,omitempty"`
	Model           string `yaml:"model,omitempty"`
}

// TokenStats enumerates all files that buildProjectContext would load,
// outputs their sizes grouped by category as YAML, and optionally calls
// the Anthropic Token Counting API for exact prompt token counts. Set
// ANTHROPIC_API_KEY to enable API counting.
func (o *Orchestrator) TokenStats() error {
	files := o.enumerateContextFiles()

	sort.Slice(files, func(i, j int) bool {
		if files[i].Category != files[j].Category {
			return files[i].Category < files[j].Category
		}
		return files[i].Path < files[j].Path
	})

	totalBytes := 0
	catBytes := map[string]int{}
	catCount := map[string]int{}
	for _, f := range files {
		totalBytes += f.Bytes
		catBytes[f.Category] += f.Bytes
		catCount[f.Category]++
	}

	cats := sortedKeys(catBytes)
	var categories []categorySummary
	for _, c := range cats {
		categories = append(categories, categorySummary{
			Category: c, Files: catCount[c], Bytes: catBytes[c],
		})
	}

	// Build measure prompt for token counting.
	prompt, err := o.buildMeasurePrompt("", "[]", 1)
	if err != nil {
		return fmt.Errorf("building measure prompt: %w", err)
	}

	ps := promptTokenSummary{
		Bytes:           len(prompt),
		EstimatedTokens: len(prompt) / 4,
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		logf("token_stats: counting tokens via API (model=%s)", tokenCountModel)
		tokens, apiErr := countTokensViaAPI(apiKey, tokenCountModel, prompt)
		if apiErr != nil {
			return fmt.Errorf("token counting API: %w", apiErr)
		}
		ps.ExactTokens = tokens
		ps.Model = tokenCountModel
	} else {
		fmt.Fprintf(os.Stderr, "Set ANTHROPIC_API_KEY for exact token counts via the Anthropic Token Counting API.\n")
	}

	report := tokenStatsReport{
		Files:      files,
		Categories: categories,
		Total:      totalSummary{Files: len(files), Bytes: totalBytes},
		Prompt:     ps,
	}

	out, err := yaml.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshalling report: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

// enumerateContextFiles lists all files that buildProjectContext loads,
// grouped by category. Delegates to resolveContextFileEntries so that
// ContextInclude, ContextExclude, and source filtering are applied
// consistently with buildProjectContext.
func (o *Orchestrator) enumerateContextFiles() []FileTokenStat {
	entries := o.resolveContextFileEntries()
	files := make([]FileTokenStat, 0, len(entries))
	for _, e := range entries {
		files = append(files, FileTokenStat{
			Category: e.Category,
			Path:     e.Path,
			Bytes:    e.Bytes,
		})
	}
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
