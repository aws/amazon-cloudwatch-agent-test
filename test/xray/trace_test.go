// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package xray

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/xray"
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
		generatorConfig *base.TraceGeneratorConfig
	}{
		"WithXray/Simple": {
			agentConfigPath: filepath.Join("resources", "xray-config.json"),
			generatorConfig: &base.TraceGeneratorConfig{
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
			XrayTestCfg := base.TraceTestConfig{
				Generator:       xray.NewLoadGenerator(testCase.generatorConfig),
				Name:            name,
				AgentConfigPath: testCase.agentConfigPath,
				AgentRuntime:    agentRuntime,
			}
			err := base.TraceTest(t, XrayTestCfg)
			require.NoError(t, err, "TraceTest failed because %s", err)

		})
	}
}
