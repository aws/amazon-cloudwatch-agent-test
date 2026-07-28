// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package otelconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	agentConfigDir  = "agent_configs"
	onPremiseMarker = "onPremise"
)

// Setup deploys the agent config. On on-prem it injects
// resource_attributes host.id = instanceID for a queryable host identifier.
func Setup(configFileName, agentStartCommand, instanceID string) error {
	src := filepath.Join(agentConfigDir, configFileName)
	if !strings.Contains(agentStartCommand, onPremiseMarker) {
		common.CopyFile(src, common.ConfigOutputPath)
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out, err := injectHostID(data, instanceID)
	if err != nil {
		return fmt.Errorf("inject host.id into %s: %w", src, err)
	}
	tmp := filepath.Join(os.TempDir(), configFileName)
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	common.CopyFile(tmp, common.ConfigOutputPath)
	return nil
}

// injectHostID sets opentelemetry.resource_attributes["host.id"] = instanceID
// in the agent-config JSON, preserving any existing resource_attributes.
func injectHostID(data []byte, instanceID string) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	otel, ok := cfg["opentelemetry"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("opentelemetry block missing")
	}
	attrs, ok := otel["resource_attributes"].(map[string]any)
	if !ok {
		attrs = map[string]any{}
	}
	attrs["host.id"] = instanceID
	otel["resource_attributes"] = attrs
	return json.MarshalIndent(cfg, "", "  ")
}
