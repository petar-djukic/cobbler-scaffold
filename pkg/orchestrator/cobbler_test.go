// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- StitchReport YAML serialization ---

func TestStitchReport_SerializesAllFields(t *testing.T) {
	report := StitchReport{
		TaskID:    "mage-abc.1",
		TaskTitle: "Add widget feature",
		Status:    "success",
		Branch:    "task/main-mage-abc.1",
		Diff:      historyDiff{Files: 3, Insertions: 120, Deletions: 15},
		Files: []FileChange{
			{Path: "pkg/widget/widget.go", Status: "A", Insertions: 80, Deletions: 0},
			{Path: "pkg/widget/widget_test.go", Status: "A", Insertions: 40, Deletions: 0},
			{Path: "go.mod", Status: "M", Insertions: 0, Deletions: 15},
		},
		LOCBefore: LocSnapshot{Production: 500, Test: 200},
		LOCAfter:  LocSnapshot{Production: 580, Test: 240},
	}

	data, err := yaml.Marshal(&report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Round-trip: unmarshal back and verify fields.
	var got StitchReport
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.TaskID != report.TaskID {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, report.TaskID)
	}
	if got.TaskTitle != report.TaskTitle {
		t.Errorf("TaskTitle: got %q, want %q", got.TaskTitle, report.TaskTitle)
	}
	if got.Status != report.Status {
		t.Errorf("Status: got %q, want %q", got.Status, report.Status)
	}
	if got.Branch != report.Branch {
		t.Errorf("Branch: got %q, want %q", got.Branch, report.Branch)
	}
	if got.Diff.Files != 3 || got.Diff.Insertions != 120 || got.Diff.Deletions != 15 {
		t.Errorf("Diff: got %+v, want {3 120 15}", got.Diff)
	}
	if len(got.Files) != 3 {
		t.Fatalf("Files: got %d entries, want 3", len(got.Files))
	}
	if got.Files[0].Path != "pkg/widget/widget.go" || got.Files[0].Status != "A" {
		t.Errorf("Files[0]: got %+v", got.Files[0])
	}
	if got.Files[2].Status != "M" || got.Files[2].Deletions != 15 {
		t.Errorf("Files[2]: got %+v", got.Files[2])
	}
	if got.LOCBefore.Production != 500 || got.LOCAfter.Production != 580 {
		t.Errorf("LOC: before=%+v after=%+v", got.LOCBefore, got.LOCAfter)
	}
}

// --- parseNameStatus and parseNumstat ---

func TestParseNameStatus_AddedModifiedDeleted(t *testing.T) {
	nsOutput := "A\tpkg/new.go\nM\tpkg/existing.go\nD\tpkg/removed.go\n"
	numOutput := "50\t0\tpkg/new.go\n10\t5\tpkg/existing.go\n0\t30\tpkg/removed.go\n"

	numMap := parseNumstat(numOutput)
	files := parseNameStatus(nsOutput, numMap)

	if len(files) != 3 {
		t.Fatalf("got %d files, want 3", len(files))
	}

	tests := []struct {
		path   string
		status string
		ins    int
		del    int
	}{
		{"pkg/new.go", "A", 50, 0},
		{"pkg/existing.go", "M", 10, 5},
		{"pkg/removed.go", "D", 0, 30},
	}
	for i, tt := range tests {
		fc := files[i]
		if fc.Path != tt.path {
			t.Errorf("[%d] Path: got %q, want %q", i, fc.Path, tt.path)
		}
		if fc.Status != tt.status {
			t.Errorf("[%d] Status: got %q, want %q", i, fc.Status, tt.status)
		}
		if fc.Insertions != tt.ins {
			t.Errorf("[%d] Insertions: got %d, want %d", i, fc.Insertions, tt.ins)
		}
		if fc.Deletions != tt.del {
			t.Errorf("[%d] Deletions: got %d, want %d", i, fc.Deletions, tt.del)
		}
	}
}

func TestParseNameStatus_Renamed(t *testing.T) {
	nsOutput := "R100\told/path.go\tnew/path.go\n"
	numOutput := "5\t3\tnew/path.go\n"

	numMap := parseNumstat(numOutput)
	files := parseNameStatus(nsOutput, numMap)

	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path != "new/path.go" {
		t.Errorf("Path: got %q, want %q", files[0].Path, "new/path.go")
	}
	if files[0].Status != "R" {
		t.Errorf("Status: got %q, want %q", files[0].Status, "R")
	}
	if files[0].Insertions != 5 || files[0].Deletions != 3 {
		t.Errorf("Counts: got ins=%d del=%d, want ins=5 del=3", files[0].Insertions, files[0].Deletions)
	}
}

