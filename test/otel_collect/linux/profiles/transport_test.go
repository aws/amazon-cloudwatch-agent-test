// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestMissingTransports(t *testing.T) {
	all := map[string]int{}
	for _, tr := range profilesTransports {
		all[tr.name] = 2
	}
	require.NoError(t, missingTransports(all))

	delete(all, "grpc")
	err := missingTransports(all)
	require.Error(t, err)
	require.Contains(t, err.Error(), "grpc")
}

func TestPostProfilesProto(t *testing.T) {
	payload := buildProfilesProto("svc", "i-0")
	for _, compress := range []bool{false, true} {
		var gotType, gotEncoding string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotType, gotEncoding = r.Header.Get("Content-Type"), r.Header.Get("Content-Encoding")
			body := io.Reader(r.Body)
			if gotEncoding == "gzip" {
				zr, err := gzip.NewReader(r.Body)
				require.NoError(t, err)
				body = zr
			}
			gotBody, _ = io.ReadAll(body)
			w.Header().Set("Content-Type", protobufContentType)
			w.WriteHeader(http.StatusOK)
		}))
		require.NoError(t, postProfilesProtoTo(srv.URL, payload, compress))
		srv.Close()

		require.Equal(t, protobufContentType, gotType)
		require.Equal(t, compress, gotEncoding == "gzip")
		require.True(t, bytes.Equal(payload, gotBody), "compress=%t: body differs", compress)
	}
}

func TestPostProfilesProtoRejected(t *testing.T) {
	var partial []byte
	partial = appendVarint(partial, 1, 1)
	partial = appendString(partial, 2, "rejected")
	resp := appendMessage(nil, 1, partial)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", protobufContentType)
		_, _ = w.Write(resp)
	}))
	defer srv.Close()
	err := postProfilesProtoTo(srv.URL, buildProfilesProto("svc", "i-0"), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected 1 profiles")

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
	}))
	defer bad.Close()
	require.Error(t, postProfilesProtoTo(bad.URL, buildProfilesProto("svc", "i-0"), false))
}

func TestExportProfilesGRPC(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var gotMethod string
	var gotBody []byte
	srv := grpc.NewServer(grpc.ForceServerCodec(rawCodec{}), grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		gotMethod, _ = grpc.MethodFromServerStream(stream)
		var req []byte
		if err := stream.RecvMsg(&req); err != nil {
			return err
		}
		gotBody = req
		resp := []byte{}
		return stream.SendMsg(&resp)
	}))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	payload := buildProfilesProto("svc", "i-0")
	require.NoError(t, exportProfilesGRPCTo(lis.Addr().String(), payload))
	require.Equal(t, profilesExportMethod, gotMethod)
	require.Equal(t, payload, gotBody)
}

func TestRawCodec(t *testing.T) {
	c := rawCodec{}
	require.Equal(t, "proto", c.Name())
	in := []byte{1, 2, 3}
	out, err := c.Marshal(&in)
	require.NoError(t, err)
	require.Equal(t, in, out)

	var back []byte
	require.NoError(t, c.Unmarshal(out, &back))
	require.Equal(t, in, back)

	_, err = c.Marshal("not bytes")
	require.Error(t, err)
	require.Error(t, c.Unmarshal(out, new(string)))
}

func TestExportPartialSuccess(t *testing.T) {
	rejected, message, err := exportPartialSuccess(nil)
	require.NoError(t, err)
	require.Zero(t, rejected)
	require.Empty(t, message)

	var partial []byte
	partial = appendVarint(partial, 1, 3)
	partial = appendString(partial, 2, "too many attributes")
	resp := appendMessage(nil, 1, partial)
	rejected, message, err = exportPartialSuccess(resp)
	require.NoError(t, err)
	require.Equal(t, uint64(3), rejected)
	require.Equal(t, "too many attributes", message)

	_, _, err = exportPartialSuccess([]byte{0x0a, 0x05, 0x01})
	require.Error(t, err, "truncated response")
}
