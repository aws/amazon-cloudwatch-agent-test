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
}

// you can't have a const map in golang
var testTypeToTestDirMap = map[string][]string{
	"ec2_gpu": {
		"./test/nvidia_gpu",
	},
	"ec2_linux": {
		"./test/ca_bundle",
		"./test/cloudwatchlogs",
		"./test/metrics_number_dimension",
		"./test/metric_value_benchmark",
		"./test/run_as_user",
		"./test/collection_interval",
		"./test/metric_dimension",
	},
	/*
		You can only place 1 mac instance on a dedicate host a single time.
		Therefore, limit down the scope for testing in Mac since EC2 can be done with Linux
		and Mac under the hood share similar plugins with Linux
	*/
	"ec2_mac": {
		"../../../test/feature/mac",
	},
	"ec2_windows": {
		"../../../test/feature/win",
	},
	"ec2_performance": {
		"../../test/performance/logs",
		"../../test/performance/statsd",
		"../../test/performance/collectd",
	},
	"ec2_stress": {
		"../../test/stress/logs",
		"../../test/stress/statsd",
		"../../test/stress/collectd",
	},
	"ecs_fargate": {
		"./test/ecs/ecs_metadata",
	},
	"ecs_ec2_daemon": {
		"./test/metric_value_benchmark",
	},
	"ec2_acceptance": {
		"./test/acceptance",
	},
}

func main() {
	for testType, testDir := range testTypeToTestDirMap {
		testMatrix := genMatrix(testType, testDir)
		writeTestMatrixFile(testType, testMatrix)
	}
}

func genMatrix(testType string, testDirList []string) []matrixRow {
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
		for _, testDirectory := range testDirList {
			row := matrixRow{TestDir: testDirectory, TestType: testType}
			err = mapstructure.Decode(test, &row)
			if err != nil {
				log.Panicf("can't decode map test %v to metric line struct with error %v", testDirectory, err)
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
