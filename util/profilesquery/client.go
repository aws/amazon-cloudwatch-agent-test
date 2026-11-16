// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package profilesquery reads profiles back from CloudWatch through its
// Pyroscope-compatible query API (querier.v1.QuerierService), signed with
// SigV4 for the monitoring service.
package profilesquery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const (
	signingService = "monitoring"
	querierPath    = "/querier.v1.QuerierService/"
	// ProfileTypeLabel is the label whose values are a service's profile type IDs.
	ProfileTypeLabel = "__profile_type__"
)

// Client queries CloudWatch profiles.
type Client struct {
	httpClient *http.Client
	signer     *v4.Signer
	creds      aws.CredentialsProvider
	endpoint   string
	region     string
	maxRetries int
	backoff    time.Duration
}

// NewClient returns a client for https://monitoring.<region>.amazonaws.com.
func NewClient(region string, creds aws.CredentialsProvider) *Client {
	return NewClientWithEndpoint(fmt.Sprintf("https://monitoring.%s.amazonaws.com", region), region, creds)
}

// NewClientWithEndpoint returns a client for the given endpoint, for tests.
func NewClientWithEndpoint(endpoint, region string, creds aws.CredentialsProvider) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		signer:     v4.NewSigner(),
		creds:      creds,
		endpoint:   endpoint,
		region:     region,
		maxRetries: 3,
		backoff:    time.Second,
	}
}

// Flamegraph is the merged stack tree SelectMergeStacktraces returns. Each
// level holds groups of four numbers per node (offset, total, self, name index).
type Flamegraph struct {
	Names  []string `json:"names"`
	Levels []struct {
		Values []Int64 `json:"values"`
	} `json:"levels"`
	Total   Int64 `json:"total"`
	MaxSelf Int64 `json:"maxSelf"`
}

// Frames is the number of distinct frame names, without the "total" root.
func (f *Flamegraph) Frames() int {
	if len(f.Names) == 0 {
		return 0
	}
	return len(f.Names) - 1
}

// RequestError is a response that retrying the same request cannot change:
// any 4xx other than 429, such as a bad selector (400) or a missing IAM
// permission (403).
type RequestError struct {
	Op         string
	StatusCode int
	Body       string
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Op, e.StatusCode, e.Body)
}

// Int64 decodes the protobuf JSON form of an int64, which is a string, and
// also accepts a bare number.
type Int64 int64

func (i *Int64) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*i = 0
			return nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*i = Int64(v)
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*i = Int64(v)
	return nil
}

// LabelValues returns the values of label name for the series matching
// matchers (Prometheus-style selectors such as {service_name="svc"}) in [start, end].
func (c *Client) LabelValues(ctx context.Context, name string, matchers []string, start, end time.Time) ([]string, error) {
	req := map[string]any{"name": name, "matchers": matchers, "start": start.UnixMilli(), "end": end.UnixMilli()}
	var resp struct {
		Names []string `json:"names"`
	}
	if err := c.call(ctx, "LabelValues", req, &resp); err != nil {
		return nil, err
	}
	return resp.Names, nil
}

// LabelNames returns the label names of the series matching matchers in [start, end].
func (c *Client) LabelNames(ctx context.Context, matchers []string, start, end time.Time) ([]string, error) {
	req := map[string]any{"matchers": matchers, "start": start.UnixMilli(), "end": end.UnixMilli()}
	var resp struct {
		Names []string `json:"names"`
	}
	if err := c.call(ctx, "LabelNames", req, &resp); err != nil {
		return nil, err
	}
	return resp.Names, nil
}

// SelectMergeStacktraces returns the merged flame graph of profileTypeID for
// the profiles matching labelSelector in [start, end].
func (c *Client) SelectMergeStacktraces(ctx context.Context, profileTypeID, labelSelector string, start, end time.Time) (*Flamegraph, error) {
	req := map[string]any{
		"profileTypeID": profileTypeID,
		"labelSelector": labelSelector,
		"start":         start.UnixMilli(),
		"end":           end.UnixMilli(),
		"maxNodes":      "16384",
	}
	var resp struct {
		Flamegraph *Flamegraph `json:"flamegraph"`
	}
	if err := c.call(ctx, "SelectMergeStacktraces", req, &resp); err != nil {
		return nil, err
	}
	if resp.Flamegraph == nil {
		return &Flamegraph{}, nil
	}
	return resp.Flamegraph, nil
}

func (c *Client) call(ctx context.Context, op string, reqBody, out any) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encoding %s request: %w", op, err)
	}
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	url := c.endpoint + querierPath + op

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.backoff << (attempt - 1)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating %s request: %w", op, err)
		}
		req.Header.Set("Content-Type", "application/json")
		creds, err := c.creds.Retrieve(ctx)
		if err != nil {
			return fmt.Errorf("retrieving credentials: %w", err)
		}
		if err := c.signer.SignHTTP(ctx, creds, req, payloadHash, signingService, c.region, time.Now()); err != nil {
			return fmt.Errorf("signing %s request: %w", op, err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("profiles %s request failed attempt=%d error=%v", op, attempt+1, err)
			continue
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("reading %s response: %w", op, readErr)
		}
		// 429 is throttling, not a bad query: retry it like a server error.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("%s: HTTP %d: %s", op, resp.StatusCode, truncate(respBody, 200))
			log.Printf("profiles %s server error, retrying status=%d attempt=%d", op, resp.StatusCode, attempt+1)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return &RequestError{Op: op, StatusCode: resp.StatusCode, Body: truncate(respBody, 200)}
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("parsing %s response: %w", op, err)
		}
		return nil
	}
	return fmt.Errorf("%s: all %d attempts failed: %w", op, c.maxRetries, lastErr)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
