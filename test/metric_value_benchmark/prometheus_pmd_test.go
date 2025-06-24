package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type PrometheusPMDTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*PrometheusPMDTestRunner)(nil)

//go:embed agent_configs/prometheus.yaml
var prometheusPMDConfig string

const prometheusPMDMetrics = `prometheus_test_untyped{include="yes",prom_type="untyped"} 1
# TYPE prometheus_test_counter counter
prometheus_test_counter{include="yes",prom_type="counter"} 1
# TYPE prometheus_test_counter_exclude counter
prometheus_test_counter_exclude{include="no",prom_type="counter"} 1
# TYPE prometheus_test_gauge gauge
prometheus_test_gauge{include="yes",prom_type="gauge"} 500
# TYPE prometheus_test_histogram histogram
prometheus_test_histogram_sum{include="yes",prom_type="histogram"} 300
prometheus_test_histogram_count{include="yes",prom_type="histogram"} 75
prometheus_test_histogram_bucket{include="yes",le="0",prom_type="histogram"} 1
prometheus_test_histogram_bucket{include="yes",le="0.5",prom_type="histogram"} 2
prometheus_test_histogram_bucket{include="yes",le="2.5",prom_type="histogram"} 3
prometheus_test_histogram_bucket{include="yes",le="5",prom_type="histogram"} 4
prometheus_test_histogram_bucket{include="yes",le="+Inf",prom_type="histogram"} 5`

const namespacePMD = "PMDTest"

func (t *PrometheusPMDTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validatePMDMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *PrometheusPMDTestRunner) GetTestName() string {
	return "PMD Prometheus Metrics"
}

func (t *PrometheusPMDTestRunner) GetAgentConfigFileName() string {
	return "pmd_config.json"
}

func (t *PrometheusPMDTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	setupCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus.yaml\n%s\nEOF", prometheusPMDConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusPMDMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	err = common.RunCommands(setupCommands)
	if err != nil {
		return err
	}
	return nil
}

func (t *PrometheusPMDTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_untyped",
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_histogram_sum",
		"prometheus_test_histogram_count",
		"prometheus_test_histogram_bucket",
	}
}

func (t *PrometheusPMDTestRunner) validatePMDMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	var dims []types.Dimension
	var failed []dimension.Instruction

	// Set up dimensions based on metric type
	switch metricName {
	case "prometheus_test_untyped":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("untyped")},
			},
		})
	case "prometheus_test_counter":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("counter")},
			},
		})
	case "prometheus_test_gauge":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("gauge")},
			},
		})
	case "prometheus_test_histogram_sum", "prometheus_test_histogram_count":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("histogram")},
			},
		})
	case "prometheus_test_histogram_bucket":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{aws.String("histogram")},
			},
			{
				Key:   "le",
				Value: dimension.ExpectedDimensionValue{aws.String("0.5")},
			},
		})
	}

	if len(failed) > 0 {
		return testResult
	}

	instanceDims, instanceFailed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.ExpectedDimensionValue{aws.String(awsservice.GetInstanceId())},
		},
	})

	if len(instanceFailed) > 0 {
		return testResult
	}

	dims = append(dims, instanceDims...)

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespacePMD, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
