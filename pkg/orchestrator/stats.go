// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// StatsRecord holds collected LOC and documentation word counts.
type StatsRecord struct {
	GoProdLOC int            `yaml:"go_loc_prod"`
	GoTestLOC int            `yaml:"go_loc_test"`
	GoLOC     int            `yaml:"go_loc"`
	SpecWords map[string]int `yaml:"spec_words"`
}

// CollectStats gathers Go LOC and documentation word counts.
func (o *Orchestrator) CollectStats() (StatsRecord, error) {
	var prodLines, testLines int

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path == "vendor" || path == ".git" || path == o.cfg.Project.BinaryDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip magefiles — they are build tooling, not project code.
		if strings.HasPrefix(path, o.cfg.Project.MagefilesDir) {
			return nil
		}
		count, countErr := countLines(path)
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
	for _, path := range resolveStandardFiles() {
		cat := classifyContextFile(path)
		if cat == "prd" || cat == "use_case" || cat == "test_suite" {
			words, wordErr := countWordsInFile(path)
			if wordErr != nil {
				continue
			}
			specWords[cat] += words
		}
	}

	return StatsRecord{
		GoProdLOC: prodLines,
		GoTestLOC: testLines,
		GoLOC:     prodLines + testLines,
		SpecWords: specWords,
	}, nil
}

// Stats prints Go lines of code and documentation word counts as YAML.
func (o *Orchestrator) Stats() error {
	rec, err := o.CollectStats()
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

func countLines(path string) (int, error) {
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

func countWordsInFile(path string) (int, error) {
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
