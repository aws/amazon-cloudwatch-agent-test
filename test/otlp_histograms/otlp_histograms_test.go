// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package otlp_histograms

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/otlp_metrics.json
var metricsJSON string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestOTLPMetrics(t *testing.T) {
	startAgent(t)
	instanceID := "ThisIsATest"
	err := runOTLPPusher(instanceID)
	assert.NoError(t, err)

	time.Sleep(2 * time.Minute)

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
					value: 2,
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
					value: 2,
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
					value: 2,
					check: func(t *testing.T, expected, actual float64) {
						assert.Equal(t, expected, actual)
					},
				},
				{
					stat:  types.StatisticSum,
					value: 2,
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
					value: 2,
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
				require.GreaterOrEqual(t, len(values), 3)

				// For cumulative metrics, we want to check that the values are monotonically increasing
				if strings.Contains(m.name, "cumulative") {
					for i := 1; i < len(values); i++ {
						if e.stat == types.StatisticSum || e.stat == types.StatisticSampleCount {
							assert.GreaterOrEqual(t, values[i], values[i-1],
								"Cumulative metric %s with stat %s should be monotonically increasing", m.name, e.stat)
						}
					}
				}

				for _, v := range values[1 : len(values)-1] {
					e.check(t, e.value, v)
				}
			}
		})
	}
}

func startAgent(t *testing.T) {
	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(10 * time.Second)
}

func runOTLPPusher(instanceID string) error {
	os.Setenv("INSTANCE_ID", instanceID)
	return common.RunAsyncCommand("resources/otlp_pusher.sh")
}
