// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package database_insights_plans

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseTopQuery(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		want         topQuery
		wantTopQuery bool
	}{
		{
			name:    "top query with a plan",
			message: `{"attributes":{"db.namespace":"testdb","db.query.text":"UPDATE pgbench_accounts SET abalance = abalance + ? WHERE aid = ?","postgresql.queryid":"42","postgresql.query_plan":"[{\"Plan\":{\"Node Type\":\"Index Scan\"}}]"},"eventName":"db.server.top_query"}`,
			want: topQuery{
				database: "testdb",
				text:     "UPDATE pgbench_accounts SET abalance = abalance + ? WHERE aid = ?",
				queryID:  "42",
				plan:     `[{"Plan":{"Node Type":"Index Scan"}}]`,
			},
			wantTopQuery: true,
		},
		{
			name:         "top query without a plan",
			message:      `{"attributes":{"postgresql.queryid":"1","postgresql.query_plan":""},"eventName":"db.server.top_query"}`,
			want:         topQuery{queryID: "1"},
			wantTopQuery: true,
		},
		{
			name:         "query ID that is a number",
			message:      `{"attributes":{"postgresql.queryid":7,"postgresql.query_plan":""}}`,
			want:         topQuery{queryID: "7"},
			wantTopQuery: true,
		},
		{
			name:         "query sample has no plan attribute",
			message:      `{"attributes":{"postgresql.query_id":"1","postgresql.state":"active"},"eventName":"db.server.query_sample"}`,
			wantTopQuery: false,
		},
		{
			name:         "not JSON",
			message:      `postgresql.query_plan`,
			wantTopQuery: false,
		},
		{
			name:         "plan attribute that is not a string",
			message:      `{"attributes":{"postgresql.query_plan":5}}`,
			want:         topQuery{},
			wantTopQuery: true,
		},
		{
			name:         "a record without the plan attribute is not a top query",
			message:      `{"attributes":{"db.namespace":"testdb"}}`,
			wantTopQuery: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, isTopQuery := parseTopQuery(tc.message)
			assert.Equal(t, tc.wantTopQuery, isTopQuery)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCountLines(t *testing.T) {
	out := `{"msg":"x","database":"testdb","a":1}
{"msg":"x","database":"testdb2"}
{"msg":"x","database":"testdb2","a":2}

`
	assert.Equal(t, 3, countLines(out, ""))
	assert.Equal(t, 1, countLines(out, "testdb"), "testdb must not match testdb2")
	assert.Equal(t, 2, countLines(out, "testdb2"))
	assert.Equal(t, 0, countLines("", ""))
}

func TestIsWorkloadQuery(t *testing.T) {
	assert.True(t, isWorkloadQuery("UPDATE pgbench_accounts SET abalance = abalance + ? WHERE aid = ?"))
	assert.True(t, isWorkloadQuery("INSERT INTO pgbench_history (tid, bid, aid, delta, mtime) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)"))
	// The receiver's own statements in the workload databases are not workload.
	assert.False(t, isWorkloadQuery("SELECT EXISTS (SELECT ? FROM pg_extension WHERE extname = ?)"))
	assert.False(t, isWorkloadQuery("SELECT plan FROM pg_store_plans WHERE queryid::text = ? ORDER BY calls DESC LIMIT ?"))
	assert.False(t, isWorkloadQuery(""))
}

func TestPlanCountsAdd(t *testing.T) {
	restart := time.Unix(1_700_000_000, 0)
	cutoff := restart.Add(time.Minute)
	workload := func(id, plan string) topQuery {
		return topQuery{database: "testdb", text: "UPDATE pgbench_tellers SET tbalance = tbalance + ? WHERE tid = ?", queryID: id, plan: plan}
	}
	other := func(plan string) topQuery {
		return topQuery{database: "testdb", text: "SELECT EXISTS (SELECT ? FROM pg_extension WHERE extname = ?)", queryID: "9", plan: plan}
	}
	planned := func(ids ...string) map[string]struct{} {
		set := map[string]struct{}{}
		for _, id := range ids {
			set[id] = struct{}{}
		}
		return set
	}

	tests := []struct {
		name string
		at   time.Time
		q    topQuery
		want planCounts
	}{
		{"workload without a plan before the restart", restart.Add(-time.Second), workload("1", ""), planCounts{emptyBefore: 1}},
		{"workload without a plan between the restart and the cut-off", restart.Add(time.Second), workload("1", ""), planCounts{emptyBefore: 1, emptyAfterRestart: 1}},
		{"workload with a plan before the cut-off", restart.Add(2 * time.Second), workload("1", "[{}]"), planCounts{plansBefore: 1}},
		{"other query with a plan before the cut-off still counts, none may have one", restart.Add(2 * time.Second), other("[{}]"), planCounts{plansBefore: 1}},
		{"other query without a plan does not count as an event", restart.Add(-time.Second), other(""), planCounts{}},
		{"other query without a plan after the restart does not count as an event", restart.Add(time.Second), other(""), planCounts{}},
		{"workload without a plan exactly at the cut-off counts as after and is ignored", cutoff, workload("1", ""), planCounts{}},
		{"workload with a plan exactly at the cut-off counts as after", cutoff, workload("1", "[{}]"), planCounts{plansAfter: 1, plannedQueries: planned("1")}},
		{"workload with a plan after the cut-off", cutoff.Add(time.Second), workload("2", "[{}]"), planCounts{plansAfter: 1, plannedQueries: planned("2")}},
		{"other query with a plan after the cut-off does not count", cutoff.Add(time.Second), other("[{}]"), planCounts{}},
		{"workload without a plan after the cut-off is not counted", cutoff.Add(time.Second), workload("1", ""), planCounts{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c planCounts
			c.add(tc.at, tc.q, restart, cutoff)
			assert.Equal(t, tc.want, c)
		})
	}
}

// TestPlanCountsAddDistinctQueries pins that plannedQueries counts queries, not events: the same
// query reported in every collection is one query.
func TestPlanCountsAddDistinctQueries(t *testing.T) {
	restart := time.Unix(1_700_000_000, 0)
	cutoff := restart.Add(time.Minute)
	var c planCounts
	for i, id := range []string{"1", "1", "1", "2", "3", "3"} {
		q := topQuery{text: "SELECT abalance FROM pgbench_accounts WHERE aid = ?", queryID: id, plan: "[{}]"}
		c.add(cutoff.Add(time.Duration(i)*time.Second), q, restart, cutoff)
	}
	assert.Equal(t, 6, c.plansAfter)
	assert.Len(t, c.plannedQueries, 3)
	assert.GreaterOrEqual(t, len(c.plannedQueries), minPlannedQueries)

	// Two queries is below the bar, however many events they produce.
	var few planCounts
	for i, id := range []string{"1", "2", "1", "2", "1", "2"} {
		q := topQuery{text: "SELECT abalance FROM pgbench_accounts WHERE aid = ?", queryID: id, plan: "[{}]"}
		few.add(cutoff.Add(time.Duration(i)*time.Second), q, restart, cutoff)
	}
	assert.Equal(t, 6, few.plansAfter)
	assert.Less(t, len(few.plannedQueries), minPlannedQueries)
}

// TestPlanCountsAddEqualCutoff pins the testdb2 case, where the cut-off is the restart
// itself: an event at that instant counts as after, so it never lands in plansBefore.
func TestPlanCountsAddEqualCutoff(t *testing.T) {
	at := time.Unix(1_700_000_000, 0)
	withPlan := topQuery{text: "UPDATE pgbench_branches SET bbalance = bbalance + ? WHERE bid = ?", queryID: "5", plan: "[{}]"}
	empty := topQuery{text: withPlan.text, queryID: "5"}

	var a, b planCounts
	a.add(at, withPlan, at, at)
	b.add(at, empty, at, at)
	assert.Equal(t, planCounts{plansAfter: 1, plannedQueries: map[string]struct{}{"5": {}}}, a)
	assert.Equal(t, planCounts{}, b)
}
