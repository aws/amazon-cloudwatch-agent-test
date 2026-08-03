// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// DefaultMaxConcurrency is the default number of concurrent in-flight queries.
const DefaultMaxConcurrency = 3

// promqlEscaper is the shared escaper for PromQL label values.
var promqlEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

func promqlMetricSelector(metricName string) string {
	if strings.Contains(metricName, ".") {
		return fmt.Sprintf(`{"__name__"="%s",`, metricName)
	}
	return metricName + "{"
}

type cacheEntry struct {
	results []MetricResult
	err     error
}

// QueryCache provides session-scoped caching of PromQL queries.
// Each unique metric name is queried exactly once; subsequent calls return cached data.
// Concurrent requests for the same metric are deduplicated via singleflight.
type QueryCache struct {
	mu         sync.RWMutex
	filtered   map[string]cacheEntry
	unfiltered map[string]cacheEntry
	client     *OtelMetricsClient
	cluster    string
	hostTypes  []string
	registry   *SourceRegistry
	sem        chan struct{}
	inflight   map[string]chan struct{} // dedup concurrent fetches for same key
}

// QueryCacheOption configures optional QueryCache behavior.
type QueryCacheOption func(*QueryCache)

func WithHostTypes(hostTypes []string) QueryCacheOption {
	return func(qc *QueryCache) { qc.hostTypes = hostTypes }
}

func WithSourceRegistry(registry *SourceRegistry) QueryCacheOption {
	return func(qc *QueryCache) { qc.registry = registry }
}

func WithMaxConcurrency(n int) QueryCacheOption {
	return func(qc *QueryCache) { qc.sem = make(chan struct{}, n) }
}

func NewQueryCache(client *OtelMetricsClient, clusterName string, opts ...QueryCacheOption) *QueryCache {
	qc := &QueryCache{
		filtered:   make(map[string]cacheEntry),
		unfiltered: make(map[string]cacheEntry),
		inflight:   make(map[string]chan struct{}),
		client:     client,
		cluster:    clusterName,
	}
	for _, opt := range opts {
		opt(qc)
	}
	if qc.sem == nil {
		qc.sem = make(chan struct{}, DefaultMaxConcurrency)
	}
	return qc
}

// Get returns cached results for a metric filtered by cluster name.
// Concurrent calls for the same metric are deduplicated.
func (qc *QueryCache) Get(ctx context.Context, metricName string) ([]MetricResult, error) {
	// Fast path: cache hit
	qc.mu.RLock()
	if entry, ok := qc.filtered[metricName]; ok {
		qc.mu.RUnlock()
		return entry.results, entry.err
	}
	// Check if another goroutine is already fetching this metric
	if ch, ok := qc.inflight[metricName]; ok {
		qc.mu.RUnlock()
		<-ch // wait for the fetch to complete
		qc.mu.RLock()
		entry := qc.filtered[metricName]
		qc.mu.RUnlock()
		return entry.results, entry.err
	}
	qc.mu.RUnlock()

	// Claim this metric fetch
	qc.mu.Lock()
	// Double-check after acquiring write lock
	if entry, ok := qc.filtered[metricName]; ok {
		qc.mu.Unlock()
		return entry.results, entry.err
	}
	if ch, ok := qc.inflight[metricName]; ok {
		qc.mu.Unlock()
		<-ch
		qc.mu.RLock()
		entry := qc.filtered[metricName]
		qc.mu.RUnlock()
		return entry.results, entry.err
	}
	ch := make(chan struct{})
	qc.inflight[metricName] = ch
	qc.mu.Unlock()

	// Fetch without holding any lock
	entry := qc.fetchFiltered(ctx, metricName)

	// Store and signal waiters
	qc.mu.Lock()
	qc.filtered[metricName] = entry
	delete(qc.inflight, metricName)
	qc.mu.Unlock()
	close(ch)

	return entry.results, entry.err
}

