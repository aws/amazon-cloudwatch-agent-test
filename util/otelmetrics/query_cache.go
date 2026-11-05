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
// Every fetch goes through QueryComplete, so results are never silently truncated, and the
// concurrency limit bounds HTTP requests, so shard re-fetches stay inside it.
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
		entry, ok := qc.filtered[metricName]
		qc.mu.RUnlock()
		if !ok {
			// Empty result was not cached; caller should retry.
			return nil, nil
		}
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
		entry, ok := qc.filtered[metricName]
		qc.mu.RUnlock()
		if !ok {
			// Empty result was not cached; caller should retry.
			return nil, nil
		}
		return entry.results, entry.err
	}
	ch := make(chan struct{})
	qc.inflight[metricName] = ch
	qc.mu.Unlock()

	// Fetch without holding any lock
	entry := qc.fetchFiltered(ctx, metricName)

	// Store and signal waiters.
	// Do not cache empty successful results; a transient miss would poison
	// all later lookups for this metric within the same test binary run.
	qc.mu.Lock()
	if len(entry.results) > 0 || entry.err != nil {
		qc.filtered[metricName] = entry
	}
	delete(qc.inflight, metricName)
	qc.mu.Unlock()
	close(ch)

	return entry.results, entry.err
}

// fetchFiltered performs the actual HTTP query for a filtered metric.
func (qc *QueryCache) fetchFiltered(ctx context.Context, metricName string) cacheEntry {
	ctx = withRequestSlots(ctx, qc.sem)
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
		r, err := qc.client.QueryComplete(ctx, promql)
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
				promql := fmt.Sprintf(`%s"@resource.k8s.cluster.name"="%s","@resource.host.type"="%s"}`, sel, escaped, hostType)
				r, err := qc.client.QueryComplete(ctx, promql)
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
		r, err := qc.client.QueryComplete(ctx, promql)
		results = r
		firstErr = err
	}

	// One failed host type makes the union incomplete, so it fails the whole fetch.
	if firstErr != nil {
		slog.Debug("query failed", "metric", metricName, "error", firstErr)
		return cacheEntry{err: firstErr}
	}
	return cacheEntry{results: results}
}

// GetWithFilter returns results with additional PromQL label filters. Not cached.
// Each HTTP request holds one slot of the cache's concurrency limit, so a suite of
// parallel filtered queries, and the shard re-fetches of a truncated one, stay inside it.
func (qc *QueryCache) GetWithFilter(ctx context.Context, metricName string, extraFilters map[string]string) ([]MetricResult, error) {
	return qc.client.QueryComplete(withRequestSlots(ctx, qc.sem), promqlMetricSelector(metricName)+qc.filterMatchers(extraFilters)+"}")
}

// filterMatchers renders the cluster matcher plus extraFilters as PromQL label
// matchers. Keys starting with "@" (@resource.*, @instrumentation.*) are quoted;
// a leading "~" makes the match a regex.
func (qc *QueryCache) filterMatchers(extraFilters map[string]string) string {
	filters := fmt.Sprintf(`"@resource.k8s.cluster.name"="%s"`, promqlEscaper.Replace(qc.cluster))
	for key, value := range extraFilters {
		escapedVal := promqlEscaper.Replace(value)
		op := "="
		if strings.HasPrefix(key, "~") {
			key, op = strings.TrimPrefix(key, "~"), "=~"
		}
		if strings.HasPrefix(key, "@") {
			key = `"` + key + `"`
		}
		filters += fmt.Sprintf(`,%s%s"%s"`, key, op, escapedVal)
	}
	return filters
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

	results, queryErr := qc.client.QueryComplete(withRequestSlots(ctx, qc.sem), metricName)
	entry := cacheEntry{results: results, err: queryErr}

	// Do not cache empty successful results; a transient miss would poison
	// all later lookups for this metric within the same test binary run.
	qc.mu.Lock()
	if len(entry.results) > 0 || entry.err != nil {
		qc.unfiltered[metricName] = entry
	}
	qc.mu.Unlock()

	return entry.results, entry.err
}
