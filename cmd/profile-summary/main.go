// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Command profile-summary parses test output from Terraform logs and writes
// GitHub Flavored Markdown to stdout (for GITHUB_STEP_SUMMARY).
//
// It extracts two data sources from the same log:
//  1. Per-test timing from `go test -v` output (--- PASS/FAIL lines)
//  2. Span-level profiling JSON (@@CWA_PROFILE_JSON@@ markers), if present
//
// Usage:
//
//	terraform apply ... 2>&1 | tee tf_output.log
//	cat tf_output.log | profile-summary >> $GITHUB_STEP_SUMMARY
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type TestResult struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Elapsed float64 `json:"elapsed_seconds"`
}

type Report struct {
	TotalElapsed    float64            `json:"total_elapsed_seconds"`
	Passed          int                `json:"passed"`
	Failed          int                `json:"failed"`
	Skipped         int                `json:"skipped"`
	Tests           []TestResult       `json:"tests"`
	CategoryBreakdown map[string]float64 `json:"category_breakdown_seconds,omitempty"`
}

// Matches: --- PASS: TestName (1.23s)
// Also matches Terraform remote-exec prefix variations
var testResultRe = regexp.MustCompile(`--- (PASS|FAIL|SKIP): (\S+) \((\d+\.?\d*)s\)`)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var (
		tests         []TestResult
		spanJSON      []string
		capturingSpan bool
	)

	for scanner.Scan() {
		line := scanner.Text()

		// Capture span-level profiling JSON
		if strings.Contains(line, "@@CWA_PROFILE_JSON@@") {
			capturingSpan = true
			continue
		}
		if strings.Contains(line, "@@END_CWA_PROFILE_JSON@@") {
			capturingSpan = false
			continue
		}
		if capturingSpan {
			spanJSON = append(spanJSON, strings.TrimSpace(line))
			continue
		}

		// Parse go test -v output
		if m := testResultRe.FindStringSubmatch(line); m != nil {
			elapsed, _ := strconv.ParseFloat(m[3], 64)
			tests = append(tests, TestResult{
				Name:    m[2],
				Status:  strings.ToLower(m[1]),
				Elapsed: elapsed,
			})
		}
	}

	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "No test results found")
		os.Exit(0)
	}

	report := buildReport(tests, spanJSON)

	// Emit machine-readable JSON for Kiro analysis
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println("@@CWA_PROFILE@@")
	fmt.Println(string(reportJSON))
	fmt.Println("@@END_CWA_PROFILE@@")

	// Emit GitHub step summary markdown
	fmt.Print(renderMarkdown(report))
}

func buildReport(tests []TestResult, spanJSON []string) Report {
	r := Report{Tests: tests, CategoryBreakdown: map[string]float64{}}
	for _, t := range tests {
		r.TotalElapsed += t.Elapsed
		switch t.Status {
		case "pass":
			r.Passed++
		case "fail":
			r.Failed++
		case "skip":
			r.Skipped++
		}
	}

	if len(spanJSON) > 0 {
		var spanData struct {
			CategoryBreakdown map[string]float64 `json:"category_breakdown_seconds"`
		}
		if err := json.Unmarshal([]byte(strings.Join(spanJSON, "")), &spanData); err == nil {
			r.CategoryBreakdown = spanData.CategoryBreakdown
		}
	}

	sort.Slice(r.Tests, func(i, j int) bool {
		return r.Tests[i].Elapsed > r.Tests[j].Elapsed
	})
	return r
}

func renderMarkdown(r Report) string {
	var b strings.Builder

	status := "✅"
	if r.Failed > 0 {
		status = "❌"
	}
	b.WriteString(fmt.Sprintf("### %s Test Results\n\n", status))
	b.WriteString(fmt.Sprintf("**Total:** %.1fs | ✅ %d passed | ❌ %d failed | ⏭️ %d skipped\n\n",
		r.TotalElapsed, r.Passed, r.Failed, r.Skipped))

	// Category breakdown (if span profiling was enabled)
	if len(r.CategoryBreakdown) > 0 {
		type kv struct{ k string; v float64 }
		var sorted []kv
		for k, v := range r.CategoryBreakdown {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })

		b.WriteString("#### ⏱️ Time Breakdown\n\n")
		b.WriteString("| Category | Duration | % |\n|----------|----------|---|\n")
		for _, e := range sorted {
			pct := 0.0
			if r.TotalElapsed > 0 {
				pct = (e.v / r.TotalElapsed) * 100
			}
			b.WriteString(fmt.Sprintf("| %s | %.1fs | %.1f%% |\n", e.k, e.v, pct))
		}

		b.WriteString("\n```mermaid\npie title Time Breakdown\n")
		for _, e := range sorted {
			pct := (e.v / r.TotalElapsed) * 100
			b.WriteString(fmt.Sprintf("    \"%s\" : %.1f\n", e.k, pct))
		}
		b.WriteString("```\n\n")
	}

	// Per-test table (always available)
	b.WriteString("#### Per-Test Timing\n\n")
	b.WriteString("| Status | Test | Duration |\n|--------|------|----------|\n")
	for _, t := range r.Tests {
		icon := "✅"
		if t.Status == "fail" {
			icon = "❌"
		} else if t.Status == "skip" {
			icon = "⏭️"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %.1fs |\n", icon, t.Name, t.Elapsed))
	}
	b.WriteString("\n")

	return b.String()
}
