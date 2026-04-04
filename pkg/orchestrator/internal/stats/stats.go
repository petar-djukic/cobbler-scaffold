// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package stats implements metrics collection and reporting for the
// orchestrator: LOC counting, token stats, release progress, generation
// run stats, and outcome reporting.
package stats

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Logger is a function that formats and emits log messages.
type Logger func(format string, args ...any)

// StatsRecord holds collected LOC and documentation word counts.
type StatsRecord struct {
	GoProdLOC int            `yaml:"go_loc_prod"`
	GoTestLOC int            `yaml:"go_loc_test"`
	GoLOC     int            `yaml:"go_loc"`
	SpecWords map[string]int `yaml:"spec_words"`
}

// StatsDeps holds dependencies for LOC collection.
type StatsDeps struct {
	BinaryDir            string
	MagefilesDir         string
	ResolveStandardFiles func() []string
	ClassifyContextFile  func(path string) string
}

// CollectStats gathers Go LOC and documentation word counts.
func CollectStats(deps StatsDeps) (StatsRecord, error) {
	var prodLines, testLines int

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path == "vendor" || path == ".git" || path == deps.BinaryDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip magefiles — they are build tooling, not project code.
		if strings.HasPrefix(path, deps.MagefilesDir) {
			return nil
		}
		count, countErr := CountLines(path)
		if countErr != nil {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			testLines += count
		} else {
			prodLines += count
		}
		return nil
	})
	if err != nil {
		return StatsRecord{}, err
	}

	specWords := make(map[string]int)
	if deps.ResolveStandardFiles != nil && deps.ClassifyContextFile != nil {
		for _, path := range deps.ResolveStandardFiles() {
			cat := deps.ClassifyContextFile(path)
			if cat == "srd" || cat == "use_case" || cat == "test_suite" {
				words, wordErr := CountWordsInFile(path)
				if wordErr != nil {
					continue
				}
				specWords[cat] += words
			}
		}
	}

	return StatsRecord{
		GoProdLOC: prodLines,
		GoTestLOC: testLines,
		GoLOC:     prodLines + testLines,
		SpecWords: specWords,
	}, nil
}

// PrintStats prints Go lines of code and documentation word counts as YAML.
func PrintStats(deps StatsDeps) error {
	rec, err := CollectStats(deps)
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(rec)
	if err != nil {
		return err
	}
	fmt.Print(string(out))
	return nil
}

// CountLines counts the number of lines in a file.
func CountLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// CountWordsInFile counts words in a file using unicode-aware word splitting.
func CountWordsInFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	count := 0
	inWord := false
	for _, r := range string(data) {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count, nil
}
