// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeEvalTime = "1700000000.123"

type fakeSeries struct {
	labels map[string]string
	stale  bool // occupies the cap like a recently ended series, but has no sample
}

type fakeRequest struct {
	query, time string
}

// fakePromQL mimics the endpoint: plain selectors are capped at limit series (stale ones
// included), aggregations warn when capped, and `=""` matches an absent label only for emptyMatchesAbsent.
type fakePromQL struct {
	t                  *testing.T
	series             []fakeSeries
	limit              int
	countOffset        int    // added to count() without by, to simulate an inconsistent endpoint
	brokenNotRegex     bool   // make !~ match everything, so shards overlap
	emptyMatchesAbsent string // label whose `=""` matches absent, as @resource.host.type does on the endpoint
	mu                 sync.Mutex
	requests           []fakeRequest
}

func (f *fakePromQL) handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, ts := q.Get("query"), q.Get("time")
	f.mu.Lock()
	f.requests = append(f.requests, fakeRequest{query: query, time: ts})
	f.mu.Unlock()
	if ts == "" {
		ts = fakeEvalTime
	}

	type result struct {
		Metric map[string]string `json:"metric"`
		Value  []json.RawMessage `json:"value"`
	}
	resp := struct {
		Status   string   `json:"status"`
		Warnings []string `json:"warnings,omitempty"`
		Data     struct {
			ResultType string   `json:"resultType"`
			Result     []result `json:"result"`
		} `json:"data"`
	}{Status: "success"}
	resp.Data.ResultType = "vector"
	sample := func(metric map[string]string, v int) result {
		return result{Metric: metric, Value: []json.RawMessage{json.RawMessage(ts), json.RawMessage(strconv.Quote(strconv.Itoa(v)))}}
	}

	switch {
	case strings.HasPrefix(query, "topk("):
		m := regexp.MustCompile(`^topk\((\d+),(.*)\)$`).FindStringSubmatch(query)
		if m == nil {
			f.badRequest(w, "unexpected topk query %q", query)
			return
		}
		k, _ := strconv.Atoi(m[1])
		for _, s := range f.match(m[2]) {
			if !s.stale && len(resp.Data.Result) < k {
				resp.Data.Result = append(resp.Data.Result, sample(s.labels, 1))
			}
		}
	case strings.HasPrefix(query, "count by ("):
		label, inner, ok := parseCountBy(query)
		if !ok {
			f.badRequest(w, "unexpected count by query %q", query)
			return
		}
		groups := map[string]int{}
		for _, s := range f.match(inner) {
			if !s.stale {
				groups[s.labels[label]]++
			}
		}
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > f.limit {
			keys = keys[:f.limit]
			resp.Warnings = []string{fmt.Sprintf("results truncated due to series limit (%d).", f.limit)}
		}
		for _, k := range keys {
			m := map[string]string{}
			if k != "" {
				m[label] = k
			}
			resp.Data.Result = append(resp.Data.Result, sample(m, groups[k]))
		}
	case strings.HasPrefix(query, "count("):
		n := 0
		for _, s := range f.match(query[len("count(") : len(query)-1]) {
			if !s.stale {
				n++
			}
		}
		if n > 0 {
			resp.Data.Result = append(resp.Data.Result, sample(map[string]string{}, n+f.countOffset))
		}
	default:
		matched := f.match(query)
		capped := len(matched) > f.limit
		if capped {
			matched = matched[:f.limit]
		}
		for _, s := range matched {
			if !s.stale {
				resp.Data.Result = append(resp.Data.Result, sample(s.labels, 1))
			}
		}
		// The endpoint warns only when the capped output is full, so stale series hide it.
		if capped && len(resp.Data.Result) == f.limit {
			resp.Warnings = []string{fmt.Sprintf("results truncated due to series limit (%d).", f.limit)}
		}
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		f.t.Errorf("encoding response: %v", err)
	}
}

