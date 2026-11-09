// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpendpoint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseSendStatuses(t *testing.T) {
	log := "otlp-e2e-send metrics 200\n" +
		"otlp-e2e-send logs 200\n" +
		"some other line\n" +
		"otlp-e2e-send traces 000\n" +
		"otlp-e2e-control 400\n" +
		"otlp-e2e-send metrics 503\n" +
		"otlp-e2e-send malformed\n" +
		"otlp-e2e-control 000\n" +
		"otlp-e2e-control\n"
	assert.Equal(t, map[string][]string{
		"metrics":     {"200", "503"},
		"logs":        {"200"},
		"traces":      {"000"},
		ControlSignal: {"400", "000"},
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

func TestPipelineExporters(t *testing.T) {
	cfg := "service:\n  pipelines:\n    profiles/cw_k8s_otlp:\n      receivers: [otlp/cw_k8s_otlp]\n      exporters: [otlphttp/cw_k8s_otlp_profiles]\n"
	exporters, found, err := PipelineExporters(cfg, ProfilesPipeline)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []string{ProfilesExporter}, exporters)

	_, found, err = PipelineExporters(cfg, "metrics/cw_k8s_otlp")
	assert.NoError(t, err)
	assert.False(t, found)

	_, _, err = PipelineExporters("service: [", ProfilesPipeline)
	assert.Error(t, err)
}

func TestExporterProblems(t *testing.T) {
	log := "2026-11-12T10:00:00Z\tinfo\tservice started\n" +
		"2026-11-12T10:00:01Z\terror\tinternal/queue_sender.go:49\tExporting failed. Dropping data.\t{\"otelcol.component.id\": \"otlphttp/cw_k8s_otlp_profiles\"}\n" +
		"2026-11-12T10:00:02Z\terror\tExporting failed.\t{\"otelcol.component.id\": \"otlphttp/cw_k8s_otlp_metrics\"}\n" +
		"2026-11-12T10:00:03Z\tinfo\tstarted\t{\"otelcol.component.id\": \"otlphttp/cw_k8s_otlp_profiles\"}\n"
	problems := ExporterProblems(log, ProfilesExporter)
	assert.Len(t, problems, 1)
	assert.Contains(t, problems[0], "Exporting failed")
	assert.Empty(t, ExporterProblems("", ProfilesExporter))
}

func TestMatchedSince(t *testing.T) {
	start := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	started := func(at time.Time) corev1.Pod {
		return corev1.Pod{Status: corev1.PodStatus{StartTime: &metav1.Time{Time: at}}}
	}
	pods := []corev1.Pod{started(start), started(start.Add(3 * time.Minute)), started(start.Add(time.Minute)), {}}
	assert.Equal(t, start.Add(3*time.Minute+PodCacheGrace), matchedSince(pods))
}
