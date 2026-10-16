// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"encoding/json"
	"errors"
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