// badRequest reports a query the fake cannot parse; the handler runs off the test goroutine.
func (f *fakePromQL) badRequest(w http.ResponseWriter, format string, args ...interface{}) {
	f.t.Errorf(format, args...)
	http.Error(w, fmt.Sprintf(format, args...), http.StatusBadRequest)
}

func (f *fakePromQL) match(selector string) []fakeSeries {
	matchers, ok := parseFakeSelector(selector)
	if !ok {
		f.t.Errorf("unparseable selector %q", selector)
		return nil
	}
	var out []fakeSeries
	for _, s := range f.series {
		ok := true
		for _, m := range matchers {
			v, present := s.labels[m.key]
			switch m.op {
			case "=":
				ok = present && v == m.val || !present && m.val == "" && m.key == f.emptyMatchesAbsent
			case "=~":
				ok = regexp.MustCompile("^(?:" + m.val + ")$").MatchString(v)
			case "!~":
				ok = f.brokenNotRegex || !regexp.MustCompile("^(?:"+m.val+")$").MatchString(v)
			default:
				f.t.Errorf("unsupported matcher op %q", m.op)
				ok = false
			}
			if !ok {
				break
			}
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

func (f *fakePromQL) queries() []fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeRequest(nil), f.requests...)
}

type fakeMatcher struct{ key, op, val string }

func parseCountBy(q string) (string, string, bool) {
	m := regexp.MustCompile(`^count by \("((?:[^"\\]|\\.)*)"\)\((.*)\)$`).FindStringSubmatch(q)
	if m == nil {
		return "", "", false
	}
	return unescape(m[1]), m[2], true
}

func parseFakeSelector(s string) ([]fakeMatcher, bool) {
	open := strings.IndexByte(s, '{')
	if open < 0 {
		return []fakeMatcher{{"__name__", "=", s}}, true
	}
	var out []fakeMatcher
	if open > 0 {
		out = append(out, fakeMatcher{"__name__", "=", s[:open]})
	}
	re := regexp.MustCompile(`^\s*(?:"((?:[^"\\]|\\.)*)"|([a-zA-Z_][a-zA-Z0-9_]*))\s*(=~|!~|=)\s*"((?:[^"\\]|\\.)*)"\s*,?`)
	body := s[open+1 : len(s)-1]
	for strings.TrimSpace(body) != "" {
		m := re.FindStringSubmatch(body)
		if m == nil {
			return nil, false
		}
		key := m[2]
		if m[1] != "" {
			key = unescape(m[1])
		}
		out = append(out, fakeMatcher{key, m[3], unescape(m[4])})
		body = body[len(m[0]):]
	}
	return out, true
}

func unescape(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(s)
}

func newFake(t *testing.T, limit int, series []fakeSeries) (*fakePromQL, *OtelMetricsClient) {
	f := &fakePromQL{t: t, series: series, limit: limit}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL)
	c.seriesLimit = limit
	return f, c
}

// podSeries builds kube_pod_info-like series: perNode pods on each node, plus extra.
func podSeries(nodes, perNode int) []fakeSeries {
	var out []fakeSeries
	for n := 0; n < nodes; n++ {
		for p := 0; p < perNode; p++ {
			out = append(out, fakeSeries{labels: map[string]string{
				"__name__":                     "kube_pod_info",
				"@resource.k8s.cluster.name":   "c1",
				"@resource.k8s.node.name":      fmt.Sprintf("ip-10-0-%d-1.ec2.internal", n),
				"@resource.k8s.pod.name":       fmt.Sprintf("pod-%d-%d", n, p),
				"@resource.k8s.namespace.name": "default",
				"@instrumentation.@name":       "ksm",
				"@aws.account":                 "123456789012",
				"__type__":                     "Gauge",
			}})
		}
	}
	return out
}

const podSelector = `kube_pod_info{"@resource.k8s.cluster.name"="c1"}`

func podNames(results []MetricResult) map[string]bool {
	out := map[string]bool{}
	for _, r := range results {
		out[r.Labels.Resource["k8s.pod.name"]] = true
	}
	return out
}

