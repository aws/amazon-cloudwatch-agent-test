// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mitchellh/mapstructure"
)

type matrixRow struct {
	TestDir             string `json:"test_dir"`
	Os                  string `json:"os"`
	Family              string `json:"family"`
	TestType            string `json:"testType"`
	Arc                 string `json:"arc"`
	InstanceType        string `json:"instanceType"`
	Ami                 string `json:"ami"`
	BinaryName          string `json:"binaryName"`
	Username            string `json:"username"`
	InstallAgentCommand string `json:"installAgentCommand"`
	AgentStartCommand   string `json:"agentStartCommand"`
	CaCertPath          string `json:"caCertPath"`
	ValuesPerMinute     int    `json:"values_per_minute"` // Number of metrics to be sent or number of log lines to write
	K8sVersion          string `json:"k8s_version"`
	TerraformDir        string `json:"terraform_dir"`
	UseSSM              bool   `json:"useSSM"`
	ExcludedTests       string `json:"excludedTests"`
	MetadataEnabled     string `json:"metadataEnabled"`
	MaxAttempts         int    `json:"max_attempts"`
}

type testConfig struct {
	// this gives more flexibility to define terraform dir when there should be a different set of terraform files
	// e.g. statsd can have a multiple terraform module sets for difference test scenarios (ecs, eks or ec2)
	testDir       string
	terraformDir  string
	runMockServer bool
	// define target matrix field as set(s)
	// empty map means a testConfig will be created with a test entry for each entry from *_test_matrix.json
	targets map[string]map[string]struct{}
	// maxAttempts limits the number of times a test will be run.
	maxAttempts int
}

const (
	testTypeKeyEc2Linux = "ec2_linux"
)

