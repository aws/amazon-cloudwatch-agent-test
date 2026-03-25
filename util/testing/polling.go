// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package testing

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	// LocalPollInterval is the default tick for local checks (process, health endpoint).
	LocalPollInterval = 500 * time.Millisecond
	// AWSPollInterval is the default tick for AWS API calls (CloudWatch, CloudWatch Logs).
	AWSPollInterval = 10 * time.Second
)

// WaitForAgentReady polls the agent health endpoint until it returns 200.
func WaitForAgentReady(t *testing.T, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://localhost:13133/health/status")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, timeout, LocalPollInterval, "agent did not become ready within %s", timeout)
}

// WaitForMetric polls CloudWatch until the named metric exists in the given namespace.
func WaitForMetric(t *testing.T, metricName, namespace string, dims []types.DimensionFilter, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		return awsservice.ValidateMetric(metricName, namespace, dims) == nil
	}, timeout, AWSPollInterval, "metric %s not found in namespace %s within %s", metricName, namespace, timeout)
}

// WaitForLogStream polls CloudWatch Logs until at least one log stream exists in the group.
func WaitForLogStream(t *testing.T, logGroup string, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		streams := awsservice.GetLogStreamNames(logGroup)
		return len(streams) > 0
	}, timeout, AWSPollInterval, "no log streams found in %s within %s", logGroup, timeout)
}

// WaitForMetricValue polls CloudWatch GetMetricStatistics until at least one datapoint is returned.
func WaitForMetricValue(t *testing.T, metricName, namespace string, dims []types.Dimension, startTime, endTime time.Time, periodSec int32, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		data, err := awsservice.GetMetricStatistics(
			metricName, namespace, dims,
			startTime, endTime, periodSec,
			[]types.Statistic{types.StatisticAverage}, nil,
		)
		return err == nil && len(data.Datapoints) > 0
	}, timeout, AWSPollInterval, fmt.Sprintf("no datapoints for metric %s within %s", metricName, timeout))
}
