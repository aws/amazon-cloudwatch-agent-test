// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// catch ResourceNotFoundException when deleting the log group and log stream, as these
// are not useful exceptions to log errors on during cleanup
var rnf *types.ResourceNotFoundException

// ValidateNumberOfLogsFound takes a log group and log stream, and fetches the log events via the GetLogEvents
// API for all the logs since a given timestamp, and checks if the number of log events matches
// the expected value.
func ValidateNumberOfLogsFound(logGroup, logStream string, numExpectedLogs int, since time.Time) error {
	log.Printf("Checking %s/%s since %s for %d expected logs", logGroup, logStream, since.UTC().Format(time.RFC3339), numExpectedLogs)

	var (
		nextToken    *string
		attempts     = 0
		sinceMs      = since.UnixNano() / 1e6 // convert to millisecond timestamp
		numLogsFound = 0
	)

	// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
	// GetLogEvents can return an empty result while still having more log events on a subsequent page,
	// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartTime:     aws.Int64(sinceMs),
	}

	for {
		if nextToken != nil {
			params.NextToken = nextToken
		}

		output, err := cwlClient.GetLogEvents(cxt, params)

		attempts += 1

		if err != nil {
			if errors.As(err, &rnf) && attempts <= StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(time.Minute)
				continue
			}

			return fmt.Errorf("error occurred while getting log events: %v", err.Error())
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
	if numExpectedLogs != numLogsFound {
		return fmt.Errorf("number of logs lines does not match with each other, expected number of logs lines/actual  number of logs lines %d/%d", numExpectedLogs, numLogsFound)
	}

	return nil
}

// ValidateLogsInOrder takes a log group, log stream, a list of specific log lines and a timestamp.
// It should query the given log stream for log events, and then confirm that the log lines that are
// returned match the expected log lines. This also sanitizes the log lines from both the output and
// the expected lines input to ensure that they don't diverge in JSON representation (" vs ')
func ValidateLogsInOrder(logGroup, logStream string, logLines []string, since time.Time) error {
	var (
		nextToken *string
		attempts  = 0
		sinceMs   = since.UnixNano() / 1e6 // convert to millisecond timestamp
		foundLogs = make([]string, 0)
	)

	log.Printf("Checking %s/%s since %s for %d expected logs", logGroup, logStream, since.UTC().Format(time.RFC3339), len(logLines))

	// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
	// GetLogEvents can return an empty result while still having more log events on a subsequent page,
	// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartTime:     aws.Int64(sinceMs),
		StartFromHead: aws.Bool(true), // read from the beginning
	}

	for {
		if nextToken != nil {
			params.NextToken = nextToken
		}

		output, err := cwlClient.GetLogEvents(cxt, params)

		attempts += 1

		if err != nil {
			if errors.As(err, &rnf) && attempts <= StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(time.Minute)
				continue
			}

			return fmt.Errorf("error occurred while getting log events: %v", err.Error())
		}

		for _, e := range output.Events {
			foundLogs = append(foundLogs, *e.Message)
		}

		if nextToken != nil && output.NextForwardToken != nil && *output.NextForwardToken == *nextToken {
			// From the docs: If you have reached the end of the stream, it returns the same token you passed in.
			log.Printf("done paginating log events for %s/%s and found %d logs", logGroup, logStream, len(foundLogs))
			break
		}

		nextToken = output.NextForwardToken
	}

	// Validate that each of the logs are found, in order and in full.
	if len(foundLogs) != len(logLines) {
		return fmt.Errorf("number of logs lines does not match with each other, expected number of logs lines/actual  number of logs lines %d/%d", len(logLines), len(foundLogs))
	}
	for i := 0; i < len(logLines); i++ {
		expectedLogs := strings.ReplaceAll(logLines[i], "'", "\"")
		actualLogs := strings.ReplaceAll(foundLogs[i], "'", "\"")
		if expectedLogs != actualLogs {
			return fmt.Errorf("logs lines does not match with each other, \n expected logs %d \n actual logs %d \n", len(logLines), len(foundLogs))

		}
	}

	return nil
}

// IsLogGroupExist confirms whether the logGroupName exists or not
func IsLogGroupExist(logGroup string) bool {

	describeLogGroupOutput, err := cwlClient.DescribeLogGroups(cxt, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroup),
	})

	if err != nil {
		return false
	}

	if len(describeLogGroupOutput.LogGroups) > 0 {
		return true
	}

	return false
}

// DeleteLogGroupAndLogStream cleans up a log group and stream by name. This gracefully handles
// ResourceNotFoundException errors from calling the APIs
func DeleteLogGroupAndLogStream(logGroup, logStream string) {
	DeleteLogStream(logGroup, logStream)
	DeleteLogGroup(logGroup)
}

func DeleteLogStream(logGroup, logStream string) {
	_, err := cwlClient.DeleteLogStream(cxt, &cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
	})
	if err != nil && !errors.As(err, &rnf) {
		log.Printf("Error occurred while deleting log stream %s: %v", logStream, err)
	}
}

func DeleteLogGroup(logGroupName string) {
	_, err := cwlClient.DeleteLogGroup(cxt, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	if err != nil && !errors.As(err, &rnf) {
		log.Printf("Error occurred while deleting log group %s: %v", logGroupName, err)
	}
}
