// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// resolveRegion returns the region, falling back to AWS env vars then us-west-2.
func resolveRegion(region string) string {
	if region != "" {
		return region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return "us-west-2"
}

// AssertMetricsPresent queries the CloudWatch OTLP PromQL endpoint and returns an error if any
// metric is missing after retries. Cross-platform (returns error, no test/status dependency).
func AssertMetricsPresent(ctx context.Context, region string, metrics []string, labels map[string]string, maxRetries int, retryInterval time.Duration) error {
	region = resolveRegion(region)
	client, err := NewClient(ctx, TestConfig{
		Region:         region,
		Endpoint:       fmt.Sprintf("https://monitoring.%s.amazonaws.com", region),
		Timeout:        30 * time.Second,
		MaxRetries:     3,
		SigningService: "monitoring",
	})
	if err != nil {
		return fmt.Errorf("failed to create otel metrics client: %w", err)
	}

	validated := make(map[string]bool, len(metrics))
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}
		for _, m := range metrics {
			if validated[m] {
				continue
			}
			query := fmt.Sprintf(`{__name__="%s"`, m)
			for k, v := range labels {
				query += fmt.Sprintf(`, "%s"=~"%s"`, k, v)
			}
			query += "}"
			if results, err := client.Query(ctx, query); err == nil && len(results) > 0 {
				validated[m] = true
			}
		}
		if len(validated) == len(metrics) {
			return nil
		}
	}

	var missing []string
	for _, m := range metrics {
		if !validated[m] {
			missing = append(missing, m)
		}
	}
	return fmt.Errorf("metrics not found after %d attempts: %s", maxRetries, strings.Join(missing, ", "))
}
