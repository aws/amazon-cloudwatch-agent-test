// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/qri-io/jsonschema"
)

const (
	logStreamRetry = 20
	retryInterval  = 10 * time.Second
	NoLogTypeFound = "NoLogTypeFound"
)

// catch ResourceNotFoundException when deleting the log group and log stream, as these
// are not useful exceptions to log errors on during cleanup
var rnf *types.ResourceNotFoundException

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

// ValidateLogs queries a given LogGroup/LogStream combination given the start and end times, and executes an
// arbitrary validator function on the found logs.
func ValidateLogs(logGroup, logStream string, since, until *time.Time, validators ...LogEventsValidator) error {
	log.Printf("Checking %s/%s", logGroup, logStream)

	events, err := GetLogsSince(logGroup, logStream, since, until)
	if err != nil {
		return err
	}

	for _, validator := range validators {
		if err = validator(events); err != nil {
			return err
		}
	}
	return nil
}

// GetLogsSince makes GetLogEvents API calls, paginates through the results for the given time frame, and returns
// the raw log strings
func GetLogsSince(logGroup, logStream string, since, until *time.Time) ([]types.OutputLogEvent, error) {
	var events []types.OutputLogEvent

	// https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/API_GetLogEvents.html
	// GetLogEvents can return an empty result while still having more log events on a subsequent page,
	// so rather than expecting all the events to show up in one GetLogEvents API call, we need to paginate.
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		StartFromHead: aws.Bool(true), // read from the beginning
	}

	if since != nil {
		params.StartTime = aws.Int64(since.UnixMilli())
	}

	if until != nil {
		params.EndTime = aws.Int64(until.UnixMilli())
	}

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
				time.Sleep(30 * time.Second)
				continue
			}

			// if the error is not a ResourceNotFoundException, we should fail here.
			return events, err
		}

		for _, e := range output.Events {
			events = append(events, e)
		}

		if nextToken != nil && output.NextForwardToken != nil && *output.NextForwardToken == *nextToken {
			// From the docs: If you have reached the end of the stream, it returns the same token you passed in.
			log.Printf("Done paginating log events for %s/%s and found %d logs", logGroup, logStream, len(events))
			break
		}

		nextToken = output.NextForwardToken
	}
	return events, nil
}

// IsLogGroupExists confirms whether the logGroupName exists or not
func IsLogGroupExists(logGroupName string, logGroupClassArg ...types.LogGroupClass) bool {
	var logGroupClass types.LogGroupClass
	if len(logGroupClassArg) > 0 {
		logGroupClass = logGroupClassArg[0]
	} else {
		logGroupClass = types.LogGroupClassStandard
	}
	describeLogGroupInput := cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(logGroupName),
		LogGroupClass:      logGroupClass,
	}

	describeLogGroupOutput, err := CwlClient.DescribeLogGroups(ctx, &describeLogGroupInput)

	if err != nil {
		log.Println("error occurred while calling DescribeLogGroups", err)
		return false
	}

	return len(describeLogGroupOutput.LogGroups) > 0
}

// GetLogQueryStats for the log group between start/end (in epoch seconds) for the
// query string.
func GetLogQueryStats(logGroupName string, startTime, endTime int64, queryString string) (*types.QueryStatistics, error) {
	output, err := CwlClient.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupName: aws.String(logGroupName),
		StartTime:    aws.Int64(startTime),
		EndTime:      aws.Int64(endTime),
		QueryString:  aws.String(queryString),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to start query for log group (%s): %w", logGroupName, err)
	}

	// Sleep a fixed amount of time after making the query to give it time to
	// process the request.
	time.Sleep(retryInterval)

	var attempts int
	for {
		results, err := CwlClient.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{
			QueryId: output.QueryId,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get query results for log group (%s): %w", logGroupName, err)
		}
		switch results.Status {
		case types.QueryStatusScheduled, types.QueryStatusRunning, types.QueryStatusUnknown:
			if attempts >= StandardRetries {
				return nil, fmt.Errorf("attempted get query results after %s without success. final status: %v", time.Duration(attempts)*retryInterval, results.Status)
			}
			attempts++
			time.Sleep(retryInterval)
		case types.QueryStatusComplete:
			return results.Statistics, nil
		default:
			return nil, fmt.Errorf("unexpected query status: %v", results.Status)
		}
	}
}

