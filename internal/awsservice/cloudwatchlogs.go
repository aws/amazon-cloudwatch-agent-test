// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
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

type cwlAPI interface {
	// ValidateLogs takes a log group and log stream, and fetches the log events via the GetLogEvents
	// API for all the logs since a given timestamp, and checks if the number of log events matches
	// the expected value.
	ValidateLogs(logGroup, logStream string, numExpectedLogs int, since time.Time) error

	// ValidateLogsInOrder takes a log group, log stream, a list of specific log lines and a timestamp.
	// It should query the given log stream for log events, and then confirm that the log lines that are
	// returned match the expected log lines. This also sanitizes the log lines from both the output and
	// the expected lines input to ensure that they don't diverge in JSON representation (" vs ')
	ValidateLogsInOrder(logGroup, logStream string, logLines []string, since time.Time) error

	// DeleteLogGroupAndLogStream cleans up a log group and stream by name. This gracefully handles
	// ResourceNotFoundException errors from calling the APIs
	DeleteLogGroupAndLogStream(logGroup, logStream string)

	// DeleteLogStream cleans up a log  stream by name. This gracefully handles
	// ResourceNotFoundException errors from calling the APIs
	DeleteLogStream(logGroup, logStream string)

	// DeleteLogGroup cleans up a log group  by name. This gracefully handles
	// ResourceNotFoundException errors from calling the APIs
	DeleteLogGroup(logGroup string)

	// IsLogGroupExist confirms whether the logGroupName exists or not
	IsLogGroupExist(logGroup string) bool
}

type cloudwatchLogConfig struct {
	cxt       context.Context
	cwlClient *cloudwatchlogs.Client
}

func NewCloudWatchLogsConfig(cfg aws.Config, cxt context.Context) cwlAPI {
	cwlClient := cloudwatchlogs.NewFromConfig(cfg)
	return &cloudwatchLogConfig{
		cxt:       cxt,
		cwlClient: cwlClient,
	}
}

// ValidateLogs takes a log group and log stream, and fetches the log events via the GetLogEvents
// API for all the logs since a given timestamp, and checks if the number of log events matches
// the expected value.
func (c *cloudwatchLogConfig) ValidateLogs(logGroup, logStream string, numExpectedLogs int, since time.Time) error {
	log.Printf("Checking %s/%s since %s for %d expected logs", logGroup, logStream, since.UTC().Format(time.RFC3339), numExpectedLogs)

	sinceMs := since.UnixNano() / 1e6 // convert to millisecond timestamp

	var nextToken *string
	attempts := 0
	numLogsFound := 0

	for {
		// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
		// GetLogEvents can return an empty result while still having more log events on a subsequent page,
		// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.
		getLogEventsInput := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(logStream),
			StartTime:     aws.Int64(sinceMs),
			NextToken:     nextToken,
		}

		output, err := c.cwlClient.GetLogEvents(c.cxt, getLogEventsInput)

		attempts += 1

		if err != nil {
			if errors.As(err, &rnf) && attempts <= StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(time.Minute)
				continue
			}

			return err
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
		return fmt.Errorf("Number of logs lines does not match with each other, expected number of logs lines/actual  number of logs lines %d/%d", numExpectedLogs, numLogsFound)
	}

	return nil
}

// ValidateLogsInOrder takes a log group, log stream, a list of specific log lines and a timestamp.
// It should query the given log stream for log events, and then confirm that the log lines that are
// returned match the expected log lines. This also sanitizes the log lines from both the output and
// the expected lines input to ensure that they don't diverge in JSON representation (" vs ')
func (c *cloudwatchLogConfig) ValidateLogsInOrder(logGroup, logStream string, logLines []string, since time.Time) error {
	log.Printf("Checking %s/%s since %s for %d expected logs", logGroup, logStream, since.UTC().Format(time.RFC3339), len(logLines))

	sinceMs := since.UnixNano() / 1e6 // convert to millisecond timestamp

	// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
	// GetLogEvents can return an empty result while still having more log events on a subsequent page,
	// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.

	var nextToken *string
	attempts := 0
	foundLogs := make([]string, 0)

	for {
		getLogEventsInput := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(logStream),
			StartTime:     aws.Int64(sinceMs),
			StartFromHead: aws.Bool(true), // read from the beginning
			NextToken:     nextToken,
		}

		output, err := c.cwlClient.GetLogEvents(c.cxt, getLogEventsInput)

		attempts += 1

		if err != nil {
			if errors.As(err, &rnf) && attempts <= StandardRetries {
				// The log group/stream hasn't been created yet, so wait and retry
				time.Sleep(time.Minute)
				continue
			}

			return err
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
func (c *cloudwatchLogConfig) IsLogGroupExist(logGroup string) bool {

	describeLogGroupOutput, err := c.cwlClient.DescribeLogGroups(c.cxt, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroup),
	})

	if err != nil {
		return false
	}

	if len(describeLogGroupOutput.LogGroups) == 0 {
		return false
	}

	return true
}

// DeleteLogGroupAndLogStream cleans up a log group and stream by name. This gracefully handles
// ResourceNotFoundException errors from calling the APIs
func (c *cloudwatchLogConfig) DeleteLogGroupAndLogStream(logGroup, logStream string) {
	c.DeleteLogStream(logGroup, logStream)
	c.DeleteLogGroup(logGroup)
}

func (c *cloudwatchLogConfig) DeleteLogStream(logGroup, logStream string) {
	_, err := c.cwlClient.DeleteLogStream(c.cxt, &cloudwatchlogs.DeleteLogStreamInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
	})
	if err != nil && !errors.As(err, &rnf) {
		log.Printf("Error occurred while deleting log stream %s: %v", logStream, err)
	}
}

func (c *cloudwatchLogConfig) DeleteLogGroup(logGroupName string) {
	_, err := c.cwlClient.DeleteLogGroup(c.cxt, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	if err != nil && !errors.As(err, &rnf) {
		log.Printf("Error occurred while deleting log group %s: %v", logGroupName, err)
	}
}
