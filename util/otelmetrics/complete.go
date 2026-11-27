// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ErrTruncated reports that the endpoint dropped series to stay under its per-query
// series limit and the complete result could not be recovered.
var ErrTruncated = errors.New("query result truncated by the endpoint series limit")

const (
	// defaultSeriesLimit is the endpoint's per-query cap ("series limit (500)" in its warnings).
	defaultSeriesLimit = 500
	// maxMatcherValueLen is the endpoint's limit on one matcher value, which bounds a shard's regex.
	maxMatcherValueLen = 1000
	sampleSize         = 50
	maxShardDepth      = 4
	maxSplitCandidates = 3
	// maxPrefixSteps bounds the one-character extensions of a value prefix that every series shares.
	maxPrefixSteps = 8
	// prefixLabel is the label that label_replace writes a value prefix to; it must not clash with a real label.
	prefixLabel      = "__shard_prefix"
	shardConcurrency = 4
	nodeNameLabel    = "@resource.k8s.node.name"
	// evalTimeLag keeps the evaluation time that the client picks behind the endpoint's clock
	// and its newest samples.
	evalTimeLag = 5 * time.Second
)

var metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

// QueryComplete returns every series matched by an instant vector selector. The endpoint caps
// selector results and does not reliably say so, so the result is compared with count() at the
// same evaluation time and, only when short, re-fetched as disjoint shards (by node first).
// The client picks that time, so the selector and its count run at once.
// Anything that is not a plain vector selector is passed to Query unchanged.
func (c *OtelMetricsClient) QueryComplete(ctx context.Context, selector string) ([]MetricResult, error) {
	if !isVectorSelector(selector) {
		return c.Query(ctx, selector)
	}
	evalTime := evalTimeAt(time.Now())
	var before int
	var countErr error
	counted := make(chan struct{})
	go func() {
		defer close(counted)
		before, countErr = c.countAt(ctx, selector, evalTime)
	}()
	results, _, err := c.queryAt(ctx, selector, evalTime)
	<-counted
	if err != nil {
		return nil, err
	}
	if countErr != nil {
		return nil, countErr
	}
	if len(results) >= before {
		return results, nil
	}
	log.Printf("result truncated: got %d of %d series, re-fetching in shards query=%s", len(results), before, selector)

	all, err := c.split(ctx, selector, evalTime, before, results, nil, nil, 1)
	if err != nil {
		return nil, err
	}
	after, err := c.countAt(ctx, selector, evalTime)
	if err != nil {
		return nil, err
	}
	lo, hi := before, after
	if lo > hi {
		lo, hi = hi, lo
	}
	if len(all) < lo || len(all) > hi {
		return nil, fmt.Errorf("%w: shards returned %d series but count() was %d before and %d after (query %s)",
			ErrTruncated, len(all), before, after, selector)
	}
	log.Printf("sharded fetch complete: %d series query=%s", len(all), selector)
	return all, nil
}

type shardGroup struct {
	value  string // "" when the label is absent
	count  int
	prefix bool // value is a prefix: the group holds every value that starts with it
}

type shard struct {
	matcher string
	want    int
	exact   bool   // the shard pins the label to one value (or to absent)
	prefix  string // every value of the label in the shard starts with this
}

// evalTimeAt is the evaluation time, in Unix seconds, that QueryComplete pins its queries to.
func evalTimeAt(now time.Time) string {
	return strconv.FormatInt(now.Add(-evalTimeLag).Unix(), 10)
}

// countAt returns count(selector) at evalTime.
func (c *OtelMetricsClient) countAt(ctx context.Context, selector, evalTime string) (int, error) {
	groups, err := c.countBy(ctx, selector, "", evalTime)
	if err != nil {
		return 0, err
	}
	return groupTotal(groups), nil
}

func groupTotal(groups []shardGroup) int {
	total := 0
	for _, g := range groups {
		total += g.count
	}
	return total
}

