// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// newTestClient creates an OtelMetricsClient pointing at a test HTTP server.
// Uses static credentials and a real signer; the test server ignores signatures.
func newTestClient(url string) *OtelMetricsClient {
	return &OtelMetricsClient{
		httpClient:     &http.Client{},
		signer:         v4.NewSigner(),
		creds:          aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "TOKEN"}, nil
		}),
		queryURL:       url + "/api/v1/query",
		region:         "us-west-2",
		signingService: "aps",
		maxRetries:     1,
	}
}

// promqlResponseJSON builds a minimal PromQL JSON response body.
func promqlResponseJSON(series []map[string]string) string {
	type result struct {
		Metric map[string]string `json:"metric"`
		Value  []json.RawMessage `json:"value"`
	}
	type data struct {
		ResultType string   `json:"resultType"`
		Result     []result `json:"result"`
	}
	type response struct {
		Status string `json:"status"`
		Data   data   `json:"data"`
	}

	results := make([]result, 0, len(series))
	for _, labels := range series {
		results = append(results, result{
			Metric: labels,
			Value:  []json.RawMessage{[]byte(`1234567890`), []byte(`"1.0"`)},
		})
	}

	resp := response{
		Status: "success",
		Data:   data{ResultType: "vector", Result: results},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func TestQueryCache_EmptyResultNotStored(t *testing.T) {
	// Server returns an empty result set (no series).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, promqlResponseJSON(nil))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	qc := NewQueryCache(client, "test-cluster")

	results, err := qc.Get(context.Background(), "empty_metric")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results, got %d", len(results))
	}

	// Verify the entry was NOT stored in the cache.
	qc.mu.RLock()
	_, cached := qc.filtered["empty_metric"]
	qc.mu.RUnlock()
	if cached {
		t.Fatal("empty result should not be cached")
	}
}

func TestQueryCache_NonEmptyResultStored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, promqlResponseJSON([]map[string]string{
			{"__name__": "cpu_usage", "host": "node-1"},
		}))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	qc := NewQueryCache(client, "test-cluster")

	results, err := qc.Get(context.Background(), "cpu_usage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify the entry WAS stored in the cache.
	qc.mu.RLock()
	_, cached := qc.filtered["cpu_usage"]
	qc.mu.RUnlock()
	if !cached {
		t.Fatal("non-empty result should be cached")
	}
}

func TestQueryCache_ErrorResultStored(t *testing.T) {
	// Server returns HTTP 500 to trigger an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	qc := NewQueryCache(client, "test-cluster")

	_, err := qc.Get(context.Background(), "error_metric")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify the error entry WAS stored in the cache.
	qc.mu.RLock()
	entry, cached := qc.filtered["error_metric"]
	qc.mu.RUnlock()
	if !cached {
		t.Fatal("error result should be cached")
	}
	if entry.err == nil {
		t.Fatal("cached entry should contain the error")
	}
}

func TestQueryCache_WaiterGetsNilOnEmptyResult(t *testing.T) {
	// Simulate the waiter path: register an inflight channel,
	// close it without storing (empty result), verify waiter gets nil, nil.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, promqlResponseJSON(nil))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	qc := NewQueryCache(client, "test-cluster")

	ch := make(chan struct{})
	qc.mu.Lock()
	qc.inflight["waiter_metric"] = ch
	qc.mu.Unlock()

	var waiterResults []MetricResult
	var waiterErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ch
		qc.mu.RLock()
		entry, ok := qc.filtered["waiter_metric"]
		qc.mu.RUnlock()
		if !ok {
			waiterResults = nil
			waiterErr = nil
			return
		}
		waiterResults = entry.results
		waiterErr = entry.err
	}()

	// Simulate: fetcher got empty, did NOT store, cleans up inflight and closes ch.
	qc.mu.Lock()
	delete(qc.inflight, "waiter_metric")
	qc.mu.Unlock()
	close(ch)

	wg.Wait()

	if waiterResults != nil {
		t.Fatalf("waiter expected nil results, got %v", waiterResults)
	}
	if waiterErr != nil {
		t.Fatalf("waiter expected nil error, got %v", waiterErr)
	}
}

func TestQueryCache_GetUnfiltered_EmptyNotStored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, promqlResponseJSON(nil))
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	qc := NewQueryCache(client, "test-cluster")

	results, err := qc.GetUnfiltered(context.Background(), "empty_unfiltered")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results, got %d", len(results))
	}

	qc.mu.RLock()
	_, cached := qc.unfiltered["empty_unfiltered"]
	qc.mu.RUnlock()
	if cached {
		t.Fatal("empty unfiltered result should not be cached")
	}
}

func TestQueryCache_GetUnfiltered_ErrorStored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	qc := NewQueryCache(client, "test-cluster")

	_, err := qc.GetUnfiltered(context.Background(), "error_unfiltered")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	qc.mu.RLock()
	entry, cached := qc.unfiltered["error_unfiltered"]
	qc.mu.RUnlock()
	if !cached {
		t.Fatal("error unfiltered result should be cached")
	}
	if entry.err == nil {
		t.Fatal("cached entry should contain the error")
	}
}

func TestPromqlMetricSelector(t *testing.T) {
	got := promqlMetricSelector("node.cpu.usage")
	want := `{"__name__"="node.cpu.usage",`
	if got != want {
		t.Fatalf("promqlMetricSelector(dotted) = %q, want %q", got, want)
	}
	got = promqlMetricSelector("cpu_usage")
	want = `cpu_usage{`
	if got != want {
		t.Fatalf("promqlMetricSelector(plain) = %q, want %q", got, want)
	}
}
