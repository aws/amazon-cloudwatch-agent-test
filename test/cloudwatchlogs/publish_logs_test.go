// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
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
			configPath:    "resources/config_log_no_class_specification.json",
			logGroupName:  "standard-no-specification",
			logGroupClass: types.LogGroupClassStandard,
		},
		{
			testName:      "Standard log config with standard class specification",
			configPath:    "resources/config_log_standard_access.json",
			logGroupName:  "standard-with-specification",
			logGroupClass: types.LogGroupClassStandard,
		},
		{
			testName:      "Standard log config with Infrequent_access class specification",
			configPath:    "resources/config_log_infrequent_access.json",
			logGroupName:  "infrequent_access",
			logGroupClass: types.LogGroupClassInfrequentAccess,
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
	logGroupClass types.LogGroupClass
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
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
	instanceId := awsservice.GetInstanceId()
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
	instanceId := awsservice.GetInstanceId()
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

			assert.True(t, awsservice.IsLogGroupExists(param.logGroupName, param.logGroupClass))
		})
	}
}

func TestResourceMetrics(t *testing.T) {
	instanceId := awsservice.GetInstanceId()
	configPath := "resources/config_log_resource.json"
	logFile, err := os.Create(logFilePath)
	assert.NoError(t, err, "Error occurred creating log file for writing")

	defer logFile.Close()
	defer os.Remove(logFilePath)
	// defer awsservice.DeleteLogGroupAndStream(instanceId, instanceId)

	// start agent and write metrics and logs
	common.CopyFile(configPath, configOutputPath)
	common.StartAgent(configOutputPath, true, false)
	time.Sleep(2 * time.Minute)
	writeLogLines(t, logFile, 100)
	time.Sleep(2 * time.Minute)
	common.StopAgent()

	// this section builds, signs, and sends the request
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	assert.NoError(t, err)
	signer := v4.NewSigner()

	body := []byte(fmt.Sprintf(`{
        "Namespace": "CWAgent",
        "MetricName": "cpu_usage_idle",
        "Dimensions": [
            {"Name": "InstanceId", "Value": "%s"},
            {"Name": "InstanceType", "Value": "t3.medium"},
            {"Name": "cpu", "Value": "cpu-total"}
        ]
    }`, instanceId))

	h := sha256.New()
	h.Write(body)
	payloadHash := hex.EncodeToString(h.Sum(nil))

	// build the request
	req, err := http.NewRequest("POST", "https://monitoring.us-west-2.amazonaws.com/", bytes.NewReader(body))
	assert.NoError(t, err, "Error creating request")

	// set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Target", "com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric")
	req.Header.Set("Content-Encoding", "amz-1.0")

	// set creds
	credentials, err := cfg.Credentials.Retrieve(context.TODO())
	assert.NoError(t, err, "Error getting credentials")

	req.Header.Set("x-amz-security-token", credentials.SessionToken)

	// sign the request
	err = signer.SignHTTP(context.TODO(), credentials, req, payloadHash, "monitoring", "us-west-2", time.Now())
	assert.NoError(t, err, "Error signing the request")

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err, "Error sending the request")
	defer resp.Body.Close()

	// parse and verify the response
	var response struct {
		Entities []struct {
			KeyAttributes struct {
				Type         string `json:"Type"`
				ResourceType string `json:"ResourceType"`
				Identifier   string `json:"Identifier"`
			} `json:"KeyAttributes"`
		} `json:"Entities"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err, "Error parsing JSON response")

	// Verify the KeyAttributes
	assert.NotEmpty(t, response.Entities, "No entities found in the response")
	entity := response.Entities[0]
	assert.Equal(t, "AWS::Resource", entity.KeyAttributes.Type)
	assert.Equal(t, "AWS::EC2::Instance", entity.KeyAttributes.ResourceType)
	assert.Equal(t, instanceId, entity.KeyAttributes.Identifier)
}



// trying to replicate this curl command essentially:
// curl -i -X POST monitoring.us-west-2.amazonaws.com -H 'Content-Type: application/json' \
//   -H 'Content-Encoding: amz-1.0' \
//   --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
//   -H "x-amz-security-token: $AWS_SESSION_TOKEN" \
//   --aws-sigv4 "aws:amz:us-west-2:monitoring" \
//   -H 'X-Amz-Target: com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric' \
//   -d '{
//     "Namespace": "CWAgent",
//     "MetricName": "cpu_usage_idle",
//     "Dimensions": [
//       {
//         "Name": "InstanceId",
//         "Value": "i-0123456789012"
//       },
//       {
//         "Name": "InstanceType",
//         "Value": "t3.medium"
//       },
//       {
//         "Name": "cpu",
//         "Value": "cpu-total"
//       }
//     ]
//   }'

// Function to build and sign the ListEntitiesForMetric POST request
func makeListEntitiesForMetricRequest() {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	if err != nil {
		fmt.Println("Error loading AWS config:", err)
		return
	}

	signer := v4.NewSigner()

	instanceID := awsservice.GetInstanceId()
	body := []byte(fmt.Sprintf(`{
        "Namespace": "CWAgent",
        "MetricName": "cpu_usage_idle",
        "Dimensions": [
            {"Name": "InstanceId", "Value": "%s"},
            {"Name": "InstanceType", "Value": "t3.medium"},
            {"Name": "cpu", "Value": "cpu-total"}
        ]
    }`, instanceID))

	h := sha256.New()
	h.Write(body)
	payloadHash := hex.EncodeToString(h.Sum(nil))

	// build the request
	req, err := http.NewRequest("POST", "https://monitoring.us-west-2.amazonaws.com/", bytes.NewReader(body))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Target", "com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric")
	req.Header.Set("Content-Encoding", "amz-1.0")

	// creds
	credentials, err := cfg.Credentials.Retrieve(context.TODO())
	if err != nil {
		fmt.Println("Error retrieving credentials:", err)
		return
	}

	req.Header.Set("x-amz-security-token", credentials.SessionToken)

	// sign the request
	err = signer.SignHTTP(context.TODO(), credentials, req, payloadHash, "monitoring", "us-west-2", time.Now())
	if err != nil {
		fmt.Println("Error signing request:", err)
		return
	}

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()
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
	instanceId := awsservice.GetInstanceId()
	awsservice.DeleteLogGroupAndStream(instanceId, instanceId)
	paths, _ := filepath.Glob(logFilePath + "*")
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// checkData queries CWL and verifies the number of log lines.
func checkData(t *testing.T, start time.Time, lineCount int) {
	instanceId := awsservice.GetInstanceId()
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
