// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header  string
		attempt int
		want    time.Duration
	}{
		{"", 0, time.Second},
		{"", 2, 4 * time.Second},
		{"not-a-delay", 1, 2 * time.Second},
		{"0", 2, 0},
		{"5", 0, 5 * time.Second},
		{"120", 0, maxRetryAfter},
		{now.Add(3 * time.Second).Format(http.TimeFormat), 0, 3 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0, 0},
	} {
		assert.Equal(t, tc.want, retryDelay(tc.header, tc.attempt, now), "Retry-After %q attempt %d", tc.header, tc.attempt)
	}
}

func TestQuery_ThrottledRequestWaitsForRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "throttled", http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	c.maxRetries = 2

	start := time.Now()
	_, err := c.Query(context.Background(), "up")
	require.NoError(t, err)
	assert.EqualValues(t, 2, atomic.LoadInt32(&calls))
	assert.Less(t, time.Since(start), 500*time.Millisecond, "Retry-After: 0 replaces the 1 s backoff")
}
