// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/mitchellh/mapstructure"
	"golang.org/x/exp/slices"
)

type matrixRow struct {
	TestName            string `json:"testName"`
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
	K8sVersion          string `json:"k8sVersion"`
	Nodes               int    `json:"nodes"`
	DeploymentStrategy  string `json:"deploymentStrategy"`
	TerraformDir        string `json:"terraform_dir"`
	UseSSM              bool   `json:"useSSM"`
	ExcludedTests       string `json:"excludedTests"`
	MetadataEnabled     string `json:"metadataEnabled"`
	MaxAttempts         int    `json:"max_attempts"`
	SELinuxBranch       string `json:"selinux_branch"`
}

type testConfig struct {
	// this gives more flexibility to define terraform dir when there should be a different set of terraform files
	// e.g. statsd can have a multiple terraform module sets for difference test scenarios (ecs, eks or ec2)
	testDir       string
	terraformDir  string
	instanceType  string
	runMockServer bool
	selinuxBranch string
	// define target matrix field as set(s)
	// empty map means a testConfig will be created with a test entry for each entry from *_test_matrix.json
	targets map[string]map[string]struct{}
	// maxAttempts limits the number of times a test will be run.
	maxAttempts int
}

const (
	testTypeKeyEc2Linux   = "ec2_linux"
)

// you can't have a const map in golang
var testTypeToTestConfig = map[string][]testConfig{
	testTypeKeyEc2Linux: {
		{testDir: "./test/metric_value_benchmark"}, // flaky
		{
			testDir:      "./test/userdata", //flaky
			terraformDir: "terraform/ec2/userdata",
			targets:      map[string]map[string]struct{}{"os": {"ol9": {}}},
		},
	},
	"ec2_windows": {
		{testDir: "../../../test/feature/windows"}, // flaky
	},
	"ec2_stress": {
		{testDir: "../../test/stress/statsd"}, // flaky
	},
	"ec2_windows_stress": {
		{testDir: "../../test/stress/windows/logs"}, // flaky
	},
	"eks_daemon": {
		{
			testDir:      "./test/metric_value_benchmark", // flaky
			targets:      map[string]map[string]struct{}{"arc": {"amd64": {}}},
			instanceType: "g4dn.xlarge",
		},
		{
			testDir:      "./test/metric_value_benchmark", // flaky
			terraformDir: "terraform/eks/daemon/windows/2019",
			targets:      map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
		{testDir: "./test/fluent", terraformDir: "terraform/eks/daemon/fluent/bit"},          // flaky
		{testDir: "./test/fluent", terraformDir: "terraform/eks/daemon/fluent/windows/2022"}, //flaky
		{
			testDir: "./test/efa", terraformDir: "terraform/eks/daemon/efa", // flaky
			targets: map[string]map[string]struct{}{"arc": {"amd64": {}}},
		},
	},
}

var testTypeToTestConfigE2E = map[string][]testConfig{
	"eks_e2e_jmx": {
		{testDir: "../../../test/e2e/jmx"},
	},
}

type partition struct {
	configName string
	tests      []string
	ami        []string
}

var partitionTests = map[string]partition{
	"commercial": {
		configName: "",
		tests:      []string{},
		ami:        []string{},
	},
	"itar": {
		configName: "_itar",
		tests:      []string{testTypeKeyEc2Linux},
		ami:        []string{"cloudwatch-agent-integration-test-aarch64-al2023*"},
	},
	"china": {configName: "_china",
		tests: []string{testTypeKeyEc2Linux},
		ami:   []string{"cloudwatch-agent-integration-test-aarch64-al2023*"},
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
	useE2E := flag.Bool("e2e", false, "Use e2e test matrix generation")
	flag.Parse()

	configMap := testTypeToTestConfig
	if !*useE2E {
		copyAllEC2LinuxTestForOnpremTesting()
	} else {
		configMap = testTypeToTestConfigE2E
	}

	for testType, testConfigs := range configMap {
		for _, partition := range partitionTests {
			if len(partition.tests) != 0 && !slices.Contains(partition.tests, testType) {
				continue
			}
			testMatrix := genMatrix(testType, testConfigs, partition.ami)
			writeTestMatrixFile(testType+partition.configName, testMatrix)
		}
	}
}
func generateTestName(test_directory string) string {
	parts := strings.Split(test_directory, "/")

	// Remove empty parts caused by leading `../`
	var cleaned []string
	for _, part := range parts {
		if part != "" && part != "." && part != ".." {
			cleaned = append(cleaned, part)
		}
	}

	// Reorder: move the first element to the end
	if len(cleaned) > 1 {
		cleaned = append(cleaned[1:], cleaned[0])
	}

	return strings.Join(cleaned, "-")
}
func genMatrix(testType string, testConfigs []testConfig, ami []string) []matrixRow {
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
			//This is to have selinux negative test
			if testConfig.selinuxBranch == "" {
				testConfig.selinuxBranch = "main"
			}

			row := matrixRow{
				TestName:      generateTestName(testConfig.testDir),
				SELinuxBranch: testConfig.selinuxBranch,
				TestDir:       testConfig.testDir,
				TestType:      testType,
				TerraformDir:  testConfig.terraformDir,
				MaxAttempts:   testConfig.maxAttempts,
			}
			err = mapstructure.Decode(test, &row)
			if err != nil {
				log.Panicf("can't decode map test %v to metric line struct with error %v", testConfig, err)
			}
			if row.Os != "" {
				row.TestName = row.Os + ":" + row.TestName
			}
			if row.TestType != "" && row.Os == "" {
				row.TestName = row.TestType + ":" + row.TestName
			}
			if testConfig.instanceType != "" {
				row.InstanceType = testConfig.instanceType
			}

			if len(ami) != 0 && !slices.Contains(ami, row.Ami) {
				continue
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
