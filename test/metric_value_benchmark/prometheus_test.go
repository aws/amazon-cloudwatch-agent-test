// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

type PrometheusTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*PrometheusTestRunner)(nil)

//go:embed agent_configs/prometheus.yaml
var prometheusConfig string

const (
	// prometheusExporterPort must match the scrape target in agent_configs/prometheus.yaml.
	prometheusExporterPort = 8101
	// prometheusLogGroup must match log_group_name in agent_configs/prometheus_config.json.
	// The agent writes EMF events here and CloudWatch extracts the metrics from them,
	// so its presence separates "agent never published" from "CloudWatch has not
	// surfaced the metrics yet" when validation fails.
	prometheusLogGroup = "prometheus_test"
	// prometheusRevalidateInterval is deliberately shorter than the shared
	// metric.RevalidateInterval (60s), which was sized for the EKS Container
	// Insights suite's ~195 (metric, dimension-set) pairs. This sub-test validates
	// five metrics from a single EMF event, so five samples 30s apart cover the same
	// two minutes of propagation lateness at half the worst-case cost.
	prometheusRevalidateInterval = 30 * time.Second
)

const prometheusMetrics = `prometheus_test_untyped{include="yes",prom_type="untyped"} 1
# TYPE prometheus_test_counter counter
prometheus_test_counter{include="yes",prom_type="counter"} 1
# TYPE prometheus_test_counter_exclude counter
prometheus_test_counter_exclude{include="no",prom_type="counter"} 1
# TYPE prometheus_test_gauge gauge
prometheus_test_gauge{include="yes",prom_type="gauge"} 500
# TYPE prometheus_test_summary summary
prometheus_test_summary_sum{include="yes",prom_type="summary"} 200
prometheus_test_summary_count{include="yes",prom_type="summary"} 50
prometheus_test_summary{include="yes",quantile="0",prom_type="summary"} 0.1
prometheus_test_summary{include="yes",quantile="0.5",prom_type="summary"} 0.25
prometheus_test_summary{include="yes",quantile="1",prom_type="summary"} 5.5
# TYPE prometheus_test_histogram histogram
prometheus_test_histogram_sum{include="yes",prom_type="histogram"} 300
prometheus_test_histogram_count{include="yes",prom_type="histogram"} 75
prometheus_test_histogram_bucket{include="yes",le="0",prom_type="histogram"} 1
prometheus_test_histogram_bucket{include="yes",le="0.5",prom_type="histogram"} 2
prometheus_test_histogram_bucket{include="yes",le="2.5",prom_type="histogram"} 3
prometheus_test_histogram_bucket{include="yes",le="5",prom_type="histogram"} 4
prometheus_test_histogram_bucket{include="yes",le="+Inf",prom_type="histogram"} 5
`

