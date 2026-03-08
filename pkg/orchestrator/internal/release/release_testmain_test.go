// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package release

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Wire up git helper functions for tests. These mirror the real
	// implementations in the parent orchestrator package (commands.go).
	GitCurrentBranchFn = func(dir string) (string, error) {
		cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		if dir != "" {
			cmd.Dir = dir
		}
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	GitListTagsFn = func(pattern, dir string) []string {
		cmd := exec.Command("git", "tag", "--list", pattern)
		if dir != "" {
			cmd.Dir = dir
		}
		out, _ := cmd.Output()
		var tags []string
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				tags = append(tags, line)
			}
		}
		return tags
	}
	GitTagFn = func(tag, dir string) error {
		cmd := exec.Command("git", "tag", tag)
		if dir != "" {
			cmd.Dir = dir
		}
		return cmd.Run()
	}
	GitStageAllFn = func(dir string) error {
		cmd := exec.Command("git", "add", "-A")
		if dir != "" {
			cmd.Dir = dir
		}
		return cmd.Run()
	}
	GitCommitFn = func(msg, dir string) error {
		cmd := exec.Command("git", "commit", "--no-verify", "-m", msg)
		if dir != "" {
			cmd.Dir = dir
		}
		return cmd.Run()
	}

	os.Exit(m.Run())
}
