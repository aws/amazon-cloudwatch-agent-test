// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// TestConfig holds configuration for the OTEL integration test suite.
type TestConfig struct {
	Region         string
	Endpoint       string
	Timeout        time.Duration
	MaxRetries     int
	ClusterName    string
	AccountID      string
	SigningService string
}

// OtelMetricsClient queries the OTLP PromQL API with SigV4 authentication.
type OtelMetricsClient struct {
	httpClient     *http.Client
	signer         *v4.Signer
	creds          aws.CredentialsProvider
	queryURL       string
	region         string
	signingService string
	maxRetries     int
	seriesLimit    int // endpoint's per-query series cap; 0 means defaultSeriesLimit
}

type promqlResponse struct {
	Status   string     `json:"status"`
	Data     promqlData `json:"data"`
	Warnings []string   `json:"warnings"`
}

type promqlData struct {
	ResultType string         `json:"resultType"`
	Result     []promqlSeries `json:"result"`
}

type promqlSeries struct {
	Metric    map[string]string `json:"metric"`
	Value     []json.RawMessage `json:"value"`
	Histogram []json.RawMessage `json:"histogram"`
}

// NewClient creates an OtelMetricsClient from the given config.
func NewClient(ctx context.Context, config TestConfig) (*OtelMetricsClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(config.Region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return &OtelMetricsClient{
		httpClient:     &http.Client{Timeout: config.Timeout},
		signer:         v4.NewSigner(),
		creds:          cfg.Credentials,
		queryURL:       config.Endpoint + "/api/v1/query",
		region:         config.Region,
		signingService: config.SigningService,
		maxRetries:     config.MaxRetries,
	}, nil
}

// Query executes a single PromQL instant query and returns parsed results. The endpoint caps
// results at 500 series, often without saying so; use QueryComplete when every series matters.
func (c *OtelMetricsClient) Query(ctx context.Context, promql string) ([]MetricResult, error) {
	results, truncated, err := c.queryAt(ctx, promql, "")
	if truncated {
		log.Printf("WARNING: result truncated by the endpoint series limit query=%s", promql)
	}
	return results, err
}

// queryAt runs one instant query, evaluated at evalTime when non-empty. It also returns
// whether the endpoint warned of truncation.
func (c *OtelMetricsClient) queryAt(ctx context.Context, promql, evalTime string) ([]MetricResult, bool, error) {
	params := url.Values{"query": {promql}}
	if evalTime != "" {
		params.Set("time", evalTime)
	}
	log.Printf("querying promql=%s", promql)

	raw, err := c.requestWithRetry(ctx, c.queryURL, params)
	if err != nil {
		return nil, false, err
	}
	results := c.parseResponse(raw)
	log.Printf("query returned count=%d", len(results))
	return results, hasTruncationWarning(raw.Warnings), nil
}

// hasTruncationWarning reports the endpoint's "results truncated due to series limit" warning.
// Aggregations carry it reliably; plain selectors only sometimes, so it is never proof of completeness.
func hasTruncationWarning(warnings []string) bool {
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "truncated") {
			return true
		}
	}
	return false
}

// MetricNames returns the distinct metric names present for the given PromQL matcher over [start, end].
// It uses the label-values endpoint, which (unlike the query and series endpoints) accepts a matcher
// without a concrete __name__, so a resource-scoped matcher enumerates every metric a run produced.
func (c *OtelMetricsClient) MetricNames(ctx context.Context, matcher string, start, end time.Time) ([]string, error) {
	namesURL := c.queryURL[:len(c.queryURL)-len("/query")] + "/label/__name__/values"
	params := url.Values{
		"match[]": {matcher},
		"start":   {fmt.Sprintf("%d", start.Unix())},
		"end":     {fmt.Sprintf("%d", end.Unix())},
	}
	raw, err := c.requestRawWithRetry(ctx, namesURL, params)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parsing label values response: %w", err)
	}
	return resp.Data, nil
}

func (c *OtelMetricsClient) requestWithRetry(ctx context.Context, baseURL string, params url.Values) (*promqlResponse, error) {
	body, err := c.requestRawWithRetry(ctx, baseURL, params)
	if err != nil {
		return nil, err
	}
	var result promqlResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return &result, nil
}

func (c *OtelMetricsClient) requestRawWithRetry(ctx context.Context, baseURL string, params url.Values) ([]byte, error) {
	// SHA256 of empty body for GET requests.
	const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"?"+params.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		creds, err := c.creds.Retrieve(ctx)
		if err != nil {
			return nil, fmt.Errorf("retrieving credentials: %w", err)
		}

		if err := c.signer.SignHTTP(ctx, creds, req, emptyPayloadHash, c.signingService, c.region, time.Now()); err != nil {
			return nil, fmt.Errorf("signing request: %w", err)
		}

		release, err := acquireRequestSlot(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			release()
			lastErr = err
			log.Printf("request failed attempt=%d error=%v", attempt+1, err)
			if attempt < c.maxRetries-1 {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		release()
		if readErr != nil {
			return nil, fmt.Errorf("reading response: %w", readErr)
		}

		// 429 is throttling, not a bad query: retry it like a server error.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, truncate(string(body), 200))
			log.Printf("server error, retrying status=%d attempt=%d", resp.StatusCode, attempt+1)
			if attempt < c.maxRetries-1 {
				time.Sleep(retryDelay(resp.Header.Get("Retry-After"), attempt, time.Now()))
			}
			continue
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		}

		return body, nil
	}
	return nil, fmt.Errorf("all %d attempts failed: %w", c.maxRetries, lastErr)
}

