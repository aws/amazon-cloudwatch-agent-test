// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package amp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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
)

type AMPResponse struct {
	Status string
	Data   AMPResponseData
}
type AMPResponseData struct {
	ResultType string
	Result     []DataResult
}
type DataResult struct {
	Metric map[string]interface{}
	Value  []interface{}
}

const (
	namespace = "AMPTest"
	// template prometheus query for getting average of 1 min
	ampQueryTemplate = "avg_over_time(%s%s[3m])"
)

// NOTE: this should match with append_dimensions under metrics in agent config
var append_dims = map[string]string{
	"d1": "foo",
	"d2": "bar",
}

var awsConfig aws.Config
var awsCreds aws.Credentials

func init() {
	environment.RegisterEnvironmentMetaDataFlags()

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
}

type AmpTestRunner struct {
	test_runner.BaseTestRunner
}

func (t AmpTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	time.Sleep(30 * time.Second)
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *AmpTestRunner) validateMetric(metricName string) status.TestResult {
	env := environment.GetEnvironmentMetaData()

	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	// NOTE: dims must match aggregation_dimensions from agent config to fetch metrics.
	// the idea is to fetch all metrics including non-aggregated metrics with matching dim set
	// then validate if the returned list of metrics include metrics (non-aggregated) with append_dimensions as labels
	dims := getDimensions()
	if len(dims) == 0 {
		return testResult
	}

	res, err := queryAMPMetrics(env.AmpWorkspaceId, buildPrometheusQuery(metricName, dims))
	if err != nil {
		fmt.Printf("failed to fetch metric values from AMP for %s: %s\n", metricName, err)
		return testResult
	}
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
			foundAppendDimMetric = foundAppendDimMetric && matchDimensions(dataResult.Metric)
		}
	}

	// at least 2 metrics are expected with 1 set of aggregation_dimensions
	// 1 non-aggregated + 1 aggregated minimum
	if len(metricVals) < 2 || !foundAppendDimMetric {
		fmt.Println("failed with less metric values than expected or missing append_dimensions")
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, metricVals, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t AmpTestRunner) GetTestName() string {
	return namespace
}

func (t AmpTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t AmpTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"CPU_USAGE_IDLE", "cpu_usage_nice", "cpu_usage_guest", "cpu_time_active", "cpu_usage_active",
		"processes_blocked", "processes_dead", "processes_idle", "processes_paging", "processes_running",
		"processes_sleeping", "processes_stopped", "processes_total", "processes_total_threads", "processes_zombies",
		//"jvm.threads.count", "jvm.memory.heap.used", "jvm.memory.heap.max", "jvm.memory.heap.init",
	}
}

func (t *AmpTestRunner) SetupBeforeAgentRun() error {
	env := environment.GetEnvironmentMetaData()
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}
	// replace AMP workspace ID placeholder with a testing workspace ID from metadata
	agentConfigPath := filepath.Join("agent_configs", t.GetAgentConfigFileName())
	ampCommands := []string{
		"sed -ie 's/{workspace_id}/" + env.AmpWorkspaceId + "/g' " + agentConfigPath,
		// use below to add JMX metrics then update agent config & GetMeasuredMetrics()
		//"nohup java -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=2030 -Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -Dcom.sun.management.jmxremote.rmi.port=2030  -Dcom.sun.management.jmxremote.host=0.0.0.0  -Djava.rmi.server.hostname=0.0.0.0 -Dserver.port=8090 -Dspring.application.admin.enabled=true -jar jars/spring-boot-web-starter-tomcat.jar > /tmp/spring-boot-web-starter-tomcat-jar.txt 2>&1 &",
	}
	err = common.RunCommands(ampCommands)
	if err != nil {
		return err
	}
	return t.SetUpConfig()
}

func TestAmp(t *testing.T) {
	runner := test_runner.TestRunner{TestRunner: &AmpTestRunner{
		test_runner.BaseTestRunner{},
	}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("AMP test failed")
		result.Print()
	}
}

func getDimensions() []types.Dimension {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
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
