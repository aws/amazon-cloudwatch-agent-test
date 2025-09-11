//go:build !windows

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package amp

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	sigv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/suite"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const (
	// template prometheus query for getting average of 3 min
	ampQueryTemplate          = "avg_over_time(%s%s[3m])"
	ampHistogramQueryTemplate = "quantile_over_time(0.5, %s_bucket%s[3m])"
)

type Source int

const (
	SourcePrometheus Source = iota
	SourceOtlp
	SourceHost
)

// NOTE: this should match with append_dimensions under metrics in agent config
var appendDims = map[string]string{
	"d1": "foo",
	"d2": "bar",
}
var awsConfig aws.Config
var awsCreds aws.Credentials
var metadata *environment.MetaData

//go:embed resources/prometheus.yaml
var prometheusConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string
var (
	testRunners []*test_runner.TestRunner = []*test_runner.TestRunner{
		{
			TestRunner: &AmpTestRunner{
				source: SourcePrometheus,
				config: "prometheus_config.json",
				name:   "prometheus",
			},
		},
	}
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}
func TestAmpTestSuite(t *testing.T) {
	suite.Run(t, new(AmpTestSuite))
}

type AMPResponse struct {
	Status string
	Data   AMPResponseData
}
type AMPResponseData struct {
	ResultType string
	Result     []AMPDataResult
}
type AMPDataResult struct {
	Metric map[string]interface{}
	Value  []interface{}
}
type AmpTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *AmpTestSuite) SetupSuite() {
	log.Println(">>>> Starting AMP Test Suite")
}
func (suite *AmpTestSuite) TearDownSuite() {
	suite.Result.Print()
	log.Println(">>>> Finished AMP Test Suite")
}
func (suite *AmpTestSuite) TestAllInSuite() {
	metadata = environment.GetEnvironmentMetaData()
	ctx := context.Background()
	var err error
	awsConfig, err = config.LoadDefaultConfig(ctx, config.WithRegion("us-west-2"))
	if err != nil {
		log.Println("There was an error trying to load default config: ", err)
	}
	awsCreds, err = awsConfig.Credentials.Retrieve(ctx)
	if err != nil {
		log.Println("There was an error trying to load credentials: ", err)
	}
	for _, testRunner := range testRunners {
		suite.AddToSuiteResult(testRunner.Run())
	}
	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "Assume Role Test Suite Failed")
}

type AmpTestRunner struct {
	test_runner.BaseTestRunner
	source Source
	config string
	name   string
}

func (t AmpTestRunner) Validate() status.TestGroupResult {
	if t.source == SourcePrometheus {
		return t.validatePrometheusMetrics()
	} else if t.source == SourceHost {
		return t.validateHostMetrics()
	} else if t.source == SourceOtlp {
		return t.validateOtlpMetrics()
	}
	return status.TestGroupResult{}
}
func (t *AmpTestRunner) validateHostMetrics() status.TestGroupResult {
	// NOTE: dims must match aggregation_dimensions from agent config to fetch metrics.
	// the idea is to fetch all metrics including non-aggregated metrics with matching dim set
	// then validate if the returned list of metrics include metrics (non-aggregated) with append_dimensions as labels
	dims := getDimensions()
	metricsToFetch := t.getHostMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateStandardMetric(metricName, dims, true)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}