func countQueries(reqs []fakeRequest, pred func(string) bool) int {
	n := 0
	for _, r := range reqs {
		if pred(r.query) {
			n++
		}
	}
	return n
}

func isPlain(q string) bool { return !strings.HasPrefix(q, "count") }

// assertOneEvalTime checks that the client pinned every request to the same evaluation time.
func assertOneEvalTime(t *testing.T, reqs []fakeRequest) {
	t.Helper()
	require.NotEmpty(t, reqs)
	require.NotEmpty(t, reqs[0].time, "the client must pick the evaluation time: %s", reqs[0].query)
	for _, r := range reqs {
		assert.Equal(t, reqs[0].time, r.time, "every query is pinned to one evaluation time: %s", r.query)
	}
}

func TestQueryComplete_NotTruncatedIsOneDataQuery(t *testing.T) {
	f, c := newFake(t, 10, podSeries(2, 3))

	results, err := c.QueryComplete(context.Background(), podSelector)
	require.NoError(t, err)
	assert.Len(t, results, 6)

	reqs := f.queries()
	require.Len(t, reqs, 2, "expected the selector plus one count(): %+v", reqs)
	assert.ElementsMatch(t, []string{podSelector, "count(" + podSelector + ")"}, []string{reqs[0].query, reqs[1].query})
	assertOneEvalTime(t, reqs)
}

func TestQueryComplete_SelectorAndCountRunTogether(t *testing.T) {
	f := &fakePromQL{t: t, series: podSeries(2, 3), limit: 10}
	var mu sync.Mutex
	arrived := 0
	both := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if arrived++; arrived == 2 {
			close(both)
		}
		mu.Unlock()
		// Answer only once both requests are in flight, which a client that waits for one cannot do.
		select {
		case <-both:
		case <-time.After(2 * time.Second):
			t.Errorf("request %q was not sent together with the other one", r.URL.Query().Get("query"))
		}
		f.handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL)
	c.seriesLimit = 10

	results, err := c.QueryComplete(context.Background(), podSelector)
	require.NoError(t, err)
	assert.Len(t, results, 6)
	assertOneEvalTime(t, f.queries())
}

func TestEvalTimeAt(t *testing.T) {
	assert.Equal(t, "1700000005", evalTimeAt(time.Unix(1700000010, 900e6)), "Unix seconds, evalTimeLag in the past")
}

func TestQueryComplete_TruncatedShardsByNode(t *testing.T) {
	series := podSeries(6, 5)
	series[0].stale, series[7].stale = true, true // stale series still use up the cap
	f, c := newFake(t, 10, series)

	results, err := c.QueryComplete(context.Background(), podSelector)
	require.NoError(t, err)
	assert.Len(t, results, 28)
	assert.Len(t, podNames(results), 28, "every live pod exactly once")

	reqs := f.queries()
	assert.Equal(t, 1, countQueries(reqs, func(q string) bool {
		return q == `count by ("@resource.k8s.node.name")(`+podSelector+")"
	}))
	shards := countQueries(reqs, func(q string) bool { return isPlain(q) && strings.Contains(q, `"@resource.k8s.node.name"`) })
	assert.GreaterOrEqual(t, shards, 4, "28 series under a cap of 10 need at least 4 shards")
	assert.Equal(t, 2, countQueries(reqs, func(q string) bool { return q == "count("+podSelector+")" }), "count() before and after")
	assertOneEvalTime(t, reqs)
}

