// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package prometheus

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

//go:embed resources/prometheus_scrape.yaml
var scrapeConfig string

//go:embed resources/prometheus_metrics
var prometheusMetrics string

const (
	tmpConfigPath = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\config.json"
	scrapePath    = "C:\\prometheus_scrape.yaml"
	exporterPort  = 8101
	runtime       = 3 * time.Minute
)

func Validate() error {
	env := environment.GetEnvironmentMetaData()

	stop, err := common.StartPrometheusFakeServer(exporterPort, prometheusMetrics)
	if err != nil {
		return fmt.Errorf("could not start fake prometheus exporter: %w", err)
	}
	defer stop()

	if err := os.WriteFile(scrapePath, []byte(scrapeConfig), 0644); err != nil {
		return fmt.Errorf("could not write scrape config: %w", err)
	}
	if err := os.WriteFile(tmpConfigPath, []byte(testConfigJSON), 0644); err != nil {
		return fmt.Errorf("could not write agent config: %w", err)
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
		[]string{"node_cpu_seconds_total", "node_memory_MemAvailable_bytes"},
		otlpvalidation.ResourceHostIDLabels(env.AgentStartCommand, env.InstanceId),
		3,
		30*time.Second,
	)
}
