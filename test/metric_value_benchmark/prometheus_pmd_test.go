package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type PrometheusPMDTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*PrometheusPMDTestRunner)(nil)

//go:embed agent_configs/prometheus.yaml
var prometheusPMDConfig string

const namespacePMD = "PMDTest6"

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

func (t *PrometheusPMDTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}
func (t *PrometheusPMDTestRunner) GetAgentConfigFileName() string {
	return "prometheus_pmd_config.json"
}

func (t *PrometheusPMDTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	setupCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus.yaml\n%s\nEOF", prometheusPMDConfig),
	}

	err = common.RunCommands(setupCommands)
	if err != nil {
		return err
	}

	// Start the Prometheus generator
	err = t.startPrometheusGenerator()
	if err != nil {
		return fmt.Errorf("failed to start prometheus generator: %v", err)
	}

	time.Sleep(5 * time.Second)

	return nil
}
func (t *PrometheusPMDTestRunner) startPrometheusGenerator() error {
	cmd := "go run ../../cmd/prometheus-generator -port=8101"
	return common.RunAsyncCommand(cmd)
}
func (t *PrometheusPMDTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_untyped",
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_histogram",
	}
}

func (t *PrometheusPMDTestRunner) validatePMDMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.ExpectedDimensionValue{aws.String(awsservice.GetInstanceId())},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}

	// Get different statistics for the metric
	stats := []metric.Statistics{metric.AVERAGE, metric.MAXIMUM, metric.MINIMUM, metric.SUM, metric.SAMPLE_COUNT}
	statValues := make(map[metric.Statistics][]float64)

	for _, stat := range stats {
		values, err := fetcher.Fetch(namespacePMD, metricName, dims, stat, metric.HighResolutionStatPeriod)
		if err != nil {
			log.Printf("Failed to fetch %s for metric %s: %v", stat, metricName, err)
			return testResult
		}
		statValues[stat] = values
	}

	switch metricName {
	case "prometheus_test_histogram":
		// For histogram, values should be between 0-10 (our generation range)
		if len(statValues[metric.MAXIMUM]) > 0 && statValues[metric.MAXIMUM][0] > 10 {
			log.Printf("Histogram max value too high: %v", statValues[metric.MAXIMUM][0])
			return testResult
		}
		if len(statValues[metric.MINIMUM]) > 0 && statValues[metric.MINIMUM][0] < 0 {
			log.Printf("Histogram min value negative: %v", statValues[metric.MINIMUM][0])
			return testResult
		}

	case "prometheus_test_gauge":
		// For gauge, values should be between 0-1000
		if len(statValues[metric.MAXIMUM]) > 0 && statValues[metric.MAXIMUM][0] > 1000 {
			log.Printf("Gauge max value too high: %v", statValues[metric.MAXIMUM][0])
			return testResult
		}
		if len(statValues[metric.MINIMUM]) > 0 && statValues[metric.MINIMUM][0] < 0 {
			log.Printf("Gauge min value negative: %v", statValues[metric.MINIMUM][0])
			return testResult
		}

	case "prometheus_test_counter":
		avgValues := statValues[metric.AVERAGE]
		for i := 1; i < len(avgValues); i++ {
			if avgValues[i] < avgValues[i-1] {
				log.Printf("Counter not monotonically increasing: %v", avgValues)
				return testResult
			}
		}
	}

	log.Printf("Metric %s statistics:", metricName)
	for stat, values := range statValues {
		if len(values) > 0 {
			log.Printf("  %v: latest=%v, count=%d", stat, values[len(values)-1], len(values))
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