func TestQueryComplete_AbsentNodeShardUsesNotRegex(t *testing.T) {
	series := podSeries(3, 4)
	for i := 0; i < 3; i++ {
		series = append(series, fakeSeries{labels: map[string]string{
			"__name__":                   "kube_pod_info",
			"@resource.k8s.cluster.name": "c1",
			"@resource.k8s.pod.name":     fmt.Sprintf("pending-%d", i),
		}})
	}
	f, c := newFake(t, 5, series)

	results, err := c.QueryComplete(context.Background(), podSelector)
	require.NoError(t, err)
	assert.Len(t, results, 15)
	names := podNames(results)
	for i := 0; i < 3; i++ {
		assert.True(t, names[fmt.Sprintf("pending-%d", i)], "series without a node must be returned")
	}

	reqs := f.queries()
	assert.Equal(t, 1, countQueries(reqs, func(q string) bool {
		return strings.Contains(q, `"@resource.k8s.node.name"!~".+"`)
	}))
	assert.Zero(t, countQueries(reqs, func(q string) bool {
		return strings.Contains(q, `"@resource.k8s.node.name"=""`)
	}), `="" does not match an absent label on this endpoint`)
}

func TestQueryComplete_SingleNodeOverLimitSplitsFurther(t *testing.T) {
	var series []fakeSeries
	for cpu := 0; cpu < 8; cpu++ {
		for _, mode := range []string{"idle", "user", "system"} {
			series = append(series, fakeSeries{labels: map[string]string{
				"__name__":                   "node_cpu_seconds_total",
				"@resource.k8s.cluster.name": "c1",
				"@resource.k8s.node.name":    "big-node",
				"cpu":                        strconv.Itoa(cpu),
				"mode":                       mode,
			}})
		}
	}
	f, c := newFake(t, 10, series)

	results, err := c.QueryComplete(context.Background(), `node_cpu_seconds_total{"@resource.k8s.cluster.name"="c1"}`)
	require.NoError(t, err)
	assert.Len(t, results, 24)
	assert.Positive(t, countQueries(f.queries(), func(q string) bool { return strings.HasPrefix(q, `count by ("cpu")`) }),
		"one node holds every series, so the split must fall through to the cpu label")
}

func TestQueryComplete_SkipsLabelWithTooManyValues(t *testing.T) {
	var series []fakeSeries
	for i := 0; i < 20; i++ {
		series = append(series, fakeSeries{labels: map[string]string{
			"__name__": "m",
			"id":       strconv.Itoa(i),
			"zone":     fmt.Sprintf("z%d", i%3),
		}})
	}
	f, c := newFake(t, 10, series)

	results, err := c.QueryComplete(context.Background(), `m{}`)
	require.NoError(t, err)
	assert.Len(t, results, 20)
	reqs := f.queries()
	assert.Equal(t, 1, countQueries(reqs, func(q string) bool { return strings.HasPrefix(q, `count by ("id")`) }))
	assert.Equal(t, 3, countQueries(reqs, func(q string) bool { return isPlain(q) && strings.Contains(q, `"zone"="z`) }))
}

func TestQueryComplete_InconsistentCountIsError(t *testing.T) {
	f, c := newFake(t, 10, podSeries(6, 5))
	f.countOffset = 3 // count() claims 33, but every split only accounts for 30

	_, err := c.QueryComplete(context.Background(), podSelector)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTruncated), "got %v", err)
}

func TestQueryComplete_ShardTotalMismatchIsError(t *testing.T) {
	series := podSeries(3, 4)
	for i := 0; i < 3; i++ {
		series = append(series, fakeSeries{labels: map[string]string{
			"__name__":                   "kube_pod_info",
			"@resource.k8s.cluster.name": "c1",
			"@resource.k8s.pod.name":     fmt.Sprintf("pending-%d", i),
		}})
	}
	f, c := newFake(t, 5, series)
	f.brokenNotRegex = true // the absent-node shard overlaps the node shards

	_, err := c.QueryComplete(context.Background(), podSelector)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTruncated), "got %v", err)
	assert.Contains(t, err.Error(), "shards returned 17 series but count() was 15 before and 15 after")
}

