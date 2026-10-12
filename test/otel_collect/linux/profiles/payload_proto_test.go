// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

// protoValues returns every value of a field in an encoded message. Length-delimited values
// are returned as bytes, varint values as their varint encoding, fixed64 values as 8 bytes.
func protoValues(t *testing.T, msg []byte, field protowire.Number) [][]byte {
	t.Helper()
	var values [][]byte
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		require.GreaterOrEqual(t, n, 0, "bad tag")
		msg = msg[n:]
		m := protowire.ConsumeFieldValue(num, typ, msg)
		require.GreaterOrEqual(t, m, 0, "bad value of field %d", num)
		if num == field {
			v := msg[:m]
			if typ == protowire.BytesType {
				v, _ = protowire.ConsumeBytes(v)
			}
			values = append(values, v)
		}
		msg = msg[m:]
	}
	return values
}

func protoValue(t *testing.T, msg []byte, path ...protowire.Number) []byte {
	t.Helper()
	for _, field := range path {
		values := protoValues(t, msg, field)
		require.Len(t, values, 1, "field %d", field)
		msg = values[0]
	}
	return msg
}

func resourceAttrKeys(t *testing.T, req []byte) map[string]string {
	t.Helper()
	resource := protoValue(t, req, 1, 1)
	attrs := map[string]string{}
	for _, kv := range protoValues(t, resource, 1) {
		key := string(protoValue(t, kv, 1))
		attrs[key] = string(protoValue(t, kv, 2, 1))
	}
	return attrs
}

func TestBuildProfilesProtoServiceName(t *testing.T) {
	withName := resourceAttrKeys(t, buildProfilesProto("svc-1", "i-0"))
	require.Equal(t, map[string]string{serviceNameKey: "svc-1", "instance_id": "i-0"}, withName)

	withoutName := resourceAttrKeys(t, buildProfilesProto("", "i-0"))
	require.Equal(t, map[string]string{"instance_id": "i-0"}, withoutName)
}

// The protobuf profile carries the same 16 raw bytes the JSON payload hex-encodes, so no
// hex or base64 question arises on this path.
func TestBuildProfilesProtoProfileID(t *testing.T) {
	req := buildProfilesProto("svc", "i-0")
	profile := protoValue(t, req, 1, 2, 2)
	id := protoValue(t, profile, 7)
	require.Len(t, id, 16)

	ts, n := protowire.ConsumeFixed64(protoValue(t, profile, 3))
	require.Equal(t, 8, n)
	require.Equal(t, profileIDBytes(ts), id)

	fromJSON, err := hex.DecodeString(profileIDJSON(ts))
	require.NoError(t, err)
	require.Equal(t, fromJSON, id)
}

// The protobuf dictionary matches the JSON payload's: same strings, and a stack whose two
// locations name testFunctionA and testFunctionB.
func TestBuildProfilesProtoDictionary(t *testing.T) {
	dict := protoValue(t, buildProfilesProto("svc", "i-0"), 2)

	var stringsGot []string
	for _, s := range protoValues(t, dict, 5) {
		stringsGot = append(stringsGot, string(s))
	}
	require.Equal(t, payloadStringTable, stringsGot)

	functions := protoValues(t, dict, 3)
	require.Len(t, functions, 3)
	require.Empty(t, functions[0], "function_table[0] must be the empty entry")
	for i, want := range []string{"testFunctionA", "testFunctionB"} {
		nameIndex, _ := protowire.ConsumeVarint(protoValue(t, functions[i+1], 1))
		require.Equal(t, want, payloadStringTable[nameIndex])
	}

	stacks := protoValues(t, dict, 7)
	require.Len(t, stacks, 2)
	require.Len(t, protoValues(t, dict, 2), 3, "location_table")
	require.Equal(t, []byte{1, 2}, protoValue(t, stacks[1], 1), "packed location_indices")
}

// goldenProfilesRequest is encodeProfilesRequest's output for service.name=svc-1,
// instance_id=i-0 and ts=1700000000000000000. It was generated once and checked out of tree:
// pdata pprofile v0.150 ProtoUnmarshaler decodes it to the same profile as the JSON payload with
// the same timestamp and profile ID. Regenerate it only for an intended payload change, and
// repeat that check when you do.
const goldenProfilesRequest = "" +
	"0a9e010a2f0a170a0c736572766963652e6e616d6512070a057376632d310a140a0b696e7374616e" +
	"63655f696412050a03692d30126b0a240a1b636c6f756477617463682d6167656e742d696e746567" +
	"2d746573741205312e302e3012430a0408011002120f08012201642a0800002a36fe9c9717190000" +
	"2a36fe9c97172080c8afa0252a04080310043080ade2043a10000000000000000117979cfe362a00" +
	"00127d0a00120012091080201a040801100a12091080401a04080210141a001a04080518071a0408" +
	"06180722002a002a0773616d706c65732a05636f756e742a036370752a0b6e616e6f7365636f6e64" +
	"732a0d7465737446756e6374696f6e412a0d7465737446756e6374696f6e422a07746573742e676f" +
	"3a003a040a020102"

// The encoder's bytes are pinned, so a change to the encoding is caught without pdata in this
// module.
func TestEncodeProfilesRequestGolden(t *testing.T) {
	want, err := hex.DecodeString(goldenProfilesRequest)
	require.NoError(t, err)
	got := encodeProfilesRequest([]resourceAttr{attr(serviceNameKey, "svc-1"), attr("instance_id", "i-0")}, 1700000000000000000)
	require.True(t, bytes.Equal(want, got), "encoded request drifted from the golden bytes:\nwant %x\ngot  %x", want, got)
}
