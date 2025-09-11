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
	log.Printf("querying metrics for %s using the following query\n%s\n", metricName, query)
	responseJSON, err := queryAMPMetrics(metadata.AmpWorkspaceId, query)
	if err != nil {
		log.Printf("failed to fetch metric values from AMP for %s: %s\n", metricName, err)
		return testResult
	}
	if len(responseJSON.Data.Result) == 0 {
		log.Printf("failed because AMP metric values are missing for %s\n", metricName)
		return testResult
	}
	foundAppendDimMetric := true
	metricVals := []float64{}
	for _, dataResult := range responseJSON.Data.Result {
		if len(dataResult.Value) == 0 {
			continue
		}
		// metric value is returned as a tuple of timestamp and value (ec. '"value": [1721843955, "26"]')
		val, _ := strconv.ParseFloat(dataResult.Value[1].(string), 64)
		metricVals = append(metricVals, val)
		// metrics with more labels than fetched dims must be non-aggregated metrics which include append_dimensions as labels
		if len(dataResult.Metric) > len(dims) {
			foundAppendDimMetric = foundAppendDimMetric && matchDimensions(dataResult.Metric)
		}
	}
	// AMP/Prometheus metrics do not have the appended dimensions
	if shouldHaveAppendDimensions {
		// at least 2 metrics are expected with 1 set of aggregation_dimensions
		// 1 non-aggregated + 1 aggregated minimum
		if len(metricVals) < 2 {
			log.Println("failed with fewer metric values than expected")
			return testResult
		}
		if !foundAppendDimMetric {
			log.Println("failed with missing append_dimensions")
			return testResult
		}
	} else {
		if len(metricVals) == 0 {
			log.Println("failed with fewer metric values than expected")
			return testResult
		}
	}
	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, metricVals, 0) {
		return testResult
	}
	testResult.Status = status.SUCCESSFUL
	return testResult
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
func (t AmpTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}
	// replace AMP workspace ID placeholder with a testing workspace ID from metadata
	agentConfigPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	ampCommands := []string{
		"sed -ie 's/{workspace_id}/" + metadata.AmpWorkspaceId + "/g' " + agentConfigPath,
		// use below to add JMX metrics then update agent config & GetMeasuredMetrics()
		//"nohup java -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=2030 -Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -Dcom.sun.management.jmxremote.rmi.port=2030 -Dcom.sun.management.jmxremote.host=0.0.0.0 -Djava.rmi.server.hostname=0.0.0.0 -Dserver.port=8090 -Dspring.application.admin.enabled=true -jar jars/spring-boot-web-starter-tomcat.jar > /tmp/spring-boot-web-starter-tomcat-jar.txt 2>&1 &",
	}
	err = common.RunCommands(ampCommands)
	if err != nil {
		return fmt.Errorf("failed to modify agent configuration: %w", err)
	}
	// Prometheus source has some special setup before the agent starts
	if t.source == SourcePrometheus {
		err = setupPrometheus()
		if err != nil {
			return fmt.Errorf("failed to setup prometheus: %w", err)
		}
	}
	return t.SetUpConfig()
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
		"sudo python3 -m http.server 8101 --bind :: &> /dev/null &", // Changed to bind on IPv6
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
func queryAMPMetrics(wsID string, q string) (AMPResponse, error) {
	url := fmt.Sprintf("https://aps-workspaces.%s.amazonaws.com/workspaces/%s/api/v1/query?query=%s", awsConfig.Region, wsID, q)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return AMPResponse{}, fmt.Errorf("failed to create AMP request: %w", err)
	}
	signer := sigv4.NewSigner()
	err = signer.SignHTTP(context.Background(), awsCreds, req, hex.EncodeToString(sha256.New().Sum(nil)), "aps", awsConfig.Region, time.Now().UTC())
	if err != nil {
		return AMPResponse{}, fmt.Errorf("failed to sign AMP request: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return AMPResponse{}, fmt.Errorf("failed to send AMP request: %w", err)
	}
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return AMPResponse{}, fmt.Errorf("failed to read AMP response: %w", err)
	}
	var responseJSON AMPResponse
	err = json.Unmarshal(bytes, &responseJSON)
	if err != nil {
		return AMPResponse{}, fmt.Errorf("failed to unmarshal AMP response: %w", err)
	}
	return responseJSON, nil
}
func matchDimensions(labels map[string]interface{}) bool {
	if len(appendDims) > len(labels) {
		return false
	}
	for k, v := range appendDims {
		if lv, found := labels[k]; !found || lv != v {
			return false
		}
	}
	return true
}

var _ test_runner.ITestRunner = (*AmpTestRunner)(nil)