func TestQueryComplete_UnsplittableIsError(t *testing.T) {
	var series []fakeSeries
	for i := 0; i < 20; i++ {
		series = append(series, fakeSeries{labels: map[string]string{"__name__": "m", "id": strconv.Itoa(i)}})
	}
	_, c := newFake(t, 10, series)
	// id has 20 values, so its count by is capped too; nothing else varies.
	_, err := c.QueryComplete(context.Background(), `m{}`)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTruncated), "got %v", err)
}

func TestQueryComplete_EmptyResultWithLiveSeries(t *testing.T) {
	series := podSeries(2, 3)
	stale := make([]fakeSeries, 0, 5)
	for i := 0; i < 5; i++ {
		s := podSeries(1, 1)[0]
		s.labels["@resource.k8s.pod.name"] = fmt.Sprintf("done-%d", i)
		s.stale = true
		stale = append(stale, s)
	}
	f, c := newFake(t, 5, append(stale, series...))

	results, err := c.QueryComplete(context.Background(), podSelector)
	require.NoError(t, err)
	assert.Len(t, results, 6, "the first query sees only stale series, yet six are live")
	assertOneEvalTime(t, f.queries())
}

func TestQueryComplete_NonSelectorPassesThrough(t *testing.T) {
	f, c := newFake(t, 10, podSeries(1, 2))
	_, err := c.QueryComplete(context.Background(), "count("+podSelector+")")
	require.NoError(t, err)
	assert.Len(t, f.queries(), 1)
}

func TestQueryComplete_WarnedSelectorStillShards(t *testing.T) {
	f, c := newFake(t, 10, podSeries(4, 5))

	results, err := c.QueryComplete(context.Background(), podSelector)
	require.NoError(t, err)
	assert.Len(t, results, 20)
	assert.Positive(t, countQueries(f.queries(), func(q string) bool { return strings.HasPrefix(q, "count by (") }))
}

func TestQuery_TruncationWarningKeepsResults(t *testing.T) {
	_, c := newFake(t, 3, podSeries(2, 3))
	results, err := c.Query(context.Background(), podSelector)
	require.NoError(t, err, "Query stays a single best-effort request")
	assert.Len(t, results, 3)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","warnings":["results truncated due to series limit (500)."],"data":{"resultType":"matrix","result":[{"metric":{"a":"b"},"values":[[1,"1"]]}]}}`)
	}))
	defer srv.Close()
	rr, err := newTestClient(srv.URL).QueryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Minute)
	require.NoError(t, err)
	assert.Len(t, rr, 1)
}

func TestQueryCache_GetReturnsEverySeriesWhenTruncated(t *testing.T) {
	f, c := newFake(t, 10, podSeries(5, 7))
	qc := NewQueryCache(c, "c1")

	results, err := qc.Get(context.Background(), "kube_pod_info")
	require.NoError(t, err)
	assert.Len(t, results, 35)

	n := len(f.queries())
	again, err := qc.Get(context.Background(), "kube_pod_info")
	require.NoError(t, err)
	assert.Len(t, again, 35)
	assert.Len(t, f.queries(), n, "a cached metric must not be re-queried")
}

func TestQueryCache_TruncatedHostTypeFailsFetch(t *testing.T) {
	series := podSeries(1, 3)
	for i := range series {
		series[i].labels["@resource.host.type"] = "a"
	}
	for i := 0; i < 20; i++ { // host type b cannot be split, so its shard fetch fails
		series = append(series, fakeSeries{labels: map[string]string{
			"__name__": "kube_pod_info", "@resource.k8s.cluster.name": "c1",
			"@resource.host.type": "b", "id": strconv.Itoa(i),
		}})
	}
	_, c := newFake(t, 10, series)
	qc := NewQueryCache(c, "c1", WithHostTypes([]string{"a", "b"}))

	results, err := qc.Get(context.Background(), "kube_pod_info")
	require.Error(t, err, "the 3 series of host type a are not the whole result")
	assert.True(t, errors.Is(err, ErrTruncated), "got %v", err)
	assert.Empty(t, results)
}