// GetLogQueryResults for the log group between start/end (in epoch seconds) for the
// query string.
func GetLogQueryResults(logGroupName string, startTime, endTime int64, queryString string) ([][]types.ResultField, error) {
	output, err := CwlClient.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupName: aws.String(logGroupName),
		StartTime:    aws.Int64(startTime),
		EndTime:      aws.Int64(endTime),
		QueryString:  aws.String(queryString),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to start query for log group (%s): %w", logGroupName, err)
	}

	// Sleep a fixed amount of time after making the query to give it time to
	// process the request.
	time.Sleep(retryInterval)

	var attempts int
	for {
		results, err := CwlClient.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{
			QueryId: output.QueryId,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get query results for log group (%s): %w", logGroupName, err)
		}
		switch results.Status {
		case types.QueryStatusScheduled, types.QueryStatusRunning, types.QueryStatusUnknown:
			if attempts >= StandardRetries {
				return nil, fmt.Errorf("attempted get query results after %s without success. final status: %v", time.Duration(attempts)*retryInterval, results.Status)
			}
			attempts++
			time.Sleep(retryInterval)
		case types.QueryStatusComplete:
			return results.Results, nil
		default:
			return nil, fmt.Errorf("unexpected query status: %v", results.Status)
		}
	}
}

func GetLogStreams(logGroupName string) []types.LogStream {
	for i := 0; i < logStreamRetry; i++ {
		describeLogStreamsOutput, err := CwlClient.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: aws.String(logGroupName),
			OrderBy:      types.OrderByLastEventTime,
			Descending:   aws.Bool(true),
			Limit:        aws.Int32(10),
		})

		if err != nil {
			log.Printf("failed to get log streams for log group: %v - err: %v", logGroupName, err)
			continue
		}

		if len(describeLogStreamsOutput.LogStreams) > 0 {
			return describeLogStreamsOutput.LogStreams
		}

		time.Sleep(retryInterval)
	}

	return []types.LogStream{}
}

func GetLogStreamNames(logGroupName string) []string {
	var logStreamNames []string
	for _, stream := range GetLogStreams(logGroupName) {
		logStreamNames = append(logStreamNames, *stream.LogStreamName)
	}
	return logStreamNames
}

type LogEventValidator func(event types.OutputLogEvent) error

type LogEventsValidator func(events []types.OutputLogEvent) error

type SchemaRetriever func(message string) (string, error)

func WithSchema(schema string) SchemaRetriever {
	return func(_ string) (string, error) {
		return schema, nil
	}
}

func AssertLogSchema(schemaRetriever SchemaRetriever) LogEventValidator {
	return func(event types.OutputLogEvent) error {
		message := *event.Message
		if schemaRetriever == nil {
			return errors.New("nil schema retriever")
		}
		schema, err := schemaRetriever(*event.Message)
		if err != nil {
			return fmt.Errorf("unable to retrieve schema: %w", err)
		}
		keyErrors, err := jsonschema.Must(schema).ValidateBytes(context.Background(), []byte(message))
		if err != nil {
			return fmt.Errorf("failed to execute schema validator: %w", err)
		} else if len(keyErrors) > 0 {
			return fmt.Errorf("failed schema validation: %v | schema: %s | log: %s", keyErrors, schema, message)
		}
		return nil
	}
}

func AssertLogContainsSubstring(substr string) LogEventValidator {
	return func(event types.OutputLogEvent) error {
		if !strings.Contains(*event.Message, substr) {
			return fmt.Errorf("log event message missing substring (%s): %s", substr, *event.Message)
		}
		return nil
	}
}

