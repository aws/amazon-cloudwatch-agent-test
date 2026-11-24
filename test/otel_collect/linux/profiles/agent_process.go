// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"fmt"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const agentUnit = "amazon-cloudwatch-agent"

// agentProcess is the agent unit's systemd state. restarts is empty when systemd does not report NRestarts.
type agentProcess struct {
	mainPID     string
	restarts    string
	activeState string
}

// runCommand runs a shell command; tests replace it to simulate older systemd versions.
var runCommand = common.RunCommand

// readAgentProcess reads the agent unit's MainPID and ActiveState, and NRestarts where systemd has it.
//
// The systemd on AL2 (219) has no NRestarts property and no `systemctl show --value` flag:
// `systemctl show -p NRestarts` exits 1 there and prints only the properties it knows. So MainPID and
// ActiveState, which every supported systemd reports, are read on their own, and an unchanged MainPID is
// the restart proof everywhere. NRestarts is read separately and only adds a check where it exists.
func readAgentProcess() (agentProcess, error) {
	out, err := runCommand("systemctl show -p MainPID -p ActiveState " + agentUnit)
	if err != nil {
		return agentProcess{}, fmt.Errorf("systemctl show %s: %w", agentUnit, err)
	}
	p := parseAgentProcess(out)
	if restarts, err := runCommand("systemctl show -p NRestarts " + agentUnit); err == nil {
		p.restarts = parseAgentProcess(restarts).restarts
	}
	return p, nil
}

func parseAgentProcess(out string) agentProcess {
	var p agentProcess
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "MainPID":
			p.mainPID = value
		case "NRestarts":
			p.restarts = value
		case "ActiveState":
			p.activeState = value
		}
	}
	return p
}

// agentPID returns the agent's main PID, or "" when the unit has no running main process.
func agentPID() string {
	p, err := readAgentProcess()
	if err != nil || p.mainPID == "" || p.mainPID == "0" {
		return ""
	}
	return p.mainPID
}
