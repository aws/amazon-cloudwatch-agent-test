package otlp

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

const (
	agentRuntime          = 5 * time.Minute
	loadGeneratorInterval = 5 * time.Second
	testSegmentCount      = 20
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestTraces(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	testCases := map[string]struct {
		agentConfigPath string
		generatorConfig *common.TraceConfig
	}{
		"WithOTLP/Simple": {
			agentConfigPath: filepath.Join("testdata", "otlp-config.json"),
			generatorConfig: &common.TraceConfig{
				Interval: loadGeneratorInterval,
				Annotations: map[string]interface{}{
					"test_type":   "simple_otlp",
					"instance_id": env.InstanceId,
					"commit_sha":  env.CwaCommitSha,
				},
				Metadata: map[string]map[string]interface{}{
					"default": {
						"custom_key": "custom_value",
					},
				},
				Attributes: []attribute.KeyValue{
					attribute.String("custom_key", "custom_value"),
					attribute.String("test_type", "simple_otlp"),
					attribute.String("instance_id", env.InstanceId),
					attribute.String("commit_sha", env.CwaCommitSha),
				},
			},
		},
	}
	t.Logf("Sanity check: number of test cases:%d", len(testCases))
	for name, testCase := range testCases {

		t.Run(name, func(t *testing.T) {

			OtlpTest := newLoadGenerator(testCase.generatorConfig)
			OtlpTest.agentConfigPath = testCase.agentConfigPath
			OtlpTest.name = name
			OtlpTest.agentRuntime = agentRuntime
			t.Logf("config interval %v", OtlpTest.cfg.Interval)
			err := common.TraceTest(t, OtlpTest)
			require.NoError(t, err, "TraceTest failed because %s", err)

		})
	}
}
