// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package orchestrator

// Paper build namespace. BuildPaperPDF compiles a paper from markdown to PDF via
// pandoc and the LaTeX toolchain; ReportPaperPlaceholders counts the data
// placeholders still open in the paper source. These are build and reporting
// utilities only — the paper constitution's consistency gates (P1, P3, P4, P5)
// live in mage analyze, not here.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultPaperSource   = "paper/paper.md"
	defaultPaperDir      = "paper"
	paperPlaceholderExpr = `\{\{DATA:([^}]+)\}\}`
	latexNonstop         = "-interaction=nonstopmode"
)

// paperProseExts are the source extensions scanned for placeholders.
var paperProseExts = map[string]bool{".md": true, ".tex": true}

// paperPlaceholderRe matches a data placeholder in paper source.
var paperPlaceholderRe = regexp.MustCompile(paperPlaceholderExpr)

// paperCommand is one step of the build: a binary and its arguments.
type paperCommand struct {
	bin  string
	args []string
}

// paperToolchain is the ordered set of binaries paper:pdf requires on PATH.
var paperToolchain = []string{binPandoc, binPdflatex, binBibtex}

// paperCommands returns the ordered build sequence for a source document. It
// converts markdown to LaTeX with pandoc, then runs the standard
// pdflatex/bibtex/pdflatex/pdflatex cycle to resolve references.
func paperCommands(source string) []paperCommand {
	stem := strings.TrimSuffix(source, filepath.Ext(source))
	tex := stem + ".tex"
	return []paperCommand{
		{binPandoc, []string{source, "-o", tex}},
		{binPdflatex, []string{latexNonstop, tex}},
		{binBibtex, []string{stem}},
		{binPdflatex, []string{latexNonstop, tex}},
		{binPdflatex, []string{latexNonstop, tex}},
	}
}

// checkPaperToolchain returns an error naming the first required binary missing
// from PATH. paper:pdf hard-requires the full toolchain.
func checkPaperToolchain() error {
	for _, bin := range paperToolchain {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("paper:pdf requires %q on PATH; install pandoc and a LaTeX distribution (for example texlive)", bin)
		}
	}
	return nil
}

// BuildPaperPDF compiles the paper at source (default paper/paper.md) to PDF. It
// hard-requires the toolchain: a missing binary is a clear error, not a skip.
func BuildPaperPDF(source string) error {
	if strings.TrimSpace(source) == "" {
		source = defaultPaperSource
	}
	if err := checkPaperToolchain(); err != nil {
		return err
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("paper source %q not found: %w", source, err)
	}
	for _, c := range paperCommands(source) {
		cmd := exec.Command(c.bin, c.args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("paper build step %s %s: %w", c.bin, strings.Join(c.args, " "), err)
		}
	}
	return nil
}

// ReportPaperPlaceholders counts the data placeholders under dir (default
// paper/) and prints a per-file and total report. It is a reporting aid, not a
// gate, so it returns nil even when placeholders remain; the P3 release gate in
// mage analyze enforces zero.
func ReportPaperPlaceholders(dir string) error {
	if strings.TrimSpace(dir) == "" {
		dir = defaultPaperDir
	}
	if _, err := os.Stat(dir); err != nil {
		fmt.Printf("paper:placeholders: no paper directory at %q — nothing to report\n", dir)
		return nil
	}
	total := 0
	for _, file := range paperSourceFiles(dir) {
		n := countPlaceholders(file)
		if n > 0 {
			fmt.Printf("  %s: %d placeholder(s)\n", file, n)
			total += n
		}
	}
	fmt.Printf("paper:placeholders: %d placeholder(s) remaining under %q\n", total, dir)
	return nil
}

// paperSourceFiles walks dir and returns the markdown and LaTeX source files.
func paperSourceFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if paperProseExts[strings.ToLower(filepath.Ext(p))] {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// countPlaceholders returns the number of data placeholders in one file.
func countPlaceholders(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return len(paperPlaceholderRe.FindAllStringIndex(string(data), -1))
}