func TestParseNameStatus_EmptyInput(t *testing.T) {
	files := parseNameStatus("", nil)
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestParseNumstat_BinaryFile(t *testing.T) {
	output := "-\t-\timage.png\n10\t2\tREADME.md\n"
	m := parseNumstat(output)

	if entry, ok := m["image.png"]; !ok {
		t.Error("missing entry for image.png")
	} else if entry.ins != 0 || entry.del != 0 {
		t.Errorf("image.png: got ins=%d del=%d, want 0 0", entry.ins, entry.del)
	}

	if entry, ok := m["README.md"]; !ok {
		t.Error("missing entry for README.md")
	} else if entry.ins != 10 || entry.del != 2 {
		t.Errorf("README.md: got ins=%d del=%d, want 10 2", entry.ins, entry.del)
	}
}

func TestParseNumstat_EmptyInput(t *testing.T) {
	m := parseNumstat("")
	if len(m) != 0 {
		t.Errorf("got %d entries, want 0", len(m))
	}
}

// --- saveHistoryReport ---

func TestSaveHistoryReport_WritesFile(t *testing.T) {
	dir := t.TempDir()
	o := &Orchestrator{
		cfg: Config{
			Cobbler: CobblerConfig{HistoryDir: dir},
		},
	}

	report := StitchReport{
		TaskID:    "test-001",
		TaskTitle: "Test task",
		Status:    "success",
		Branch:    "task/main-test-001",
		Diff:      historyDiff{Files: 1, Insertions: 10, Deletions: 2},
		Files: []FileChange{
			{Path: "pkg/foo.go", Status: "M", Insertions: 10, Deletions: 2},
		},
		LOCBefore: LocSnapshot{Production: 100, Test: 50},
		LOCAfter:  LocSnapshot{Production: 108, Test: 50},
	}

	ts := "2026-02-24-10-00-00"
	o.saveHistoryReport(ts, report)

	path := filepath.Join(dir, ts+"-stitch-report.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report file: %v", err)
	}

	var got StitchReport
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if got.TaskID != "test-001" {
		t.Errorf("TaskID: got %q, want %q", got.TaskID, "test-001")
	}
	if len(got.Files) != 1 || got.Files[0].Path != "pkg/foo.go" {
		t.Errorf("Files: got %+v", got.Files)
	}
}

// --- extractTextFromStreamJSON ---

func TestExtractTextFromStreamJSON_SingleAssistantMessage(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"system","message":"ready"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Here is the output:\n\n` + "```yaml\\n- index: 0\\n  title: Task one\\n```" + `"}]}}
{"type":"result","usage":{"input_tokens":100,"output_tokens":50}}
`)
	text := extractTextFromStreamJSON(raw)
	if text == "" {
		t.Fatal("expected non-empty text")
	}
	if !contains(text, "Task one") {
		t.Errorf("text missing expected content, got: %s", text)
	}
}

func TestExtractTextFromStreamJSON_NoAssistantMessages(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"system","message":"ready"}
{"type":"result","usage":{"input_tokens":100,"output_tokens":0}}
`)
	text := extractTextFromStreamJSON(raw)
	if text != "" {
		t.Errorf("expected empty text, got: %s", text)
	}
}

func TestExtractTextFromStreamJSON_MultipleTextBlocks(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"first "},{"type":"text","text":"second"}]}}
`)
	text := extractTextFromStreamJSON(raw)
	if text != "first second" {
		t.Errorf("expected 'first second', got: %q", text)
	}
}

// --- extractYAMLBlock ---

func TestExtractYAMLBlock_ValidFencedBlock(t *testing.T) {
	t.Parallel()
	text := "Here is the YAML:\n\n```yaml\n- index: 0\n  title: Task one\n  dependency: -1\n```\n\nDone."
	yaml, err := extractYAMLBlock(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "- index: 0\n  title: Task one\n  dependency: -1"
	if string(yaml) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(yaml), expected)
	}
}

func TestExtractYAMLBlock_YmlFence(t *testing.T) {
	t.Parallel()
	text := "```yml\n- index: 0\n```\n"
	yaml, err := extractYAMLBlock(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(yaml) != "- index: 0" {
		t.Errorf("got: %q", string(yaml))
	}
}

func TestExtractYAMLBlock_NoFencedBlock(t *testing.T) {
	t.Parallel()
	text := "Here is some text with no YAML block."
	_, err := extractYAMLBlock(text)
	if err == nil {
		t.Error("expected error for missing YAML block")
	}
}

func TestExtractYAMLBlock_UnclosedBlock(t *testing.T) {
	t.Parallel()
	text := "```yaml\n- index: 0\n  title: Task one"
	_, err := extractYAMLBlock(text)
	if err == nil {
		t.Error("expected error for unclosed YAML block")
	}
}

func TestSaveHistoryReport_NoOpWhenHistoryDirEmpty(t *testing.T) {
	o := &Orchestrator{
		cfg: Config{
			Cobbler: CobblerConfig{HistoryDir: ""},
		},
	}

	// Should not panic or create any files.
	o.saveHistoryReport("2026-02-24-10-00-00", StitchReport{
		TaskID: "test-noop",
		Status: "success",
	})
}
