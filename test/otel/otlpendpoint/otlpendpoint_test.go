// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpendpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestParseSendStatuses(t *testing.T) {
	log := "otlp-e2e-send metrics 200\n" +
		"otlp-e2e-send logs 200\n" +
		"some other line\n" +
		"otlp-e2e-send traces 000\n" +
		"otlp-e2e-send metrics 503\n" +
		"otlp-e2e-send malformed\n"
	assert.Equal(t, map[string][]string{
		"metrics": {"200", "503"},
		"logs":    {"200"},
		"traces":  {"000"},
	}, ParseSendStatuses(log))
	assert.Empty(t, ParseSendStatuses(""))
}

func TestNodeInstanceID(t *testing.T) {
	node := corev1.Node{Spec: corev1.NodeSpec{ProviderID: "aws:///us-west-2a/i-0123456789abcdef0"}}
	assert.Equal(t, "i-0123456789abcdef0", NodeInstanceID(node))
}

func TestHostPorts(t *testing.T) {
	containers := []corev1.Container{{Ports: []corev1.ContainerPort{
		{ContainerPort: 4317},
		{ContainerPort: 8080, HostPort: 18080},
	}}}
	assert.Equal(t, []int32{4317, 8080}, hostPorts(corev1.PodSpec{HostNetwork: true, Containers: containers}))
	assert.Equal(t, []int32{18080}, hostPorts(corev1.PodSpec{Containers: containers}))
}
