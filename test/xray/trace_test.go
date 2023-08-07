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
	loadGeneratorInterval = 6 * time.Second
)

// WARNING: If you increase number of segments generated
// You might see that the xray is dropping segments 
// To overcome this please update the sampling-rule such that the "rate" is higher.
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
