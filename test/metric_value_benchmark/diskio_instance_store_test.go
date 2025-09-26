// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	instanceStoreEntityResourceType = "AWS::EC2::Instance"
	instanceStoreEntityType         = "AWS::Resource"
)

type DiskIOInstanceStoreTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*DiskIOInstanceStoreTestRunner)(nil)

func (m *DiskIOInstanceStoreTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := m.GetMeasuredMetrics()
	testResults := make([]status.TestResult, 2*len(metricsToFetch))
	for i, name := range metricsToFetch {
		testResults[i] = m.validateInstanceStoreMetric(name)
		if os.Getenv("AWS_REGION") == "us-west-2" {
			testResults[i+len(metricsToFetch)] = m.validateInstanceStoreEntity(name)
		}
	}

	return status.TestGroupResult{
		Name:        m.GetTestName(),
		TestResults: testResults,
	}
}

func (m *DiskIOInstanceStoreTestRunner) GetTestName() string {
	return "DiskIOInstanceStore"
}

func (m *DiskIOInstanceStoreTestRunner) GetAgentConfigFileName() string {
	return "diskio_instance_store_config.json"
}

func (m *DiskIOInstanceStoreTestRunner) SetupBeforeAgentRun() error {
	err := m.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	err = common.RunCommands([]string{"sudo setcap cap_sys_admin+ep /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent"})
	if err != nil {
		log.Printf("unable to setcap: %s", err)
	}
	return m.SetUpConfig()
}

func (m *DiskIOInstanceStoreTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"diskio_instance_store_total_read_ops",
		"diskio_instance_store_total_write_ops",
		"diskio_instance_store_total_read_bytes",
		"diskio_instance_store_total_write_bytes",
		"diskio_instance_store_total_read_time",
		"diskio_instance_store_total_write_time",
		"diskio_instance_store_performance_exceeded_iops",
		"diskio_instance_store_performance_exceeded_tp",
		"diskio_instance_store_volume_queue_length",
	}
}

func (m *DiskIOInstanceStoreTestRunner) GetAgentRunDuration() time.Duration {
	return 4 * time.Minute
}

func (m *DiskIOInstanceStoreTestRunner) validateInstanceStoreMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := m.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "SerialId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (m *DiskIOInstanceStoreTestRunner) validateInstanceStoreEntity(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   fmt.Sprintf("%s_entity", metricName),
		Status: status.FAILED,
	}
	env := environment.GetEnvironmentMetaData()
	serialID, err := common.GetAnyInstanceStoreSerialID()
	if err != nil {
		return testResult
	}

	requestBody := []byte(fmt.Sprintf(`{
        "Namespace": "%s",
        "MetricName": "%s",
        "Dimensions": [
            {"Name": "InstanceId", "Value": "%s"},
            {"Name": "SerialId", "Value": "%s"}
        ]
    }`, namespace, metricName, env.InstanceId, serialID))

	req, err := common.BuildListEntitiesForMetricRequest(requestBody, "us-west-2")
	if err != nil {
		log.Printf("Failed to build ListEntitiesForMetric request for metric: '%s': %v", metricName, err)
		return testResult
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send request for metric: '%s': %v", metricName, err)
		return testResult
	}
	defer resp.Body.Close()

	var response struct {
		Entities []struct {
			KeyAttributes struct {
				Type         string `json:"Type"`
				ResourceType string `json:"ResourceType"`
				Identifier   string `json:"Identifier"`
			} `json:"KeyAttributes"`
		} `json:"Entities"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Failed to decode response for metric: '%s': %v", metricName, err)
		return testResult
	}

	if len(response.Entities) != 1 {
		log.Printf("Response does not contain the correct number of entities for metric '%s'", metricName)
		return testResult
	}

	entity := response.Entities[0]
	if entity.KeyAttributes.Identifier != env.InstanceId ||
		entity.KeyAttributes.ResourceType != instanceStoreEntityResourceType ||
		entity.KeyAttributes.Type != instanceStoreEntityType {

		log.Printf("Entity mismatch for metric '%s':\n"+
			"Expected:\n"+
			"  Type: %s\n"+
			"  ResourceType: %s\n"+
			"  InstanceId: %s\n"+
			"Got:\n"+
			"  Type: %s\n"+
			"  ResourceType: %s\n"+
			"  InstanceId: %s",
			metricName,
			instanceStoreEntityType,
			instanceStoreEntityResourceType,
			env.InstanceId,
			entity.KeyAttributes.Type,
			entity.KeyAttributes.ResourceType,
			entity.KeyAttributes.Identifier)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
