// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"encoding/binary"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

// This file hand-encodes the OTLP protobuf ExportProfilesServiceRequest with protowire.
// Using pdata's pprofile package would force newer Go, otel and grpc than this module (and every
// test in it) can take, so the encoder is small instead. Field numbers follow
// opentelemetry/proto/profiles/v1development; TestEncodeProfilesRequestGolden pins the bytes.

// Shared by the JSON and protobuf payloads. Index 0 must be the empty string, as the profiles
// data model requires.
var payloadStringTable = []string{"", "samples", "count", "cpu", "nanoseconds", "testFunctionA", "testFunctionB", "test.go"}

func profileIDBytes(seed uint64) []byte {
	id := make([]byte, 16)
	binary.BigEndian.PutUint64(id[:8], 1)
	binary.BigEndian.PutUint64(id[8:], seed)
	return id
}

func unixNano(t time.Time) uint64 {
	return uint64(t.UnixNano()) //nolint:gosec // the test runs long after 1970, so this is positive
}

// An empty serviceName leaves service.name out, matching buildProfilesPayload.
func buildProfilesProto(serviceName, instanceID string) []byte {
	var attrs []resourceAttr
	if serviceName != "" {
		attrs = append(attrs, attr(serviceNameKey, serviceName))
	}
	attrs = append(attrs, attr("instance_id", instanceID))

	return encodeProfilesRequest(attrs, unixNano(time.Now()))
}

// Deterministic (ts drives both the timestamp and the profile ID seed) so the golden test can
// pin the bytes.
func encodeProfilesRequest(attrs []resourceAttr, ts uint64) []byte {
	var req []byte
	req = appendMessage(req, 1, encodeResourceProfiles(attrs, ts))
	req = appendMessage(req, 2, encodeDictionary())
	return req
}

func encodeResourceProfiles(attrs []resourceAttr, ts uint64) []byte {
	var resource []byte
	for _, a := range attrs {
		var anyValue []byte
		anyValue = appendString(anyValue, 1, a.Value.StringValue)
		var kv []byte
		kv = appendString(kv, 1, a.Key)
		kv = appendMessage(kv, 2, anyValue)
		resource = appendMessage(resource, 1, kv)
	}

	var scope []byte
	scope = appendString(scope, 1, "cloudwatch-agent-integ-test")
	scope = appendString(scope, 2, "1.0.0")

	var sample []byte
	sample = appendVarint(sample, 1, 1) // stack_index
	sample = appendMessage(sample, 4, protowire.AppendVarint(nil, 100))
	sample = appendMessage(sample, 5, protowire.AppendFixed64(nil, ts))

	var profile []byte
	profile = appendMessage(profile, 1, encodeValueType(1, 2)) // sample_type samples/count
	profile = appendMessage(profile, 2, sample)
	profile = protowire.AppendTag(profile, 3, protowire.Fixed64Type)
	profile = protowire.AppendFixed64(profile, ts)
	profile = appendVarint(profile, 4, uint64(10*time.Second))
	profile = appendMessage(profile, 5, encodeValueType(3, 4)) // period_type cpu/nanoseconds
	profile = appendVarint(profile, 6, uint64(10*time.Millisecond))
	profile = appendMessage(profile, 7, profileIDBytes(ts))

	var scopeProfiles []byte
	scopeProfiles = appendMessage(scopeProfiles, 1, scope)
	scopeProfiles = appendMessage(scopeProfiles, 2, profile)

	var rp []byte
	rp = appendMessage(rp, 1, resource)
	rp = appendMessage(rp, 2, scopeProfiles)
	return rp
}

func encodeValueType(typeIndex, unitIndex uint64) []byte {
	var vt []byte
	vt = appendVarint(vt, 1, typeIndex)
	return appendVarint(vt, 2, unitIndex)
}

// Same dictionary as the JSON payload.
func encodeDictionary() []byte {
	location := func(address, functionIndex, line uint64) []byte {
		var l []byte
		l = appendVarint(l, 1, functionIndex)
		l = appendVarint(l, 2, line)
		var loc []byte
		loc = appendVarint(loc, 2, address)
		return appendMessage(loc, 3, l)
	}
	function := func(nameIndex, filenameIndex uint64) []byte {
		var f []byte
		f = appendVarint(f, 1, nameIndex)
		return appendVarint(f, 3, filenameIndex)
	}
	var locationIndices []byte
	locationIndices = protowire.AppendVarint(locationIndices, 1)
	locationIndices = protowire.AppendVarint(locationIndices, 2)

	var d []byte
	d = appendMessage(d, 1, nil) // mapping_table
	d = appendMessage(d, 2, nil) // location_table
	d = appendMessage(d, 2, location(4096, 1, 10))
	d = appendMessage(d, 2, location(8192, 2, 20))
	d = appendMessage(d, 3, nil) // function_table
	d = appendMessage(d, 3, function(5, 7))
	d = appendMessage(d, 3, function(6, 7))
	d = appendMessage(d, 4, nil) // link_table
	for _, s := range payloadStringTable {
		d = appendString(d, 5, s)
	}
	d = appendMessage(d, 7, nil)                                    // stack_table
	d = appendMessage(d, 7, appendMessage(nil, 1, locationIndices)) // location_indices, packed
	return d
}

func appendVarint(b []byte, field protowire.Number, v uint64) []byte {
	// A zero value is left out on purpose: proto3 encoders skip fields at their default value.
	if v == 0 {
		return b
	}
	b = protowire.AppendTag(b, field, protowire.VarintType)
	return protowire.AppendVarint(b, v)
}

func appendString(b []byte, field protowire.Number, s string) []byte {
	b = protowire.AppendTag(b, field, protowire.BytesType)
	return protowire.AppendString(b, s)
}

// A nil value still appends the field, encoding an empty message.
func appendMessage(b []byte, field protowire.Number, value []byte) []byte {
	b = protowire.AppendTag(b, field, protowire.BytesType)
	return protowire.AppendBytes(b, value)
}