func TestQueryCache_ShardRequestsStayWithinConcurrencyLimit(t *testing.T) {
	var series []fakeSeries
	for _, ht := range []string{"a", "b"} {
		for _, s := range podSeries(6, 5) {
			s.labels["@resource.host.type"] = ht
			s.labels["@resource.k8s.node.name"] += "-" + ht
			series = append(series, s)
		}
	}
	f := &fakePromQL{t: t, series: series, limit: 10}
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
	qc := NewQueryCache(c, "c1", WithHostTypes([]string{"a", "b"}), WithMaxConcurrency(2))

	results, err := qc.Get(context.Background(), "kube_pod_info")
	require.NoError(t, err)
	assert.Len(t, results, 60)
	assert.LessOrEqual(t, peak, 2, "shard re-fetches must share the cache's concurrency limit")
}

func TestIsVectorSelector(t *testing.T) {
	for s, want := range map[string]bool{
		`up`:                     true,
		`kube_pod_info{"a"="b"}`: true,
		`{"__name__"="k8s.pod.cpu.usage","a"="}"}`: true,
		` m{a=~"x|y", b!~".+"} `:                   true,
		`k8s.pod.cpu.usage`:                        false,
		`count(m{a="b"})`:                          false,
		`m{a="b"}[5m]`:                             false,
		`m{a="b"} or n{c="d"}`:                     false,
		`rate(m[5m])`:                              false,
		`m{a="b"} offset 5m`:                       false,
	} {
		assert.Equal(t, want, isVectorSelector(s), s)
	}
}

func TestWithMatcher(t *testing.T) {
	assert.Equal(t, `m{x="1"}`, withMatcher("m", `x="1"`))
	assert.Equal(t, `m{x="1"}`, withMatcher("m{}", `x="1"`))
	assert.Equal(t, `m{a="b",x="1"}`, withMatcher(`m{a="b"}`, `x="1"`))
	assert.Equal(t, `{"__name__"="a.b",x="1"}`, withMatcher(`{"__name__"="a.b",}`, `x="1"`))
}

func TestPackShards(t *testing.T) {
	groups := []shardGroup{{"a.b", 30}, {"c", 25}, {"", 4}, {"d", 10}, {"e", 50}}
	shards := packShards("@resource.k8s.node.name", groups, 40)

	total := 0
	for _, s := range shards {
		total += s.want
		assert.LessOrEqual(t, s.want, 50)
	}
	assert.Equal(t, 119, total)
	assert.Contains(t, shards, shard{matcher: `"@resource.k8s.node.name"!~".+"`, want: 4, exact: true})
	assert.Contains(t, shards, shard{matcher: `"@resource.k8s.node.name"="e"`, want: 50, exact: true})
	assert.Contains(t, shards, shard{matcher: `"@resource.k8s.node.name"=~"a\\.b|d"`, want: 40, exact: false})

	var many []shardGroup
	for i := 0; i < 200; i++ {
		many = append(many, shardGroup{fmt.Sprintf("ip-10-0-%d-%d.ec2.internal", i/50, i), 1})
	}
	for _, s := range packShards("@resource.k8s.node.name", many, 400) {
		value := s.matcher[strings.Index(s.matcher, `"=~"`)+4 : len(s.matcher)-1]
		assert.LessOrEqual(t, len(unescape(value)), maxMatcherValueLen, "the endpoint rejects longer matcher values")
	}
}

func TestQueryCache_ParallelGetWithFilterStaysWithinConcurrencyLimit(t *testing.T) {
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
	qc := NewQueryCache(c, "c1", WithMaxConcurrency(2))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ { // more callers than slots, each needing several shard requests
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := qc.GetWithFilter(ctx, "kube_pod_info", map[string]string{"@resource.k8s.namespace.name": "default"})
			assert.NoError(t, err, "a call that held a slot while its shards waited for one would deadlock here")
			assert.Len(t, results, 30)
		}()
	}
	wg.Wait()
	assert.LessOrEqual(t, peak, 2, "shard re-fetches must share the cache's concurrency limit")
}
