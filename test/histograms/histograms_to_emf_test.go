// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package amp

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
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func TestOTLPMetrics(t *testing.T) {
	startAgent(t)
	instanceID := "ThisIsATest6"
	err := runOTLPPusher(instanceID)
	assert.NoError(t, err)

	time.Sleep(3 * time.Minute)

	metrics := []struct {
		name       string
		dimensions []types.Dimension
		expected   []struct {
			stat  types.Statistic
			value float64
			check func(t *testing.T, expected, actual float64)
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
				check func(t *testing.T, expected, actual float64)
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticMaximum,
					value: 2,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticSum,
					value: 10,
					check: func(t *testing.T, expected, actual float64) {
						assert.GreaterOrEqual(t, actual, expected)
					},
				},
				{
					stat:  types.StatisticAverage,
					value: 1,
					check: func(t *testing.T, expected, actual float64) {
						assert.InDelta(t, expected, actual, 0.01)
					},
				},
				{
					stat:  types.StatisticSampleCount,
					value: 10,
					check: func(t *testing.T, expected, actual float64) {
						assert.GreaterOrEqual(t, actual, expected)
					},
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
					Name:  aws.String("service.name"),
					Value: aws.String("my.service"),
				},
			},
			expected: []struct {
				stat  types.Statistic
				value float64
				check func(t *testing.T, expected, actual float64)
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticMaximum,
					value: 0,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticSum,
					value: 10,
					check: func(t *testing.T, expected, actual float64) {
						assert.GreaterOrEqual(t, actual, expected)
					},
				},
				{
					stat:  types.StatisticAverage,
					value: 1,
					check: func(t *testing.T, expected, actual float64) {
						assert.InDelta(t, expected, actual, 0.01)
					},
				},
				{
					stat:  types.StatisticSampleCount,
					value: 10,
					check: func(t *testing.T, expected, actual float64) {
						assert.GreaterOrEqual(t, actual, expected)
					},
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
				check func(t *testing.T, expected, actual float64)
			}{
				{
					stat:  types.StatisticMinimum,
					value: 0,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticMaximum,
					value: 5,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticSum,
					value: 10,
					check: func(t *testing.T, expected, actual float64) {
						assert.GreaterOrEqual(t, actual, expected)
					},
				},
				{
					stat:  types.StatisticAverage,
					value: 3.33,
					check: func(t *testing.T, expected, actual float64) {
						assert.InDelta(t, expected, actual, 0.01)
					},
				},
				{
					stat:  types.StatisticSampleCount,
					value: 3,
					check: func(t *testing.T, expected, actual float64) {
						assert.GreaterOrEqual(t, actual, expected)
					},
				},
			},
		},
	}

	fetcher := metric.MetricValueFetcher{}
	namespace := "CWAgent"

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			for _, e := range m.expected {
				values, err := fetcher.Fetch(namespace, m.name, m.dimensions, metric.Statistics(e.stat), metric.MinuteStatPeriod)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(values), 2)

				// Skip first and last values
				middleValues := values[1 : len(values)-1]

				// Check if all values are >= expected value
				for _, v := range middleValues {
					assert.GreaterOrEqual(t, v, e.value,
						"Metric %s with stat %s should have values >= %f, got %f",
						m.name, e.stat, e.value, v)
				}
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
