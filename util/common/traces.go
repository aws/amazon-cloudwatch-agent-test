package common

import (
	"context"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/xray/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

const testSegmentCount = 20

type TraceConfig struct {
	Interval    time.Duration
	Annotations map[string]interface{}
	Metadata    map[string]map[string]interface{}
	Attributes  []attribute.KeyValue
}
type TracesTestInterface interface {
	StartSendingTraces(ctx context.Context) error
	StopSendingTraces()
	Generate(ctx context.Context) error
	validateSegments(t *testing.T, segments []types.Segment, config *TraceConfig)
	GetTestCount() (int, int)
	GetAgentConfigPath() string
	GetGeneratorConfig() *TraceConfig
	GetContext() context.Context
	GetAgentRuntime() time.Duration
	GetName() string
}

func TraceTest(t *testing.T, traceTest TracesTestInterface) error {
	startTime := time.Now()
	t.Logf("AgentConfigpath %s", traceTest.GetAgentConfigPath())
	CopyFile(traceTest.GetAgentConfigPath(), ConfigOutputPath)
	require.NoError(t, StartAgent(ConfigOutputPath, true, false))
	t.Logf("Successfully copied file.")
	t.Log("Starting to send traces")
	go func() {
		require.NoError(t, traceTest.StartSendingTraces(context.Background()), "load generator exited with error")
	}()
	time.Sleep(traceTest.GetAgentRuntime())
	traceTest.StopSendingTraces()
	t.Logf("Stopped Traces")
	StopAgent()
	t.Logf("Stopped Agent")
	testsGenerated, testsEnded := traceTest.GetTestCount()
	t.Logf("For %s , Test Cases Generated %d | Test Cases Ended: %d", traceTest.GetName(), testsGenerated, testsEnded)
	endTime := time.Now()
	t.Logf("Agent has been running for %s", endTime.Sub(startTime))
	time.Sleep(5 * time.Second)

	traceIDs, err := awsservice.GetTraceIDs(startTime, endTime, awsservice.FilterExpression(
		traceTest.GetGeneratorConfig().Annotations))
	require.NoError(t, err, "unable to get trace IDs")
	segments, err := awsservice.GetSegments(traceIDs)
	require.NoError(t, err, "unable to get segments")

	assert.True(t, len(segments) >= testSegmentCount,
		"FAILED: Not enough segments, expected %d but got %d",
		testSegmentCount, len(segments))
	// traceTest.validateSegments(t, segments, traceTest.GetGeneratorConfig())
	return nil
}
