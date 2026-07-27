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
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	otel, ok := cfg["opentelemetry"].(map[string]any)
	if !ok {
		return fmt.Errorf("opentelemetry block missing in %s", src)
	}
	attrs, ok := otel["resource_attributes"].(map[string]any)
	if !ok {
		attrs = map[string]any{}
	}
	attrs["host.id"] = instanceID
	otel["resource_attributes"] = attrs

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), configFileName)
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	common.CopyFile(tmp, common.ConfigOutputPath)
	return nil
}
