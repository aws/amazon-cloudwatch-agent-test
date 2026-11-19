// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package database_insights_plans

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopQueryPlan(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		wantPlan     string
		wantTopQuery bool
	}{
		{
			name:         "top query with a plan",
			message:      `{"attributes":{"db.namespace":"testdb","postgresql.queryid":"1","postgresql.query_plan":"[{\"Plan\":{\"Node Type\":\"Index Scan\"}}]"},"eventName":"db.server.top_query"}`,
			wantPlan:     `[{"Plan":{"Node Type":"Index Scan"}}]`,
			wantTopQuery: true,
		},
		{
			name:         "top query without a plan",
			message:      `{"attributes":{"postgresql.queryid":"1","postgresql.query_plan":""},"eventName":"db.server.top_query"}`,
			wantPlan:     "",
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
			wantPlan:     "",
			wantTopQuery: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, isTopQuery := topQueryPlan(tc.message)
			assert.Equal(t, tc.wantTopQuery, isTopQuery)
			assert.Equal(t, tc.wantPlan, plan)
		})
	}
}

func TestTopQueryEventDatabase(t *testing.T) {
	database, plan, isTopQuery := topQueryEvent(`{"attributes":{"db.namespace":"testdb2","postgresql.query_plan":"[{}]"}}`)
	assert.True(t, isTopQuery)
	assert.Equal(t, "testdb2", database)
	assert.Equal(t, "[{}]", plan)

	database, plan, isTopQuery = topQueryEvent(`{"attributes":{"postgresql.query_plan":""}}`)
	assert.True(t, isTopQuery)
	assert.Empty(t, database)
	assert.Empty(t, plan)

	_, _, isTopQuery = topQueryEvent(`{"attributes":{"db.namespace":"testdb"}}`)
	assert.False(t, isTopQuery, "a record without the plan attribute is not a top query")
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
