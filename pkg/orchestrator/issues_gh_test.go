// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"strings"
	"testing"
)

// TestIssuesContextJSON_ParseableByParseIssuesJSON verifies that the JSON
// produced by issuesContextJSON (from internal/github) round-trips through
// parseIssuesJSON (from context.go). This is a cross-boundary integration
// test that validates both packages agree on the JSON format.
func TestIssuesContextJSON_ParseableByParseIssuesJSON(t *testing.T) {
	t.Parallel()
	issues := []cobblerIssue{
		{Number: 115, Title: "cmd/wc core implementation", Labels: []string{cobblerLabelReady}},
	}
	jsonStr, err := issuesContextJSON(issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the output is parseable by parseIssuesJSON (the function that was broken).
	parsed := parseIssuesJSON(jsonStr)
	if len(parsed) != 1 {
		t.Fatalf("parseIssuesJSON returned %d issues, want 1; input: %s", len(parsed), jsonStr)
	}
	if parsed[0].ID != "115" {
		t.Errorf("ID = %q, want %q", parsed[0].ID, "115")
	}
	if !strings.Contains(jsonStr, "cmd/wc core implementation") {
		t.Errorf("JSON does not contain expected title: %s", jsonStr)
	}
}
