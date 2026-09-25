// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otelconfig

import (
	"encoding/json"
	"testing"
)

func TestInjectHostID(t *testing.T) {
	t.Run("adds host.id when resource_attributes absent", func(t *testing.T) {
		in := []byte(`{"opentelemetry":{"cluster_name":"c","collect":{}}}`)
		out, err := injectHostID(in, "i-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		attrs := resourceAttrs(t, out)
		if attrs["host.id"] != "i-123" {
			t.Fatalf("host.id = %v, want i-123", attrs["host.id"])
		}
	})

	t.Run("preserves existing resource_attributes", func(t *testing.T) {
		in := []byte(`{"opentelemetry":{"resource_attributes":{"service.name":"svc"}}}`)
		out, err := injectHostID(in, "i-456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		attrs := resourceAttrs(t, out)
		if attrs["service.name"] != "svc" {
			t.Fatalf("service.name lost: %v", attrs["service.name"])
		}
		if attrs["host.id"] != "i-456" {
			t.Fatalf("host.id = %v, want i-456", attrs["host.id"])
		}
	})

	t.Run("errors when opentelemetry block missing", func(t *testing.T) {
		if _, err := injectHostID([]byte(`{"agent":{}}`), "i-789"); err == nil {
			t.Fatal("expected error for missing opentelemetry block")
		}
	})

	t.Run("errors on invalid JSON", func(t *testing.T) {
		if _, err := injectHostID([]byte(`not json`), "i-000"); err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func resourceAttrs(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	otel, ok := cfg["opentelemetry"].(map[string]any)
	if !ok {
		t.Fatal("opentelemetry block missing in output")
	}
	attrs, ok := otel["resource_attributes"].(map[string]any)
	if !ok {
		t.Fatal("resource_attributes missing in output")
	}
	return attrs
}
