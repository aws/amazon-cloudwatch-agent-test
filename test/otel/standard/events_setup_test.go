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

// Events log group follows the convention:
// /aws/containerinsights/{clusterName}/events
func eventsLogGroup() string {
	return fmt.Sprintf("/aws/containerinsights/%s/events", cfg.ClusterName)
}

const pipelineEvents = "events"

var (
	eventsLogsClient     *otellogs.OtelLogsClient
	eventsLogQueryCache  *otellogs.LogQueryCache
	eventsLogsLookback   = 10 * time.Minute
)

// initEventsLogsClient is called from TestMain (setup_test.go) after cfg is populated.
func initEventsLogsClient(ctx context.Context) error {
	var err error
	logsCfg := otellogs.LogsConfig{
		Region:         cfg.Region,
		ClusterName:    cfg.ClusterName,
		AccountID:      cfg.AccountID,
		LookbackWindow: eventsLogsLookback,
	}
	eventsLogsClient, err = otellogs.NewClient(ctx, logsCfg)
	if err != nil {
		return fmt.Errorf("creating events logs client: %w", err)
	}
	eventsLogQueryCache = otellogs.NewLogQueryCache(eventsLogsClient, cfg.ClusterName, eventsLogsLookback)
	return nil
}
