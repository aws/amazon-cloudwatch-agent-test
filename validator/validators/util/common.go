// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package util

import (
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/xray"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func LogCloudWatchDimension(dims []types.Dimension) string {
	var dimension string
	for _, d := range dims {
		if d.Name != nil && d.Value != nil {
			dimension += fmt.Sprintf(" dimension(name=%q, val=%q) ", *d.Name, *d.Value)
		}
	}
	return dimension
}

func StartTraceGeneration(receiver string, agentConfigPath string,agentRuntime time.Duration ,traceSendingInterval time.Duration) error{
	cfg := common.TraceTestConfig{
		Generator: nil,
		Name: "",
		AgentConfigPath:  agentConfigPath,
		AgentRuntime: agentRuntime,
	}
	xrayGenCfg := common.TraceGeneratorConfig{
		Interval: traceSendingInterval,
		Annotations:map[string]interface{}{
					"test_type":   "simple_otlp",
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
	switch receiver{
	case "xray":
		cfg.Generator = xray.NewLoadGenerator(&xrayGenCfg)
		cfg.Name = "xray-performance-test"
	case "otlp":
	default:
		panic("Invalid trace receiver")	
	}
	err := common.GenerateTraces(cfg)
	return err
}
