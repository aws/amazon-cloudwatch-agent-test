// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
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
}

type promqlResponse struct {
	Status string     `json:"status"`
	Data   promqlData `json:"data"`
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

// Query executes a PromQL instant query and returns parsed results.
func (c *OtelMetricsClient) Query(ctx context.Context, promql string) ([]MetricResult, error) {
	params := url.Values{"query": {promql}}
	slog.Debug("querying", "promql", promql)

	raw, err := c.requestWithRetry(ctx, c.queryURL, params)
	if err != nil {
		return nil, err
	}
	results := c.parseResponse(raw)
	slog.Debug("query returned", "count", len(results))
	return results, nil
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

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			slog.Warn("request failed", "attempt", attempt+1, "error", err)
			if attempt < c.maxRetries-1 {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading response: %w", readErr)
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, truncate(string(body), 200))
			slog.Warn("server error, retrying", "status", resp.StatusCode, "attempt", attempt+1)
			if attempt < c.maxRetries-1 {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
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
	Status string          `json:"status"`
	Data   promqlRangeData `json:"data"`
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