func (t *PrometheusTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))

	// Re-validate with bounded retries. CloudWatch does not always surface
	// EMF-extracted metrics by the time the first query runs, so a single query
	// fails with "No values found" even when the agent published correctly.
	ok := metric.RetryValidation(metric.RevalidateMaxAttempts, prometheusRevalidateInterval, func() bool {
		failed := 0
		for i, metricName := range metricsToFetch {
			testResults[i] = t.validatePrometheusMetric(metricName)
			if testResults[i].Status == status.FAILED {
				failed++
				log.Printf("Validation of %s failed: %v", metricName, testResults[i].Reason)
			}
		}
		return failed == 0
	})
	if !ok {
		logPrometheusDiagnostics()
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

// logPrometheusDiagnostics records the state that distinguishes the possible
// causes of a "No values found" failure: whether the agent ever published EMF
// events, whether the scrape target was still serving, and what the agent logged.
func logPrometheusDiagnostics() {
	log.Print("Prometheus validation failed; collecting diagnostics")

	// If the log group is absent, the agent never published and the problem is
	// upstream of CloudWatch metric extraction.
	log.Printf("EMF log group %q exists: %t", prometheusLogGroup, awsservice.IsLogGroupExists(prometheusLogGroup))

	diagnostics := []struct {
		label   string
		command string
	}{
		{"scrape target response", fmt.Sprintf(
			"curl -s -o /dev/null -w 'HTTP %%{http_code}' http://localhost:%d/metrics || true",
			prometheusExporterPort)},
		{"listening sockets", fmt.Sprintf(
			"sudo ss -lntp 2>/dev/null | grep ':%d' || echo 'nothing listening on %d'",
			prometheusExporterPort, prometheusExporterPort)},
		// prometheus_config.json sets "logfile": "", so the agent logs to stderr
		// rather than common.AgentLogFile. journalctl is the only place to read it.
		{"agent log (errors)",
			"sudo journalctl -u amazon-cloudwatch-agent --no-pager 2>/dev/null " +
				"| grep -iE 'error|prometheus|emf' | tail -50 || true"},
	}
	for _, d := range diagnostics {
		out, err := common.RunCommand(d.command)
		if err != nil {
			log.Printf("%s: could not collect (%v)", d.label, err)
			continue
		}
		log.Printf("%s:\n%s", d.label, out)
	}
}

func (t *PrometheusTestRunner) GetTestName() string {
	return "Prometheus"
}

func (t *PrometheusTestRunner) GetAgentConfigFileName() string {
	return "prometheus_config.json"
}

// GetAgentRunDuration overrides the 30s default. 30s yields only a single
// datapoint per metric at a 10s stat period, so any propagation lag at all is
// enough to fail validation. Two minutes yields roughly a dozen, which buys
// margin against propagation rather than extra scrape coverage (the delta-based
// counter and summary metrics need only two scrapes to report).
func (t *PrometheusTestRunner) GetAgentRunDuration() time.Duration {
	return 2 * time.Minute
}

func (t *PrometheusTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	// Quoted heredoc: the config must be written verbatim, with no parameter or
	// command substitution.
	writeConfig := []string{
		fmt.Sprintf("cat <<'EOF' | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
	}
	if err = common.RunCommands(writeConfig); err != nil {
		return err
	}

	// Serve the exposition from an in-process Go server rather than shelling out
	// to `python3 -m http.server --directory /tmp &> /dev/null &`. The old form
	// discarded startup errors and always reported success because backgrounding
	// makes the shell exit 0, so a target that never bound looked identical to a
	// healthy one until validation failed. net.Listen here fails synchronously,
	// and the listener is bound before this returns, so no readiness poll is
	// needed. It also drops the --directory flag, which requires Python 3.7+ and
	// is unavailable on the EL8 hosts in this matrix.
	stop, err := common.StartPrometheusFakeServer(prometheusExporterPort, prometheusMetrics)
	if err != nil {
		return fmt.Errorf("failed to start prometheus scrape target: %w", err)
	}
	t.RegisterCleanup(func() error { stop(); return nil })

	return nil
}

func (t *PrometheusTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_summary_count",
		"prometheus_test_summary_sum",
		"prometheus_test_summary",
	}
}

func (t *PrometheusTestRunner) validatePrometheusMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	var dims []types.Dimension
	var failed []dimension.Instruction

	switch metricName {
	case "prometheus_test_counter":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("counter")},
			},
		})
	case "prometheus_test_gauge":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("gauge")},
			},
		})
	case "prometheus_test_summary_count":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("summary")},
			},
		})
	case "prometheus_test_summary_sum":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("summary")},
			},
		})
	case "prometheus_test_summary":
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{
			{
				Key:   "prom_type",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("summary")},
			},
			{
				Key:   "quantile",
				Value: dimension.ExpectedDimensionValue{Value: aws.String("0.5")},
			},
		})
	default:
		dims, failed = t.DimensionFactory.GetDimensions([]dimension.Instruction{})
	}

	if len(failed) > 0 {
		testResult.Reason = fmt.Errorf("could not resolve dimensions for %s", metricName)
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		// Record the error rather than discarding it. Without this, a
		// GetMetricData throttling or credential failure is indistinguishable
		// from the metric genuinely having no datapoints yet, which makes the
		// difference between a test-timing problem and an agent problem
		// impossible to tell apart from the job output.
		testResult.Reason = fmt.Errorf("fetching %s: %w", metricName, err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		testResult.Reason = fmt.Errorf("no acceptable values for %s (got %v)", metricName, values)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
