// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package lvm

import (
	"log"
	"os"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const namespace = "LVMTest"

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type LVMTestRunner struct {
	test_runner.BaseTestRunner
}

func (t LVMTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateDiskMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *LVMTestRunner) validateDiskMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	hostName, err := os.Hostname()
	if err != nil {
		log.Printf("Hostname was not found")
	}
	log.Printf("Hostname found %s", hostName)

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   common.Host,
			Value: dimension.ExpectedDimensionValue{Value: aws.String(hostName)},
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)

	log.Printf("metric values are %v", values)
	if err != nil {
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func (t LVMTestRunner) GetTestName() string {
	return namespace
}

func (t LVMTestRunner) GetAgentConfigFileName() string {
	return "config.json"
}

func (t LVMTestRunner) GetMeasuredMetrics() []string {
	return []string{"disk_free", "disk_used", "disk_total"}
}

func (t *LVMTestRunner) SetupBeforeAgentRun() error {
	commands := []string{
		"sudo dd if=/dev/zero of=/tmp/lvm0.img bs=50 count=1M",
		"sudo losetup /dev/loop0 /tmp/lvm0.img",
		"sudo pvcreate /dev/loop0",
		"sudo vgcreate vg0 /dev/loop0",
		"sudo lvcreate -l 100%VG --name lv0 vg0",
		"sudo mkfs.ext2 /dev/mapper/vg0-lv0",
		"sudo mkdir /mnt/lvm",
		"sudo mount /dev/mapper/vg0-lv0 /mnt/lvm/",
	}
	err := common.RunCommands(commands)
	if err != nil {
		return err
	}
	return t.SetUpConfig()
}

var _ test_runner.ITestRunner = (*LVMTestRunner)(nil)

func TestLVM(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	factory := dimension.GetDimensionFactory(*env)
	runner := test_runner.TestRunner{TestRunner: &LVMTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}}}
	result := runner.Run()
	if result.GetStatus() != status.SUCCESSFUL {
		t.Fatal("LVM test failed")
		result.Print()
	}
}