// AssertPerLog runs each validator on each of the log events. Fails fast.
func AssertPerLog(validators ...LogEventValidator) LogEventsValidator {
	return func(events []types.OutputLogEvent) error {
		for _, event := range events {
			for _, validator := range validators {
				if err := validator(event); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func AssertLogsNotEmpty() LogEventsValidator {
	return func(events []types.OutputLogEvent) error {
		if len(events) == 0 {
			return errors.New("no log events")
		}
		return nil
	}
}

func AssertLogsCount(count int) LogEventsValidator {
	return func(events []types.OutputLogEvent) error {
		if len(events) != count {
			return fmt.Errorf("actual log events count (%v) does not match expected (%v)", len(events), count)
		}
		return nil
	}
}

func AssertNoDuplicateLogs() LogEventsValidator {
	return func(events []types.OutputLogEvent) error {
		byTimestamp := make(map[int64]map[string]struct{})
		for _, event := range events {
			message := *event.Message
			timestamp := *event.Timestamp
			messages, ok := byTimestamp[timestamp]
			if !ok {
				messages = map[string]struct{}{}
				byTimestamp[timestamp] = messages
			}
			_, ok = messages[message]
			if ok {
				return fmt.Errorf("duplicate message found at %v | message: %s", time.UnixMilli(timestamp), message)
			}
			messages[message] = struct{}{}
		}
		return nil
	}
}

func GetLogEventCountPerType(logGroup, logStream string, since, until *time.Time) (map[string]int, error) {
	var typeFrequency = make(map[string]int)
	events, err := GetLogsSince(logGroup, logStream, since, until)

	// if there is an error, return the empty map
	if err != nil {
		return typeFrequency, err
	}

	typeFrequency[NoLogTypeFound] = 0
	for _, event := range events {
		message := *event.Message
		var eksClusterType EKSClusterType
		innerErr := json.Unmarshal([]byte(message), &eksClusterType)
		if innerErr != nil {
			typeFrequency[NoLogTypeFound]++
		}

		typeFrequency[eksClusterType.Type]++
	}

	return typeFrequency, nil
}

type CloudWatchEMF struct {
	CloudWatchMetrics []struct {
		Namespace  string     `json:"Namespace"`
		Dimensions [][]string `json:"Dimensions"`
		Metrics    []struct {
			Name              string `json:"Name"`
			Unit              string `json:"Unit"`
			StorageResolution int    `json:"StorageResolution"`
		} `json:"Metrics"`
	} `json:"CloudWatchMetrics"`
}

func CountMetricsInEMFLogs(logGroupName string) (int, error) {
	streams := GetLogStreams(logGroupName)
	if len(streams) == 0 {
		return 0, fmt.Errorf("no log streams found")
	}

	// Get all logs from the most recent stream
	events, err := GetLogsSince(logGroupName, *streams[0].LogStreamName, nil, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get log events: %v", err)
	}

	totalMetrics := 0
	eventCount := 0

	// Process each event and count metrics
	for _, event := range events {
		var metricLog CloudWatchEMF
		if err := json.Unmarshal([]byte(*event.Message), &metricLog); err != nil {
			log.Printf("Failed to parse log event: %v", err)
			continue
		}

		for _, cwMetric := range metricLog.CloudWatchMetrics {
			totalMetrics += len(cwMetric.Metrics)
		}
		eventCount++
	}

	return totalMetrics, nil
}

func GetNeuronCoreUtilizationPerCore(logGroup, logStream string, since, until *time.Time) (map[string]float64, error) {
	var coreUtilization = make(map[string]float64)
	var data map[string]interface{}

	events, err := GetLogsSince(logGroup, logStream, since, until)

	// if there is an error, return the empty map
	if err != nil {
		return coreUtilization, err
	}

	for _, event := range events {
		message := *event.Message

		var eksClusterType EKSClusterType
		innerErr := json.Unmarshal([]byte(message), &eksClusterType)
		if innerErr != nil || !strings.Contains(eksClusterType.Type, "NodeAWSNeuronCore") {
			continue
		}

		err := json.Unmarshal([]byte(message), &data)
		if err != nil {
			return coreUtilization, err
		}

		if core, ok := data["NeuronCore"].(string); ok {
			if util, ok := data["node_neuroncore_utilization"].(float64); ok {
				coreUtilization[core] = util
			}
		}
	}

	return coreUtilization, nil
}
