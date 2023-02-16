// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metrics_number_dimension

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configOutputPath         = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	configJSON               = "/config.json"
	namespace                = "MetricNumberDimensionTest"
	instanceId               = "InstanceId"
	MaxDimensionCountAllowed = 30
)

// Let the agent run for 2 minutes. This will give agent enough time to call server
const agentRuntime = 2 * time.Minute

type input struct {
	resourcePath         string
	failToStart          bool
	numberDimensionsInCW int
	metricName           string
}

type metric struct {
	name  string
	value string
}

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// Must run this test with parallel 1 since this will fail if more than one test is running at the same time
func TestNumberMetricDimension(t *testing.T) {

	parameters := []input{
		{
			resourcePath:         "resources/10_dimension",
			failToStart:          false,
			numberDimensionsInCW: 10,
			metricName:           "mem_used_percent",
		},
		{
			resourcePath:         "resources/30_dimension",
			failToStart:          false,
			numberDimensionsInCW: 30,
			metricName:           "mem_used_percent",
		},
		{
			resourcePath:         "resources/35_dimension",
			failToStart:          true,
			numberDimensionsInCW: 35,
			metricName:           "mem_used_percent",
		},
	}

	for _, parameter := range parameters {
		//before test run
		log.Printf("resource file location %s fail to start %t input number dimension %d metric name %s",
			parameter.resourcePath, parameter.failToStart, parameter.numberDimensionsInCW, parameter.metricName)

		t.Run(fmt.Sprintf("resource file location %s find target %t", parameter.resourcePath, parameter.failToStart), func(t *testing.T) {
			common.CopyFile(parameter.resourcePath+configJSON, configOutputPath)
			err := common.StartAgent(configOutputPath, false)

			// for append dimension we auto fail over 30 for custom metrics (statsd we collect remove dimension and continue)
			// Go output starts at the time of failure so the failure message gets chopped off. Thus have to use if there is an error and just assume reason.
			if parameter.failToStart && err == nil {
				t.Fatalf("Agent should not have started for append %v dimension", parameter.numberDimensionsInCW)
			} else if parameter.failToStart {
				t.Logf("Agent could not start due to appending more than %v dimension", MaxDimensionCountAllowed)
				return
			}

			time.Sleep(agentRuntime)
			log.Printf("Agent has been running for : %s", agentRuntime.String())
			common.StopAgent()

			// test for cloud watch metrics
			dimensionFilter, err := awsservice.BuildDimensionFilterList(parameter.numberDimensionsInCW)
			if err != nil {
				t.Fatalf("Failed to build dimension filter list: %v", err)
			}

			err = awsservice.ValidateMetric(parameter.metricName, namespace, dimensionFilter)
			if err != nil {
				t.Fatalf("Validate metrics failed: %v", err)
			}
		})
	}
}
