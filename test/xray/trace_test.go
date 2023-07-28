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
	agentRuntime          = 1 * time.Minute
	loadGeneratorInterval = 20 * time.Second
)

// WARNING: This test can only run 20 traces total
// This means that if you are running on 14 linux version.
// Each version can run only 3 traces each.(agentRuntime:1 , interval: 20)
// This is a known bug, for more info contact: okankoAMZ
func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestTraces(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	testCases := map[string]struct {
		agentConfigPath string
		generatorConfig *common.TraceGeneratorConfig
	}{
		"WithXray/Simple": {
			agentConfigPath: filepath.Join("resources", "xray-config.json"),
			generatorConfig: &common.TraceGeneratorConfig{
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
	for name, testCase := range testCases {

		t.Run(name, func(t *testing.T) {

			XrayTest := newLoadGenerator(testCase.generatorConfig)
			XrayTest.AgentConfigPath = testCase.agentConfigPath
			XrayTest.Name = name
			XrayTest.AgentRuntime = agentRuntime
			t.Logf("config interval %v", XrayTest.Cfg.Interval)
			err := common.TraceTest(t, XrayTest)
			require.NoError(t, err, "TraceTest failed because %s", err)

		})
	}
}
