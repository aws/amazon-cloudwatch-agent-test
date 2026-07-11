// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otellogs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// LogQueryCache provides session-scoped caching of Logs Insights queries.
// Each unique (logGroup, pipeline) pair is queried once; subsequent calls
// return cached results.
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
	query := fmt.Sprintf(
		"fields @message"+
			" | filter resource.attributes.k8s.cluster.name = %q"+
			" | filter scope.attributes.cloudwatch.pipeline = %q"+
			" | limit 200",
		c.cluster, pipeline,
	)

	rows, err := c.client.QueryRaw(ctx, logGroup, query, c.lookback)
	if err != nil {
		return cacheEntry{err: err}
	}
	results := parseMessageRows(rows)
	return cacheEntry{results: results}
}

// parseMessageRows extracts @message from each Logs Insights row and parses
// the OTLP JSON into LogResult structs.
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

// otlpLogJSON mirrors the JSON structure of an OTLP log record as stored in
// CloudWatch Logs when ingested via the OTLP endpoint.
type otlpLogJSON struct {
	Resource struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"resource"`
	Scope struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"scope"`
	Body               string            `json:"body"`
	Attributes         map[string]string `json:"attributes"`
	SeverityText       string            `json:"severityText"`
	SeverityNumber     json.Number       `json:"severityNumber"`
	TimeUnixNano       json.Number       `json:"timeUnixNano"`
	ObservedTimeUnixNano json.Number     `json:"observedTimeUnixNano"`
	TraceID            string            `json:"traceId"`
	SpanID             string            `json:"spanId"`
}

// ParseOTLPLogJSON parses a raw OTLP log JSON string (as stored in CW Logs)
// into a LogResult.
func ParseOTLPLogJSON(raw string) (LogResult, error) {
	var j otlpLogJSON
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return LogResult{}, fmt.Errorf("parsing OTLP log JSON: %w", err)
	}

	lr := LogResult{
		Resource:             j.Resource.Attributes,
		Scope:                j.Scope.Attributes,
		Attributes:           j.Attributes,
		Body:                 j.Body,
		SeverityText:         j.SeverityText,
		SeverityNumber:       j.SeverityNumber.String(),
		TimeUnixNano:         j.TimeUnixNano.String(),
		ObservedTimeUnixNano: j.ObservedTimeUnixNano.String(),
		TraceID:              j.TraceID,
		SpanID:               j.SpanID,
	}
	if lr.Resource == nil {
		lr.Resource = make(map[string]string)
	}
	if lr.Scope == nil {
		lr.Scope = make(map[string]string)
	}
	if lr.Attributes == nil {
		lr.Attributes = make(map[string]string)
	}
	return lr, nil
}
