package xray

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
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
		"WithXray/Simple": {
			agentConfigPath: filepath.Join("resources", "xray-config.json"),
			generatorConfig: &common.TraceConfig{
				Interval: loadGeneratorInterval,
				Annotations: map[string]interface{}{
					"test_type":   "simple_xray",
					"instance_id": env.InstanceId,
					"commit_sha":  env.CwaCommitSha,
				},
				Metadata: map[string]map[string]interface{}{
					"default": {
						"nested": map[string]interface{}{
							"key": "value",
						},
					},
					"custom_namespace": {
						"custom_key": "custom_value",
					},
				},
			},
		},
	}
	t.Logf("Sanity check: number of test cases:%d", len(testCases))
	for name, testCase := range testCases {

		t.Run(name, func(t *testing.T) {

			XrayTest := newLoadGenerator(testCase.generatorConfig)
			XrayTest.agentConfigPath = testCase.agentConfigPath
			XrayTest.name = name
			XrayTest.agentRuntime = agentRuntime
			t.Logf("config interval %v", XrayTest.cfg.Interval)
			err := common.TraceTest(t, XrayTest)
			require.NoError(t, err, "TraceTest failed because %s", err)

		})
	}
}
