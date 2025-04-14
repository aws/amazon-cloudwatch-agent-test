package emf_prometheus

import (
	_ "embed"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/prometheus.yaml
var prometheusConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string

const (
	prometheusNamespace = "PrometheusEMFTest"
	testDuration        = 5 * time.Minute
)

func TestPrometheusEMFTestSuite(t *testing.T) {
	suite.Run(t, new(PrometheusEMFTestSuite))
}

type PrometheusEMFTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *PrometheusEMFTestSuite) SetupSuite() {
	log.Println(">>>> Starting Prometheus EMF Test Suite")
}

func (suite *PrometheusEMFTestSuite) TearDownSuite() {
	suite.Result.Print()
	log.Println(">>>> Finished Prometheus EMF Test Suite")
}

var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &PrometheusEMFTestRunner{},
		},
	}
)

func (suite *PrometheusEMFTestSuite) TestAllInSuite() {
	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Prometheus EMF Test Suite Failed")
}

type PrometheusEMFTestRunner struct {
	test_runner.BaseTestRunner
	testName string
}

func (t *PrometheusEMFTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{
		t.validateUntypedMetricAbsence(),
		t.validateOtherMetricsPresence(),
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *PrometheusEMFTestRunner) validateUntypedMetricAbsence() status.TestResult {
	testResult := status.TestResult{
		Name:   "UntypedMetricAbsence",
		Status: status.FAILED,
	}

	// Wait for metrics to be published
	time.Sleep(2 * time.Minute)

	dims := []types.Dimension{
		{
			Name:  aws.String("prom_type"),
			Value: aws.String("untyped"),
		},
	}

	valueFetcher := metric.MetricValueFetcher{}
	_, err := valueFetcher.Fetch(prometheusNamespace, "prometheus_test_untyped", dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
	if err == nil {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *PrometheusEMFTestRunner) validateOtherMetricsPresence() status.TestResult {
	testResult := status.TestResult{
		Name:   "OtherMetricsPresence",
		Status: status.FAILED,
	}

	metricsToCheck := []struct {
		name     string
		promType string
	}{
		{"prometheus_test_counter", "counter"},
		{"prometheus_test_gauge", "gauge"},
		{"prometheus_test_summary_sum", "summary"},
		{"prometheus_test_histogram_sum", "histogram"},
	}

	valueFetcher := metric.MetricValueFetcher{}

	for _, m := range metricsToCheck {
		dims := []types.Dimension{
			{
				Name:  aws.String("prom_type"),
				Value: aws.String(m.promType),
			},
		}

		values, err := valueFetcher.Fetch(prometheusNamespace, m.name, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
		if err != nil {
			return testResult
		}

		if len(values) == 0 {
			return testResult
		}

		if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(m.name, values, 0) {
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t *PrometheusEMFTestRunner) GetTestName() string {
	return "PrometheusEMFValidation"
}

func (t *PrometheusEMFTestRunner) GetAgentConfigFileName() string {
	return "prometheus_emf_config.json"
}

func (t *PrometheusEMFTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_summary_sum",
		"prometheus_test_histogram_sum",
		"prometheus_test_untyped",
	}
}
func (t *PrometheusEMFTestRunner) SetupBeforeAgentRun() error {
	err := setupPrometheus()
	if err != nil {
		return fmt.Errorf("failed to setup prometheus: %w", err)
	}

	return t.SetUpConfig()
}

func setupPrometheus() error {
	startPrometheusCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	return common.RunCommands(startPrometheusCommands)
}
