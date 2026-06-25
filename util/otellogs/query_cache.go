// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otellogs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// LogQueryCache provides session-scoped caching of Logs Insights queries.
type LogQueryCache struct {
	mu       sync.RWMutex
	cache    map[string]cacheEntry
	inflight map[string]chan struct{}
	client   *OtelLogsClient
	cluster  string
	lookback time.Duration
}

type cacheEntry struct {
	results []LogResult
	err     error
}

// NewLogQueryCache creates a cache backed by the given client.
func NewLogQueryCache(client *OtelLogsClient, clusterName string, lookback time.Duration) *LogQueryCache {
	if lookback == 0 {
		lookback = 10 * time.Minute
	}
	return &LogQueryCache{
		cache:    make(map[string]cacheEntry),
		inflight: make(map[string]chan struct{}),
		client:   client,
		cluster:  clusterName,
		lookback: lookback,
	}
}

// Get returns cached log results for a given log group filtered by pipeline.
func (c *LogQueryCache) Get(ctx context.Context, logGroup, pipeline string) ([]LogResult, error) {
	key := logGroup + "|" + pipeline

	c.mu.RLock()
	if entry, ok := c.cache[key]; ok {
		c.mu.RUnlock()
		return entry.results, entry.err
	}
	if ch, ok := c.inflight[key]; ok {
		c.mu.RUnlock()
		<-ch
		c.mu.RLock()
		entry := c.cache[key]
		c.mu.RUnlock()
		return entry.results, entry.err
	}
	c.mu.RUnlock()

	c.mu.Lock()
	if entry, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return entry.results, entry.err
	}
	if ch, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-ch
		c.mu.RLock()
		entry := c.cache[key]
		c.mu.RUnlock()
		return entry.results, entry.err
	}
	ch := make(chan struct{})
	c.inflight[key] = ch
	c.mu.Unlock()

	entry := c.fetch(ctx, logGroup, pipeline)

	c.mu.Lock()
	c.cache[key] = entry
	delete(c.inflight, key)
	c.mu.Unlock()
	close(ch)

	return entry.results, entry.err
}

func (c *LogQueryCache) fetch(ctx context.Context, logGroup, pipeline string) cacheEntry {
	// Use @message substring matching because OTLP log JSON has dots in key
	// names (e.g. "k8s.cluster.name") which Insights cannot filter structurally.
	query := fmt.Sprintf(
		`fields @message`+
			` | filter @message like %q`+
			` | filter @message like %q`+
			` | limit 200`,
		c.cluster, pipeline,
	)
	rows, err := c.client.QueryRaw(ctx, logGroup, query, c.lookback)
	if err != nil {
		return cacheEntry{err: err}
	}
	results := parseMessageRows(rows)
	return cacheEntry{results: results}
}

func parseMessageRows(rows [][]types.ResultField) []LogResult {
	var results []LogResult
	for _, row := range rows {
		msg := extractField(row, "@message")
		if msg == "" {
			continue
		}
		lr, err := ParseOTLPLogJSON(msg)
		if err != nil {
			continue
		}
		results = append(results, lr)
	}
	return results
}

func extractField(row []types.ResultField, name string) string {
	for _, f := range row {
		if f.Field != nil && *f.Field == name {
			if f.Value != nil {
				return *f.Value
			}
		}
	}
	return ""
}
