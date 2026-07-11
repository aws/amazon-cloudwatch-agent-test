// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otellogs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// LogsConfig holds configuration for the OTEL logs integration test client.
type LogsConfig struct {
	Region      string
	ClusterName string
	AccountID   string
	// LookbackWindow is how far back to query logs. Defaults to 10 minutes.
	LookbackWindow time.Duration
}

// OtelLogsClient queries CloudWatch Logs Insights for OTLP-ingested logs.
type OtelLogsClient struct {
	cwl            *cloudwatchlogs.Client
	region         string
	pollInterval   time.Duration
	maxPollRetries int
}

// NewClient creates an OtelLogsClient.
func NewClient(ctx context.Context, cfg LogsConfig) (*OtelLogsClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return &OtelLogsClient{
		cwl:            cloudwatchlogs.NewFromConfig(awsCfg),
		region:         cfg.Region,
		pollInterval:   2 * time.Second,
		maxPollRetries: 30,
	}, nil
}

// Query executes a Logs Insights query against the given log group and returns
// parsed LogResult entries. The query should select fields needed for validation.
// timeRange specifies how far back to look (from now).
func (c *OtelLogsClient) Query(ctx context.Context, logGroup string, query string, timeRange time.Duration) ([]LogResult, error) {
	end := time.Now()
	start := end.Add(-timeRange)

	slog.Debug("starting logs query", "logGroup", logGroup, "query", query)

	output, err := c.cwl.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupNames: []string{logGroup},
		StartTime:     aws.Int64(start.Unix()),
		EndTime:       aws.Int64(end.Unix()),
		QueryString:   aws.String(query),
		Limit:         aws.Int32(1000),
	})
	if err != nil {
		return nil, fmt.Errorf("StartQuery: %w", err)
	}

	results, err := c.pollResults(ctx, output.QueryId)
	if err != nil {
		return nil, err
	}

	slog.Debug("logs query returned", "count", len(results))
	return results, nil
}

// QueryRaw executes a Logs Insights query and returns the raw result fields.
func (c *OtelLogsClient) QueryRaw(ctx context.Context, logGroup string, query string, timeRange time.Duration) ([][]types.ResultField, error) {
	end := time.Now()
	start := end.Add(-timeRange)

	output, err := c.cwl.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupNames: []string{logGroup},
		StartTime:     aws.Int64(start.Unix()),
		EndTime:       aws.Int64(end.Unix()),
		QueryString:   aws.String(query),
		Limit:         aws.Int32(1000),
	})
	if err != nil {
		return nil, fmt.Errorf("StartQuery: %w", err)
	}

	for attempt := 0; attempt < c.maxPollRetries; attempt++ {
		resp, err := c.cwl.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{
			QueryId: output.QueryId,
		})
		if err != nil {
			return nil, fmt.Errorf("GetQueryResults: %w", err)
		}
		switch resp.Status {
		case types.QueryStatusComplete:
			return resp.Results, nil
		case types.QueryStatusFailed, types.QueryStatusCancelled, types.QueryStatusTimeout:
			return nil, fmt.Errorf("query ended with status: %s", resp.Status)
		}
		time.Sleep(c.pollInterval)
	}
	return nil, fmt.Errorf("query did not complete after %d attempts", c.maxPollRetries)
}

func (c *OtelLogsClient) pollResults(ctx context.Context, queryID *string) ([]LogResult, error) {
	for attempt := 0; attempt < c.maxPollRetries; attempt++ {
		resp, err := c.cwl.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{
			QueryId: queryID,
		})
		if err != nil {
			return nil, fmt.Errorf("GetQueryResults: %w", err)
		}

		switch resp.Status {
		case types.QueryStatusComplete:
			return parseQueryResults(resp.Results), nil
		case types.QueryStatusFailed, types.QueryStatusCancelled, types.QueryStatusTimeout:
			return nil, fmt.Errorf("query ended with status: %s", resp.Status)
		}
		time.Sleep(c.pollInterval)
	}
	return nil, fmt.Errorf("query did not complete after %d attempts", c.maxPollRetries)
}

func parseQueryResults(rows [][]types.ResultField) []LogResult {
	results := make([]LogResult, 0, len(rows))
	for _, row := range rows {
		lr := LogResult{
			Resource:   make(map[string]string),
			Scope:      make(map[string]string),
			Attributes: make(map[string]string),
		}
		for _, field := range row {
			if field.Field == nil || field.Value == nil {
				continue
			}
			lr.SetField(*field.Field, *field.Value)
		}
		results = append(results, lr)
	}
	return results
}
