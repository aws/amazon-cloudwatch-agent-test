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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient creates an OtelMetricsClient pointing at a test HTTP server.
// Uses static credentials and a real signer; the test server ignores signatures.
func newTestClient(url string) *OtelMetricsClient {
	return &OtelMetricsClient{
		httpClient: &http.Client{},
		signer:     v4.NewSigner(),
		creds: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
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

func TestFilterMatchers(t *testing.T) {
	qc := NewQueryCache(nil, "my-cluster")
	cases := []struct {
		key, value, want string
	}{
		{"@resource.k8s.pod.name", "p", `,"@resource.k8s.pod.name"="p"`},
		{"~@resource.k8s.pod.name", "p-.*", `,"@resource.k8s.pod.name"=~"p-.*"`},
		{"@instrumentation.cloudwatch.pipeline", "vllm", `,"@instrumentation.cloudwatch.pipeline"="vllm"`},
		{"model_name", "m", `,model_name="m"`},
		{"~model_name", "m.*", `,model_name=~"m.*"`},
	}
	for _, c := range cases {
		got := qc.filterMatchers(map[string]string{c.key: c.value})
		want := `"@resource.k8s.cluster.name"="my-cluster"` + c.want
		if got != want {
			t.Errorf("filterMatchers(%q) = %s, want %s", c.key, got, want)
		}
	}
}

// A cluster-scoped series that wrongly gained host keys must reach the absence tests, not be filtered out.
func TestQueryCache_ClusterScopedReadsHostStampedSeries(t *testing.T) {
	hostTypes := []string{"t3.medium"}
	for _, tc := range []struct {
		source MetricSource
		metric string
	}{
		{SourceKubeStateMetrics, "kube_deployment_status_replicas"},
		{SourceControlPlane, "apiserver_request_total"},
	} {
		t.Run(tc.metric, func(t *testing.T) {
			f, c := newFake(t, 10, []fakeSeries{
				{labels: map[string]string{"__name__": tc.metric, "@resource.k8s.cluster.name": "c1", "id": "clean"}},
				{labels: map[string]string{"__name__": tc.metric, "@resource.k8s.cluster.name": "c1", "id": "leaked",
					"@resource.host.type": "t3.medium", "@resource.host.id": "i-0abc"}},
				{labels: map[string]string{"__name__": tc.metric, "@resource.k8s.cluster.name": "c2", "id": "other-cluster"}},
			})
			f.emptyMatchesAbsent = "@resource.host.type"
			registry := NewSourceRegistry(hostTypes,
				[]SourceHostMapping{{Source: SourceCadvisor, HostTypes: hostTypes}, {Source: tc.source, HostTypes: nil}},
				SourceMapping{Source: tc.source, Metrics: []MetricDefinition{{Name: tc.metric}}},
			)
			qc := NewQueryCache(c, "c1", WithHostTypes(hostTypes), WithSourceRegistry(registry))

			results, err := qc.Get(context.Background(), tc.metric)
			require.NoError(t, err)
			hostTypeByID := map[string]string{}
			for _, r := range results {
				hostTypeByID[r.Labels.Datapoint["id"]] = r.Labels.Resource["host.type"]
			}
			assert.Equal(t, map[string]string{"clean": "", "leaked": "t3.medium"}, hostTypeByID,
				"the host-stamped series must be returned so the absence tests can fail on it")
			for _, r := range f.queries() {
				assert.NotContains(t, r.query, "host.type", "cluster-scoped metrics are read with only the cluster filter")
			}
		})
	}
}

func TestQueryCache_SharedConcurrencyBoundsBothCaches(t *testing.T) {
	f := &fakePromQL{t: t, series: podSeries(6, 5), limit: 10}
	var inflight, peak int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inflight++
		if inflight > peak {
			peak = inflight
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		f.handler(w, r)
		mu.Lock()
		inflight--
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL)
	c.seriesLimit = 10
	pinned := NewQueryCache(c, "c1", WithMaxConcurrency(2))
	allNodes := NewQueryCache(c, "c1", WithSharedConcurrency(pinned))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, qc := range []*QueryCache{pinned, pinned, allNodes, allNodes} { // each call needs several shard requests
		wg.Add(1)
		go func(qc *QueryCache) {
			defer wg.Done()
			results, err := qc.GetWithFilter(ctx, "kube_pod_info", nil)
			assert.NoError(t, err)
			assert.Len(t, results, 30)
		}(qc)
	}
	wg.Wait()
	assert.LessOrEqual(t, peak, 2, "the requests of both caches must count against one limit")
}