// countBy returns the series count per value of label (or the total when label is "").
// It fails with ErrTruncated when the label has too many values to list in one response.
func (c *OtelMetricsClient) countBy(ctx context.Context, selector, label, evalTime string) ([]shardGroup, error) {
	promql := "count(" + selector + ")"
	if label != "" {
		promql = fmt.Sprintf("count by (%s)(%s)", quoteLabel(label), selector)
	}
	results, truncated, err := c.queryAt(ctx, promql, evalTime)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf("%w: too many groups (query %s)", ErrTruncated, promql)
	}
	groups := make([]shardGroup, 0, len(results))
	for _, r := range results {
		g := shardGroup{count: int(r.Value)}
		if label != "" {
			g.value = r.Labels.AllLabels()[label]
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// split re-fetches a truncated selector of want series as disjoint shards over the first label that divides it.
// A label with more values than one response can list is split by value prefix instead, after the others.
// prefixes holds, per label, a prefix that every value in selector starts with.
func (c *OtelMetricsClient) split(ctx context.Context, selector, evalTime string, want int, sample []MetricResult, pinned map[string]bool, prefixes map[string]string, depth int) ([]MetricResult, error) {
	if depth > maxShardDepth {
		return nil, fmt.Errorf("%w: still truncated after %d shard levels (query %s)", ErrTruncated, maxShardDepth, selector)
	}
	if len(sample) == 0 {
		// Stale series filled the cap; topk evaluates fully, so it yields live series to inspect.
		sample, _, _ = c.queryAt(ctx, fmt.Sprintf("topk(%d,%s)", sampleSize, selector), evalTime)
	}
	candidates := splitCandidates(sample, pinned)
	var tooMany, rest []string
	for i, label := range candidates {
		if i == maxSplitCandidates {
			rest = candidates[i:]
			break
		}
		groups, err := c.countBy(ctx, selector, label, evalTime)
		if errors.Is(err, ErrTruncated) {
			tooMany = append(tooMany, label)
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(groups) < 2 || groupTotal(groups) < want {
			continue // label does not divide the set, or its groups were cut short
		}
		shards := packShards(label, groups, c.shardTarget(depth))
		log.Printf("fetching %d shards by %s depth=%d query=%s", len(shards), label, depth, selector)
		return c.fetchShards(ctx, selector, evalTime, label, shards, pinned, prefixes, depth)
	}
	// Split by value prefix, which needs no list of every value: first the labels that had too
	// many values to list, then the labels past maxSplitCandidates, up to maxSplitCandidates labels.
	for i, label := range append(tooMany, rest...) {
		if i == maxSplitCandidates {
			break
		}
		groups, prefix, err := c.prefixGroups(ctx, selector, label, prefixes[label], evalTime, want, sample)
		if err != nil {
			return nil, err
		}
		if groups == nil {
			continue
		}
		shards := packShards(label, groups, c.shardTarget(depth))
		for j := range shards {
			if shards[j].prefix == "" {
				shards[j].prefix = prefix
			}
		}
		log.Printf("fetching %d shards by %s prefix %q depth=%d query=%s", len(shards), label, prefix, depth, selector)
		return c.fetchShards(ctx, selector, evalTime, label, shards, pinned, prefixes, depth)
	}
	return nil, fmt.Errorf("%w: no label divides the result further (query %s)", ErrTruncated, selector)
}

// prefixGroups counts the series of selector by the value prefix of label one character past
// prefix, which every value in selector already starts with. The group without a longer prefix
// holds the series whose value is exactly prefix, or that lack the label when prefix is "".
// While one group holds every series, the prefix grows: first to the longest prefix of the
// sample's common prefix that every series extends, then one character at a time. The groups
// must add up to want. It returns nil groups when the label does not divide selector.
func (c *OtelMetricsClient) prefixGroups(ctx context.Context, selector, label, prefix, evalTime string, want int, sample []MetricResult) ([]shardGroup, string, error) {
	groups, longer, err := c.longestSharedPrefix(ctx, selector, label, prefix, evalTime, want, sample)
	if err != nil {
		return nil, "", err
	}
	if groups != nil {
		prefix = longer
	}
	for step := 0; ; step++ {
		if groups == nil {
			var err error
			groups, err = c.countByPrefix(ctx, selector, label, prefix, evalTime)
			if errors.Is(err, ErrTruncated) {
				return nil, "", nil
			}
			if err != nil {
				return nil, "", err
			}
		}
		if groupTotal(groups) != want {
			return nil, "", nil // groups were cut short, or do not match count()
		}
		for _, g := range groups {
			if g.prefix && !fitsMatcher(g.value) {
				return nil, "", nil
			}
		}
		if len(groups) > 1 {
			return groups, prefix, nil
		}
		if !groups[0].prefix || step == maxPrefixSteps {
			return nil, "", nil
		}
		prefix, groups = groups[0].value, nil
	}
}

// longestSharedPrefix finds the longest prefix of the sample's common prefix, past prefix, that
// every series of selector extends, and returns its groups. The common prefix of all values is a
// prefix of the sample's, and a prefix that holds implies that every shorter one holds, so a
// binary search over its rune boundaries finds it. It returns nil groups when none past prefix holds.
func (c *OtelMetricsClient) longestSharedPrefix(ctx context.Context, selector, label, prefix, evalTime string, want int, sample []MetricResult) ([]shardGroup, string, error) {
	common := commonPrefix(sample, label, prefix)
	var ends []int // byte lengths of the candidate prefixes, shortest first
	for i := range common[len(prefix):] {
		if n := len(prefix) + i; n > len(prefix) && fitsMatcher(common[:n]) {
			ends = append(ends, n)
		}
	}
	if len(common) > len(prefix) && fitsMatcher(common) {
		ends = append(ends, len(common))
	}
	holds := func(n int) ([]shardGroup, error) {
		g, err := c.countByPrefix(ctx, selector, label, common[:n], evalTime)
		if errors.Is(err, ErrTruncated) {
			return nil, nil
		}
		if err != nil || groupTotal(g) != want || hasExact(g) {
			return nil, err
		}
		return g, nil
	}
	var best []shardGroup
	bestLen := len(prefix)
	// The sample's whole common prefix usually holds, so try it first.
	lo, hi := 0, len(ends)-1
	for first := true; lo <= hi; first = false {
		mid := (lo + hi + 1) / 2
		if first {
			mid = hi
		}
		g, err := holds(ends[mid])
		if err != nil {
			return nil, "", err
		}
		if g != nil {
			best, bestLen, lo = g, ends[mid], mid+1
		} else {
			hi = mid - 1
		}
	}
	return best, common[:bestLen], nil
}

// countByPrefix returns the series count per value prefix of label one character longer than
// prefix. Series whose value does not match keep no prefix label and form the non-prefix group.
func (c *OtelMetricsClient) countByPrefix(ctx context.Context, selector, label, prefix, evalTime string) ([]shardGroup, error) {
	promql := fmt.Sprintf(`count by (%s)(label_replace(%s, %s, "$1", %s, %s))`,
		quoteLabel(prefixLabel), selector, quoteLabel(prefixLabel), quoteLabel(label),
		quoteLabel("("+regexp.QuoteMeta(prefix)+".).*"))
	results, truncated, err := c.queryAt(ctx, promql, evalTime)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf("%w: too many groups (query %s)", ErrTruncated, promql)
	}
	groups := make([]shardGroup, 0, len(results))
	for _, r := range results {
		g := shardGroup{value: prefix, count: int(r.Value)}
		if v, ok := r.Labels.AllLabels()[prefixLabel]; ok && v != "" {
			g = shardGroup{value: v, count: int(r.Value), prefix: true}
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func hasExact(groups []shardGroup) bool {
	for _, g := range groups {
		if !g.prefix {
			return true
		}
	}
	return false
}

// fitsMatcher reports whether a prefix shard's regex for prefix stays within maxMatcherValueLen.
func fitsMatcher(prefix string) bool {
	return len(prefixPattern(prefix)) <= maxMatcherValueLen
}

func prefixPattern(prefix string) string {
	return regexp.QuoteMeta(prefix) + ".*"
}

// commonPrefix returns the longest prefix of label shared by every sampled value that starts with
// prefix, or prefix when there are none. It is only a guess, which prefixGroups checks.
func commonPrefix(sample []MetricResult, label, prefix string) string {
	common, found := "", false
	for _, r := range sample {
		v, ok := r.Labels.AllLabels()[label]
		if !ok || !strings.HasPrefix(v, prefix) {
			continue
		}
		if !found {
			common, found = v, true
			continue
		}
		n := 0
		for n < len(common) && n < len(v) && common[n] == v[n] {
			n++
		}
		common = common[:n]
	}
	if !found {
		return prefix
	}
	// Do not cut a multi-byte character in half.
	for len(common) > len(prefix) && !utf8.ValidString(common) {
		common = common[:len(common)-1]
	}
	return common
}

func (c *OtelMetricsClient) fetchShards(ctx context.Context, selector, evalTime, label string, shards []shard, pinned map[string]bool, prefixes map[string]string, depth int) ([]MetricResult, error) {
	type outcome struct {
		results []MetricResult
		err     error
	}
	outcomes := make([]outcome, len(shards))
	sem := make(chan struct{}, shardConcurrency)
	var wg sync.WaitGroup
	for i := range shards {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sh := shards[i]
			sub := withMatcher(selector, sh.matcher)
			results, _, err := c.queryAt(ctx, sub, evalTime)
			if err == nil && len(results) < sh.want {
				subPinned, subPrefixes := pinned, prefixes
				if sh.exact {
					subPinned = withPinned(pinned, label)
				} else if sh.prefix != "" {
					subPrefixes = withPrefix(prefixes, label, sh.prefix)
				}
				results, err = c.split(ctx, sub, evalTime, sh.want, results, subPinned, subPrefixes, depth+1)
			}
			outcomes[i] = outcome{results: results, err: err}
		}(i)
	}
	wg.Wait()

	var all []MetricResult
	for _, o := range outcomes {
		if o.err != nil {
			return nil, o.err
		}
		all = append(all, o.results...)
	}
	return all, nil
}

// shardTarget is the series budget per shard: 80% of the cap, since stale series also count
// against it, halved at each level so a re-split always yields smaller shards.
func (c *OtelMetricsClient) shardTarget(depth int) int {
	limit := c.seriesLimit
	if limit <= 0 {
		limit = defaultSeriesLimit
	}
	return (limit * 4 / 5) >> (depth - 1)
}

// splitCandidates orders the labels worth splitting on: node name first, then the sample's
// labels by number of distinct values. Labels already pinned to one value are skipped.
// split counts the first maxSplitCandidates by value and the rest only by value prefix.
func splitCandidates(sample []MetricResult, pinned map[string]bool) []string {
	distinct := make(map[string]map[string]struct{})
	for _, r := range sample {
		for k, v := range r.Labels.AllLabels() {
			if distinct[k] == nil {
				distinct[k] = make(map[string]struct{})
			}
			distinct[k][v] = struct{}{}
		}
	}
	var out []string
	if !pinned[nodeNameLabel] && (len(sample) == 0 || distinct[nodeNameLabel] != nil) {
		out = append(out, nodeNameLabel)
	}
	var keys []string
	for k, values := range distinct {
		if k != nodeNameLabel && !pinned[k] && len(values) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if ni, nj := len(distinct[keys[i]]), len(distinct[keys[j]]); ni != nj {
			return ni > nj
		}
		return keys[i] < keys[j]
	})
	return append(out, keys...)
}

// packShards bins label values (or value prefixes) into shards of at most target series. The
// absent value gets its own !~".+" shard because ="" does not match a missing label on this endpoint.
func packShards(label string, groups []shardGroup, target int) []shard {
	if target < 1 {
		target = 1
	}
	sorted := append([]shardGroup(nil), groups...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].value < sorted[j].value
	})

	var out []shard
	type bin struct {
		groups      []shardGroup
		want, chars int
	}
	var bins []bin
	for _, g := range sorted {
		if g.value == "" && !g.prefix {
			out = append(out, shard{matcher: quoteLabel(label) + `!~".+"`, want: g.count, exact: true})
			continue
		}
		chars := len(groupPattern(g))
		placed := false
		for i := range bins {
			if bins[i].want+g.count <= target && bins[i].chars+1+chars <= maxMatcherValueLen {
				bins[i].groups = append(bins[i].groups, g)
				bins[i].want += g.count
				bins[i].chars += 1 + chars
				placed = true
				break
			}
		}
		if !placed {
			bins = append(bins, bin{groups: []shardGroup{g}, want: g.count, chars: chars})
		}
	}
	for _, b := range bins {
		sh := shard{matcher: binMatcher(label, b.groups), want: b.want}
		if len(b.groups) == 1 {
			if b.groups[0].prefix {
				sh.prefix = b.groups[0].value
			} else {
				sh.exact = true
			}
		}
		out = append(out, sh)
	}
	return out
}

// groupPattern is the regex alternative that matches a group's values.
func groupPattern(g shardGroup) string {
	if g.prefix {
		return prefixPattern(g.value)
	}
	return regexp.QuoteMeta(g.value)
}

// binMatcher matches every value of the groups in a bin: = for one exact value, else a regex.
func binMatcher(label string, groups []shardGroup) string {
	if len(groups) == 1 && !groups[0].prefix {
		return fmt.Sprintf(`%s="%s"`, quoteLabel(label), promqlEscaper.Replace(groups[0].value))
	}
	alts := make([]string, len(groups))
	for i, g := range groups {
		alts[i] = groupPattern(g)
	}
	sort.Strings(alts)
	return fmt.Sprintf(`%s=~"%s"`, quoteLabel(label), promqlEscaper.Replace(strings.Join(alts, "|")))
}

func quoteLabel(label string) string {
	return `"` + promqlEscaper.Replace(label) + `"`
}

func withPrefix(prefixes map[string]string, label, prefix string) map[string]string {
	out := make(map[string]string, len(prefixes)+1)
	for k, v := range prefixes {
		out[k] = v
	}
	out[label] = prefix
	return out
}

func withPinned(pinned map[string]bool, label string) map[string]bool {
	out := make(map[string]bool, len(pinned)+1)
	for k := range pinned {
		out[k] = true
	}
	out[label] = true
	return out
}

// withMatcher adds one label matcher to a vector selector.
func withMatcher(selector, matcher string) string {
	s := strings.TrimSpace(selector)
	if !strings.HasSuffix(s, "}") {
		return s + "{" + matcher + "}"
	}
	body := strings.TrimSpace(s[:len(s)-1])
	if strings.HasSuffix(body, "{") || strings.HasSuffix(body, ",") {
		return body + matcher + "}"
	}
	return body + "," + matcher + "}"
}

// isVectorSelector reports whether s is a single instant vector selector (name, name{...}
// or {...}) with no range, offset, function or operator around it.
func isVectorSelector(s string) bool {
	s = strings.TrimSpace(s)
	open := strings.IndexByte(s, '{')
	if open < 0 {
		return metricNameRE.MatchString(s)
	}
	if name := strings.TrimSpace(s[:open]); name != "" && !metricNameRE.MatchString(name) {
		return false
	}
	var quote byte
	for i := open + 1; i < len(s); i++ {
		ch := s[i]
		switch {
		case quote != 0 && ch == '\\' && quote != '`':
			i++
		case quote != 0 && ch == quote:
			quote = 0
		case quote != 0:
		case ch == '"' || ch == '\'' || ch == '`':
			quote = ch
		case ch == '}':
			return i == len(s)-1
		}
	}
	return false
}
