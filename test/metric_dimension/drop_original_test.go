//go:build !windows

package metric_dimension

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	testNamespace = "TestDropOriginalMetrics"
)

type DropOriginalMetricsTestRunner struct {
	test_runner.BaseTestRunner
}

type expectation struct {
	shouldExist bool
	metricNames []string
	dimensions  []dimension.Instruction
}

var _ test_runner.ITestRunner = (*DropOriginalMetricsTestRunner)(nil)

func (t *DropOriginalMetricsTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	for name, wants := range t.testCases() {
		result := status.TestResult{Name: name, Status: status.SUCCESSFUL}
		for _, want := range wants {
			if err := t.validate(want); err != nil {
				log.Printf("failed %s/%s: %v", t.GetTestName(), name, err)
				result.Status = status.FAILED
				break
			}
		}
		results = append(results, result)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: results,
	}
}

func (t *DropOriginalMetricsTestRunner) GetTestName() string {
	return "DropOriginalMetrics"
}

func (t *DropOriginalMetricsTestRunner) GetAgentConfigFileName() string {
	return "drop_original_metrics.json"
}

// unused
func (t *DropOriginalMetricsTestRunner) GetMeasuredMetrics() []string {
	return []string{}
}

func (t *DropOriginalMetricsTestRunner) validate(want expectation) error {
	fetcher := metric.MetricValueFetcher{}
	dimensions, _ := t.DimensionFactory.GetDimensions(want.dimensions)
	for _, metricName := range want.metricNames {
		values, err := fetcher.Fetch(testNamespace, metricName, dimensions, metric.AVERAGE, metric.HighResolutionStatPeriod)
		if err != nil {
			return err
		}
		if want.shouldExist && len(values) == 0 {
			return fmt.Errorf("missing metric %q with dimensions %+v", metricName, want.dimensions)
		}
		if !want.shouldExist && len(values) > 0 {
			return fmt.Errorf("found invalid metric %q with dimensions %+v", metricName, want.dimensions)
		}
	}
	return nil
}

func (t *DropOriginalMetricsTestRunner) testCases() map[string][]expectation {
	return map[string][]expectation{
		"None": {
			{
				shouldExist: true,
				metricNames: []string{"swap_free", "swap_used"},
				dimensions:  []dimension.Instruction{},
			},
			{
				shouldExist: true,
				metricNames: []string{"swap_free", "swap_used"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
				},
			},
			{
				shouldExist: true,
				metricNames: []string{"swap_free", "swap_used"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
					{"InstanceType", dimension.UnknownDimensionValue()},
				},
			},
		},
		"StandardWithRename": {
			{
				shouldExist: true,
				metricNames: []string{"cpu_usage_visitor", "cpu_usage_idle", "cpu_usage_user"},
				dimensions:  []dimension.Instruction{},
			},
			{
				shouldExist: true,
				metricNames: []string{"cpu_usage_visitor", "cpu_usage_idle", "cpu_usage_user"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
				},
			},
			{
				shouldExist: true,
				metricNames: []string{"cpu_usage_visitor", "cpu_usage_idle", "cpu_usage_user"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
					{"InstanceType", dimension.UnknownDimensionValue()},
				},
			},
			{
				shouldExist: true,
				metricNames: []string{"cpu_usage_user"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
					{"InstanceType", dimension.UnknownDimensionValue()},
					{"cpu", dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")}},
				},
			},
			{
				shouldExist: false,
				metricNames: []string{"cpu_usage_visitor", "cpu_usage_idle"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
					{"InstanceType", dimension.UnknownDimensionValue()},
					{"cpu", dimension.ExpectedDimensionValue{Value: aws.String("cpu-total")}},
				},
			},
		},
		"Wildcard": {
			{
				shouldExist: true,
				metricNames: []string{"mem_available", "mem_used_percent"},
				dimensions:  []dimension.Instruction{},
			},
			{
				shouldExist: true,
				metricNames: []string{"mem_available", "mem_used_percent"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
				},
			},
			{
				shouldExist: false,
				metricNames: []string{"mem_available", "mem_used_percent"},
				dimensions: []dimension.Instruction{
					{"InstanceId", dimension.UnknownDimensionValue()},
					{"InstanceType", dimension.UnknownDimensionValue()},
				},
			},
		},
	}
}
