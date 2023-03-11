// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
)

// catch ResourceNotFoundException when deleting the log group and log stream, as these
// are not useful exceptions to log errors on during cleanup
var rnf *types.ResourceNotFoundException

// ValidateLogs takes a log group and log stream, and fetches the log events via the GetLogEvents
// API for all the logs since a given timestamp, and checks if the number of log events matches
// the expected value.
func ValidateLogs(t *testing.T, logGroup, logStream string, numExpectedLogs int, since time.Time) {
	log.Printf("Checking %s/%s since %s for %d expected logs", logGroup, logStream, since.UTC().Format(time.RFC3339), numExpectedLogs)

	sinceMs := since.UnixNano() / 1e6 // convert to millisecond timestamp

	// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
	// GetLogEvents can return an empty result while still having more log events on a subsequent page,
	// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartTime:     aws.Int64(sinceMs),
	}

	numLogsFound := 0
	attempts := 0
	var nextToken *string

	for {
		if nextToken != nil {
			params.NextToken = nextToken
		}
		output, err := CwlClient.GetLogEvents(ctx, params)

		attempts += 1

		if err != nil {
			if errors.As(err, &rnf) && attempts <= StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(time.Minute)
				continue
			}

			// if the error is not a ResourceNotFoundException, we should fail here.
			t.Fatalf("Error occurred while getting log events: %v", err.Error())
		}

		if nextToken != nil && output.NextForwardToken != nil && *output.NextForwardToken == *nextToken {
			// From the docs: If you have reached the end of the stream, it returns the same token you passed in.
			log.Printf("Done paginating log events for %s/%s and found %d logs", logGroup, logStream, numLogsFound)
			break
		}

		nextToken = output.NextForwardToken
		numLogsFound += len(output.Events)
	}

	// using assert.Len() prints out the whole splice of log events which bloats the test log
	assert.Equal(t, numExpectedLogs, numLogsFound)
}

// DeleteLogGroupAndStream cleans up a log group and stream by name. This gracefully handles
// ResourceNotFoundException errors from calling the APIs
func DeleteLogGroupAndStream(logGroupName, logStreamName string) {
	DeleteLogStream(logGroupName, logStreamName)
	DeleteLogGroup(logGroupName)
}

// DeleteLogStream cleans up log stream by name
func DeleteLogStream(logGroupName, logStreamName string) {

	_, err := CwlClient.DeleteLogStream(ctx, &cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(logStreamName),
	})
	if err != nil && !errors.As(err, &rnf) {
		log.Printf("Error occurred while deleting log stream %s: %v", logStreamName, err)
	}
}

// DeleteLogGroup cleans up log group by name
func DeleteLogGroup(logGroupName string) {

	_, err := CwlClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	if err != nil && !errors.As(err, &rnf) {
		log.Printf("Error occurred while deleting log group %s: %v", logGroupName, err)
	}
}

// ValidateLogsInOrder takes a log group, log stream, a list of specific log lines and a timestamp.
// It should query the given log stream for log events, and then confirm that the log lines that are
// returned match the expected log lines. This also sanitizes the log lines from both the output and
// the expected lines input to ensure that they don't diverge in JSON representation (" vs ')
func ValidateLogsInOrder(t *testing.T, logGroup, logStream string, logLines []string, since time.Time) {
	log.Printf("Checking %s/%s since %s for %d expected logs", logGroup, logStream, since.UTC().Format(time.RFC3339), len(logLines))

	sinceMs := since.UnixNano() / 1e6 // convert to millisecond timestamp

	// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
	// GetLogEvents can return an empty result while still having more log events on a subsequent page,
	// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartTime:     aws.Int64(sinceMs),
		StartFromHead: aws.Bool(true), // read from the beginning
	}

	foundLogs := make([]string, 0)
	var nextToken *string
	attempts := 0

	for {
		if nextToken != nil {
			params.NextToken = nextToken
		}
		output, err := CwlClient.GetLogEvents(ctx, params)

		attempts += 1

		if err != nil {
			if errors.As(err, &rnf) && attempts <= StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(time.Minute)
				continue
			}

			// if the error is not a ResourceNotFoundException, we should fail here.
			t.Fatalf("Error occurred while getting log events: %v", err.Error())
		}

		for _, e := range output.Events {
			foundLogs = append(foundLogs, *e.Message)
		}

		if nextToken != nil && output.NextForwardToken != nil && *output.NextForwardToken == *nextToken {
			// From the docs: If you have reached the end of the stream, it returns the same token you passed in.
			log.Printf("Done paginating log events for %s/%s and found %d logs", logGroup, logStream, len(foundLogs))
			break
		}

		nextToken = output.NextForwardToken
	}

	// Validate that each of the logs are found, in order and in full.
	assert.Len(t, foundLogs, len(logLines))
	for i := 0; i < len(logLines); i++ {
		expected := strings.ReplaceAll(logLines[i], "'", "\"")
		actual := strings.ReplaceAll(foundLogs[i], "'", "\"")
		assert.Equal(t, expected, actual)
	}
}

// isLogGroupExists confirms whether the logGroupName exists or not
func IsLogGroupExists(t *testing.T, logGroupName string) bool {

	describeLogGroupInput := cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroupName),
	}

	describeLogGroupOutput, err := CwlClient.DescribeLogGroups(ctx, &describeLogGroupInput)

	if err != nil {
		t.Errorf("Error getting log group data %v", err)
	}

	if len(describeLogGroupOutput.LogGroups) > 0 {
		return true
	}

	return false
}
