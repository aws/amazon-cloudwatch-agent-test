// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"fmt"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

// testDescriptions maps test names to what they validate.
var testDescriptions = map[string]string{
	// Existing tests
	"NoAppendDimension":               "baseline, host present",
	"GlobalAppendDimension":           "global, host dropped",
	"GlobalAppendDimensions":          "global, host dropped",
	"OneAggregatedDimension":          "single aggregation dim",
	"OneAggregateDimension":           "single aggregation dim",
	"AggregationDimensions":           "multiple aggregation dims",
	"AggregationDimensionsTestRunner": "multiple aggregation dims",
	"DropOriginalMetrics":             "drop_original_metrics",
	// Collectd tests
	"CollectdNoAppendDimensions":     "collectd baseline, host present",
	"CollectdGlobalAppendDimensions": "collectd global, host dropped",
	"CollectdAppendDimensions":       "collectd plugin-level, host kept",
	"CollectdFleetAggregation":       "collectd plugin + aggregation",
	// CPU tests
	"CpuGlobalAppendDimensions": "cpu global, host dropped",
	// Ethtool tests
	"EthtoolAppendDimensions":       "ethtool global, host dropped",
	"EthtoolPluginAppendDimensions": "ethtool plugin-level, host kept",
}

// printTestSummaryTable prints a clean, GitHub Actions-friendly summary table.
// Column widths are dynamically calculated based on content to prevent truncation.
func printTestSummaryTable(result status.TestSuiteResult) {
	testNameWidth := len("TEST NAME")
	descWidth := len("VALIDATES")
	statusWidth := 8

	for _, group := range result.TestGroupResults {
		if len(group.Name) > testNameWidth {
			testNameWidth = len(group.Name)
		}
		desc := testDescriptions[group.Name]
		if desc == "" {
			for _, test := range group.TestResults {
				if len(test.Name) > descWidth {
					descWidth = len(test.Name)
				}
			}
		} else if len(desc) > descWidth {
			descWidth = len(desc)
		}
	}

	testNameWidth += 2
	descWidth += 2
	totalWidth := testNameWidth + descWidth + statusWidth + 10

	fmt.Println()
	fmt.Println("+" + strings.Repeat("-", totalWidth-2) + "+")
	title := "METRIC DIMENSION TEST SUITE SUMMARY"
	titlePadding := (totalWidth - 2 - len(title)) / 2
	fmt.Printf("|%s%s%s|\n", strings.Repeat(" ", titlePadding), title, strings.Repeat(" ", totalWidth-2-titlePadding-len(title)))
	fmt.Printf("+%s+%s+%s+\n", strings.Repeat("-", testNameWidth+2), strings.Repeat("-", descWidth+2), strings.Repeat("-", statusWidth+2))

	fmt.Printf("| %-*s | %-*s | %-*s |\n", testNameWidth, "TEST NAME", descWidth, "VALIDATES", statusWidth, "STATUS")
	fmt.Printf("+%s+%s+%s+\n", strings.Repeat("-", testNameWidth+2), strings.Repeat("-", descWidth+2), strings.Repeat("-", statusWidth+2))

	passed := 0
	failed := 0

	for _, group := range result.TestGroupResults {
		for _, test := range group.TestResults {
			var statusStr string
			if test.Status == status.FAILED {
				statusStr = "FAIL"
				failed++
			} else {
				statusStr = "PASS"
				passed++
			}

			description := testDescriptions[group.Name]
			if description == "" {
				description = test.Name
			}

			fmt.Printf("| %-*s | %-*s | %-*s |\n", testNameWidth, group.Name, descWidth, description, statusWidth, statusStr)
		}
	}

	fmt.Printf("+%s+%s+%s+\n", strings.Repeat("-", testNameWidth+2), strings.Repeat("-", descWidth+2), strings.Repeat("-", statusWidth+2))

	totalTests := passed + failed
	var overallStatus string
	if failed > 0 {
		overallStatus = fmt.Sprintf("TOTAL: %d | PASSED: %d | FAILED: %d | SOME FAILED", totalTests, passed, failed)
	} else {
		overallStatus = fmt.Sprintf("TOTAL: %d | PASSED: %d | FAILED: %d | ALL PASSED", totalTests, passed, failed)
	}

	summaryPadding := (totalWidth - 2 - len(overallStatus)) / 2
	fmt.Printf("|%s%s%s|\n", strings.Repeat(" ", summaryPadding), overallStatus, strings.Repeat(" ", totalWidth-2-summaryPadding-len(overallStatus)))

	fmt.Println("+" + strings.Repeat("-", totalWidth-2) + "+")
	fmt.Println()
}