func (t *AmpTestRunner) validatePrometheusMetrics() status.TestGroupResult {
	// we do not support appending dimensions to prometheus metrics
	dims := []types.Dimension{}
	standardMetrics, histogramMetrics := t.getPrometheusMetrics()
	testResults := make([]status.TestResult, 0, len(standardMetrics)+len(histogramMetrics))
	for _, metricName := range standardMetrics {
		testResults = append(testResults, t.validateStandardMetric(metricName, dims, false))
	}
	for _, metricName := range histogramMetrics {
		testResults = append(testResults, t.validateHistogramMetric(metricName))
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}
func (t *AmpTestRunner) validateOtlpMetrics() status.TestGroupResult {
	dims := getDimensions()
	standardMetrics, histogramMetrics := t.getOtlpMetrics()
	testResults := make([]status.TestResult, len(standardMetrics)+len(histogramMetrics))
	for i, metricName := range standardMetrics {
		testResults[i] = t.validateStandardMetric(metricName, dims, false)
	}
	for i, metricName := range histogramMetrics {
		testResults[i+len(standardMetrics)] = t.validateHistogramMetric(metricName)
	}
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}
func (t *AmpTestRunner) validateStandardMetric(metricName string, dims []types.Dimension, shouldHaveAppendDimensions bool) status.TestResult {
	return t.validateMetric(ampQueryTemplate, metricName, dims, shouldHaveAppendDimensions)
}
func (t *AmpTestRunner) validateHistogramMetric(metricName string) status.TestResult {
	return t.validateMetric(ampHistogramQueryTemplate, metricName, []types.Dimension{}, false)
}
func (t *AmpTestRunner) validateMetric(queryTemplate string, metricName string, dims []types.Dimension, shouldHaveAppendDimensions bool) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	query := buildPrometheusQuery(queryTemplate, metricName, dims)
	log.Printf("[DEBUG] Query built for metric %s: %s", metricName, query)

	responseJSON, err := queryAMPMetrics(metadata.AmpWorkspaceId, query)
	if err != nil {
		log.Printf("[DEBUG] Failed to fetch metric values from AMP for %s: %s", metricName, err)
		return testResult
	}

	log.Printf("[DEBUG] AMP Response for %s: %+v", metricName, responseJSON)

	if len(responseJSON.Data.Result) == 0 {
		log.Printf("[DEBUG] No AMP metric values returned for %s", metricName)
		return testResult
	}

	foundAppendDimMetric := true
	metricVals := []float64{}

	for _, dataResult := range responseJSON.Data.Result {
		log.Printf("[DEBUG] DataResult: Metric=%v Value=%v", dataResult.Metric, dataResult.Value)

		if len(dataResult.Value) == 0 {
			continue
		}

		val, parseErr := strconv.ParseFloat(dataResult.Value[1].(string), 64)
		if parseErr != nil {
			log.Printf("[DEBUG] Failed to parse metric value: %v", parseErr)
			continue
		}

		metricVals = append(metricVals, val)

		if len(dataResult.Metric) > len(dims) {
			foundAppendDimMetric = foundAppendDimMetric && matchDimensions(dataResult.Metric)
			log.Printf("[DEBUG] foundAppendDimMetric updated to %v", foundAppendDimMetric)
		}
	}

	log.Printf("[DEBUG] Collected metric values for %s: %v", metricName, metricVals)

	if shouldHaveAppendDimensions {
		if len(metricVals) < 2 {
			log.Println("[DEBUG] Failed: fewer metric values than expected")
			return testResult
		}
		if !foundAppendDimMetric {
			log.Println("[DEBUG] Failed: missing append_dimensions")
			return testResult
		}
	} else {
		if len(metricVals) == 0 {
			log.Println("[DEBUG] Failed: no metric values found")
			return testResult
		}
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, metricVals, 0) {
		log.Println("[DEBUG] Metric values did not meet expected threshold")
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	log.Printf("[DEBUG] Metric %s validation successful", metricName)
	return testResult
}

func queryAMPMetrics(wsID string, q string) (AMPResponse, error) {
	log.Printf("[DEBUG] Querying AMP: workspace=%s, query=%s", wsID, q)
	url := fmt.Sprintf("https://aps-workspaces.%s.amazonaws.com/workspaces/%s/api/v1/query?query=%s", awsConfig.Region, wsID, q)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		log.Printf("[DEBUG] Failed to create AMP request: %v", err)
		return AMPResponse{}, fmt.Errorf("failed to create AMP request: %w", err)
	}

	signer := sigv4.NewSigner()
	err = signer.SignHTTP(context.Background(), awsCreds, req, hex.EncodeToString(sha256.New().Sum(nil)), "aps", awsConfig.Region, time.Now().UTC())
	if err != nil {
		log.Printf("[DEBUG] Failed to sign AMP request: %v", err)
		return AMPResponse{}, fmt.Errorf("failed to sign AMP request: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[DEBUG] Failed to send AMP request: %v", err)
		return AMPResponse{}, fmt.Errorf("failed to send AMP request: %w", err)
	}

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("[DEBUG] Failed to read AMP response: %v", err)
		return AMPResponse{}, fmt.Errorf("failed to read AMP response: %w", err)
	}

	var responseJSON AMPResponse
	err = json.Unmarshal(bytes, &responseJSON)
	if err != nil {
		log.Printf("[DEBUG] Failed to unmarshal AMP response: %v", err)
		return AMPResponse{}, fmt.Errorf("failed to unmarshal AMP response: %w", err)
	}

	log.Printf("[DEBUG] AMP response unmarshaled successfully")
	return responseJSON, nil
}
func (t AmpTestRunner) GetTestName() string {
	return t.name
}
func (t AmpTestRunner) GetAgentConfigFileName() string {
	return t.config
}
func (t AmpTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
}
func (t AmpTestRunner) getHostMetrics() []string {
	return []string{
		"CPU_USAGE_IDLE", "cpu_usage_nice", "cpu_usage_guest", "cpu_time_active", "cpu_usage_active",
		"processes_blocked", "processes_dead", "processes_idle", "processes_paging", "processes_running",
		"processes_sleeping", "processes_stopped", "processes_total", "processes_total_threads", "processes_zombies",
		//"jvm.threads.count", "jvm.memory.heap.used", "jvm.memory.heap.max", "jvm.memory.heap.init",
	}
}
func (t AmpTestRunner) getPrometheusMetrics() ([]string, []string) {
	return []string{
			"prometheus_test_counter",
			"prometheus_test_summary",
		}, []string{
			"prometheus_test_histogram",
		}
}
func (t AmpTestRunner) getOtlpMetrics() ([]string, []string) {
	return []string{
			"my_gauge",
			"my_cumulative_counter",
			"my_delta_counter",
		}, []string{
			"my_cumulative_histogram",
			"my_delta_histogram",
		}
}
func (t AmpTestRunner) SetupAfterAgentRun() error {
	// OTLP source has some special setup after the agent starts
	if t.source == SourceOtlp {
		return setupOtlp()
	}
	return nil
}
func setupPrometheus() error {
	startPrometheusCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo pkill -f 'python3 -m http.server 8101'", // kill any old python servers
		"nohup sudo python3 -m http.server 8101 --bind :: >/tmp/prometheus.log 2>&1 &",
		"sleep 2", // wait a bit for the server to start
	}
	return common.RunCommands(startPrometheusCommands)
}
func setupOtlp() error {
	return common.RunAsyncCommand("resources/otlp_pusher.sh")
}
func getDimensions() []types.Dimension {
	factory := dimension.GetDimensionFactory(*metadata)
	dims, failed := factory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "InstanceType",
			Value: dimension.UnknownDimensionValue(),
		},
	})
	if len(failed) > 0 {
		return []types.Dimension{}
	}
	return dims
}
func buildPrometheusQuery(template string, metricName string, dims []types.Dimension) string {
	dimsStr := ""
	for _, dim := range dims {
		dimsStr = fmt.Sprintf("%s%s=\"%s\", ", dimsStr, *dim.Name, *dim.Value)
	}
	if len(dimsStr) > 0 {
		dimsStr = dimsStr[:len(dimsStr)-1]
	}
	return fmt.Sprintf(template, metricName, "{"+dimsStr+"}")
}

