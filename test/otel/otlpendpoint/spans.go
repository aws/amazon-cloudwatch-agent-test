// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpendpoint

import (
	"context"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// SpansLogGroup is the log group Transaction Search writes spans to.
const SpansLogGroup = "aws/spans"

var (
	spansOnce    sync.Once
	spansEnabled bool
	spansErr     error
)

// TransactionSearchEnabled reports whether the test account has Transaction Search in the test region,
// that is, whether SpansLogGroup exists. Without it (no CloudWatch Logs trace segment destination),
// CloudWatch rejects the agent's OTLP traces with HTTP 400 and no span can be read back. It needs the
// awsservice clients to be configured, and it is checked once per test binary.
func TransactionSearchEnabled() (bool, error) {
	spansOnce.Do(func() {
		out, err := awsservice.CwlClient.DescribeLogGroups(context.Background(), &cloudwatchlogs.DescribeLogGroupsInput{
			LogGroupNamePrefix: aws.String(SpansLogGroup),
		})
		if err != nil {
			spansErr = err
			return
		}
		for _, g := range out.LogGroups {
			if aws.ToString(g.LogGroupName) == SpansLogGroup {
				spansEnabled = true
			}
		}
	})
	return spansEnabled, spansErr
}

// SkipWithoutTransactionSearch skips t when the test account has no Transaction Search in the test
// region, so span checks report what is missing in the account instead of failing on
// ResourceNotFoundException. Enable it with
// `aws xray update-trace-segment-destination --destination CloudWatchLogs` (plus the CloudWatch Logs
// resource policy it needs).
func SkipWithoutTransactionSearch(t *testing.T) {
	t.Helper()
	enabled, err := TransactionSearchEnabled()
	if err != nil {
		t.Fatalf("checking for the %s log group (Transaction Search): %v", SpansLogGroup, err)
	}
	if !enabled {
		t.Skipf("no %s log group: Transaction Search (CloudWatch Logs trace segment destination) is not enabled in this account and region, so CloudWatch rejects OTLP traces", SpansLogGroup)
	}
}