// fetchFiltered performs the actual HTTP query for a filtered metric.
func (qc *QueryCache) fetchFiltered(ctx context.Context, metricName string) cacheEntry {
	escaped := promqlEscaper.Replace(qc.cluster)
	sel := promqlMetricSelector(metricName)

	var results []MetricResult
	var firstErr error

	var targetHosts []string
	clusterScopedOnly := false

	if qc.registry != nil {
		if qc.registry.IsClusterScoped(metricName) {
			clusterScopedOnly = true
		} else {
			targetHosts = qc.registry.HostTypesFor(metricName)
		}
	} else if len(qc.hostTypes) > 0 {
		targetHosts = qc.hostTypes
	}

	if clusterScopedOnly {
		promql := fmt.Sprintf(`%s"@resource.k8s.cluster.name"="%s","@resource.host.type"=""}`, sel, escaped)
		r, err := qc.client.Query(ctx, promql)
		if err != nil {
			firstErr = err
		} else {
			results = append(results, r...)
		}
	} else if len(targetHosts) > 0 {
		type queryResult struct {
			results []MetricResult
			err     error
		}
		ch := make(chan queryResult, len(targetHosts))
		var wg sync.WaitGroup

		for _, ht := range targetHosts {
			wg.Add(1)
			go func(hostType string) {
				defer wg.Done()
				select {
				case qc.sem <- struct{}{}:
					defer func() { <-qc.sem }()
				case <-ctx.Done():
					ch <- queryResult{err: ctx.Err()}
					return
				}
				promql := fmt.Sprintf(`%s"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`, sel, escaped, hostType)
				r, err := qc.client.Query(ctx, promql)
				ch <- queryResult{results: r, err: err}
			}(ht)
		}

		go func() { wg.Wait(); close(ch) }()

		for qr := range ch {
			if qr.err != nil {
				if firstErr == nil {
					firstErr = qr.err
				}
				continue
			}
			results = append(results, qr.results...)
		}
	} else {
		promql := fmt.Sprintf(`%s"@resource.k8s.cluster.name"="%s"}`, sel, escaped)
		r, err := qc.client.Query(ctx, promql)
		results = r
		firstErr = err
	}

	if len(results) == 0 && firstErr != nil {
		slog.Debug("query failed", "metric", metricName, "error", firstErr)
		return cacheEntry{err: firstErr}
	}
	return cacheEntry{results: results}
}

// GetWithFilter returns results with additional PromQL label filters. Not cached.
func (qc *QueryCache) GetWithFilter(ctx context.Context, metricName string, extraFilters map[string]string) ([]MetricResult, error) {
	escaped := promqlEscaper.Replace(qc.cluster)
	sel := promqlMetricSelector(metricName)

	filters := fmt.Sprintf(`"@resource.k8s.cluster.name"="%s"`, escaped)
	for key, value := range extraFilters {
		escapedVal := promqlEscaper.Replace(value)
		switch {
		case strings.HasPrefix(key, "~@resource."):
			filters += fmt.Sprintf(`,"%s"=~"%s"`, strings.TrimPrefix(key, "~"), escapedVal)
		case strings.HasPrefix(key, "@resource."):
			filters += fmt.Sprintf(`,"%s"="%s"`, key, escapedVal)
		case strings.HasPrefix(key, "~"):
			filters += fmt.Sprintf(`,%s=~"%s"`, strings.TrimPrefix(key, "~"), escapedVal)
		default:
			filters += fmt.Sprintf(`,%s="%s"`, key, escapedVal)
		}
	}

	return qc.client.Query(ctx, sel+filters+"}")
}

// GetUnfiltered returns results without cluster filtering. Cached separately.
func (qc *QueryCache) GetUnfiltered(ctx context.Context, metricName string) ([]MetricResult, error) {
	qc.mu.RLock()
	if entry, ok := qc.unfiltered[metricName]; ok {
		qc.mu.RUnlock()
		return entry.results, entry.err
	}
	qc.mu.RUnlock()

	qc.mu.Lock()
	if entry, ok := qc.unfiltered[metricName]; ok {
		qc.mu.Unlock()
		return entry.results, entry.err
	}
	qc.mu.Unlock()

	results, queryErr := qc.client.Query(ctx, metricName)
	entry := cacheEntry{results: results, err: queryErr}

	qc.mu.Lock()
	qc.unfiltered[metricName] = entry
	qc.mu.Unlock()

	return entry.results, entry.err
}
