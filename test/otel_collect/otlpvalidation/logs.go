// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpvalidation

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// LogRecord is the subset of a stored OTLP log event (the CloudWatch Logs message JSON) validated here.
type LogRecord struct {
	Body         string         `json:"body"`
	SeverityText string         `json:"severityText"`
	Attributes   map[string]any `json:"attributes"`
	Scope        struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"scope"`
	Resource struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"resource"`
}

// AssertLogRecord adapts a LogRecord check into an awsservice per-log validator: it parses each event's
// message as an OTLP LogRecord and runs check against it.
func AssertLogRecord(check func(LogRecord) error) awsservice.LogEventValidator {
	return func(event types.OutputLogEvent) error {
		var rec LogRecord
		if err := json.Unmarshal([]byte(aws.ToString(event.Message)), &rec); err != nil {
			return fmt.Errorf("unparseable OTLP log record: %w", err)
		}
		return check(rec)
	}
}

// AssertLogContent asserts a log record's body and severity, plus its resource attributes against the
// given expectations (see AssertAttributes for the exact/PresenceOnly semantics).
func AssertLogContent(rec LogRecord, wantBody, wantSeverity string, resource map[string]string) error {
	if rec.Body != wantBody {
		return fmt.Errorf("log body = %q, want %q", rec.Body, wantBody)
	}
	if rec.SeverityText != wantSeverity {
		return fmt.Errorf("log severityText = %q, want %q", rec.SeverityText, wantSeverity)
	}
	if err := AssertAttributes("log scope", ScopeExpectations(), rec.Scope.Attributes); err != nil {
		return err
	}
	return AssertAttributes("log", resource, rec.Resource.Attributes)
}