// you can't have a const map in golang
var testTypeToTestConfig = map[string][]testConfig{
	"ec2_gpu": {
		{testDir: "./test/nvidia_gpu"},
	},
	testTypeKeyEc2Linux: {
		{testDir: "./test/ca_bundle"},
		{testDir: "./test/cloudwatchlogs"},
		{
			testDir: "./test/metrics_number_dimension",
			targets: map[string]map[string]struct{}{"os": {"al2": {}}},
		},
		{
			testDir:     "./test/emf_concurrent",
			targets:     map[string]map[string]struct{}{"os": {"al2": {}}},
			maxAttempts: 1,
		},
		{testDir: "./test/metric_value_benchmark"},
		{testDir: "./test/run_as_user"},
		{testDir: "./test/collection_interval"},
		{testDir: "./test/metric_dimension"},
		{testDir: "./test/restart"},
		{testDir: "./test/xray"},
		{testDir: "./test/otlp"},
		{
			testDir: "./test/acceptance",
			targets: map[string]map[string]struct{}{"os": {"ubuntu-20.04": {}}},
		},
		// skipping FIPS test as the test cannot be verified
		// neither ssh nor SSM works after a reboot once FIPS is enabled
		//{
		//	testDir: "./test/fips",
		//	targets: map[string]map[string]struct{}{"os": {"rhel8": {}}},
		//},
		{
			testDir: "./test/lvm",
			targets: map[string]map[string]struct{}{"os": {"al2": {}}},
		},
		{
			testDir: "./test/proxy",
			targets: map[string]map[string]struct{}{"os": {"al2": {}}},
		},
		{
			testDir: "./test/ssl_cert",
			targets: map[string]map[string]struct{}{"os": {"al2": {}}},
		},
		{
			testDir:      "./test/userdata",
			terraformDir: "terraform/ec2/userdata",
			targets:      map[string]map[string]struct{}{"os": {"ol9": {}}},
		},
		{
			testDir:      "./test/assume_role",
			terraformDir: "terraform/ec2/creds",
			targets:      map[string]map[string]struct{}{"os": {"al2": {}}},
		},
	},
	/*
		You can only place 1 mac instance on a dedicate host a single time.
		Therefore, limit down the scope for testing in Mac since EC2 can be done with Linux
		and Mac under the hood share similar plugins with Linux
	*/
	"ec2_mac": {
		{testDir: "../../../test/feature/mac"},
	},
	"ec2_windows": {
		{testDir: "../../../test/feature/windows"},
		{testDir: "../../../test/restart"},
		{testDir: "../../../test/acceptance"},
		{testDir: "../../../test/feature/windows/event_logs"},
		{
			testDir: "../../../test/feature/windows/custom_start/userdata",
			targets: map[string]map[string]struct{}{"os": {"win-2019": {}}},
		},
		{
			testDir: "../../../test/feature/windows/custom_start/ssm_start",
			targets: map[string]map[string]struct{}{"os": {"win-2019": {}}},
		},
		// assume role test doesn't add much value, and it already being tested with linux
		//{testDir: "../../../test/assume_role"},
	},
	"ec2_performance": {
		{testDir: "../../test/performance/emf"},
		{testDir: "../../test/performance/logs"},
		{testDir: "../../test/performance/system"},
		{testDir: "../../test/performance/statsd"},
		{testDir: "../../test/performance/collectd"},
		{testDir: "../../test/performance/trace/xray", runMockServer: true},
	},
	"ec2_windows_performance": {
		{testDir: "../../test/performance/windows/logs"},
		{testDir: "../../test/performance/windows/system"},
	},
	"ec2_stress": {
		{testDir: "../../test/stress/emf"},
		{testDir: "../../test/stress/logs"},
		{testDir: "../../test/stress/system"},
		{testDir: "../../test/stress/statsd"},
		{testDir: "../../test/stress/collectd"},
	},
	"ec2_windows_stress": {
		{testDir: "../../test/stress/windows/logs"},
		{testDir: "../../test/stress/windows/system"},
	},
	"ecs_fargate": {
		{testDir: "./test/ecs/ecs_metadata"},
	},
	"ecs_ec2_daemon": {
		{
			testDir: "./test/metric_value_benchmark",
			targets: map[string]map[string]struct{}{"metadataEnabled": {"enabled": {}}},
		},
		{
			testDir: "./test/statsd",
			targets: map[string]map[string]struct{}{"metadataEnabled": {"enabled": {}}},
		},
		{
			testDir: "./test/emf",
			targets: map[string]map[string]struct{}{"metadataEnabled": {"disabled": {}}},
		},
		{
			testDir: "./test/emf",
			targets: map[string]map[string]struct{}{"metadataEnabled": {"enabled": {}}},
		},
	},
	"eks_daemon": {
		{
			testDir: "./test/metric_value_benchmark",
			targets: map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
		{
			testDir: "./test/statsd", terraformDir: "terraform/eks/daemon/statsd",
			targets: map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
		{
			testDir: "./test/emf", terraformDir: "terraform/eks/daemon/emf",
			targets: map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
		{
			testDir: "./test/fluent", terraformDir: "terraform/eks/daemon/fluent/d",
			targets: map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
		{testDir: "./test/fluent", terraformDir: "terraform/eks/daemon/fluent/bit"},
		{testDir: "./test/app_signals", terraformDir: "terraform/eks/daemon/app_signals",
			targets: map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
	},
	"eks_deployment": {
		{testDir: "./test/metric_value_benchmark"},
	},
}

func copyAllEC2LinuxTestForOnpremTesting() {
	/* Some tests need to be fixed in order to run in both environment, so for now for PoC, run one that works.
	   testTypeToTestConfig["ec2_linux_onprem"] = testTypeToTestConfig[testTypeKeyEc2Linux]
	*/
	testTypeToTestConfig["ec2_linux_onprem"] = []testConfig{
		{
			testDir: "./test/lvm",
			targets: map[string]map[string]struct{}{"os": {"al2": {}}},
		},
	}
}

func main() {
	copyAllEC2LinuxTestForOnpremTesting()

	for testType, testConfigs := range testTypeToTestConfig {
		testMatrix := genMatrix(testType, testConfigs)
		writeTestMatrixFile(testType, testMatrix)
	}
}

func genMatrix(testType string, testConfigs []testConfig) []matrixRow {
	openTestMatrix, err := os.Open(fmt.Sprintf("generator/resources/%v_test_matrix.json", testType))

	if err != nil {
		log.Panicf("can't read file %v_test_matrix.json err %v", testType, err)
	}

	defer openTestMatrix.Close()

	byteValueTestMatrix, _ := io.ReadAll(openTestMatrix)

	var testMatrix []map[string]interface{}
	err = json.Unmarshal(byteValueTestMatrix, &testMatrix)
	if err != nil {
		log.Panicf("can't unmarshall file %v_test_matrix.json err %v", testType, err)
	}

	testMatrixComplete := make([]matrixRow, 0, len(testMatrix))
	for _, test := range testMatrix {
		for _, testConfig := range testConfigs {
			row := matrixRow{
				TestDir:      testConfig.testDir,
				TestType:     testType,
				TerraformDir: testConfig.terraformDir,
				MaxAttempts:  testConfig.maxAttempts,
			}
			err = mapstructure.Decode(test, &row)
			if err != nil {
				log.Panicf("can't decode map test %v to metric line struct with error %v", testConfig, err)
			}

			if testConfig.targets == nil || shouldAddTest(&row, testConfig.targets) {
				testMatrixComplete = append(testMatrixComplete, row)
			}
		}
	}
	return testMatrixComplete
}

// not so robust way to determine a matrix entry should be included to complete test matrix, but it serves the purpose
// struct (matrixRow) field should be added as elif to support more. could use reflection with some tradeoffs
func shouldAddTest(row *matrixRow, targets map[string]map[string]struct{}) bool {
	for key, set := range targets {
		var rowVal string
		if key == "arc" {
			rowVal = row.Arc
		} else if key == "os" {
			rowVal = row.Os
		} else if key == "metadataEnabled" {
			rowVal = row.MetadataEnabled
		}

		if rowVal == "" {
			continue
		}
		_, ok := set[rowVal]
		if !ok {
			return false
		}
	}
	return true
}

func writeTestMatrixFile(testType string, testMatrix []matrixRow) {
	bytes, err := json.MarshalIndent(testMatrix, "", " ")
	if err != nil {
		log.Panicf("Can't marshal json for target os %v, err %v", testType, err)
	}
	err = os.WriteFile(fmt.Sprintf("generator/resources/%v_complete_test_matrix.json", testType), bytes, os.ModePerm)
	if err != nil {
		log.Panicf("Can't write json to file for target os %v, err %v", testType, err)
	}
}
