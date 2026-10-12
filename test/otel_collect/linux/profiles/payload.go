// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type resourceAttr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}

func attr(key, value string) resourceAttr {
	a := resourceAttr{Key: key}
	a.Value.StringValue = value
	return a
}

// Hex, not the base64 the OTLP JSON spec requires for bytes fields, because the agent's OTLP
// receiver (pdata) decodes profileId as hex only and 400s on base64. Switch to base64 once the
// receiver accepts it.
func profileIDJSON(seed uint64) string {
	return hex.EncodeToString(profileIDBytes(seed))
}

// An empty serviceName leaves the service.name attribute out entirely.
func buildProfilesPayload(serviceName, instanceID string) []byte {
	var attrs []resourceAttr
	if serviceName != "" {
		attrs = append(attrs, attr(serviceNameKey, serviceName))
	}
	attrs = append(attrs, attr("instance_id", instanceID))
	return buildResourceProfilesPayload(attrs)
}

func buildResourceProfilesPayload(resources ...[]resourceAttr) []byte {
	seed := unixNano(time.Now())
	resourceProfiles := make([]any, 0, len(resources))
	for _, attrs := range resources {
		seed++
		ts := fmt.Sprint(seed)
		resourceProfiles = append(resourceProfiles, map[string]any{
			"resource": map[string]any{"attributes": attrs},
			"scopeProfiles": []any{map[string]any{
				"scope": map[string]any{"name": "cloudwatch-agent-integ-test", "version": "1.0.0"},
				"profiles": []any{map[string]any{
					"sampleType": map[string]any{"typeStrindex": 1, "unitStrindex": 2},
					"samples": []any{map[string]any{
						"stackIndex":         1,
						"values":             []string{"100"},
						"timestampsUnixNano": []string{ts},
					}},
					"timeUnixNano": ts,
					"durationNano": "10000000000",
					"periodType":   map[string]any{"typeStrindex": 3, "unitStrindex": 4},
					"period":       "10000000",
					"profileId":    profileIDJSON(seed),
				}},
			}},
		})
	}
	payload := map[string]any{
		"resourceProfiles": resourceProfiles,
		"dictionary": map[string]any{
			"mappingTable": []any{map[string]any{}},
			"locationTable": []any{
				map[string]any{},
				map[string]any{"address": "4096", "lines": []any{map[string]any{"functionIndex": 1, "line": "10"}}},
				map[string]any{"address": "8192", "lines": []any{map[string]any{"functionIndex": 2, "line": "20"}}},
			},
			"functionTable": []any{
				map[string]any{},
				map[string]any{"nameStrindex": 5, "filenameStrindex": 7},
				map[string]any{"nameStrindex": 6, "filenameStrindex": 7},
			},
			"stringTable": payloadStringTable,
			"stackTable":  []any{map[string]any{}, map[string]any{"locationIndices": []int{1, 2}}},
			"linkTable":   []any{map[string]any{}},
		},
	}
	out, err := json.Marshal(payload)
	if err != nil {
		// Every value above is a plain map, slice or string, so marshalling cannot fail.
		panic(err)
	}
	return out
}
