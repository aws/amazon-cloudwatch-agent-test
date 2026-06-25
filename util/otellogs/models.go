// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otellogs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LogResult represents a single OTLP log record parsed from CW Logs.
type LogResult struct {
	Resource             map[string]string
	Scope                map[string]string
	Attributes           map[string]string
	Body                 string
	SeverityText         string
	SeverityNumber       string
	TimeUnixNano         string
	ObservedTimeUnixNano string
	TraceID              string
	SpanID               string
}

// SetField routes a Logs Insights result field into the appropriate LogResult
// field based on its dot-path prefix.
func (lr *LogResult) SetField(field, value string) {
	switch {
	case field == "body":
		lr.Body = value
	case field == "severityText":
		lr.SeverityText = value
	case field == "severityNumber":
		lr.SeverityNumber = value
	case field == "timeUnixNano":
		lr.TimeUnixNano = value
	case field == "observedTimeUnixNano":
		lr.ObservedTimeUnixNano = value
	case field == "traceId":
		lr.TraceID = value
	case field == "spanId":
		lr.SpanID = value
	case strings.HasPrefix(field, "resource.attributes."):
		lr.Resource[strings.TrimPrefix(field, "resource.attributes.")] = value
	case strings.HasPrefix(field, "scope.attributes."):
		lr.Scope[strings.TrimPrefix(field, "scope.attributes.")] = value
	case strings.HasPrefix(field, "attributes."):
		lr.Attributes[strings.TrimPrefix(field, "attributes.")] = value
	}
}

// HasResource returns true if the resource attribute key exists and is non-empty.
func (lr *LogResult) HasResource(key string) bool {
	v, ok := lr.Resource[key]
	return ok && v != ""
}

// HasScope returns true if the scope attribute key exists and is non-empty.
func (lr *LogResult) HasScope(key string) bool {
	v, ok := lr.Scope[key]
	return ok && v != ""
}

// otlpLogJSON mirrors the JSON structure of an OTLP log record as stored in
// CloudWatch Logs when ingested via the OTLP endpoint.
type otlpLogJSON struct {
	Resource struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"resource"`
	Scope struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"scope"`
	Body                 string         `json:"body"`
	Attributes           map[string]any `json:"attributes"`
	SeverityText         string         `json:"severityText"`
	SeverityNumber       json.Number    `json:"severityNumber"`
	TimeUnixNano         json.Number    `json:"timeUnixNano"`
	ObservedTimeUnixNano json.Number    `json:"observedTimeUnixNano"`
	TraceID              string         `json:"traceId"`
	SpanID               string         `json:"spanId"`
}

// ParseOTLPLogJSON parses a raw OTLP log JSON string into a LogResult.
func ParseOTLPLogJSON(raw string) (LogResult, error) {
	var j otlpLogJSON
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return LogResult{}, fmt.Errorf("parsing OTLP log JSON: %w", err)
	}
	lr := LogResult{
		Resource:             toStringMap(j.Resource.Attributes),
		Scope:                toStringMap(j.Scope.Attributes),
		Attributes:           toStringMap(j.Attributes),
		Body:                 j.Body,
		SeverityText:         j.SeverityText,
		SeverityNumber:       j.SeverityNumber.String(),
		TimeUnixNano:         j.TimeUnixNano.String(),
		ObservedTimeUnixNano: j.ObservedTimeUnixNano.String(),
		TraceID:              j.TraceID,
		SpanID:               j.SpanID,
	}
	if lr.Resource == nil {
		lr.Resource = make(map[string]string)
	}
	if lr.Scope == nil {
		lr.Scope = make(map[string]string)
	}
	if lr.Attributes == nil {
		lr.Attributes = make(map[string]string)
	}
	return lr, nil
}

func toStringMap(m map[string]any) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}
