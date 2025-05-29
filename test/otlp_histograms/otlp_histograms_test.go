// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
package otlp_histograms

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

//go:embed resources/otlp_pusher.sh
var otlpPusherScript string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestOTLPMetrics(t *testing.T) {
	startAgent(t)
	instanceID := awsservice.GetInstanceId()
	err := startOtlpPusher()
	require.NoError(t, err, "Failed to start OTLP pusher")

	metrics := []struct {
		name       string
		dimensions []types.Dimension
		expected   float64
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
			expected: 2,
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
			expected: 0,
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
			expected: 5,
		},
	}

	fetcher := metric.MetricValueFetcher{}
	namespace := "CWAgent"
	stats := []metric.Statistics{
		metric.MAXUMUM,
		metric.MINIMUM,
		metric.SUM,
		metric.AVERAGE,
	}

	time.Sleep(3 * time.Minute)

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			for _, stat := range stats {
				values, err := fetcher.Fetch(namespace, m.name, m.dimensions, stat, metric.HighResolutionStatPeriod)
				if err != nil {
					t.Logf("Failed to fetch metrics for %s with stat %s: %v", m.name, stat, err)
					continue
				}

				t.Logf("Metrics retrieved from CloudWatch for %s (stat: %s): %v", m.name, stat, values)

				if len(values) == 0 {
					t.Errorf("No values returned for %s with stat %s", m.name, stat)
					continue
				}

				switch m.name {
				case "my.cumulative.histogram":
					assert.Greater(t, values[0], float64(0),
						fmt.Sprintf("Expected increasing value > 0, got %v for metric %s with stat %s",
							values[0], m.name, stat))
				case "my.delta.histogram":
					assert.Equal(t, m.expected, values[0],
						fmt.Sprintf("Expected %v, got %v for metric %s with stat %s",
							m.expected, values[0], m.name, stat))
				default:
					assert.GreaterOrEqual(t, values[0], float64(0),
						fmt.Sprintf("Expected value >= 0, got %v for metric %s with stat %s",
							values[0], m.name, stat))
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

func startOtlpPusher() error {
	return common.RunAsyncCommand("sudo resources/otlp_pusher.sh")
}
