// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package collection_interval

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	// Let the agent run for 2 minute intervals.
	agentRuntime    = 2 * time.Minute
	metricName      = "disk_used_percent"
	periodInSeconds = 60
	retryCount      = 3
)

type input struct {
	lowerBoundInclusive int
	upperBoundInclusive int
	dataInput           string
	testDescription     string
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// Sets the collection interval and makes sure we get the numbers of expected metrics
// With the chance of jitter the sample count of metrics could be lower or higher than expected number of metrics
// The bounds are to account for jitter
// Add a shorter force flush interval than collection interval to make sure sample count are closer to correct
func TestCollectionInterval(t *testing.T) {

	parameters := []input{
		{
			testDescription:     "No collection interval given default to 60s",
			dataInput:           "resources/default.json",
			lowerBoundInclusive: 1,
			upperBoundInclusive: 3,
		},
		{
			testDescription:     "Agent has 10s second collection interval",
			dataInput:           "resources/agent_interval_10s.json",
			lowerBoundInclusive: 11,
			upperBoundInclusive: 13,
		},
		{
			testDescription:     "Metric disk has 10s collection interval",
			dataInput:           "resources/metric_interval_10s.json",
			lowerBoundInclusive: 11,
			upperBoundInclusive: 13,
		},
		{
			testDescription:     "Agent has 60s collection interval, disk has 10s collection interval use disk collection interval",
			dataInput:           "resources/metric_override_interval_10s.json",
			lowerBoundInclusive: 11,
			upperBoundInclusive: 13,
		},
	}

	for _, parameter := range parameters {
		t.Run(fmt.Sprintf("test description %s resource file location %s number of metrics lower bound %d and upper bound %d",
			parameter.testDescription, parameter.dataInput, parameter.lowerBoundInclusive, parameter.upperBoundInclusive), func(t *testing.T) {
			common.CopyFile(parameter.dataInput, common.ConfigOutputPath)
			hostName, err := os.Hostname()
			if err != nil {
				t.Fatalf("Can't get hostname")
			}
			dimensions := []types.Dimension{
				{
					Name:  aws.String(common.Host),
					Value: aws.String(hostName),
				},
			}
			pass := false
			for i := 0; i < retryCount; i++ {
				// Start at the beginning of next minute so cw metrics sample count will only be in the next minute for minute aggregation
				currentTime := time.Now()
				startTime := currentTime.Truncate(time.Minute).Add(time.Minute)
				duration := startTime.Sub(currentTime)
				time.Sleep(duration)
				common.StartAgent(common.ConfigOutputPath, true, false)
				time.Sleep(agentRuntime)
				common.StopAgent()
				endTime := time.Now()
				if awsservice.ValidateSampleCount(metricName, common.Namespace, dimensions,
					startTime, endTime,
					parameter.lowerBoundInclusive, parameter.upperBoundInclusive, periodInSeconds) {
					pass = true
					break
				}
			}
			if !pass {
				t.Fail()
			}
		})
	}
}
