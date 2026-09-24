// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpvalidation

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

// SpanRecord is the subset of an aws/spans (Transaction Search) record that trace validation asserts on.
type SpanRecord struct {
	TraceID    string         `json:"traceId"`
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Attributes map[string]any `json:"attributes"`
	Scope      struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"scope"`
	Resource struct {
		Attributes map[string]any `json:"attributes"`
	} `json:"resource"`
}

// AssertSpanContent asserts a span's name and kind, its span-level and resource attributes against the
// given expectations, and the CloudWatch instrumentation-scope markers (see AssertAttributes for the
// exact/PresenceOnly semantics).
func AssertSpanContent(s SpanRecord, wantName, wantKind string, attrs, resource map[string]string) error {
	if s.Name != wantName {
		return fmt.Errorf("span %s: name = %q, want %q", s.TraceID, s.Name, wantName)
	}
	if s.Kind != wantKind {
		return fmt.Errorf("span %s: kind = %q, want %q", s.TraceID, s.Kind, wantKind)
	}
	if err := AssertAttributes("span "+s.TraceID+" attributes", attrs, s.Attributes); err != nil {
		return err
	}
	if err := AssertAttributes("span "+s.TraceID+" scope", ScopeExpectations(), s.Scope.Attributes); err != nil {
		return err
	}
	return AssertAttributes("span "+s.TraceID+" resource", resource, s.Resource.Attributes)
}

// ValidateOtlpTraces confirms every trace ID reached the Transaction Search log group, then runs spanCheck on each
// parsed span. Delivery is retried for ingestion lag. A failure from spanCheck is final, and spanCheck may be nil.
func ValidateOtlpTraces(testName, spansLogGroup string, traceIDs []string, spanCheck func(SpanRecord) error) status.TestResult {
	testResult := status.TestResult{Name: testName, Status: status.FAILED}
	if len(traceIDs) == 0 {
		testResult.Reason = fmt.Errorf("no trace IDs were generated during the load window")
		return testResult
	}

	quoted := make([]string, len(traceIDs))
	for i, id := range traceIDs {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	// Pull the full span record (@message), not just traceId, so spanCheck can assert span content.
	query := fmt.Sprintf("fields @message | filter traceId in [%s]", strings.Join(quoted, ", "))
	log.Printf("[%s] expecting %d trace IDs in %s (sample: %s)", testName, len(traceIDs), spansLogGroup, traceIDs[0])

	const maxRetries = 5
	const retryInterval = 60 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Generous lower bound: trace IDs are unique per run, so a wide window only affects the query cost.
		since := time.Now().Add(-30 * time.Minute)
		rows, err := awsservice.GetLogQueryResults(spansLogGroup, since.Unix(), time.Now().Unix(), query)
		if err != nil {
			testResult.Reason = fmt.Errorf("attempt %d: %s query failed (is Transaction Search enabled in the account?): %w",
				attempt, spansLogGroup, err)
		} else {
			spans := make(map[string]SpanRecord, len(rows))
			for _, row := range rows {
				for _, field := range row {
					if aws.ToString(field.Field) != "@message" {
						continue
					}
					var s SpanRecord
					if uerr := json.Unmarshal([]byte(aws.ToString(field.Value)), &s); uerr != nil {
						log.Printf("[%s] skipping unparseable span record: %v", testName, uerr)
						continue
					}
					spans[s.TraceID] = s
				}
			}
			var missing []string
			for _, id := range traceIDs {
				if _, ok := spans[id]; !ok {
					missing = append(missing, id)
				}
			}
			if len(missing) == 0 {
				if spanCheck != nil {
					for _, id := range traceIDs {
						if err = spanCheck(spans[id]); err != nil {
							testResult.Reason = err
							return testResult
						}
					}
				}
				log.Printf("[%s] attempt %d: all %d traces delivered with expected content", testName, attempt, len(traceIDs))
				testResult.Status = status.SUCCESSFUL
				return testResult
			}
			testResult.Reason = fmt.Errorf("attempt %d: %d/%d traces missing from %s (first missing: %s)",
				attempt, len(missing), len(traceIDs), spansLogGroup, missing[0])
		}
		if attempt < maxRetries {
			log.Printf("[%s] %v — retrying in %v", testName, testResult.Reason, retryInterval)
			time.Sleep(retryInterval)
		}
	}
	return testResult
}
