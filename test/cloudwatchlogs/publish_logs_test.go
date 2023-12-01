// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath              = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	logLineId1                    = "foo"
	logLineId2                    = "bar"
	logFilePath                   = "/tmp/cwagent_log_test.log" // TODO: not sure how well this will work on Windows
	sleepForFlush                 = 20 * time.Second            // default flush interval is 5 seconds
	configPathAutoRemoval         = "resources/config_auto_removal.json"
	standardLogGroupClass         = "STANDARD"
	infrequentAccessLogGroupClass = "INFREQUENT_ACCESS"
)

var (
	logLineIds                      = []string{logLineId1, logLineId2}
	instanceId                      = awsservice.GetInstanceId()
	ctx                             = context.Background()
	awsCfg, _                       = config.LoadDefaultConfig(ctx)
	writeToCloudWatchTestParameters = []writeToCloudWatchTestInput{
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
	cloudWatchLogGroupClassTestParameters = []cloudWatchLogGroupClassTestInput{
		{
			testName:      "Standard log config",
			configPath:    "resources/config_log.json",
			logGroupName:  instanceId,
			logGroupClass: standardLogGroupClass,
		},
		{
			testName:      "Standard log config with standard class specification",
			configPath:    "resources/config_log_standard_access.json",
			logGroupName:  instanceId + "-standard",
			logGroupClass: standardLogGroupClass,
		},
		{
			testName:      "Standard log config with Infrequent_access class specification",
			configPath:    "resources/config_log_infrequent_access.json",
			logGroupName:  instanceId + "-infrequent_access",
			logGroupClass: infrequentAccessLogGroupClass,
		},
	}
)

type writeToCloudWatchTestInput struct {
	testName        string
	iterations      int
	numExpectedLogs int
	configPath      string
}

type cloudWatchLogGroupClassTestInput struct {
	testName      string
	configPath    string
	logGroupName  string
	logGroupClass string
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestWriteLogsToCloudWatch writes N number of logs, and then validates that N logs
// are queryable from CloudWatch Logs
func TestWriteLogsToCloudWatch(t *testing.T) {
	// this uses the {instance_id} placeholder in the agent configuration,
	// so we need to determine the host's instance ID for validation
	log.Printf("Found instance id %s", instanceId)

	defer awsservice.DeleteLogGroupAndStream(instanceId, instanceId)

	f, err := os.Create(logFilePath)
	if err != nil {
		t.Fatalf("Error occurred creating log file for writing: %v", err)
	}
	defer f.Close()
	defer os.Remove(logFilePath)

	for _, param := range writeToCloudWatchTestParameters {
		t.Run(param.testName, func(t *testing.T) {
			common.DeleteFile(common.AgentLogFile)
			common.TouchFile(common.AgentLogFile)
			start := time.Now()

			common.CopyFile(param.configPath, configOutputPath)

			common.StartAgent(configOutputPath, true, false)

			// ensure that there is enough time from the "start" time and the first log line,
			// so we don't miss it in the GetLogEvents call
			time.Sleep(sleepForFlush)
			writeLogLines(t, f, param.iterations)
			time.Sleep(sleepForFlush)
			common.StopAgent()
			end := time.Now()

			// check CWL to ensure we got the expected number of logs in the log stream
			err = awsservice.ValidateLogs(
				instanceId,
				instanceId,
				&start,
				&end,
				awsservice.AssertLogsCount(param.numExpectedLogs),
				awsservice.AssertNoDuplicateLogs(),
			)
			assert.NoError(t, err)
		})
	}
}

// TestAutoRemovalStopAgent configures agent to monitor a file with auto removal on.
// Then it restarts the agent.
// Verify the file is NOT removed.
func TestAutoRemovalStopAgent(t *testing.T) {
	defer autoRemovalTestCleanup()
	f, _ := os.Create(logFilePath + "1")
	defer f.Close()
	// Restart the agent multiple times.
	loopCount := 5
	linesPerLoop := 1000
	start := time.Now()
	for i := 0; i < loopCount; i++ {
		writeSleepRestart(t, f, configPathAutoRemoval, linesPerLoop, true)
	}
	checkData(t, start, loopCount*linesPerLoop*2)
}

// TestAutoRemovalFileRotation repeatedly creates files matching the monitored pattern.
// After creating each file, write some log lines, sleep and verify previous_file was auto removed.
// Retrieve LogEvents from CWL and verify all log lines were uploaded.
func TestAutoRemovalFileRotation(t *testing.T) {
	defer autoRemovalTestCleanup()
	common.StartAgent(configPathAutoRemoval, true, false)
	loopCount := 5
	linesPerLoop := 1000
	start := time.Now()
	for i := 0; i < loopCount; i++ {
		// Create new file each minute and run for 5 minutes.
		f, _ := os.Create(logFilePath + strconv.Itoa(i))
		defer f.Close()
		writeSleepRestart(t, f, configPathAutoRemoval, linesPerLoop, false)
	}
	checkData(t, start, loopCount*linesPerLoop*2)
}

// TestRotatingLogsDoesNotSkipLines validates https://github.com/aws/amazon-cloudwatch-agent/issues/447
// The following should happen in the test:
// 1. A log line of size N should be written
// 2. The file should be rotated, and a new log line of size N should be written
// 3. The file should be rotated again, and a new log line of size GREATER THAN N should be written
// 4. All three log lines, in full, should be visible in CloudWatch Logs
func TestRotatingLogsDoesNotSkipLines(t *testing.T) {
	cfgFilePath := "resources/config_log_rotated.json"

	log.Printf("Found instance id %s", instanceId)
	logGroup := instanceId
	logStream := instanceId + "Rotated"

	defer awsservice.DeleteLogGroupAndStream(logGroup, logStream)

	start := time.Now()
	common.CopyFile(cfgFilePath, configOutputPath)

	common.StartAgent(configOutputPath, true, false)

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the GetLogEvents call
	time.Sleep(sleepForFlush)
	t.Log("Writing logs and rotating")
	// execute the script used in the repro case
	common.RunCommand("/usr/bin/python3 resources/write_and_rotate_logs.py")
	time.Sleep(sleepForFlush)
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

	err := awsservice.ValidateLogs(
		logGroup,
		logStream,
		&start,
		&end,
		awsservice.AssertLogsCount(len(lines)),
		func(events []types.OutputLogEvent) error {
			for i := 0; i < len(events); i++ {
				expected := strings.ReplaceAll(lines[i], "'", "\"")
				actual := strings.ReplaceAll(*events[i].Message, "'", "\"")
				if expected != actual {
					return fmt.Errorf("actual log event %q does not match the expected %q", actual, expected)
				}
			}
			return nil
		},
	)
	assert.NoError(t, err)
}

func TestLogGroupClass(t *testing.T) {
	logFile, err := os.Create(logFilePath)
	agentRuntime := 20 * time.Second // default flush interval is 5 seconds
	if err != nil {
		t.Fatalf("Error occurred creating log file for writing: %v", err)
	}
	defer logFile.Close()
	defer os.Remove(logFilePath)

	for _, param := range cloudWatchLogGroupClassTestParameters {
		t.Run(param.testName, func(t *testing.T) {
			defer awsservice.DeleteLogGroupAndStream(param.logGroupName, instanceId)
			common.DeleteFile(common.AgentLogFile)
			common.TouchFile(common.AgentLogFile)

			common.CopyFile(param.configPath, configOutputPath)

			err := common.StartAgent(configOutputPath, true, false)
			assert.Nil(t, err)
			// ensure that there is enough time from the "start" time and the first log line,
			time.Sleep(agentRuntime)
			writeLogLines(t, logFile, 100)
			time.Sleep(agentRuntime)
			common.StopAgent()

			agentLog, err := os.ReadFile(common.AgentLogFile)
			if err != nil {
				return
			}
			t.Logf("Agent logs %s", string(agentLog))

			assert.True(t, awsservice.IsLogGroupExists(param.logGroupName))
		})
	}
}

func writeLogLines(t *testing.T, f *os.File, iterations int) {
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

func writeSleepRestart(t *testing.T, f *os.File, configPath string, linesPerLoop int, doRestart bool) {
	if doRestart {
		common.StartAgent(configPath, true, false)
	}
	// Sleep to ensure agent detects file before it is written.
	time.Sleep(sleepForFlush)
	writeLogLines(t, f, linesPerLoop)
	time.Sleep(sleepForFlush)
	if doRestart {
		common.StopAgent()
	}
	c, _ := filepath.Glob(logFilePath + "*")
	assert.Equal(t, 1, len(c))
}

func autoRemovalTestCleanup() {
	awsservice.DeleteLogGroupAndStream(instanceId, instanceId)
	paths, _ := filepath.Glob(logFilePath + "*")
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// checkData queries CWL and verifies the number of log lines.
func checkData(t *testing.T, start time.Time, lineCount int) {
	end := time.Now()
	// Sleep to ensure backend stores logs.
	time.Sleep(time.Second * 60)
	err := awsservice.ValidateLogs(
		instanceId,
		instanceId,
		&start,
		&end,
		// *2 because 2 lines per loop
		awsservice.AssertLogsCount(lineCount),
		awsservice.AssertNoDuplicateLogs(),
	)
	assert.NoError(t, err)
}
