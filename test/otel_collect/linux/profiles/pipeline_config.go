// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	profilesPipelineID = "profiles/otlp"

	profilesResourceProcessorID = "resource/profiles"

	serviceNameKey = "service.name"

	fallbackServiceName = "unknown_service"
)

type collectorConfig struct {
	Processors map[string]processorConfig `yaml:"processors"`
	Service    struct {
		Pipelines map[string]pipelineConfig `yaml:"pipelines"`
	} `yaml:"service"`
}

type pipelineConfig struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}

type processorConfig struct {
	Attributes []attributeAction `yaml:"attributes"`
}

type attributeAction struct {
	Action string `yaml:"action"`
	Key    string `yaml:"key"`
	Value  any    `yaml:"value"`
}

type profilesPipeline struct {
	pipelineConfig
	resourceActions []attributeAction
}

func parseProfilesPipeline(content []byte) (profilesPipeline, error) {
	var cfg collectorConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return profilesPipeline{}, fmt.Errorf("parsing translated config: %w", err)
	}
	pipeline, ok := cfg.Service.Pipelines[profilesPipelineID]
	if !ok {
		return profilesPipeline{}, fmt.Errorf("translated config has no %s pipeline", profilesPipelineID)
	}
	return profilesPipeline{
		pipelineConfig:  pipeline,
		resourceActions: cfg.Processors[profilesResourceProcessorID].Attributes,
	}, nil
}

func componentType(id string) string {
	typ, _, _ := strings.Cut(id, "/")
	return typ
}

// checkProfilesPipelineInvariants enforces two properties of the generated profiles pipeline:
//   - no batch processor: the batch processor has no profiles support, so placing it in this
//     pipeline makes collector startup fail and takes every other pipeline down with it.
//   - resource/profiles is in the pipeline and inserts service.name: the backend rejects a
//     resource without one, so profiles from clients that do not set it would be dropped.
func checkProfilesPipelineInvariants(p profilesPipeline) error {
	var problems []string
	hasResource := false
	for _, id := range p.Processors {
		if componentType(id) == "batch" {
			problems = append(problems, fmt.Sprintf("processor %s is in the %s pipeline", id, profilesPipelineID))
		}
		if id == profilesResourceProcessorID {
			hasResource = true
		}
	}
	if !hasResource {
		problems = append(problems, fmt.Sprintf("processor %s is missing from the %s pipeline (processors: %v)", profilesResourceProcessorID, profilesPipelineID, p.Processors))
	} else if len(serviceNameInserts(p.resourceActions)) == 0 {
		problems = append(problems, fmt.Sprintf("processor %s has no insert or upsert action for %s", profilesResourceProcessorID, serviceNameKey))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// In order; with insert actions the first value wins, because a later insert skips a key that is
// already present.
func serviceNameInserts(actions []attributeAction) []string {
	var values []string
	for _, a := range actions {
		if a.Key != serviceNameKey {
			continue
		}
		if a.Action == "insert" || a.Action == "upsert" {
			values = append(values, fmt.Sprint(a.Value))
		}
	}
	return values
}
