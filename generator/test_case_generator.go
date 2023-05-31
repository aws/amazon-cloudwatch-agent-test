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
}

// you can't have a const map in golang
var testTypeToTestConfig = map[string][]testConfig{
	"ec2_gpu": {
		{"./test/nvidia_gpu", ""},
	},
	"ec2_linux": {
		{"./test/ca_bundle", ""},
		{"./test/cloudwatchlogs", ""},
		{"./test/metrics_number_dimension", ""},
		{"./test/metric_value_benchmark", ""},
		{"./test/run_as_user", ""},
		{"./test/collection_interval", ""},
		{"./test/metric_dimension", ""},
	},
	/*
		You can only place 1 mac instance on a dedicate host a single time.
		Therefore, limit down the scope for testing in Mac since EC2 can be done with Linux
		and Mac under the hood share similar plugins with Linux
	*/
	"ec2_mac": {
		{"../../../test/feature/mac", ""},
	},
	"ec2_windows": {
		{"../../../test/feature/windows", ""},
	},
	"ec2_performance": {
		{"../../test/performance/emf", ""},
		{"../../test/performance/logs", ""},
		{"../../test/performance/system", ""},
		{"../../test/performance/statsd", ""},
		{"../../test/performance/collectd", ""},
	},
	"ec2_stress": {
		{"../../test/stress/emf", ""},
		{"../../test/stress/logs", ""},
		{"../../test/stress/system", ""},
		{"../../test/stress/statsd", ""},
		{"../../test/stress/collectd", ""},
	},
	"ecs_fargate": {
		{"./test/ecs/ecs_metadata", ""},
	},
	"ecs_ec2_daemon": {
		{"./test/metric_value_benchmark", ""},
		{"./test/statsd", ""},
		{"./test/emf", ""},
	},
	"ec2_acceptance": {
		{"./test/acceptance", ""},
	},
	"ec2_userdata": {
		{"./test/userdata", ""},
  },

	"eks_daemon": {
		{"./test/metric_value_benchmark", ""},
		{"./test/statsd", "terraform/eks/daemon/statsd"},
		{"./test/emf", "terraform/eks/daemon/emf"},
	},
	"eks_deployment": {
		{"./test/metric_value_benchmark", ""},
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
	for _, test := range testMatrix {
		for _, testConfig := range testConfigs {
			row := matrixRow{TestDir: testConfig.testDir, TestType: testType, TerraformDir: testConfig.terraformDir}
			err = mapstructure.Decode(test, &row)
			if err != nil {
				log.Panicf("can't decode map test %v to metric line struct with error %v", testConfig, err)
			}
			testMatrixComplete = append(testMatrixComplete, row)
		}
	}
	return testMatrixComplete
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