// maxRetryAfter caps a server-requested wait so one throttled request cannot stall a suite.
const maxRetryAfter = 30 * time.Second

// retryDelay honors a Retry-After header (seconds or HTTP date) and otherwise backs off exponentially.
func retryDelay(retryAfter string, attempt int, now time.Time) time.Duration {
	d := time.Duration(1<<attempt) * time.Second
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(retryAfter); err == nil {
		d = at.Sub(now)
	}
	if d < 0 {
		d = 0
	}
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}

type requestSlotsKey struct{}

// withRequestSlots makes every request issued under ctx hold one slot of sem while it is in flight.
func withRequestSlots(ctx context.Context, sem chan struct{}) context.Context {
	return context.WithValue(ctx, requestSlotsKey{}, sem)
}

func acquireRequestSlot(ctx context.Context) (release func(), err error) {
	sem, _ := ctx.Value(requestSlotsKey{}).(chan struct{})
	if sem == nil {
		return func() {}, nil
	}
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *OtelMetricsClient) parseResponse(response *promqlResponse) []MetricResult {
	var results []MetricResult
	for _, series := range response.Data.Result {
		metricName := series.Metric["__name__"]
		labels := ParseLabels(series.Metric)

		if series.Histogram != nil && len(series.Histogram) >= 1 {
			var tsFloat float64
			if err := json.Unmarshal(series.Histogram[0], &tsFloat); err != nil {
				continue
			}
			ts := time.Unix(int64(tsFloat), int64((tsFloat-float64(int64(tsFloat)))*1e9))
			results = append(results, MetricResult{
				MetricName:  metricName,
				Labels:      labels,
				Timestamp:   ts,
				IsHistogram: true,
			})
			continue
		}

		if len(series.Value) < 2 {
			continue
		}

		var tsFloat float64
		if err := json.Unmarshal(series.Value[0], &tsFloat); err != nil {
			continue
		}
		var valStr string
		if err := json.Unmarshal(series.Value[1], &valStr); err != nil {
			continue
		}
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}

		ts := time.Unix(int64(tsFloat), int64((tsFloat-float64(int64(tsFloat)))*1e9))
		results = append(results, MetricResult{
			MetricName: metricName,
			Labels:     labels,
			Value:      val,
			Timestamp:  ts,
		})
	}
	return results
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RangeResult holds a single time series from a range query.
type RangeResult struct {
	Labels     MetricLabels
	Timestamps []time.Time
	Values     []float64
}

type promqlRangeResponse struct {
	Status   string          `json:"status"`
	Data     promqlRangeData `json:"data"`
	Warnings []string        `json:"warnings"`
}

type promqlRangeData struct {
	ResultType string              `json:"resultType"`
	Result     []promqlRangeSeries `json:"result"`
}

type promqlRangeSeries struct {
	Metric map[string]string   `json:"metric"`
	Values [][]json.RawMessage `json:"values"`
}

// QueryRange executes a PromQL range query and returns time series with multiple samples.
func (c *OtelMetricsClient) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) ([]RangeResult, error) {
	rangeURL := c.queryURL[:len(c.queryURL)-len("/query")] + "/query_range"
	params := url.Values{
		"query": {promql},
		"start": {fmt.Sprintf("%d", start.Unix())},
		"end":   {fmt.Sprintf("%d", end.Unix())},
		"step":  {fmt.Sprintf("%ds", int(step.Seconds()))},
	}

	rawBytes, err := c.requestRawWithRetry(ctx, rangeURL, params)
	if err != nil {
		return nil, err
	}

	var rangeResp promqlRangeResponse
	if err := json.Unmarshal(rawBytes, &rangeResp); err != nil {
		return nil, fmt.Errorf("parsing range response: %w", err)
	}
	if hasTruncationWarning(rangeResp.Warnings) {
		log.Printf("WARNING: range result truncated by the endpoint series limit query=%s", promql)
	}

	var results []RangeResult
	for _, series := range rangeResp.Data.Result {
		labels := ParseLabels(series.Metric)
		rr := RangeResult{Labels: labels}
		for _, pair := range series.Values {
			if len(pair) < 2 {
				continue
			}
			var tsFloat float64
			if err := json.Unmarshal(pair[0], &tsFloat); err != nil {
				continue
			}
			var valStr string
			if err := json.Unmarshal(pair[1], &valStr); err != nil {
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			rr.Timestamps = append(rr.Timestamps, time.Unix(int64(tsFloat), 0))
			rr.Values = append(rr.Values, val)
		}
		results = append(results, rr)
	}
	return results, nil
}
