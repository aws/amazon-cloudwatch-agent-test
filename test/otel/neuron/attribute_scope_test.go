//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package neuron

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/otelmetrics"
)

// TestNeuronDeprecatedSemconvAttributesRemoved asserts the neuron pipeline deletes
// the deprecated semantic-convention attributes the Prometheus receiver injects.
//
// url.scheme is not asserted present here. Trainium nodes carry enough labels to
// reach max_total_attributes, and the attribute-limit processor then drops by tier.
//
// The neuron pipeline needs no resource-scope raw-label check. Its groupbyattrs
// processor lifts the semconv names and runtime_tag rather than raw Prometheus
// names, and neuron_promote renames then deletes runtime_tag.
func TestNeuronDeprecatedSemconvAttributesRemoved(t *testing.T) {
	t.Parallel()
	for _, metricName := range neuronMetricNames() {
		metricName := metricName
		t.Run(metricName, func(t *testing.T) {
			t.Parallel()
			results, err := queryCache.Get(context.Background(), metricName)
			require.NoError(t, err, "querying %s", metricName)
			require.NotEmpty(t, results, "%s not available (no Neuron nodes?)", metricName)
			require.NoError(t, otelmetrics.CheckDeprecatedSemconvRemoved(metricName, results))
		})
	}
}