func matchDimensions(labels map[string]interface{}) bool {
	log.Printf("[DEBUG] Matching appendDims=%v against labels=%v", appendDims, labels)
	if len(appendDims) > len(labels) {
		log.Println("[DEBUG] Labels length less than appendDims length, returning false")
		return false
	}
	for k, v := range appendDims {
		if lv, found := labels[k]; !found || lv != v {
			log.Printf("[DEBUG] Dimension mismatch for key %s: expected=%v got=%v", k, v, lv)
			return false
		}
	}
	return true
}

func (t AmpTestRunner) SetupBeforeAgentRun() error {
	log.Println("[DEBUG] SetupBeforeAgentRun started")
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		log.Printf("[DEBUG] BaseTestRunner.SetupBeforeAgentRun failed: %v", err)
		return err
	}

	agentConfigPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	log.Printf("[DEBUG] Modifying agent config at %s", agentConfigPath)
	ampCommands := []string{
		"sed -ie 's/{workspace_id}/" + metadata.AmpWorkspaceId + "/g' " + agentConfigPath,
	}
	err = common.RunCommands(ampCommands)
	if err != nil {
		log.Printf("[DEBUG] Failed to modify agent config: %v", err)
		return fmt.Errorf("failed to modify agent configuration: %w", err)
	}

	if t.source == SourcePrometheus {
		log.Println("[DEBUG] Setting up Prometheus before agent run")
		err = setupPrometheus()
		if err != nil {
			log.Printf("[DEBUG] Failed to setup Prometheus: %v", err)
			return fmt.Errorf("failed to setup prometheus: %w", err)
		}
	}

	return t.SetUpConfig()
}

var _ test_runner.ITestRunner = (*AmpTestRunner)(nil)
