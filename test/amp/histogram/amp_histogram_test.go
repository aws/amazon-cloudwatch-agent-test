// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package amp

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sigv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	// template prometheus query for getting average of 3 min
	ampQueryTemplate          = "avg_over_time(%s%s[3m])"
	ampHistogramQueryTemplate = "quantile_over_time(0.5, %s_bucket[3m])"
)

type Source int

const (
	SourcePrometheus Source = iota
	SourceOtlp
	SourceHost
)

// NOTE: this should match with append_dimensions under metrics in agent config
var append_dims = map[string]string{
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
				source: SourceHost,
				config: "host_config.json",
				name:   "host",
			},
		},
		{
			TestRunner: &AmpTestRunner{
				source: SourcePrometheus,
				config: "prometheus_config.json",
				name:   "prometheus",
			},
		},
		{
			TestRunner: &AmpTestRunner{
				source: SourceOtlp,
				config: "otlp_config.json",
				name:   "otlp",
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
		fmt.Println("There was an error trying to load default config: ", err)
	}
	awsCreds, err = awsConfig.Credentials.Retrieve(ctx)
	if err != nil {
		fmt.Println("There was an error trying to load credentials: ", err)
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
	// wait for agent to push some metrics
	time.Sleep(30 * time.Second)

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

	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName, dims, true)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AmpTestRunner) validatePrometheusMetrics() status.TestGroupResult {

	dims := []types.Dimension{}

	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch)+1)
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName, dims, false)
	}

	query := buildPrometheusHistogramQuery("prometheus_test_histogram")
	fmt.Printf("query: %s\n", query)
	res, err := queryAMPMetrics(metadata.AmpWorkspaceId, query)
	if err != nil {
		fmt.Printf("failed to fetch metric values from AMP for %s: %s\n", "prometheus_test_histogram", err)
	}
	fmt.Printf("res: %s\n", res)
	var responseJson AMPResponse
	err = json.Unmarshal(res, &responseJson)
	if err != nil {
		fmt.Printf("failed to unmarshal AMP response: %s\n", err)
	}

	if len(responseJson.Data.Result) == 0 {
		fmt.Printf("AMP metric values are missing for %s\n", "prometheus_test_histogram")
	}

	fmt.Printf("%+v\n", responseJson)

	metricVals := []float64{}
	for _, dataResult := range responseJson.Data.Result {
		if len(dataResult.Value) < 1 {
			continue
		}
		// metric value is returned as a tuple of timestamp and value (ec. '"value": [1721843955, "26"]')
		val, _ := strconv.ParseFloat(dataResult.Value[1].(string), 64)
		metricVals = append(metricVals, val)
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue("prometheus_test_histogram", metricVals, 0) {
		testResults[len(testResults)-1] = status.TestResult{
			Name:   "prometheus_test_histogram",
			Status: status.FAILED,
		}
	} else {
		testResults[len(testResults)-1] = status.TestResult{
			Name:   "prometheus_test_histogram",
			Status: status.SUCCESSFUL,
		}
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AmpTestRunner) validateOtlpMetrics() status.TestGroupResult {
	dims := []types.Dimension{}

	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch)+1)
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName, dims, false)
	}

	query := buildPrometheusHistogramQuery("my_cumulative_histogram")
	fmt.Printf("query: %s\n", query)
	res, err := queryAMPMetrics(metadata.AmpWorkspaceId, query)
	if err != nil {
		fmt.Printf("failed to fetch metric values from AMP for %s: %s\n", "my_cumulative_histogram", err)
	}
	fmt.Printf("res: %s\n", res)
	var responseJson AMPResponse
	err = json.Unmarshal(res, &responseJson)
	if err != nil {
		fmt.Printf("failed to unmarshal AMP response: %s\n", err)
	}

	if len(responseJson.Data.Result) == 0 {
		fmt.Printf("AMP metric values are missing for %s\n", "my_cumulative_histogram")
	}

	fmt.Printf("%+v\n", responseJson)

	metricVals := []float64{}
	for _, dataResult := range responseJson.Data.Result {
		if len(dataResult.Value) < 1 {
			continue
		}
		// metric value is returned as a tuple of timestamp and value (ec. '"value": [1721843955, "26"]')
		val, _ := strconv.ParseFloat(dataResult.Value[1].(string), 64)
		metricVals = append(metricVals, val)
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue("my_cumulative_histogram", metricVals, 0) {
		testResults[len(testResults)-1] = status.TestResult{
			Name:   "my_cumulative_histogram",
			Status: status.FAILED,
		}
	} else {
		testResults[len(testResults)-1] = status.TestResult{
			Name:   "my_cumulative_histogram",
			Status: status.SUCCESSFUL,
		}
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AmpTestRunner) validateMetric(metricName string, dims []types.Dimension, shouldHaveAppendDimensions bool) status.TestResult {

	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	query := buildPrometheusQuery(metricName, dims)
	fmt.Printf("query: %s\n", query)
	res, err := queryAMPMetrics(metadata.AmpWorkspaceId, query)
	if err != nil {
		fmt.Printf("failed to fetch metric values from AMP for %s: %s\n", metricName, err)
		return testResult
	}
	fmt.Printf("res: %s\n", res)
	var responseJson AMPResponse
	err = json.Unmarshal(res, &responseJson)
	if err != nil {
		fmt.Printf("failed to unmarshal AMP response: %s\n", err)
		return testResult
	}

	if len(responseJson.Data.Result) == 0 {
		fmt.Printf("AMP metric values are missing for %s\n", metricName)
		return testResult
	}

	foundAppendDimMetric := true
	metricVals := []float64{}
	for _, dataResult := range responseJson.Data.Result {
		if len(dataResult.Value) < 1 {
			continue
		}
		// metric value is returned as a tuple of timestamp and value (ec. '"value": [1721843955, "26"]')
		val, _ := strconv.ParseFloat(dataResult.Value[1].(string), 64)
		metricVals = append(metricVals, val)

		// metrics with more labels than fetched dims must be non-aggregated metrics which include append_dimensions as labels
		if len(dataResult.Metric) > len(dims) {
			// doesnt work for otlp/prometheus where we don't append dimensionsm
			foundAppendDimMetric = foundAppendDimMetric && matchDimensions(dataResult.Metric)
		}
	}

	if shouldHaveAppendDimensions {
		// at least 2 metrics are expected with 1 set of aggregation_dimensions
		// 1 non-aggregated + 1 aggregated minimum
		if len(metricVals) < 2 {
			fmt.Println("failed with fewer metric values than expected")
			return testResult
		}

		if !foundAppendDimMetric {
			fmt.Println("failed with missing append_dimensions")
			return testResult
		}
	} else {
		if len(metricVals) < 1 {
			fmt.Println("failed with fewer metric values than expected")
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
	if t.source == SourcePrometheus {
		return t.getPrometheusMetrics()
	} else if t.source == SourceHost {
		return t.getHostMetrics()
	} else if t.source == SourceOtlp {
		return t.getOtlpMetrics()
	}

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

func (t AmpTestRunner) getPrometheusMetrics() []string {
	return []string{
		"prometheus_test_counter",
		"prometheus_test_summary",
	}
}

func (t AmpTestRunner) getOtlpMetrics() []string {
	return []string{
		"my_gauge",
		"my_cumulative_counter",
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
		//"nohup java -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=2030 -Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -Dcom.sun.management.jmxremote.rmi.port=2030  -Dcom.sun.management.jmxremote.host=0.0.0.0  -Djava.rmi.server.hostname=0.0.0.0 -Dserver.port=8090 -Dspring.application.admin.enabled=true -jar jars/spring-boot-web-starter-tomcat.jar > /tmp/spring-boot-web-starter-tomcat-jar.txt 2>&1 &",
	}
	err = common.RunCommands(ampCommands)
	if err != nil {
		return err
	}

	// Prometheus source has some special setup before the agent starts
	if t.source == SourcePrometheus {
		err = t.setupPrometheus()
		if err != nil {
			return err
		}
	}

	return t.SetUpConfig()
}

func (t AmpTestRunner) SetupAfterAgentRun() error {

	// OTLP source has some special setup after the agent starts
	if t.source == SourceOtlp {
		return common.RunAsyncCommand("resources/otlp_pusher.sh")
	}

	return nil
}

func (t AmpTestRunner) setupPrometheus() error {
	startPrometheusCommands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", prometheusConfig),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	return common.RunCommands(startPrometheusCommands)
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

func buildPrometheusQuery(metricName string, dims []types.Dimension) string {
	dimsStr := ""
	for _, dim := range dims {
		dimsStr = fmt.Sprintf("%s%s=\"%s\", ", dimsStr, *dim.Name, *dim.Value)
	}
	if len(dimsStr) > 0 {
		dimsStr = dimsStr[:len(dimsStr)-1]
	}
	return fmt.Sprintf(ampQueryTemplate, metricName, "{"+dimsStr+"}")
}

func buildPrometheusHistogramQuery(metricName string) string {
	return fmt.Sprintf(ampHistogramQueryTemplate, metricName)
}

func queryAMPMetrics(wsId string, q string) ([]byte, error) {
	url := fmt.Sprintf("https://aps-workspaces.%s.amazonaws.com/workspaces/%s/api/v1/query?query=%s", awsConfig.Region, wsId, q)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	signer := sigv4.NewSigner()
	err = signer.SignHTTP(context.Background(), awsCreds, req, hex.EncodeToString(sha256.New().Sum(nil)), "aps", awsConfig.Region, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(res.Body)
}

func matchDimensions(labels map[string]interface{}) bool {
	if len(append_dims) > len(labels) {
		return false
	}
	for k, v := range append_dims {
		if lv, found := labels[k]; !found || lv != v {
			return false
		}
	}
	return true
}

var _ test_runner.ITestRunner = (*AmpTestRunner)(nil)
