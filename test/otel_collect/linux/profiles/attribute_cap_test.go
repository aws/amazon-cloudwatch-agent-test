// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

func TestBuildAttributeCapPayload(t *testing.T) {
	payload := buildAttributeCapPayload("svc", "i-0", capExtraAttributes)
	var req struct {
		ResourceProfiles []struct {
			Resource struct {
				Attributes []resourceAttr `json:"attributes"`
			} `json:"resource"`
		} `json:"resourceProfiles"`
	}
	require.NoError(t, json.Unmarshal(payload, &req))
	require.Len(t, req.ResourceProfiles, 2, "the request has to mix a valid and an oversized resource")
	require.Len(t, req.ResourceProfiles[0].Resource.Attributes, 2)
	oversized := req.ResourceProfiles[1].Resource.Attributes
	require.Len(t, oversized, capExtraAttributes+2)
	require.Greater(t, len(oversized), 150)
	require.Equal(t, serviceNameKey, oversized[0].Key)
}

func TestParseAgentProcess(t *testing.T) {
	p := parseAgentProcess("MainPID=1234\nNRestarts=0\nActiveState=active\n")
	require.Equal(t, agentProcess{mainPID: "1234", restarts: "0", activeState: "active"}, p)
}

// fakeSystemctl behaves like `systemctl show` for a unit with MainPID 1234. With oldSystemd it acts like
// AL2's systemd 219: no NRestarts property (exit 1, printing only the known properties) and no --value flag.
func fakeSystemctl(t *testing.T, oldSystemd bool) {
	t.Helper()
	orig := runCommand
	t.Cleanup(func() { runCommand = orig })
	runCommand = func(cmd string) (string, error) {
		if oldSystemd && strings.Contains(cmd, "--value") {
			return "", errors.New("exit status 1: systemctl: unrecognized option '--value'")
		}
		var out []string
		for _, prop := range []string{"MainPID=1234", "NRestarts=0", "ActiveState=active"} {
			name, _, _ := strings.Cut(prop, "=")
			if !strings.Contains(cmd, "-p "+name) || (oldSystemd && name == "NRestarts") {
				continue
			}
			out = append(out, prop)
		}
		if oldSystemd && strings.Contains(cmd, "-p NRestarts") {
			return strings.Join(out, "\n"), errors.New("exit status 1")
		}
		return strings.Join(out, "\n"), nil
	}
}

func TestReadAgentProcess(t *testing.T) {
	t.Run("systemd with NRestarts", func(t *testing.T) {
		fakeSystemctl(t, false)
		p, err := readAgentProcess()
		require.NoError(t, err)
		require.Equal(t, agentProcess{mainPID: "1234", restarts: "0", activeState: "active"}, p)
	})
	t.Run("AL2 systemd without NRestarts", func(t *testing.T) {
		fakeSystemctl(t, true)
		p, err := readAgentProcess()
		require.NoError(t, err)
		require.Equal(t, agentProcess{mainPID: "1234", activeState: "active"}, p)
	})
}

func TestAgentPIDOnAL2(t *testing.T) {
	fakeSystemctl(t, true)
	require.Equal(t, "1234", agentPID(), "the main PID must be readable without systemctl --value")
}

func TestCheckAgentStaysUp(t *testing.T) {
	up := agentProcess{mainPID: "1234", restarts: "0", activeState: "active"}
	al2 := agentProcess{mainPID: "1234", activeState: "active"}
	cases := []struct {
		name          string
		before, after agentProcess
		wantOK        bool
		wantReason    string
	}{
		{"unchanged with NRestarts", up, up, true, ""},
		{"unchanged on AL2", al2, al2, true, ""},
		{"restarted on AL2", al2, agentProcess{mainPID: "5678", activeState: "active"}, false, "main PID changed"},
		{"restart counted", up, agentProcess{mainPID: "1234", restarts: "1", activeState: "active"}, false, "restart count changed"},
		{"unit not active", up, agentProcess{mainPID: "0", restarts: "0", activeState: "failed"}, false, `"failed"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := (&ProfilesAttributeCapTestRunner{done: true, before: c.before, after: c.after}).checkAgentStaysUp()
			if c.wantOK {
				require.Equal(t, status.SUCCESSFUL, r.Status, "%v", r.Reason)
				return
			}
			require.Equal(t, status.FAILED, r.Status)
			require.ErrorContains(t, r.Reason, c.wantReason)
		})
	}
}

func TestCheckPartialSuccessSurfaced(t *testing.T) {
	ok := checkPartialSuccessSurfaced([]string{
		`W! {"caller":"otlphttpexporter/otlp.go:455","msg":"Partial success response","otelcol.component.id":"otlp_http/profiles","dropped_samples":1}`,
	}, nil)
	require.Equal(t, status.SUCCESSFUL, ok.Status)

	failed := checkPartialSuccessSurfaced([]string{`E! Exporting failed. Dropping data. otlp_http/profiles 404`}, nil)
	require.Equal(t, status.FAILED, failed.Status)
	require.ErrorContains(t, failed.Reason, "export failed before the backend")

	silent := checkPartialSuccessSurfaced(nil, nil)
	require.ErrorContains(t, silent.Reason, "not visible in the agent log")

	unreadable := checkPartialSuccessSurfaced(nil, errors.New("no log"))
	require.ErrorContains(t, unreadable.Reason, "no log")
}
