package common

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/xray/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

const (
	AGENT_SHUTDOWN_DELAY = 20 * time.Second // this const is the delay between stopping trace generation and stopping agent
)
type TraceTestConfig struct {
	Generator       TraceGeneratorInterface
	Name            string
	AgentConfigPath string
	AgentRuntime    time.Duration
}
type TraceGeneratorConfig struct {
	Interval    time.Duration
	Annotations map[string]interface{}
	Metadata    map[string]map[string]interface{}
	Attributes  []attribute.KeyValue
}
type TraceGenerator struct {
	Cfg                     *TraceGeneratorConfig
	SegmentsGenerationCount int
	SegmentsEndedCount      int
	AgentConfigPath         string
	AgentRuntime            time.Duration
	Name                    string
	Done                    chan struct{}
}
type TraceGeneratorInterface interface {
	StartSendingTraces(ctx context.Context) error
	StopSendingTraces()
	Generate(ctx context.Context) error
	GetSegmentCount() (int, int)
	GetAgentConfigPath() string
	GetGeneratorConfig() *TraceGeneratorConfig
	GetContext() context.Context
	GetAgentRuntime() time.Duration
	GetName() string
}
func TraceTest(t *testing.T, traceTest TraceTestConfig) error {
	t.Helper()
	startTime := time.Now()
	CopyFile(traceTest.AgentConfigPath, ConfigOutputPath)
	require.NoError(t, StartAgent(ConfigOutputPath, true, false), "Couldn't Start the agent")
	go func() {
		require.NoError(t, traceTest.Generator.StartSendingTraces(context.Background()), "load generator exited with error")
	}()
	time.Sleep(traceTest.AgentRuntime)
	traceTest.Generator.StopSendingTraces()
	time.Sleep(AGENT_SHUTDOWN_DELAY)
	StopAgent()
	testsGenerated, testsEnded := traceTest.Generator.GetSegmentCount()
	t.Logf("For %s , Test Cases Generated %d | Test Cases Ended: %d", traceTest.Name, testsGenerated, testsEnded)
	endTime := time.Now()
	t.Logf("Agent has been running for %s", endTime.Sub(startTime))
	time.Sleep(10 * time.Second)

	traceIDs, err := awsservice.GetTraceIDs(startTime, endTime, awsservice.FilterExpression(
		traceTest.Generator.GetGeneratorConfig().Annotations))
	require.NoError(t, err, "unable to get trace IDs")
	segments, err := awsservice.GetSegments(traceIDs)
	require.NoError(t, err, "unable to get segments")

	assert.True(t, len(segments) >= testsGenerated,
		"FAILED: Not enough segments, expected %d but got %d , traceIDCount: %d",
		testsGenerated, len(segments), len(traceIDs))
	require.NoError(t, SegmentValidationTest(t, traceTest, segments), "Segment Validation Failed")
	return nil
}

func SegmentValidationTest(t *testing.T, traceTest TraceTestConfig, segments []types.Segment) error {
	t.Helper()
	cfg := traceTest.Generator.GetGeneratorConfig()
	for _, segment := range segments {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(*segment.Document), &result))
		if _, ok := result["parent_id"]; ok {
			// skip subsegments
			continue
		}
		annotations, ok := result["annotations"]
		assert.True(t, ok, "missing annotations")
		assert.True(t, reflect.DeepEqual(annotations, cfg.Annotations), "mismatching annotations")
		metadataByNamespace, ok := result["metadata"].(map[string]interface{})
		assert.True(t, ok, "missing metadata")
		for namespace, wantMetadata := range cfg.Metadata {
			var gotMetadata map[string]interface{}
			gotMetadata, ok = metadataByNamespace[namespace].(map[string]interface{})
			assert.Truef(t, ok, "missing metadata in namespace: %s", namespace)
			for key, wantValue := range wantMetadata {
				var gotValue interface{}
				gotValue, ok = gotMetadata[key]
				assert.Truef(t, ok, "missing expected metadata key: %s", key)
				assert.Truef(t, reflect.DeepEqual(gotValue, wantValue), "mismatching values for key (%s):\ngot\n\t%v\nwant\n\t%v", key, gotValue, wantValue)
			}
		}
	}
	return nil

}
func GenerateTraces(traceTest TraceTestConfig) error{
		CopyFile(traceTest.AgentConfigPath, ConfigOutputPath)
		go func() {
			traceTest.Generator.StartSendingTraces(context.Background())
		}()
		time.Sleep(traceTest.AgentRuntime)
		traceTest.Generator.StopSendingTraces()
		time.Sleep(AGENT_SHUTDOWN_DELAY)
		return nil
}
