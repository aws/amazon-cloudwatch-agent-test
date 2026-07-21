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

// SendOTLPMetrics pushes OTLP metrics to the agent's OTLP/HTTP receiver until the duration elapses (cross-platform).
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

// buildOTLPMetricsPayload builds an OTLP/HTTP JSON metrics payload with an InstanceId attribute for isolation.
func buildOTLPMetricsPayload(instanceID string) []byte {
	now := time.Now().UnixNano()
	return []byte(fmt.Sprintf(`{
  "resourceMetrics": [{
    "resource": {"attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]},
    "scopeMetrics": [{
      "metrics": [
        {
          "name": "otlp_test_counter",
          "sum": {
            "dataPoints": [{"asInt": "1", "timeUnixNano": "%d", "attributes": [{"key": "InstanceId", "value": {"stringValue": "%s"}}]}],
            "isMonotonic": true,
            "aggregationTemporality": 2
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
}`, instanceID, now, instanceID, now, instanceID))
}

// StartPrometheusFakeServer serves static Prometheus metrics on the port until stop() is called (cross-platform).
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
