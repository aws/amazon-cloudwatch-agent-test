// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpvalidation

import (
	"fmt"
	"regexp"
)

// PresenceOnly is the expected-value sentinel meaning "assert the attribute/label is present and
// non-empty, without checking a specific value". Recognized by ExpectedValue and AssertAttributes.
const PresenceOnly = ""

// CloudWatchSource is the instrumentation-scope marker the CloudWatch agent stamps on all OTLP telemetry
// (scope attribute cloudwatch.source, or the @instrumentation.cloudwatch.source metric label).
const CloudWatchSource = "cloudwatch-agent"

// ScopeExpectations are the instrumentation-scope attributes the CloudWatch agent stamps on all OTLP
// telemetry. On metrics, they surface as @instrumentation.* labels, on logs/traces as scope attributes.
func ScopeExpectations() map[string]string {
	return map[string]string{
		"cloudwatch.source":   CloudWatchSource,
		"cloudwatch.solution": PresenceOnly,
	}
}

// ExactMatch returns a PromQL label-matcher regex that matches v exactly (QuoteMeta escapes any
// metacharacters, the anchors force a full match).
func ExactMatch(v string) string { return "^" + regexp.QuoteMeta(v) + "$" }

// ExpectedValue returns an exact-match matcher when a value is known, else a presence matcher (".+")
// for the PresenceOnly sentinel.
func ExpectedValue(v string) string {
	if v == PresenceOnly {
		return ".+"
	}
	return ExactMatch(v)
}

// AssertAttributes checks each expected attribute against a parsed attribute map (resource or record
// attributes from any signal): an exact match when the expected value is set, else a presence check for
// the PresenceOnly sentinel. scope is a caller-supplied label for the error message (e.g. "span <id>").
func AssertAttributes(scope string, expected map[string]string, attrs map[string]any) error {
	for key, want := range expected {
		raw, ok := attrs[key]
		if !ok {
			return fmt.Errorf("%s: attribute %q missing", scope, key)
		}
		got := fmt.Sprint(raw)
		if want == PresenceOnly {
			if got == "" {
				return fmt.Errorf("%s: attribute %q is empty", scope, key)
			}
			continue
		}
		if got != want {
			return fmt.Errorf("%s: attribute %q = %q, want %q", scope, key, got, want)
		}
	}
	return nil
}
