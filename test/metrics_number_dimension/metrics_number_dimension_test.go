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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configOutputPath         = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	configJSON               = "/config.json"
	namespace                = "MetricNumberDimensionTest"
	instanceId               = "InstanceId"
	loremIpsum               = "Lorem ipsum dolor sit amet consectetur adipiscing elit Vivamus non mauris malesuada mattis ex eget porttitor purus Suspendisse potenti Praesent vel sollicitudin ipsum Quisque luctus pretium lorem non faucibus Ut vel quam dui Nunc fermentum condimentum consectetur Morbi tellus mauris tristique tincidunt elit consectetur hendrerit placerat dui In nulla erat finibus eget erat a hendrerit sodales urna In sapien purus auctor sit amet congue ut congue eget nisi Vivamus sed neque ut ligula lobortis accumsan quis id metus In feugiat velit et leo mattis non fringilla dui elementum Proin a nisi ac sapien vulputate consequat Vestibulum eu tellus mi Integer consectetur efficitur"
	appendMetric             = "append"
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
			err := common.StartAgent(configOutputPath, false, false)

			// for append dimension we auto fail over 30 for custom metrics (statsd we collect remove dimension and continue)
			// Go output starts at the time of failure so the failure message gets chopped off. Thus have to use if there is an error and just assume reason.
			if parameter.failToStart && err == nil {
				t.Fatalf("Agent should not have started for append %v dimension", parameter.numberDimensionsInCW)
			} else if parameter.failToStart {
				log.Printf("Agent could not start due to appending more than %v dimension", MaxDimensionCountAllowed)
				return
			}

			time.Sleep(agentRuntime)
			log.Printf("Agent has been running for : %s", agentRuntime.String())
			common.StopAgent()

			// test for cloud watch metrics
			dimensionFilter := buildDimensionFilterList(parameter.numberDimensionsInCW)
			awsservice.ValidateMetrics(t, parameter.metricName, namespace, dimensionFilter)
		})
	}
}

func buildDimensionFilterList(appendDimension int) []types.DimensionFilter {
	// we append dimension from 0 to max number - 2
	// then we add dimension instance id
	// thus for max dimension 10, 0 to 8 + instance id = 10 dimension
	ec2InstanceId := awsservice.GetInstanceId()
	dimensionFilter := make([]types.DimensionFilter, appendDimension)
	for i := 0; i < appendDimension-1; i++ {
		dimensionFilter[i] = types.DimensionFilter{
			Name:  aws.String(fmt.Sprintf("%s%d", appendMetric, i)),
			Value: aws.String(fmt.Sprintf("%s%d", loremIpsum+appendMetric, i)),
		}
	}
	dimensionFilter[appendDimension-1] = types.DimensionFilter{
		Name:  aws.String(instanceId),
		Value: aws.String(ec2InstanceId),
	}
	return dimensionFilter
}
