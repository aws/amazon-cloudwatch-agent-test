// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"fmt"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	exportFailedLogText    = "exporting failed"
	exportDroppingLogText  = "Dropping data"
	exportRejectingLogText = "Rejecting"
	partialSuccessLogText  = "Partial success response"
	componentIDLogKey      = "otelcol.component.id"
)

func profilesLogLines() ([]string, error) {
	content, err := common.RunCommand("sudo cat " + common.AgentLogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent log %s: %w", common.AgentLogFile, err)
	}
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "profiles") {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func lastExportFailure(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(lines[i]), exportFailedLogText) {
			return lines[i]
		}
	}
	return ""
}

func profilesExporterLogProblem(line string) bool {
	if !strings.Contains(line, "profiles") {
		return false
	}
	if strings.Contains(line, "E! ") {
		return true
	}
	if strings.Contains(line, exportDroppingLogText) || strings.Contains(line, exportRejectingLogText) {
		return true
	}
	return strings.Contains(line, partialSuccessLogText)
}

func resetAgentLog() {
	common.RecreateAgentLogfile(common.AgentLogFile)
}
