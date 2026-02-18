// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package profiling

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const (
	envS3Bucket  = "CWA_TEST_PROFILE_S3_BUCKET"
	envS3Key     = "CWA_TEST_PROFILE_S3_KEY"
	markerPrefix = "@@CWA_PROFILE_JSON@@"
	markerSuffix = "@@END_CWA_PROFILE_JSON@@"
)

// EmitSummary outputs the profiling summary in a way that can be captured
// by the GitHub Actions runner. It prints the JSON between markers so it
// can be parsed from Terraform remote-exec output.
func (p *Profiler) EmitSummary() {
	s := p.Summary()
	data, err := json.Marshal(s)
	if err != nil {
		log.Printf("profiling: failed to marshal summary: %v", err)
		return
	}
	// Print between markers for GHA parsing
	fmt.Println(markerPrefix)
	fmt.Println(string(data))
	fmt.Println(markerSuffix)
}

// GenerateMarkdown returns a GitHub Flavored Markdown string with
// a table and mermaid pie chart of the profiling results.
func GenerateMarkdown(jsonData []byte) (string, error) {
	var s Summary
	if err := json.Unmarshal(jsonData, &s); err != nil {
		return "", err
	}
	return summaryToMarkdown(s), nil
}

// GenerateMarkdownFromFile reads a profiling JSON file and returns markdown.
func GenerateMarkdownFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return GenerateMarkdown(data)
}

func summaryToMarkdown(s Summary) string {
	md := "### ⏱️ Test Profiling Results\n\n"
	md += fmt.Sprintf("**Total Duration:** %.1fs\n\n", s.TotalDuration)

	// Category breakdown table
	md += "| Category | Duration | % |\n"
	md += "|----------|----------|---|\n"

	icons := map[SpanCategory]string{
		CategorySetup:      "🔧",
		CategoryAgentWait:  "🟡",
		CategoryAPICall:    "🔵",
		CategoryValidation: "🟣",
		CategorySleep:      "💤",
		CategoryCleanup:    "🧹",
		CategoryOther:      "⚪",
	}

	type catEntry struct {
		cat SpanCategory
		dur float64
		pct float64
	}
	var entries []catEntry
	for cat, dur := range s.CategoryBreakdown {
		pct := 0.0
		if s.TotalDuration > 0 {
			pct = (dur / s.TotalDuration) * 100
		}
		entries = append(entries, catEntry{cat, dur, pct})
	}
	// Sort by duration descending
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].dur > entries[i].dur {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	for _, e := range entries {
		icon := icons[e.cat]
		if icon == "" {
			icon = "⚪"
		}
		md += fmt.Sprintf("| %s %s | %.1fs | %.1f%% |\n", icon, e.cat, e.dur, e.pct)
	}

	// Mermaid pie chart
	md += "\n```mermaid\npie title Time Breakdown\n"
	for _, e := range entries {
		md += fmt.Sprintf("    \"%s\" : %.1f\n", e.cat, e.pct)
	}
	md += "```\n"

	// Per-test details (collapsible)
	md += "\n<details><summary>Per-Test Breakdown</summary>\n\n"
	for _, t := range s.Tests {
		md += fmt.Sprintf("**%s** (%.1fs)\n\n", t.TestName, t.Duration)
		if len(t.Spans) > 0 {
			md += "| Span | Category | Duration |\n"
			md += "|------|----------|----------|\n"
			for _, span := range t.Spans {
				md += fmt.Sprintf("| %s | %s | %.1fs |\n", span.Name, span.Category, span.Duration)
			}
			md += "\n"
		}
	}
	md += "</details>\n"

	return md
}
