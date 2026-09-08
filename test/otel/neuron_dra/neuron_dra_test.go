//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

// Package neuron_dra validates per-device/per-core Neuron pod correlation on the
// Dynamic Resource Allocation (DRA) path — the counterpart of the device-plugin
// `neuron` package's multi_device tests. Neuron devices are allocated via the
// AWS Neuron DRA driver (DeviceClass neuron.aws.com, driver neuron.aws.com) through
// a ResourceClaimTemplate instead of the aws.amazon.com/neuron device-plugin
// resource. The emitted metrics are identical, so the assertions mirror the
// device-plugin tests.
//
// Cluster topology:
//   - 1x trn1.2xlarge = 1 Neuron (Trainium) device × 2 cores = 2 cores
//   - Neuron exposed via the Neuron DRA driver, not the device plugin
//   - A burn workload (neuron-burn-dra) claims 1 whole device via DRA
//
// Why Trainium (trn1) and not Inferentia (inf2): the AWS Neuron DRA driver
// (neuron-helm-chart, driver image 1.2.0) supports Trainium only — it explicitly
// rejects inf1/inf2 at device discovery ("unsupported instance type"). trn1.2xlarge
// is the smallest, most reliably-available Trainium instance (1 device / 2 cores),
// which is enough to exercise the DRA correlation code path end to end: a claimed
// device's two cores must both attribute to the claiming pod, and to no other pod.
// The multi-device "1 claimed + N-1 unclaimed" matrix is covered by the
// device-plugin multi_device_test; here the device count is 1 so the whole node's
// single device is the claimed device.
package neuron_dra

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	neuronInstanceType        = "trn1.2xlarge"
	expectedNeuronDeviceCount = 1
	expectedNeuronCoreCount   = 2
	expectedCoresPerDevice    = 2

	// burnPodPrefix is the DRA burn workload that claims 1 whole Neuron device
	// (= 2 cores) via a ResourceClaimTemplate.
	burnPodPrefix = "neuron-burn-dra"
	// A claim for 1 whole device yields 2 cores on 1 device.
	expectedClaimedDevices = 1
	expectedClaimedCores   = 2
)

// TestNeuronDRADeviceCount validates the node exposes the expected Neuron devices.
func TestNeuronDRADeviceCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	node := filterByHostType(results, neuronInstanceType)
	require.True(t, len(node) > 0, "No results from %s node", neuronInstanceType)

	devices := uniqueDatapointValuesList(node, "aws.neuron.device")
	require.Equal(t, expectedNeuronDeviceCount, len(devices),
		"Expected %d distinct aws.neuron.device on %s, got %d: %v",
		expectedNeuronDeviceCount, neuronInstanceType, len(devices), devices)
}

// TestNeuronDRACoreCount validates the total (device, core) pair count.
func TestNeuronDRACoreCount(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	node := filterByHostType(results, neuronInstanceType)
	require.True(t, len(node) > 0, "No results from %s node", neuronInstanceType)

	pairs := uniqueDatapointPairs(node, "aws.neuron.device", "aws.neuron.core")
	require.Equal(t, expectedNeuronCoreCount, len(pairs),
		"Expected %d (device, core) pairs on %s, got %d",
		expectedNeuronCoreCount, neuronInstanceType, len(pairs))
}

// TestNeuronDRACoresPerDevice validates each device exposes the expected cores.
func TestNeuronDRACoresPerDevice(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	require.NotEmpty(t, results, "neuroncore_utilization_ratio not available")

	node := filterByHostType(results, neuronInstanceType)
	require.True(t, len(node) > 0, "No results from %s node", neuronInstanceType)

	coresByDevice := make(map[string]map[string]struct{})
	for _, r := range node {
		r := r
		dev := r.Labels.Datapoint["aws.neuron.device"]
		core := r.Labels.Datapoint["aws.neuron.core"]
		if dev == "" || core == "" {
			continue
		}
		if coresByDevice[dev] == nil {
			coresByDevice[dev] = make(map[string]struct{})
		}
		coresByDevice[dev][core] = struct{}{}
	}
	require.True(t, len(coresByDevice) > 0, "No devices with core metrics on %s", neuronInstanceType)
	for dev, cores := range coresByDevice {
		require.Equal(t, expectedCoresPerDevice, len(cores),
			"Neuron device %s: expected %d cores, got %d", dev, expectedCoresPerDevice, len(cores))
	}
}

