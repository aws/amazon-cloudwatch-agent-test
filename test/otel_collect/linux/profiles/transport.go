// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	otlpHTTPEndpoint   = "http://127.0.0.1:4318"
	otlpGRPCAddr       = "127.0.0.1:4317"
	profilesURLPath    = "/v1development/profiles"
	profilesExporterID = "otlp_http/profiles"

	profilesExportMethod = "/opentelemetry.proto.collector.profiles.v1development.ProfilesService/Export"

	protobufContentType = "application/x-protobuf"

	pushTimeout = 10 * time.Second
)

// profilesTransport is one way a producer sends profiles to the agent. Real producers differ:
// SDKs and the eBPF profiler send protobuf over HTTP (often gzipped) or gRPC, hand-written
// clients send JSON. Each transport uses its own service name so its resources stay distinct.
type profilesTransport struct {
	name string
	// serviceSuffix is appended to the run's service name; the JSON transport keeps the plain
	// name the other checks use.
	serviceSuffix string
	send          func(serviceName, instanceID string) error
}

var profilesTransports = []profilesTransport{
	{name: "http/json", send: func(service, instance string) error {
		return postProfiles(buildProfilesPayload(service, instance))
	}},
	{name: "http/protobuf", serviceSuffix: "-proto", send: func(service, instance string) error {
		return postProfilesProto(buildProfilesProto(service, instance), false)
	}},
	{name: "http/protobuf+gzip", serviceSuffix: "-protogzip", send: func(service, instance string) error {
		return postProfilesProto(buildProfilesProto(service, instance), true)
	}},
	{name: "grpc", serviceSuffix: "-grpc", send: func(service, instance string) error {
		return exportProfilesGRPC(buildProfilesProto(service, instance))
	}},
}

func postProfiles(payload []byte) error {
	resp, err := http.Post(otlpHTTPEndpoint+profilesURLPath, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("POST %s failed: %w", profilesURLPath, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s returned %d: %s", profilesURLPath, resp.StatusCode, string(body))
	}
	return nil
}

func postProfilesProto(payload []byte, compress bool) error {
	return postProfilesProtoTo(otlpHTTPEndpoint, payload, compress)
}

func postProfilesProtoTo(endpoint string, payload []byte, compress bool) error {
	body := payload
	if compress {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(payload); err != nil {
			return fmt.Errorf("gzip payload: %w", err)
		}
		if err := zw.Close(); err != nil {
			return fmt.Errorf("gzip payload: %w", err)
		}
		body = buf.Bytes()
	}

	req, err := http.NewRequest(http.MethodPost, endpoint+profilesURLPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating POST %s: %w", profilesURLPath, err)
	}
	req.Header.Set("Content-Type", protobufContentType)
	if compress {
		req.Header.Set("Content-Encoding", "gzip")
	}
	client := &http.Client{Timeout: pushTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s (protobuf, gzip=%t) failed: %w", profilesURLPath, compress, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s (protobuf, gzip=%t) returned %d: %q", profilesURLPath, compress, resp.StatusCode, respBody)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), protobufContentType) {
		return nil
	}
	return checkExportResponse(respBody)
}

// rawCodec passes already-encoded protobuf through gRPC unchanged. It keeps the "proto" name,
// so the request carries the content type the receiver expects.
type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error) {
	b, ok := v.(*[]byte)
	if !ok {
		return nil, fmt.Errorf("rawCodec: unexpected message type %T", v)
	}
	return *b, nil
}

func (rawCodec) Unmarshal(data []byte, v any) error {
	b, ok := v.(*[]byte)
	if !ok {
		return fmt.Errorf("rawCodec: unexpected message type %T", v)
	}
	*b = append((*b)[:0], data...)
	return nil
}

func (rawCodec) Name() string { return "proto" }

func exportProfilesGRPC(payload []byte) error {
	return exportProfilesGRPCTo(otlpGRPCAddr, payload)
}

func exportProfilesGRPCTo(addr string, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	var resp []byte
	if err := conn.Invoke(ctx, profilesExportMethod, &payload, &resp, grpc.ForceCodec(rawCodec{})); err != nil {
		return fmt.Errorf("gRPC %s on %s failed: %w", profilesExportMethod, addr, err)
	}
	return checkExportResponse(resp)
}

func checkExportResponse(resp []byte) error {
	rejected, message, err := exportPartialSuccess(resp)
	if err != nil {
		return fmt.Errorf("decoding export response: %w", err)
	}
	if rejected > 0 {
		return fmt.Errorf("receiver rejected %d profiles: %s", rejected, message)
	}
	return nil
}

func exportPartialSuccess(resp []byte) (uint64, string, error) {
	var rejected uint64
	var message string
	partial, err := protoField(resp, 1)
	if err != nil || partial == nil {
		return 0, "", err
	}
	for len(partial) > 0 {
		num, typ, n := protowire.ConsumeTag(partial)
		if n < 0 {
			return 0, "", protowire.ParseError(n)
		}
		partial = partial[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, m := protowire.ConsumeVarint(partial)
			if m < 0 {
				return 0, "", protowire.ParseError(m)
			}
			rejected, n = v, m
		case num == 2 && typ == protowire.BytesType:
			v, m := protowire.ConsumeString(partial)
			if m < 0 {
				return 0, "", protowire.ParseError(m)
			}
			message, n = v, m
		default:
			n = protowire.ConsumeFieldValue(num, typ, partial)
			if n < 0 {
				return 0, "", protowire.ParseError(n)
			}
		}
		partial = partial[n:]
	}
	return rejected, message, nil
}

func protoField(msg []byte, field protowire.Number) ([]byte, error) {
	var found []byte
	for len(msg) > 0 {
		num, typ, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		msg = msg[n:]
		if num == field && typ == protowire.BytesType {
			v, m := protowire.ConsumeBytes(msg)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			found, n = v, m
		} else {
			n = protowire.ConsumeFieldValue(num, typ, msg)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
		}
		msg = msg[n:]
	}
	return found, nil
}
