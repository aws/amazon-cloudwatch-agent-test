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
	CaCertPath          string `json:"caCertPath"`
	ValuesPerMinute     int    `json:"values_per_minute"` // Number of metrics to be sent or number of log lines to write
	K8sVersion          string `json:"k8s_version"`
	TerraformDir        string `json:"terraform_dir"`
	UseSSM              bool   `json:"useSSM"`
}

type testConfig struct {
	// this gives more flexibility to define terraform dir when there should be a different set of terraform files
	// e.g. statsd can have a multiple terraform module sets for difference test scenarios (ecs, eks or ec2)
	testDir      string
	terraformDir string
	// target specific OS(es) so that only limited targets are tested. doesn't apply to containers
	// e.g. ["rhel8"] or ["rhel8", "ubuntu-20.04"]
	targetOs map[string]struct{}
}

// you can't have a const map in golang
var testTypeToTestConfig = map[string][]testConfig{
	"ec2_gpu": {
		{"./test/nvidia_gpu", "", nil},
	},
	"ec2_linux": {
		//{"./test/ca_bundle", "", nil},
		//{"./test/cloudwatchlogs", "", nil},
		//{"./test/metrics_number_dimension", "", nil},
		{"./test/metric_value_benchmark", "", nil},
		//{"./test/run_as_user", "", nil},
		//{"./test/collection_interval", "", nil},
		//{"./test/metric_dimension", "", nil},
		//{"./test/restart", "", nil},
		//{"./test/acceptance", "", map[string]struct{}{"ubuntu-20.04": {}}},
		//{"./test/fips", "", map[string]struct{}{"al2": {}}},
		{"./test/lvm", "", map[string]struct{}{"al2": {}}},
		{"./test/proxy", "", map[string]struct{}{"al2": {}}},
		{"./test/ssl_cert", "", map[string]struct{}{"al2": {}}},
		{"./test/userdata", "terraform/ec2/userdata", map[string]struct{}{"ol19": {}}},
	},
	/*
		You can only place 1 mac instance on a dedicate host a single time.
		Therefore, limit down the scope for testing in Mac since EC2 can be done with Linux
		and Mac under the hood share similar plugins with Linux
	*/
	"ec2_mac": {
		{"./test/feature/mac", "", nil},
	},
	"ec2_windows": {
		{"./test/feature/windows", "", nil},
		{"./test/restart", "", nil},
	},
	"ec2_performance": {
		{"../../test/performance/emf", "", nil},
		{"../../test/performance/logs", "", nil},
		{"../../test/performance/system", "", nil},
		{"../../test/performance/statsd", "", nil},
		{"../../test/performance/collectd", "", nil},
	},
	"ec2_stress": {
		{"../../test/stress/emf", "", nil},
		{"../../test/stress/logs", "", nil},
		{"../../test/stress/system", "", nil},
		{"../../test/stress/statsd", "", nil},
		{"../../test/stress/collectd", "", nil},
	},
	"ecs_fargate": {
		{"./test/ecs/ecs_metadata", "", nil},
	},
	"ecs_ec2_daemon": {
		{"./test/metric_value_benchmark", "", nil},
		{"./test/statsd", "", nil},
		{"./test/emf", "", nil},
	},
	"eks_daemon": {
		{"./test/metric_value_benchmark", "", nil},
		{"./test/statsd", "terraform/eks/daemon/statsd", nil},
		{"./test/emf", "terraform/eks/daemon/emf", nil},
	},
	"eks_deployment": {
		{"./test/metric_value_benchmark", "", nil},
	},
}

func main() {
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
	for _, testConfig := range testConfigs {
		for _, test := range testMatrix {
			row := matrixRow{TestDir: testConfig.testDir, TestType: testType, TerraformDir: testConfig.terraformDir}
			err = mapstructure.Decode(test, &row)
			if err != nil {
				log.Panicf("can't decode map test %v to metric line struct with error %v", testConfig, err)
			}

			if shouldAddTest(row.Os, testConfig.targetOs) {
				testMatrixComplete = append(testMatrixComplete, row)
			}
		}
	}
	return testMatrixComplete
}

func shouldAddTest(os string, targets map[string]struct{}) bool {
	if targets == nil {
		return true
	}
	if _, ok := targets[os]; ok {
		return true
	}

	return false
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
