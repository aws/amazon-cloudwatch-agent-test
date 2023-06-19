// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	logLineId1       = "foo"
	logLineId2       = "bar"
	logFilePath      = "/tmp/test.log"  // TODO: not sure how well this will work on Windows
	agentRuntime     = 20 * time.Second // default flush interval is 5 seconds
)

var logLineIds = []string{logLineId1, logLineId2}

type input struct {
	testName        string
	iterations      int
	numExpectedLogs int
	configPath      string
}

var testParameters = []input{
	{
		testName:        "Happy path",
		iterations:      100,
		numExpectedLogs: 200,
		configPath:      "resources/config_log.json",
	},
	{
		testName:        "Client-side log filtering",
		iterations:      100,
		numExpectedLogs: 100,
		configPath:      "resources/config_log_filter.json",
	},
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// TestWriteLogsToCloudWatch writes N number of logs, and then validates that N logs
// are queryable from CloudWatch Logs
func TestWriteLogsToCloudWatch(t *testing.T) {
	// this uses the {instance_id} placeholder in the agent configuration,
	// so we need to determine the host's instance ID for validation
	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)

	defer awsservice.DeleteLogGroupAndStream(instanceId, instanceId)

	f, err := os.Create(logFilePath)
	if err != nil {
		t.Fatalf("Error occurred creating log file for writing: %v", err)
	}
	defer f.Close()
	defer os.Remove(logFilePath)

	for _, param := range testParameters {
		t.Run(param.testName, func(t *testing.T) {
			common.DeleteFile(common.AgentLogFile)
			common.TouchFile(common.AgentLogFile)
			start := time.Now()

			common.CopyFile(param.configPath, configOutputPath)

			common.StartAgent(configOutputPath, true, false)

			// ensure that there is enough time from the "start" time and the first log line,
			// so we don't miss it in the GetLogEvents call
			time.Sleep(agentRuntime)
			writeLogs(t, f, param.iterations)
			time.Sleep(agentRuntime)
			common.StopAgent()

			agentLog, err := common.RunCommand(common.CatCommand + common.AgentLogFile)
			if err != nil {
				return
			}
			t.Logf("Agent logs %s", agentLog)

			end := time.Now()

			// check CWL to ensure we got the expected number of logs in the log stream
			ok, err := awsservice.ValidateLogs(instanceId, instanceId, &start, &end, func(logs []string) bool {
				return param.numExpectedLogs == len(logs)
			})
			assert.NoError(t, err)
			assert.True(t, ok)
		})
	}
}

// TestRotatingLogsDoesNotSkipLines validates https://github.com/aws/amazon-cloudwatch-agent/issues/447
// The following should happen in the test:
// 1. A log line of size N should be written
// 2. The file should be rotated, and a new log line of size N should be written
// 3. The file should be rotated again, and a new log line of size GREATER THAN N should be written
// 4. All three log lines, in full, should be visible in CloudWatch Logs
func TestRotatingLogsDoesNotSkipLines(t *testing.T) {
	cfgFilePath := "resources/config_log_rotated.json"

	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)
	logGroup := instanceId
	logStream := instanceId + "Rotated"

	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)

	start := time.Now()
	common.CopyFile(cfgFilePath, configOutputPath)

	common.StartAgent(configOutputPath, true, false)

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the GetLogEvents call
	time.Sleep(agentRuntime)
	t.Log("Writing logs and rotating")
	// execute the script used in the repro case
	common.RunCommand("/usr/bin/python3 resources/write_and_rotate_logs.py")
	time.Sleep(agentRuntime)
	common.StopAgent()

	// These expected log lines are created using resources/write_and_rotate_logs.py,
	// which are taken directly from the repro case in https://github.com/aws/amazon-cloudwatch-agent/issues/447
	// logging.info(json.dumps({"Metric": "12345"*10}))
	// logging.info(json.dumps({"Metric": "09876"*10}))
	// logging.info({"Metric": "1234567890"*10})
	lines := []string{
		fmt.Sprintf("{\"Metric\": \"%s\"}", strings.Repeat("12345", 10)),
		fmt.Sprintf("{\"Metric\": \"%s\"}", strings.Repeat("09876", 10)),
		fmt.Sprintf("{\"Metric\": \"%s\"}", strings.Repeat("1234567890", 10)),
	}

	end := time.Now()

	ok, err := awsservice.ValidateLogs(logGroup, logStream, &start, &end, func(logs []string) bool {
		if len(logs) != len(lines) {
			return false
		}

		for i := 0; i < len(logs); i++ {
			expected := strings.ReplaceAll(lines[i], "'", "\"")
			actual := strings.ReplaceAll(logs[i], "'", "\"")
			if expected != actual {
				return false
			}
		}

		return true
	})
	assert.NoError(t, err)
	assert.True(t, ok)
}

