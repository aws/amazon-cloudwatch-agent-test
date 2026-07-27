// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpvalidation

import (
	"os"
	"strings"
)

// onPremiseMarker identifies the on-prem agent start command (-m onPremise).
const onPremiseMarker = "onPremise"

// ResourceHostIDLabels returns the label filter for host_metrics/prometheus tests.
// Both EC2 and on-prem filter on host.id: EC2 gets it from IMDS, while on-prem
// injects it via resource_attributes since IMDS is disabled.
func ResourceHostIDLabels(agentStartCommand, instanceID string) map[string]string {
	return map[string]string{"@resource.host.id": instanceID}
}

// OtlpMetricLabels returns the label filter for the OTLP test.
// EC2 uses host.id; on-prem uses the injected InstanceId attribute.
func OtlpMetricLabels(agentStartCommand, instanceID string) map[string]string {
	if strings.Contains(agentStartCommand, onPremiseMarker) {
		return map[string]string{"InstanceId": instanceID}
	}
	return map[string]string{"@resource.host.id": instanceID}
}

// getRegion returns the region, falling back to AWS env vars then us-west-2.
func getRegion(region string) string {
	if region != "" {
		return region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return "us-west-2"
}
