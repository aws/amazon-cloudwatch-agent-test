// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// DefaultOTLPHTTPEndpoint is the agent's default OTLP/HTTP receiver endpoint.
const DefaultOTLPHTTPEndpoint = "http://127.0.0.1:4318"

// WaitForOTLPEndpoint polls the OTLP HTTP endpoint until it responds or timeout elapses.
// Use after StartAgent instead of a fixed sleep.
func WaitForOTLPEndpoint(endpoint string, timeout time.Duration) error {
	if endpoint == "" {
		endpoint = DefaultOTLPHTTPEndpoint
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("OTLP endpoint %s not ready after %s", endpoint, timeout)
}

// SendOTLPMetrics pushes OTLP metrics to the agent's HTTP receiver until duration elapses.
func SendOTLPMetrics(endpoint, instanceID string, sendingInterval, duration time.Duration) error {
	if endpoint == "" {
		endpoint = DefaultOTLPHTTPEndpoint
	}
	deadline := time.Now().Add(duration)
	ticker := time.NewTicker(sendingInterval)
	defer ticker.Stop()

	for {
		payload := buildOTLPMetricsPayload(instanceID)
		req, err := http.NewRequest(http.MethodPost, endpoint+"/v1/metrics", bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to build OTLP metrics request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := http.DefaultClient.Do(req); err != nil {
			log.Printf("SendOTLPMetrics: post failed: %v", err)
		} else {
			resp.Body.Close()
		}

		if time.Now().After(deadline) {
			return nil
		}
		<-ticker.C
	}
}

// buildOTLPMetricsPayload builds a delta counter + gauge payload tagged with instanceID.
func buildOTLPMetricsPayload(instanceID string) []byte {
	now := time.Now().UnixNano()
	start := now - int64(10*time.Second)
	return []byte(fmt.Sprintf(`{
  "resourceMetrics": [{
    "resource": {"attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]},
    "scopeMetrics": [{
      "metrics": [
        {
          "name": "otlp_test_counter",
          "sum": {
            "dataPoints": [{"asInt": "1", "startTimeUnixNano": "%d", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}],
            "isMonotonic": true,
            "aggregationTemporality": 1
          }
        },
        {
          "name": "otlp_test_gauge",
          "gauge": {
            "dataPoints": [{"asDouble": 42.0, "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}]
          }
        }
      ]
    }]
  }]
}`, instanceID, start, now, instanceID, now, instanceID))
}

// StartPrometheusFakeServer serves static Prometheus metrics on the given port until stop() is called.
func StartPrometheusFakeServer(port int, exposition string) (stop func(), err error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(exposition))
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	srv := &http.Server{Handler: mux}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("StartPrometheusFakeServer: serve stopped: %v", serveErr)
		}
	}()

	return func() { _ = srv.Close() }, nil
}
