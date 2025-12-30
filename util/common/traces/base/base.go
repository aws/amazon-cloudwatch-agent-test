// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/xray/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
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
	common.CopyFile(traceTest.AgentConfigPath, common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false), "Couldn't Start the agent")
	go func() {
		require.NoError(t, traceTest.Generator.StartSendingTraces(context.Background()), "load generator exited with error")
	}()
	time.Sleep(traceTest.AgentRuntime)
	traceTest.Generator.StopSendingTraces()
	time.Sleep(AGENT_SHUTDOWN_DELAY)
	common.StopAgent()
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
	return validateSegments(segments, cfg.Annotations, cfg.Metadata)
}

// ValidateTraceSegments validates traces by fetching and validating segments
func ValidateTraceSegments(startTime, endTime time.Time, annotations map[string]interface{}, metadata map[string]map[string]interface{}) error {
	traceIDs, err := awsservice.GetTraceIDs(startTime, endTime, awsservice.FilterExpression(annotations))
	if err != nil {
		return fmt.Errorf("unable to get trace IDs: %w", err)
	}
	if len(traceIDs) == 0 {
		return fmt.Errorf("no traces found")
	}

	segments, err := awsservice.GetSegments(traceIDs)
	if err != nil {
		return fmt.Errorf("unable to get segments: %w", err)
	}

	return validateSegments(segments, annotations, metadata)
}

// validateSegments contains the core validation logic
func validateSegments(segments []types.Segment, expectedAnnotations map[string]interface{}, expectedMetadata map[string]map[string]interface{}) error {
	for _, segment := range segments {
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(*segment.Document), &result); err != nil {
			return err
		}
		if _, ok := result["parent_id"]; ok {
			continue // skip subsegments
		}

		annotations, ok := result["annotations"]
		if !ok || !reflect.DeepEqual(annotations, expectedAnnotations) {
			return fmt.Errorf("annotation validation failed")
		}

		if expectedMetadata != nil {
			metadataByNamespace, ok := result["metadata"].(map[string]interface{})
			if !ok {
				return fmt.Errorf("missing metadata")
			}

			for namespace, wantMetadata := range expectedMetadata {
				gotMetadata, ok := metadataByNamespace[namespace].(map[string]interface{})
				if !ok {
					return fmt.Errorf("missing metadata in namespace: %s", namespace)
				}

				for key, wantValue := range wantMetadata {
					gotValue, ok := gotMetadata[key]
					if !ok || !reflect.DeepEqual(gotValue, wantValue) {
						return fmt.Errorf("metadata validation failed for key: %s", key)
					}
				}
			}
		}
	}
	return nil
}
func GenerateTraces(traceTest TraceTestConfig) error {
	common.CopyFile(traceTest.AgentConfigPath, common.ConfigOutputPath)
	go func() {
		traceTest.Generator.StartSendingTraces(context.Background())
	}()
	time.Sleep(traceTest.AgentRuntime)
	traceTest.Generator.StopSendingTraces()
	time.Sleep(AGENT_SHUTDOWN_DELAY)
	return nil
}
