// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otlpendpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// ProfilesEnabledEnv turns on the profiles checks. The suite's Terraform sets it to its
	// otlp_profiles_enabled variable, which also installs the chart with otlp.profiles.enabled=true and
	// makes the app sender post profiles. It is off by default: the chart's profiles pipeline needs an
	// agent image with the collector's profiles support (1.300075.0 or later).
	ProfilesEnabledEnv = "OTLP_PROFILES_ENABLED"

	// ProfilesPipeline and ProfilesExporter are the chart's profiles pipeline and its exporter.
	ProfilesPipeline = "profiles/cw_k8s_otlp"
	ProfilesExporter = "otlphttp/cw_k8s_otlp_profiles"

	// ProfilesSignal is the signal name otlp_service_sender.sh logs for its profiles posts.
	ProfilesSignal = "profiles"
)

// ProfilesEnabled reports whether ProfilesEnabledEnv is "true".
func ProfilesEnabled() bool {
	return os.Getenv(ProfilesEnabledEnv) == "true"
}

// NodeAgentOtelConfig returns the otelConfig of the node agent's AmazonCloudWatchAgent resource, which
// the chart renders.
func NodeAgentOtelConfig(ctx context.Context, cs kubernetes.Interface) (string, error) {
	raw, err := cs.CoreV1().RESTClient().Get().
		AbsPath("/apis/cloudwatch.aws.amazon.com/v1alpha1/namespaces", AgentNamespace, "amazoncloudwatchagents", AgentName).
		DoRaw(ctx)
	if err != nil {
		return "", err
	}
	var cr struct {
		Spec struct {
			OtelConfig string `json:"otelConfig"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("AmazonCloudWatchAgent %s/%s: %w", AgentNamespace, AgentName, err)
	}
	return cr.Spec.OtelConfig, nil
}

// PipelineExporters returns the exporters of pipeline in an OTel collector config. found is false when
// the config has no such pipeline.
func PipelineExporters(otelConfig, pipeline string) (exporters []string, found bool, err error) {
	var cfg struct {
		Service struct {
			Pipelines map[string]struct {
				Exporters []string `yaml:"exporters"`
			} `yaml:"pipelines"`
		} `yaml:"service"`
	}
	if err := yaml.Unmarshal([]byte(otelConfig), &cfg); err != nil {
		return nil, false, err
	}
	p, found := cfg.Service.Pipelines[pipeline]
	return p.Exporters, found, nil
}

// AgentLogs returns the logs of the containers of the running node agent pods, the pods the node agent
// DaemonSet selects, keyed by "<pod>/<container>".
func AgentLogs(ctx context.Context, cs kubernetes.Interface) (map[string]string, error) {
	pods, err := AgentPods(ctx, cs)
	if err != nil {
		return nil, err
	}
	logs := map[string]string{}
	for _, pod := range pods {
		for _, c := range pod.Spec.Containers {
			raw, err := cs.CoreV1().Pods(AgentNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: c.Name}).DoRaw(ctx)
			if err != nil {
				return nil, err
			}
			logs[pod.Name+"/"+c.Name] = string(raw)
		}
	}
	return logs, nil
}

// ExporterProblems returns the lines of an agent log that name exporter and report a failed, dropped,
// rejected or partially accepted export, or an error.
func ExporterProblems(log, exporter string) []string {
	var problems []string
	for _, line := range strings.Split(log, "\n") {
		if !strings.Contains(line, exporter) {
			continue
		}
		lower := strings.ToLower(line)
		for _, marker := range []string{"exporting failed", "dropping data", "rejecting", "partial success", "\terror\t", "e! "} {
			if strings.Contains(lower, marker) {
				problems = append(problems, line)
				break
			}
		}
	}
	return problems
}
