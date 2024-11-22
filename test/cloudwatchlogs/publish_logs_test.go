// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudwatchlogs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/uuid"
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
	sleepForExtendedFlush         = 180 * time.Second           // increase flush time for the two main tests
	retryWaitTime                 = 30 * time.Second
	configPathAutoRemoval         = "resources/config_auto_removal.json"
	standardLogGroupClass         = "STANDARD"
	infrequentAccessLogGroupClass = "INFREQUENT_ACCESS"
	cwlPerfEndpoint               = "https://logs.us-west-2.amazonaws.com"
	pdxRegionalCode               = "us-west-2"

	entityType        = "@entity.KeyAttributes.Type"
	entityName        = "@entity.KeyAttributes.Name"
	entityEnvironment = "@entity.KeyAttributes.Environment"
	entityPlatform    = "@entity.Attributes.PlatformType"
	entityInstanceId  = "@entity.Attributes.EC2.InstanceId"
	queryString       = "fields @message, @entity.KeyAttributes.Type, @entity.KeyAttributes.Name, @entity.KeyAttributes.Environment, @entity.Attributes.PlatformType, @entity.Attributes.EC2.InstanceId"
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
	resourceNotFoundException *types.ResourceNotFoundException
	cwlClient                 *cloudwatchlogs.Client
	ec2Client                 *ec2.Client
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

type expectedEntity struct {
	entityType   string
	name         string
	environment  string
	platformType string
	instanceId   string
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
	awsCfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(pdxRegionalCode),
	)
	if err != nil {
		log.Fatalf("Failed to load default config: %v", err)
	}

	cwlClient = cloudwatchlogs.NewFromConfig(awsCfg, func(o *cloudwatchlogs.Options) {
		o.BaseEndpoint = aws.String(cwlPerfEndpoint)
	})
	ec2Client = ec2.NewFromConfig(awsCfg)
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
			time.Sleep(sleepForExtendedFlush)
			writeLogLines(t, f, param.iterations)
			time.Sleep(sleepForExtendedFlush)
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

// TestWriteLogsWithEntityInfo writes logs and validates that the
// log events are associated with entities from CloudWatch Logs
func TestWriteLogsWithEntityInfo(t *testing.T) {
	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)

	// Define tags to create for EC2 test case
	tagsToCreate := []ec2Types.Tag{
		{
			Key:   aws.String("service"),
			Value: aws.String("service-test"),
		},
	}

	testCases := map[string]struct {
		agentConfigPath string
		iterations      int
		useEC2Tag       bool
		expectedEntity  expectedEntity
	}{
		"IAMRole": {
			agentConfigPath: filepath.Join("resources", "config_log.json"),
			iterations:      1000,
			expectedEntity: expectedEntity{
				entityType:   "Service",
				name:         "cwa-e2e-iam-role", //should match the name of the IAM role used in our testing
				environment:  "ec2:default",
				platformType: "AWS::EC2",
				instanceId:   instanceId,
			},
		},
		"ServiceInConfig": {
			agentConfigPath: filepath.Join("resources", "config_log_service_name.json"),
			iterations:      1000,
			expectedEntity: expectedEntity{
				entityType:   "Service",
				name:         "service-in-config",     //should match the service.name value in the config file
				environment:  "environment-in-config", //should match the deployment.environment value in the config file
				platformType: "AWS::EC2",
				instanceId:   instanceId,
			},
		},
		"EC2Tags": {
			agentConfigPath: filepath.Join("resources", "config_log.json"),
			iterations:      1000,
			useEC2Tag:       true,
			expectedEntity: expectedEntity{
				entityType:   "Service",
				name:         "service-test", //should match the value in tagsToCreate
				environment:  "ec2:default",
				platformType: "AWS::EC2",
				instanceId:   instanceId,
			},
		},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() {
				// delete the log group/stream after each test case
				awsservice.DeleteLogGroupAndStream(instanceId, instanceId)

				// delete EC2 tags added to the instance for the test
				if testCase.useEC2Tag {
					input := &ec2.DeleteTagsInput{
						Resources: []string{instanceId},
						Tags:      tagsToCreate,
					}
					_, err := ec2Client.DeleteTags(context.TODO(), input)
					assert.NoError(t, err)
					// Add a short delay to ensure tag deletion propagates
					time.Sleep(5 * time.Second)
				}
			})
			if testCase.useEC2Tag {
				// enable instance metadata tags
				modifyInput := &ec2.ModifyInstanceMetadataOptionsInput{
					InstanceId:           aws.String(instanceId),
					InstanceMetadataTags: ec2Types.InstanceMetadataTagsStateEnabled,
				}
				_, modifyErr := ec2Client.ModifyInstanceMetadataOptions(context.TODO(), modifyInput)
				assert.NoError(t, modifyErr)

				input := &ec2.CreateTagsInput{
					Resources: []string{instanceId},
					Tags:      tagsToCreate,
				}
				_, createErr := ec2Client.CreateTags(context.TODO(), input)
				assert.NoError(t, createErr)
			}
			id := uuid.New()
			f, err := os.Create(logFilePath + "-" + id.String())
			if err != nil {
				t.Fatalf("Error occurred creating log file for writing: %v", err)
			}
			common.DeleteFile(common.AgentLogFile)
			common.TouchFile(common.AgentLogFile)

			common.CopyFile(testCase.agentConfigPath, configOutputPath)

			common.StartAgent(configOutputPath, true, false)
			time.Sleep(sleepForExtendedFlush)
			writeLogLines(t, f, testCase.iterations)
			time.Sleep(sleepForExtendedFlush)
			common.StopAgent()
			end := time.Now()

			ValidateEntity(t, instanceId, instanceId, &end, testCase.expectedEntity)

			f.Close()
			os.Remove(logFilePath + "-" + id.String())
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

