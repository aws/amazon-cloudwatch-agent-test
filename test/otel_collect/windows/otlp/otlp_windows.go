// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package otlp

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

//go:embed resources/config.json
var testConfigJSON string

const (
	tmpConfigPath = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\config.json"
	otlpRuntime   = 3 * time.Minute
	sendInterval  = 10 * time.Second
)

// Validate runs the Windows OTLP test: start the agent with the V2 opentelemetry OTLP config,
// push OTLP metrics to the agent, then validate they reach CloudWatch via the PromQL endpoint.
func Validate() error {
	instanceID := awsservice.GetInstanceId()

	if err := os.WriteFile(tmpConfigPath, []byte(testConfigJSON), 0644); err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}
	if err := common.CopyFile(tmpConfigPath, common.ConfigOutputPath); err != nil {
		return fmt.Errorf("could not copy config: %w", err)
	}
	if err := common.StartAgent(common.ConfigOutputPath, true, false); err != nil {
		return fmt.Errorf("could not start agent: %w", err)
	}
	time.Sleep(5 * time.Second)

	// Push OTLP metrics to the agent's OTLP/HTTP receiver for the collection period.
	if err := common.SendOTLPMetrics(common.DefaultOTLPHTTPEndpoint, instanceID, sendInterval, otlpRuntime); err != nil {
		return fmt.Errorf("failed to send OTLP metrics: %w", err)
	}

	_ = common.StopAgent()

	// Validate the pushed metrics landed in CloudWatch, isolated by the instance's host.id.
	return otelmetrics.AssertMetricsPresent(
		context.Background(),
		"", // region resolved from AWS_REGION env
		[]string{"otlp_test_counter", "otlp_test_gauge"},
		map[string]string{"@resource.host.id": instanceID},
		3,
		30*time.Second,
	)
}
