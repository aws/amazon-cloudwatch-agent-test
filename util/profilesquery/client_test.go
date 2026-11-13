// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package profilesquery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCreds() aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret", SessionToken: "token"}, nil
	})
}

func newTestClient(url string) *Client {
	c := NewClientWithEndpoint(url, "us-west-2", testCreds())
	c.backoff = time.Millisecond
	return c
}

func TestSelectMergeStacktracesRequestAndDecode(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/querier.v1.QuerierService/SelectMergeStacktraces", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		auth := r.Header.Get("Authorization")
		assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 "), auth)
		assert.Contains(t, auth, "/us-west-2/monitoring/aws4_request")
		assert.Equal(t, "token", r.Header.Get("X-Amz-Security-Token"))
		body, _ := io.ReadAll(r.Body)
		assert.NoError(t, json.Unmarshal(body, &got))
		_, _ = io.WriteString(w, `{"flamegraph":{"names":["total","main","work"],"levels":[{"values":["0","300","0","0"]},{"values":[0,300,100,1]}],"total":"300","maxSelf":"200"}}`)
	}))
	defer srv.Close()

	start := time.UnixMilli(1000)
	end := time.UnixMilli(61000)
	fg, err := newTestClient(srv.URL).SelectMergeStacktraces(context.Background(), "wall:wall:nanoseconds:wall:nanoseconds", `{service_name="svc"}`, start, end)
	require.NoError(t, err)
	assert.Equal(t, Int64(300), fg.Total)
	assert.Equal(t, Int64(200), fg.MaxSelf)
	assert.Equal(t, 2, fg.Frames())
	require.Len(t, fg.Levels, 2)
	assert.Equal(t, []Int64{0, 300, 100, 1}, fg.Levels[1].Values)

	assert.Equal(t, "wall:wall:nanoseconds:wall:nanoseconds", got["profileTypeID"])
	assert.Equal(t, `{service_name="svc"}`, got["labelSelector"])
	assert.Equal(t, float64(1000), got["start"])
	assert.Equal(t, float64(61000), got["end"])
}

func TestSelectMergeStacktracesWithoutFlamegraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	fg, err := newTestClient(srv.URL).SelectMergeStacktraces(context.Background(), "t", "{}", time.Now(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, Int64(0), fg.Total)
	assert.Equal(t, 0, fg.Frames())
}

func TestLabelValuesAndNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		body, _ := io.ReadAll(r.Body)
		assert.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, []any{`{service_name="svc"}`}, req["matchers"])
		switch r.URL.Path {
		case "/querier.v1.QuerierService/LabelValues":
			assert.Equal(t, ProfileTypeLabel, req["name"])
			_, _ = io.WriteString(w, `{"names":["wall:wall:nanoseconds:wall:nanoseconds"]}`)
		case "/querier.v1.QuerierService/LabelNames":
			_, _ = io.WriteString(w, `{"names":["@sample.thread.state","service_name"]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	values, err := c.LabelValues(context.Background(), ProfileTypeLabel, []string{`{service_name="svc"}`}, time.Now(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, []string{"wall:wall:nanoseconds:wall:nanoseconds"}, values)
	names, err := c.LabelNames(context.Background(), []string{`{service_name="svc"}`}, time.Now(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, []string{"@sample.thread.state", "service_name"}, names)
}

func TestRetriesServerErrorsAndThrottling(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			_, _ = io.WriteString(w, `{"names":["RUNNABLE"]}`)
		}
	}))
	defer srv.Close()
	values, err := newTestClient(srv.URL).LabelValues(context.Background(), "@sample.thread.state", nil, time.Now(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, []string{"RUNNABLE"}, values)
	assert.Equal(t, int32(3), calls.Load())
}

func TestDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"denied"}`)
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).LabelValues(context.Background(), "x", nil, time.Now(), time.Now())
	var reqErr *RequestError
	require.ErrorAs(t, err, &reqErr)
	assert.Equal(t, http.StatusForbidden, reqErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 403")
	assert.Equal(t, int32(1), calls.Load())
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).LabelNames(context.Background(), nil, time.Now(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 3 attempts failed")
	var reqErr *RequestError
	assert.False(t, errors.As(err, &reqErr), "a server error is not a RequestError")
	assert.Equal(t, int32(3), calls.Load())
}

func TestInt64Decode(t *testing.T) {
	var v struct {
		A Int64 `json:"a"`
		B Int64 `json:"b"`
		C Int64 `json:"c"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"a":"8692757040","b":12,"c":""}`), &v))
	assert.Equal(t, Int64(8692757040), v.A)
	assert.Equal(t, Int64(12), v.B)
	assert.Equal(t, Int64(0), v.C)
	assert.Error(t, json.Unmarshal([]byte(`{"a":"x"}`), &v))
}