func ValidateEntity(t *testing.T, logGroup, logStream string, end *time.Time, expectedEntity expectedEntity) {
	log.Printf("Validating entity for log group: %s, stream: %s", logGroup, logStream)

	logGroupInfo, err := getLogGroup()
	for _, lg := range logGroupInfo {
		if *lg.LogGroupName == logGroup {
			log.Println("Log group " + *lg.LogGroupName + " exists")
			break
		}
	}
	assert.NoError(t, err)
	begin := end.Add(-sleepForExtendedFlush * 4)
	log.Printf("Query start time is " + begin.String() + " and end time is " + end.String())
	queryId, err := getLogQueryId(logGroup, &begin, end)
	assert.NoError(t, err)
	log.Printf("queryId is " + *queryId)
	result, err := getQueryResult(queryId)
	assert.NoError(t, err)
	if !assert.NotZero(t, len(result)) {
		return
	}
	requiredEntityFields := map[string]bool{
		entityType:        false,
		entityName:        false,
		entityEnvironment: false,
		entityPlatform:    false,
		entityInstanceId:  false,
	}
	for _, field := range result[0] {
		switch aws.ToString(field.Field) {
		case entityType:
			requiredEntityFields[entityType] = true
			assert.Equal(t, expectedEntity.entityType, aws.ToString(field.Value))
		case entityName:
			requiredEntityFields[entityName] = true
			assert.Equal(t, expectedEntity.name, aws.ToString(field.Value))
		case entityEnvironment:
			requiredEntityFields[entityEnvironment] = true
			assert.Equal(t, expectedEntity.environment, aws.ToString(field.Value))
		case entityPlatform:
			requiredEntityFields[entityPlatform] = true
			assert.Equal(t, expectedEntity.platformType, aws.ToString(field.Value))
		case entityInstanceId:
			requiredEntityFields[entityInstanceId] = true
			assert.Equal(t, expectedEntity.instanceId, aws.ToString(field.Value))
		}
		fmt.Printf("%s: %s\n", aws.ToString(field.Field), aws.ToString(field.Value))
	}
	allEntityFieldsFound := true
	for field, value := range requiredEntityFields {
		if !value {
			log.Printf("Missing required entity field: %s", field)
			allEntityFieldsFound = false
		}
	}
	assert.True(t, allEntityFieldsFound)
}

func getLogQueryId(logGroup string, since, until *time.Time) (*string, error) {
	var queryId *string
	params := &cloudwatchlogs.StartQueryInput{
		QueryString:  aws.String(queryString),
		LogGroupName: aws.String(logGroup),
	}
	if since != nil {
		params.StartTime = aws.Int64(since.UnixMilli())
	}
	if until != nil {
		params.EndTime = aws.Int64(until.UnixMilli())
	}
	attempts := 0

	for {
		output, err := cwlClient.StartQuery(context.Background(), params)
		attempts += 1

		if err != nil {
			if errors.As(err, &resourceNotFoundException) && attempts <= awsservice.StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(retryWaitTime)
				continue
			}

			// if the error is not a ResourceNotFoundException, we should fail here.
			return queryId, err
		}
		queryId = output.QueryId
		return queryId, err
	}
}

func getQueryResult(queryId *string) ([][]types.ResultField, error) {
	attempts := 0
	var results [][]types.ResultField
	params := &cloudwatchlogs.GetQueryResultsInput{
		QueryId: aws.String(*queryId),
	}
	for {
		if attempts > awsservice.StandardRetries {
			return results, errors.New("exceeded retry count")
		}
		result, err := cwlClient.GetQueryResults(context.Background(), params)
		log.Printf("GetQueryResult status is: %v", result.Status)
		attempts += 1
		if result.Status != types.QueryStatusComplete {
			log.Printf("GetQueryResult: sleeping for 5 seconds until status is complete")
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("GetQueryResult: result length is %d", len(result.Results))
		if err != nil {
			if errors.As(err, &resourceNotFoundException) {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(retryWaitTime)
				continue
			}

			// if the error is not a ResourceNotFoundException, we should fail here.
			return results, err
		}
		results = result.Results
		return results, err
	}
}

func getLogGroup() ([]types.LogGroup, error) {
	attempts := 0
	var logGroups []types.LogGroup
	params := &cloudwatchlogs.DescribeLogGroupsInput{}
	for {
		output, err := cwlClient.DescribeLogGroups(context.Background(), params)

		attempts += 1

		if err != nil {
			if errors.As(err, &resourceNotFoundException) && attempts <= awsservice.StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(retryWaitTime)
				continue
			}

			// if the error is not a ResourceNotFoundException, we should fail here.
			return logGroups, err
		}
		logGroups = output.LogGroups
		return logGroups, err
	}
}