// TestCloudWatchLogsBuffer writes N number of logs, and then validates that N logs
// are queryable from CloudWatch Logs
// and buffer size works
// buffer size is 1mb
func TestCloudWatchLogsBuffer(t *testing.T) {
	// this uses the {instance_id} placeholder in the agent configuration,
	// so we need to determine the host's instance ID for validation
	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)
	common.DeleteFile(common.AgentLogFile)
	common.TouchFile(common.AgentLogFile)
	start := time.Now()

	numberOfLogsPerFile := 1000
	numberOfLogFiles := 100
	filePathFormat := "/tmp/logs_buffer/"
	fileName := filePathFormat + "test.log"

	for i := 0; i < numberOfLogFiles; i++ {
		fileDir := filePathFormat
		filePath := fileName + "." + strconv.Itoa(i)
		defer awsservice.DeleteLogGroupAndStream(instanceId, filePath)
		err := os.MkdirAll(fileDir, os.ModePerm)
		if err != nil {
			t.Fatalf("Error occurred creating log dir for writing: %v", err)
		}
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Error occurred creating log file for writing: %v", err)
		}
		defer f.Close()
		defer os.Remove(filePath)
		go writeLogs(t, f, numberOfLogsPerFile)
	}

	common.CopyFile("resources/config_log_buffer.json", configOutputPath)
	common.StartAgent(configOutputPath, true, false)
	time.Sleep(time.Minute * 2)
	common.StopAgent()

	agentLog, err := common.RunCommand(common.CatCommand + common.AgentLogFile)
	if err != nil {
		return
	}
	t.Logf("Agent logs %s", agentLog)
	// confirm buffer is hit
	// confirm blocking of next log search tick
	// confirm blocking of reading next line
	// confirm buffer is cleared at the end
	assert.True(t, strings.Contains(agentLog, "blocking adding new files for one second"))
	assert.True(t, strings.Contains(agentLog, "max buffer of logs size sending to cloudwatch blocking reading for one second"))
	agentLogLines := strings.Split(agentLog, "\n")
	found := false
	for i := len(agentLogLines) - 1; i >= 0; i-- {
		if strings.Contains(agentLogLines[i], "D! [logagent] total buffer size to cloudwatch") {
			found = true
			lineSplit := strings.Split(agentLogLines[i], " ")
			endingBuffer := lineSplit[len(lineSplit)-1]
			endingBufferValue, err := strconv.Atoi(endingBuffer)
			assert.Nil(t, err)
			assert.Equal(t, 0, endingBufferValue)
			break
		}
	}
	assert.True(t, found)

	end := time.Now()

	for i := 0; i < numberOfLogFiles; i++ {
		filePath := fileName + "." + strconv.Itoa(i)
		// check CWL to ensure we got the expected number of logs in the log stream
		ok, err := awsservice.ValidateLogs(instanceId, filePath, &start, &end, func(logs []string) bool {
			return numberOfLogsPerFile*len(logLineIds) == len(logs)
		})
		assert.NoError(t, err)
		assert.True(t, ok)
	}
}

func writeLogs(t *testing.T, f *os.File, iterations int) {
	log.Printf("Writing %d lines to %s", iterations*len(logLineIds), f.Name())

	for i := 0; i < iterations; i++ {
		ts := time.Now()
		for _, id := range logLineIds {
			_, err := f.WriteString(fmt.Sprintf("%s - [%s] #%d This is a log line.\n", ts.Format(time.StampMilli), id, i))
			if err != nil {
				// don't need to fatal error here. if a log line doesn't get written, the count
				// when validating the log stream should be incorrect and fail there.
				t.Logf("Error occurred writing log line: %v", err)
			}
		}
		time.Sleep(1 * time.Millisecond)
	}
}
