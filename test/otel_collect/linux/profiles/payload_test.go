// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildProfilesPayloadServiceName(t *testing.T) {
	withName := buildProfilesPayload("svc-1", "i-0")
	require.True(t, json.Valid(withName))
	require.Contains(t, string(withName), `"service.name"`)

	withoutName := buildProfilesPayload("", "i-0")
	require.True(t, json.Valid(withoutName))
	require.NotContains(t, string(withoutName), `"service.name"`)
}

// Every profile carries a 16-byte hex profileId, unique within the request, because that is what
// the agent's OTLP JSON receiver decodes (see profileIDJSON).
func TestBuildProfilesPayloadProfileID(t *testing.T) {
	payload := buildResourceProfilesPayload(
		[]resourceAttr{attr(serviceNameKey, "a")},
		[]resourceAttr{attr(serviceNameKey, "b")},
	)
	var decoded struct {
		ResourceProfiles []struct {
			ScopeProfiles []struct {
				Profiles []struct {
					ProfileID string `json:"profileId"`
				} `json:"profiles"`
			} `json:"scopeProfiles"`
		} `json:"resourceProfiles"`
	}
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Len(t, decoded.ResourceProfiles, 2)

	seen := map[string]bool{}
	for _, rp := range decoded.ResourceProfiles {
		id := rp.ScopeProfiles[0].Profiles[0].ProfileID
		raw, err := hex.DecodeString(id)
		require.NoError(t, err, "profileId %q is not hex", id)
		require.Len(t, raw, 16)
		require.NotEqual(t, make([]byte, 16), raw, "an all-zero profileId means unset")
		require.False(t, seen[id], "profileId %q repeats", id)
		seen[id] = true
	}
}
