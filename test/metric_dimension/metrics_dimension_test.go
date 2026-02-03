// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

type MetricsAppendDimensionTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *MetricsAppendDimensionTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting MetricAppendDimensionTestSuite")
}

func (suite *MetricsAppendDimensionTestSuite) TearDownSuite() {
	suite.Result.Print()
	printTestSummaryTable(suite.Result)
	fmt.Println(">>>> Finished MetricAppendDimensionTestSuite")
}

// testDescriptions maps test names to what they validate
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
	// Calculate dynamic column widths based on content
	testNameWidth := len("TEST NAME")
	descWidth := len("VALIDATES")
	statusWidth := len("STATUS")

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

	// Add padding for readability
	testNameWidth += 2
	descWidth += 2
	statusWidth = 8 // Fixed width for PASS/FAIL

	// Calculate total width for borders
	totalWidth := testNameWidth + descWidth + statusWidth + 10 // 10 for separators and padding

	// Print header
	fmt.Println()
	fmt.Println("+" + strings.Repeat("-", totalWidth-2) + "+")
	title := "METRIC DIMENSION TEST SUITE SUMMARY"
	titlePadding := (totalWidth - 2 - len(title)) / 2
	fmt.Printf("|%s%s%s|\n", strings.Repeat(" ", titlePadding), title, strings.Repeat(" ", totalWidth-2-titlePadding-len(title)))
	fmt.Printf("+%s+%s+%s+\n", strings.Repeat("-", testNameWidth+2), strings.Repeat("-", descWidth+2), strings.Repeat("-", statusWidth+2))

	// Header row
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

			testName := group.Name
			description := testDescriptions[group.Name]
			if description == "" {
				description = test.Name // fallback to metric name
			}

			fmt.Printf("| %-*s | %-*s | %-*s |\n", testNameWidth, testName, descWidth, description, statusWidth, statusStr)
		}
	}

	fmt.Printf("+%s+%s+%s+\n", strings.Repeat("-", testNameWidth+2), strings.Repeat("-", descWidth+2), strings.Repeat("-", statusWidth+2))

	// Summary line
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

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var (
	testRunners []*test_runner.TestRunner
)

func getTestRunners(env *environment.MetaData) []*test_runner.TestRunner {
	if testRunners == nil {
		factory := dimension.GetDimensionFactory(*env)
		testRunners = []*test_runner.TestRunner{
			{TestRunner: &NoAppendDimensionTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &GlobalAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &OneAggregateDimensionTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &AggregationDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &DropOriginalMetricsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			// Collectd tests: baseline (no append_dimensions) vs with append_dimensions
			{TestRunner: &CollectdNoAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CollectdGlobalAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CollectdAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CollectdFleetAggregationTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &CpuGlobalAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &EthtoolAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
			{TestRunner: &EthtoolPluginAppendDimensionsTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}},
		}
	}
	return testRunners
}

func (suite *MetricsAppendDimensionTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	for _, testRunner := range getTestRunners(env) {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Metric Append Dimension Test Suite Failed")
}

func (suite *MetricsAppendDimensionTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestMetricsAppendDimensionTestSuite(t *testing.T) {
	suite.Run(t, new(MetricsAppendDimensionTestSuite))
}

func isAllValuesGreaterThanOrEqualToZero(metricName string, values []float64) bool {
	if len(values) == 0 {
		fmt.Printf("No values found %v\n", metricName)
		return false
	}
	for _, value := range values {
		if value < 0 {
			fmt.Printf("Values are not all greater than or equal to zero for %v\n", metricName)
			return false
		}
	}
	fmt.Printf("Values are all greater than or equal to zero for %v\n", metricName)
	return true
}
