//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package standard

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otellogs"
)

// Log group names follow the convention set in the helm chart:
// /aws/otel/containerinsights/{clusterName}/{pipeline}
func appLogGroup() string {
	return fmt.Sprintf("/aws/otel/containerinsights/%s/application", cfg.ClusterName)
}

// Pipeline identifiers matching scope.attributes.cloudwatch.pipeline values.
const (
	pipelineAppLogs = "application-logs"
)

var (
	logsClient    *otellogs.OtelLogsClient
	logQueryCache *otellogs.LogQueryCache
	logsLookback  = 10 * time.Minute
)

// initLogsClient is called from TestMain (setup_test.go) after cfg is populated.
func initLogsClient(ctx context.Context) error {
	var err error
	logsCfg := otellogs.LogsConfig{
		Region:         cfg.Region,
		ClusterName:    cfg.ClusterName,
		AccountID:      cfg.AccountID,
		LookbackWindow: logsLookback,
	}

	logsClient, err = otellogs.NewClient(ctx, logsCfg)
	if err != nil {
		return fmt.Errorf("creating logs client: %w", err)
	}

	logQueryCache = otellogs.NewLogQueryCache(logsClient, cfg.ClusterName, logsLookback)
	return nil
}
