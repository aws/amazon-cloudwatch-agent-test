// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otellogs

import "strings"

// LogResult represents a single OTLP log record as returned by CW Logs Insights.
// Fields are populated from the Logs Insights query result fields, which use
// dot-path notation matching the stored OTLP JSON structure:
//
//	resource.attributes.<key>  → Resource map
//	scope.attributes.<key>     → Scope map
//	attributes.<key>           → Attributes map (log record attributes)
//	body                       → Body
//	severityText               → SeverityText
//	severityNumber             → SeverityNumber
//	timeUnixNano               → TimeUnixNano
//	observedTimeUnixNano       → ObservedTimeUnixNano
//	traceId                    → TraceID
//	spanId                     → SpanID
type LogResult struct {
	Resource           map[string]string
	Scope              map[string]string
	Attributes         map[string]string
	Body               string
	SeverityText       string
	SeverityNumber     string
	TimeUnixNano       string
	ObservedTimeUnixNano string
	TraceID            string
	SpanID             string
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
