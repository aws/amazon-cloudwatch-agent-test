// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package histograms

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func TestOTLPMetrics(t *testing.T) {
	instanceID := awsservice.GetInstanceId()

	startAgent(t)
	err := runOTLPPusher(instanceID)
	assert.NoError(t, err)

	time.Sleep(5 * time.Minute)

	metrics := []struct {
		name       string
		dimensions []types.Dimension
		expected   []struct {
			stat  types.Statistic
			value float64
		}
	}{
		{
			name: "my.delta.histogram",
			dimensions: []types.Dimension{
				{
					Name:  aws.String("my.delta.histogram.attr"),
					Value: aws.String("some value"),
				},
				{
					Name:  aws.String("instance_id"),
					Value: aws.String(instanceID),
				},
				{
					Name:  aws.String("service.name"),
					Value: aws.String("my.service"),
				},
			},
			expected: []struct {
				stat  types.Statistic
				value float64
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0,
				},
				{
					stat:  types.StatisticMaximum,
					value: 2,
				},
				{
					stat:  types.StatisticSum,
					value: 12, // we send Sum=2 six times in one minute
				},
				{
					stat:  types.StatisticAverage,
					value: 1, // sum / samplecount = 2/2
				},
				{
					stat:  types.StatisticSampleCount,
					value: 12, // we send Count=2 six times in one minute
				},
				{
					stat:  "p90",
					value: 5,
				},
			},
		},
		{
			name: "my.cumulative.histogram",
			dimensions: []types.Dimension{
				{
					Name:  aws.String("my.cumulative.histogram.attr"),
					Value: aws.String("some value"),
				},
				{
					Name:  aws.String("instance_id"),
					Value: aws.String(instanceID),
				},
				{
					Name:  aws.String("service.name"),
					Value: aws.String("my.service"),
				},
			},
			expected: []struct {
				stat  types.Statistic
				value float64
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0, // min/max are invalid for cumulative histograms converted to delta
				},
				{
					stat:  types.StatisticMaximum,
					value: 0, // min/max are invalid for cumulative histograms converted to delta
				},
				{
					stat:  types.StatisticSum,
					value: 12, // we send Sum=2 six times in one minute
				},
				{
					stat:  types.StatisticAverage,
					value: 1,
				},
				{
					stat:  types.StatisticSampleCount,
					value: 12, // we send Count=2 six times in one minute
				},
				{
					stat:  "p90",
					value: 5,
				},
			},
		},
		{
			name: "my.delta.exponential.histogram",
			dimensions: []types.Dimension{
				{
					Name:  aws.String("my.delta.exponential.histogram.attr"),
					Value: aws.String("some value"),
				},
				{
					Name:  aws.String("instance_id"),
					Value: aws.String(instanceID),
				},
				{
					Name:  aws.String("service.name"),
					Value: aws.String("my.service"),
				},
			},
			expected: []struct {
				stat  types.Statistic
				value float64
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0,
				},
				{
					stat:  types.StatisticMaximum,
					value: 5,
				},
				{
					stat:  types.StatisticSum,
					value: 60, // we send Sum=10 six times in one minute
				},
				{
					stat:  types.StatisticAverage,
					value: 3.33, // sum / samplecount = 10/3
				},
				{
					stat:  types.StatisticSampleCount,
					value: 18, // we send Count=3 six times in one minute
				},
				{
					stat:  "p90",
					value: 5,
				},
			},
		},
		{
			name: "my.cumulative.exponential.histogram",
			dimensions: []types.Dimension{
				{
					Name:  aws.String("service.name"),
					Value: aws.String("my.service"),
				},
				{
					Name:  aws.String("my.cumulative.exponential.histogram.attr"),
					Value: aws.String("some value"),
				},
				{
					Name:  aws.String("instance_id"),
					Value: aws.String(instanceID),
				},
			},
			expected: []struct {
				stat  types.Statistic
				value float64
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0, // min/max are invalid for cumulative histograms converted to delta
				},
				{
					stat:  types.StatisticMaximum,
					value: 0, // min/max are invalid for cumulative histograms converted to delta
				},
				{
					stat:  types.StatisticSum,
					value: 12, // we send Sum=2 six times in one minute
				},
				{
					stat:  types.StatisticAverage,
					value: 1, // sum / samplecount = 2/2
				},
				{
					stat:  types.StatisticSampleCount,
					value: 12, // we send Count=2 six times in one minute
				},
				{
					stat:  "p90",
					value: 5,
				},
			},
		},
	}

	fetcher := metric.MetricValueFetcher{}
	namespace := "CWAgent"

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			for _, e := range m.expected {
				values, err := fetcher.Fetch(namespace, m.name, m.dimensions, e.stat, metric.MinuteStatPeriod)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(values), 3, "Not enough metrics retrieved for namespace {%s} metric Name {%s} stat {%s}", namespace, m.name, e.stat)

				// omit first/last metric as the 1m collection intervals may be missing data points from when the agent was started/stopped
				middleValues := values[1 : len(values)-1]
				err = metric.IsAllValuesGreaterThanOrEqualToExpectedValueWithError(m.name, middleValues, e.value)
				require.NoError(t, err)
			}
		})
	}
}

func startAgent(t *testing.T) {
	common.CopyFile(filepath.Join("agent_configs", "otlp_emf_config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(10 * time.Second)
}

func runOTLPPusher(instanceID string) error {
	cmd := exec.Command("/bin/bash", "resources/otlp_emf_pusher.sh")
	cmd.Env = append(os.Environ(), fmt.Sprintf("INSTANCE_ID=%s", instanceID))
	return cmd.Start()
}