// TestNeuronDRADeviceIndicesAreIntegers validates device and core are integers.
func TestNeuronDRADeviceIndicesAreIntegers(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	node := filterByHostType(results, neuronInstanceType)
	require.True(t, len(node) > 0, "No results from %s node", neuronInstanceType)

	for _, r := range node {
		r := r
		dev := r.Labels.Datapoint["aws.neuron.device"]
		core := r.Labels.Datapoint["aws.neuron.core"]
		require.True(t, isIntLike(dev), "aws.neuron.device should be integer, got '%s'", dev)
		require.True(t, isIntLike(core), "aws.neuron.core should be integer, got '%s'", core)
	}
}

// TestNeuronDRAClaimedDeviceCorrelation is the DRA-path per-device correlation guard.
// The burn pod claims exactly 1 whole Neuron device via DRA, so both of that device's
// cores must attribute to that single pod — and to exactly one pod. This fails if DRA
// correlation collapses cores onto the wrong pod, attributes an unclaimed core to a
// pod, or splits a device across pods. On this single-device node the node's one
// device is the claimed device.
func TestNeuronDRAClaimedDeviceCorrelation(t *testing.T) {
	t.Parallel()
	results, err := queryCache.Get(context.Background(), "neuroncore_utilization_ratio")
	require.NoError(t, err, "querying neuroncore_utilization_ratio")
	node := filterByHostType(results, neuronInstanceType)
	require.NotEmpty(t, node, "No neuroncore_utilization_ratio results from %s node", neuronInstanceType)

	// For each device, collect the distinct pods its cores are attributed to
	// (empty = unclaimed), and count how many of its cores carry the burn pod.
	devicePods := make(map[string]map[string]struct{})
	claimedCoresByDevice := make(map[string]map[string]struct{})
	for _, r := range node {
		r := r
		dev := r.Labels.Datapoint["aws.neuron.device"]
		core := r.Labels.Datapoint["aws.neuron.core"]
		if dev == "" {
			continue
		}
		if devicePods[dev] == nil {
			devicePods[dev] = make(map[string]struct{})
		}
		pod := r.Labels.Resource["k8s.pod.name"]
		if pod == "" {
			continue
		}
		devicePods[dev][pod] = struct{}{}
		if len(pod) >= len(burnPodPrefix) && pod[:len(burnPodPrefix)] == burnPodPrefix {
			if claimedCoresByDevice[dev] == nil {
				claimedCoresByDevice[dev] = make(map[string]struct{})
			}
			claimedCoresByDevice[dev][core] = struct{}{}
		}
	}
	require.Len(t, devicePods, expectedNeuronDeviceCount,
		"expected %d Neuron device(s) on the node, got %d", expectedNeuronDeviceCount, len(devicePods))

	var claimedDevices []string
	for dev, pods := range devicePods {
		switch len(pods) {
		case 0:
			// No pod on any core of this device.
		case 1:
			claimedDevices = append(claimedDevices, dev)
		default:
			// A single device whose cores span multiple pods is a collapse symptom.
			t.Errorf("Neuron device %s correlated to multiple pods %v", dev, setKeys(pods))
		}
	}

	// Exactly 1 device correlated, and exactly 2 of its cores → the burn pod.
	require.Len(t, claimedDevices, expectedClaimedDevices,
		"expected %d claimed Neuron device(s), got %d: %v",
		expectedClaimedDevices, len(claimedDevices), claimedDevices)
	for _, dev := range claimedDevices {
		require.Len(t, claimedCoresByDevice[dev], expectedClaimedCores,
			fmt.Sprintf("expected %d cores of device %s correlated to %s*, got %d",
				expectedClaimedCores, dev, burnPodPrefix, len(claimedCoresByDevice[dev])))
	}
}

// TestNeuronDRABurnPodLabels validates the correlated burn pod's labels via a
// targeted PromQL query (avoids stale series in the shared OTLP store).
func TestNeuronDRABurnPodLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	promql := fmt.Sprintf(
		`neuroncore_utilization_ratio{"@resource.k8s.cluster.name"="%s","@resource.k8s.pod.name"=~"%s.*"}`,
		escapePromQL(cfg.ClusterName), burnPodPrefix)
	burn, err := client.Query(ctx, promql)
	require.NoError(t, err, "querying neuroncore_utilization_ratio for %s", burnPodPrefix)
	require.Equal(t, expectedClaimedCores, len(burn),
		"Expected %d cores correlated to %s (1 whole device × 2 cores), got %d",
		expectedClaimedCores, burnPodPrefix, len(burn))

	devices := make(map[string]struct{})
	for _, r := range burn {
		r := r
		devices[r.Labels.Datapoint["aws.neuron.device"]] = struct{}{}
		require.Equal(t, "default", r.Labels.Resource["k8s.namespace.name"], "%s namespace", burnPodPrefix)
	}
	require.Len(t, devices, expectedClaimedDevices,
		"Expected %s cores on %d device, got %d", burnPodPrefix, expectedClaimedDevices, len(devices))
}
