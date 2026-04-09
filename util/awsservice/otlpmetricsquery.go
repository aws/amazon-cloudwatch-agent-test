// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
)

type PrometheusResponse struct {
	Status    string                 `json:"status"`
	Data      PrometheusResponseData `json:"data"`
	Error     string                 `json:"error"`
	ErrorType string                 `json:"errorType"`
}

type PrometheusResponseData struct {
	ResultType string                 `json:"resultType"`
	Result     []PrometheusDataResult `json:"result"`
}

type PrometheusDataResult struct {
	Metric map[string]interface{} `json:"metric"`
	Value  []interface{}          `json:"value"`
}

func QueryOtlpMetrics(region string, promql string) (PrometheusResponse, error) {
	ctx := context.Background()
	endpoint := fmt.Sprintf("https://monitoring.%s.amazonaws.com/api/v1/query", region)
	body := url.Values{"query": {promql}}.Encode()
	bodyBytes := []byte(body)

	h := sha256.Sum256(bodyBytes)
	payloadHash := hex.EncodeToString(h[:])

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, creds, req, payloadHash, "monitoring", region, time.Now())
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to sign request: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer res.Body.Close()

	respBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var resp PrometheusResponse
	err = json.Unmarshal(respBytes, &resp)
	if err != nil {
		return PrometheusResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return resp, nil
}

func QueryOtlpMetricsWithRetry(region string, promql string, retries int, retryInterval time.Duration) (PrometheusResponse, error) {
	var lastErr error
	for i := 0; i < retries; i++ {
		resp, err := QueryOtlpMetrics(region, promql)
		if err == nil && resp.Status == "success" && len(resp.Data.Result) > 0 {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("otlp query unsuccessful: status=%s, results=%d, error=%s", resp.Status, len(resp.Data.Result), resp.Error)
		}
		time.Sleep(retryInterval)
	}
	return PrometheusResponse{}, fmt.Errorf("otlp query failed after %d retries: %w", retries, lastErr)
}
