// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package host_metrics

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

//go:embed resources/config.json
var testConfigJSON string

const (
	tmpConfigPath = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\config.json"
	runtime       = 3 * time.Minute
)

func Validate() error {
	env := environment.GetEnvironmentMetaData()

	if err := os.WriteFile(tmpConfigPath, []byte(testConfigJSON), 0644); err != nil {
		return fmt.Errorf("could not write config: %w", err)
	}
	if err := common.CopyFile(tmpConfigPath, common.ConfigOutputPath); err != nil {
		return fmt.Errorf("could not copy config: %w", err)
	}
	if err := common.StartAgent(common.ConfigOutputPath, true, false); err != nil {
		return fmt.Errorf("could not start agent: %w", err)
	}
	time.Sleep(runtime)
	_ = common.StopAgent()

	return otelmetrics.AssertMetricsPresent(
		context.Background(),
		env.Region,
		[]string{"system.cpu.utilization", "system.memory.utilization", "system.network.io", "system.disk.operations"},
		otlpvalidation.ResourceHostIDLabels(env.AgentStartCommand, env.InstanceId),
		3,
		30*time.Second,
	)
}
