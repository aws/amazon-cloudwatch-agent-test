// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package traces

import (
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/xray"
)

func StartTraceGeneration(receiver string, agentConfigPath string, agentRuntime time.Duration, traceSendingInterval time.Duration) error {
	cfg := base.TraceTestConfig{
		Generator:       nil,
		Name:            "",
		AgentConfigPath: agentConfigPath,
		AgentRuntime:    agentRuntime,
	}
	xrayGenCfg := base.TraceGeneratorConfig{
		Interval: traceSendingInterval,
		Annotations: map[string]interface{}{
			"test_type": "simple_otlp",
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
	}
	switch receiver {
	case "xray":
		cfg.Generator = xray.NewLoadGenerator(&xrayGenCfg)
		cfg.Name = "xray-performance-test"
	case "otlp":
	default:
		return fmt.Errorf("%s is not supported.", receiver)
	}
	err := base.GenerateTraces(cfg)
	return err
}
