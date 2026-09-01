// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package syslog

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

// The syslog receiver binds to loopback by default, so all traffic in these
// tests is sent to 127.0.0.1. Ports match the agent's defaults for each
// transport (5514 TCP, 1514 UDP, 6514 TLS) as resolved by the translator.
const (
	tcpAddr = "127.0.0.1:5514"
	udpAddr = "127.0.0.1:1514"
	tlsAddr = "127.0.0.1:6514"

	// tlsCertDir is where the agent expects the server/CA material referenced
	// by the TLS test configs.
	tlsCertDir = "/opt/aws/amazon-cloudwatch-agent/etc/tls"

	// sleepForFlush allows the agent to batch and publish to CloudWatch Logs
	// before assertions run.
	sleepForFlush = 60 * time.Second

	// agentStartupDelay gives the listener time to bind before sending.
	agentStartupDelay = 10 * time.Second
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// instanceID resolves the host's instance ID, preferring the value supplied by
// the test framework and falling back to IMDS.
func instanceID() string {
	env := environment.GetEnvironmentMetaData()
	if env.InstanceId != "" {
		return env.InstanceId
	}
	return awsservice.GetInstanceId()
}

// logGroupName builds a unique log group name per test scenario so parallel
// runs against the same account do not collide.
func logGroupName(scenario string) string {
	return fmt.Sprintf("/aws/cwagent/syslog-test/%s/%s", instanceID(), scenario)
}

// rfc5424Msg builds an RFC 5424 syslog message.
func rfc5424Msg(facility, severity int, hostname, appName, msg string) string {
	pri := facility*8 + severity
	ts := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("<%d>1 %s %s %s 1234 - - %s", pri, ts, hostname, appName, msg)
}

// rfc3164Msg builds an RFC 3164 (BSD) syslog message.
func rfc3164Msg(facility, severity int, hostname, appName, msg string) string {
	pri := facility*8 + severity
	ts := time.Now().UTC().Format("Jan  2 15:04:05")
	return fmt.Sprintf("<%d>%s %s %s[1234]: %s", pri, ts, hostname, appName, msg)
}

// sendTCP writes newline-framed syslog messages over a plain TCP connection.
func sendTCP(t *testing.T, addr string, messages []string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	require.NoError(t, err, "dial tcp %s", addr)
	defer conn.Close()
	for _, msg := range messages {
		_, err := fmt.Fprintf(conn, "%s\n", msg)
		require.NoError(t, err, "write tcp message")
	}
}

// sendUDP writes one syslog message per datagram.
func sendUDP(t *testing.T, addr string, messages []string) {
	t.Helper()
	conn, err := net.DialTimeout("udp", addr, 10*time.Second)
	require.NoError(t, err, "dial udp %s", addr)
	defer conn.Close()
	for _, msg := range messages {
		_, err := fmt.Fprint(conn, msg)
		require.NoError(t, err, "write udp message")
		// Pace datagrams slightly; UDP has no flow control and a tight loop can
		// overrun the receiver's socket buffer.
		time.Sleep(10 * time.Millisecond)
	}
}

// dialTLS opens a TLS connection trusting caFile, optionally presenting a
// client certificate. It returns the connection and any handshake error so
// callers can assert on rejection.
func dialTLS(addr, caFile, clientCert, clientKey string) (*tls.Conn, error) {
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA bundle %s", caFile)
	}
	cfg := &tls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	}
	if clientCert != "" && clientKey != "" {
		cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, cfg)
}

// sendTLS sends newline-framed messages over TLS.
func sendTLS(t *testing.T, addr, caFile, clientCert, clientKey string, messages []string) {
	t.Helper()
	conn, err := dialTLS(addr, caFile, clientCert, clientKey)
	require.NoError(t, err, "tls dial %s", addr)
	defer conn.Close()
	for _, msg := range messages {
		_, err := fmt.Fprintf(conn, "%s\n", msg)
		require.NoError(t, err, "write tls message")
	}
}

// startAgentWithConfig renders a config template, starts the agent, and
// registers cleanup. Placeholders let each test target unique log groups.
func startAgentWithConfig(t *testing.T, configPath string, placeholders map[string]string) {
	t.Helper()
	common.CopyFile(configPath, common.ConfigOutputPath)
	if len(placeholders) > 0 {
		require.NoError(t, common.ReplacePlaceholders(common.ConfigOutputPath, placeholders))
	}
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	t.Cleanup(common.StopAgent)
	time.Sleep(agentStartupDelay)
}

// installTLSMaterial stages server (and optionally CA) material where the TLS
// configs expect to find it.
func installTLSMaterial(t *testing.T, withClientCA bool) {
	t.Helper()
	require.NoError(t, common.MkdirAll(tlsCertDir))
	common.CopyFile("resources/tls/server.pem", tlsCertDir+"/server.pem")
	common.CopyFile("resources/tls/server-key.pem", tlsCertDir+"/server-key.pem")
	if withClientCA {
		common.CopyFile("resources/tls/ca.pem", tlsCertDir+"/ca.pem")
	}
}

// TestSyslogTCP covers the basic happy path: RFC 5424 messages over plain TCP
// are received and published to CloudWatch Logs with their content intact.
func TestSyslogTCP(t *testing.T) {
	logGroup := logGroupName("tcp")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "tcp-stream")

	startAgentWithConfig(t, "resources/config_tcp_basic.json", map[string]string{
		"{log_group}": logGroup,
	})

	const want = 10
	marker := fmt.Sprintf("tcp-%d", time.Now().UnixNano())
	msgs := make([]string, want)
	for i := range msgs {
		msgs[i] = rfc5424Msg(1, 6, "test-host", "myapp", fmt.Sprintf("%s message %d", marker, i))
	}

	start := time.Now()
	sendTCP(t, tcpAddr, msgs)
	time.Sleep(sleepForFlush)
	end := time.Now()

	assert.NoError(t, awsservice.ValidateLogs(
		logGroup, "tcp-stream", &start, &end,
		awsservice.AssertLogsCount(want),
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
		awsservice.AssertNoDuplicateLogs(),
	))
}

// TestSyslogUDP covers the basic happy path over UDP.
func TestSyslogUDP(t *testing.T) {
	logGroup := logGroupName("udp")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "udp-stream")

	startAgentWithConfig(t, "resources/config_udp_basic.json", map[string]string{
		"{log_group}": logGroup,
	})

	const want = 10
	marker := fmt.Sprintf("udp-%d", time.Now().UnixNano())
	msgs := make([]string, want)
	for i := range msgs {
		msgs[i] = rfc5424Msg(1, 6, "test-host", "myapp", fmt.Sprintf("%s message %d", marker, i))
	}

	start := time.Now()
	sendUDP(t, udpAddr, msgs)
	time.Sleep(sleepForFlush)
	end := time.Now()

	// UDP is lossless over loopback in practice, but it offers no delivery
	// guarantee. Assert every delivered record is well-formed and that the
	// receiver is working, without making the test flaky on an exact count.
	assert.NoError(t, awsservice.ValidateLogs(
		logGroup, "udp-stream", &start, &end,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
	))
}

// TestSyslogRFC3164 verifies the legacy BSD format is parsed and forwarded.
func TestSyslogRFC3164(t *testing.T) {
	logGroup := logGroupName("rfc3164")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "rfc3164-stream")

	startAgentWithConfig(t, "resources/config_rfc3164.json", map[string]string{
		"{log_group}": logGroup,
	})

	const want = 5
	marker := fmt.Sprintf("bsd-%d", time.Now().UnixNano())
	msgs := make([]string, want)
	for i := range msgs {
		msgs[i] = rfc3164Msg(1, 6, "legacy-host", "syslogd", fmt.Sprintf("%s message %d", marker, i))
	}

	start := time.Now()
	sendTCP(t, tcpAddr, msgs)
	time.Sleep(sleepForFlush)
	end := time.Now()

	assert.NoError(t, awsservice.ValidateLogs(
		logGroup, "rfc3164-stream", &start, &end,
		awsservice.AssertLogsCount(want),
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
	))
}

// TestSyslogMultipleListeners verifies a single syslog section can serve TCP
// and UDP listeners concurrently into the same destination.
func TestSyslogMultipleListeners(t *testing.T) {
	logGroup := logGroupName("multi")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "multi-stream")

	startAgentWithConfig(t, "resources/config_multi_listener.json", map[string]string{
		"{log_group}": logGroup,
	})

	marker := fmt.Sprintf("multi-%d", time.Now().UnixNano())
	start := time.Now()
	sendTCP(t, tcpAddr, []string{
		rfc5424Msg(1, 6, "host-a", "app", marker+" via tcp 1"),
		rfc5424Msg(1, 6, "host-a", "app", marker+" via tcp 2"),
	})
	sendUDP(t, udpAddr, []string{
		rfc5424Msg(1, 6, "host-b", "app", marker+" via udp 1"),
		rfc5424Msg(1, 6, "host-b", "app", marker+" via udp 2"),
	})
	time.Sleep(sleepForFlush)
	end := time.Now()

	events, err := awsservice.GetLogsSince(logGroup, "multi-stream", &start, &end)
	require.NoError(t, err)

	var viaTCP, viaUDP int
	for _, e := range events {
		msg := *e.Message
		if strings.Contains(msg, marker+" via tcp") {
			viaTCP++
		}
		if strings.Contains(msg, marker+" via udp") {
			viaUDP++
		}
	}
	assert.Equal(t, 2, viaTCP, "expected both TCP messages")
	assert.Positive(t, viaUDP, "expected at least one UDP message")
}

// TestSyslogContentFilter verifies an exclude filter drops matching messages
// and forwards the rest.
func TestSyslogContentFilter(t *testing.T) {
	logGroup := logGroupName("filter")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "filter-stream")

	startAgentWithConfig(t, "resources/config_content_filter.json", map[string]string{
		"{log_group}": logGroup,
	})

	marker := fmt.Sprintf("filter-%d", time.Now().UnixNano())
	start := time.Now()
	sendTCP(t, tcpAddr, []string{
		rfc5424Msg(1, 6, "web-01", "nginx", marker+" healthcheck ping"),  // dropped
		rfc5424Msg(1, 6, "web-01", "nginx", marker+" healthcheck again"), // dropped
		rfc5424Msg(1, 6, "web-01", "nginx", marker+" real error occurred"),
		rfc5424Msg(1, 6, "web-01", "nginx", marker+" request processed"),
	})
	time.Sleep(sleepForFlush)
	end := time.Now()

	events, err := awsservice.GetLogsSince(logGroup, "filter-stream", &start, &end)
	require.NoError(t, err)

	var kept int
	for _, e := range events {
		msg := *e.Message
		if !strings.Contains(msg, marker) {
			continue
		}
		kept++
		assert.NotContains(t, msg, "healthcheck", "excluded message was forwarded")
	}
	assert.Equal(t, 2, kept, "expected only non-healthcheck messages")
}

// TestSyslogRouting verifies hostname- and facility-based routing rules deliver
// to their own log groups, with unmatched traffic falling through to the default.
func TestSyslogRouting(t *testing.T) {
	id := instanceID()
	webGroup := fmt.Sprintf("/aws/cwagent/syslog-test/%s/routing-web", id)
	authGroup := fmt.Sprintf("/aws/cwagent/syslog-test/%s/routing-auth", id)
	defaultGroup := fmt.Sprintf("/aws/cwagent/syslog-test/%s/routing-default", id)
	defer awsservice.DeleteLogGroupAndStream(webGroup, "web-stream")
	defer awsservice.DeleteLogGroupAndStream(authGroup, "auth-stream")
	defer awsservice.DeleteLogGroupAndStream(defaultGroup, "default-stream")

	startAgentWithConfig(t, "resources/config_routing.json", map[string]string{
		"{log_group_web}":     webGroup,
		"{log_group_auth}":    authGroup,
		"{log_group_default}": defaultGroup,
	})

	marker := fmt.Sprintf("route-%d", time.Now().UnixNano())
	start := time.Now()
	sendTCP(t, tcpAddr, []string{
		// hostname matches web-* -> web group
		rfc5424Msg(1, 6, "web-01", "nginx", marker+" web request"),
		// facility 4 (auth) -> auth group
		rfc5424Msg(4, 6, "db-01", "sshd", marker+" auth event"),
		// neither -> default group
		rfc5424Msg(1, 6, "db-01", "postgres", marker+" default msg"),
	})
	time.Sleep(sleepForFlush)
	end := time.Now()

	assert.NoError(t, awsservice.ValidateLogs(
		webGroup, "web-stream", &start, &end,
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker+" web request")),
	), "web routing")

	assert.NoError(t, awsservice.ValidateLogs(
		authGroup, "auth-stream", &start, &end,
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker+" auth event")),
	), "facility routing")

	assert.NoError(t, awsservice.ValidateLogs(
		defaultGroup, "default-stream", &start, &end,
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker+" default msg")),
	), "default routing")
}

// TestSyslogTLS verifies messages are accepted over a TLS-encrypted listener.
func TestSyslogTLS(t *testing.T) {
	logGroup := logGroupName("tls")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "tls-stream")

	installTLSMaterial(t, false)
	startAgentWithConfig(t, "resources/config_tls.json", map[string]string{
		"{log_group}": logGroup,
	})

	const want = 5
	marker := fmt.Sprintf("tls-%d", time.Now().UnixNano())
	msgs := make([]string, want)
	for i := range msgs {
		msgs[i] = rfc5424Msg(1, 6, "secure-host", "vault", fmt.Sprintf("%s message %d", marker, i))
	}

	start := time.Now()
	sendTLS(t, tlsAddr, "resources/tls/ca.pem", "", "", msgs)
	time.Sleep(sleepForFlush)
	end := time.Now()

	assert.NoError(t, awsservice.ValidateLogs(
		logGroup, "tls-stream", &start, &end,
		awsservice.AssertLogsCount(want),
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
	))
}

// TestSyslogTLSRejectsPlaintext verifies a TLS listener does not accept
// plaintext syslog traffic. This is a security constraint: a misconfigured or
// malicious client must not be able to bypass encryption by simply speaking
// plain TCP to the TLS port.
func TestSyslogTLSRejectsPlaintext(t *testing.T) {
	logGroup := logGroupName("tls-plaintext")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "tls-stream")

	installTLSMaterial(t, false)
	startAgentWithConfig(t, "resources/config_tls.json", map[string]string{
		"{log_group}": logGroup,
	})

	marker := fmt.Sprintf("plaintext-%d", time.Now().UnixNano())
	start := time.Now()

	// Write plaintext to the TLS port. The TCP connection itself may be
	// accepted, but the TLS handshake never completes, so no syslog record
	// should ever be published.
	conn, err := net.DialTimeout("tcp", tlsAddr, 10*time.Second)
	if err == nil {
		_, _ = fmt.Fprintf(conn, "%s\n", rfc5424Msg(1, 6, "attacker", "curl", marker+" plaintext payload"))
		conn.Close()
	} else {
		log.Printf("plaintext dial to TLS port refused outright: %v", err)
	}

	time.Sleep(sleepForFlush)
	end := time.Now()

	// The log group may not even exist if nothing was ever published, which is
	// itself a pass. Only fail if the plaintext payload actually landed.
	events, err := awsservice.GetLogsSince(logGroup, "tls-stream", &start, &end)
	if err != nil {
		log.Printf("no log stream produced for plaintext traffic (expected): %v", err)
		return
	}
	for _, e := range events {
		assert.NotContains(t, *e.Message, marker,
			"plaintext payload was accepted on a TLS-only listener")
	}
}

// TestSyslogMTLSAcceptsValidClientCert verifies that when client_ca_file is
// configured, a client presenting a certificate signed by that CA is accepted.
func TestSyslogMTLSAcceptsValidClientCert(t *testing.T) {
	logGroup := logGroupName("mtls-valid")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "mtls-stream")

	installTLSMaterial(t, true)
	startAgentWithConfig(t, "resources/config_mtls.json", map[string]string{
		"{log_group}": logGroup,
	})

	const want = 3
	marker := fmt.Sprintf("mtls-%d", time.Now().UnixNano())
	msgs := make([]string, want)
	for i := range msgs {
		msgs[i] = rfc5424Msg(1, 6, "authed-host", "vault", fmt.Sprintf("%s message %d", marker, i))
	}

	start := time.Now()
	sendTLS(t, tlsAddr, "resources/tls/ca.pem",
		"resources/tls/client.pem", "resources/tls/client-key.pem", msgs)
	time.Sleep(sleepForFlush)
	end := time.Now()

	assert.NoError(t, awsservice.ValidateLogs(
		logGroup, "mtls-stream", &start, &end,
		awsservice.AssertLogsCount(want),
		awsservice.AssertPerLog(awsservice.AssertLogContainsSubstring(marker)),
	))
}

// TestSyslogMTLSRejectsMissingClientCert verifies the mTLS security constraint:
// a client that presents no certificate is rejected at the TLS handshake and
// its data never reaches CloudWatch Logs.
func TestSyslogMTLSRejectsMissingClientCert(t *testing.T) {
	logGroup := logGroupName("mtls-reject")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "mtls-stream")

	installTLSMaterial(t, true)
	startAgentWithConfig(t, "resources/config_mtls.json", map[string]string{
		"{log_group}": logGroup,
	})

	marker := fmt.Sprintf("nocert-%d", time.Now().UnixNano())
	start := time.Now()

	// No client certificate: the handshake must fail. Depending on TLS version
	// the error can surface on Handshake() or on the first Write, so attempt to
	// push a payload and require that the exchange does not succeed cleanly.
	conn, err := dialTLS(tlsAddr, "resources/tls/ca.pem", "", "")
	if err == nil {
		defer conn.Close()
		_, werr := fmt.Fprintf(conn, "%s\n", rfc5424Msg(1, 6, "anon", "curl", marker+" unauthenticated"))
		if werr == nil {
			// Force the server's alert to be read.
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			buf := make([]byte, 1)
			_, rerr := conn.Read(buf)
			assert.Error(t, rerr, "server should have terminated the unauthenticated connection")
		}
	} else {
		log.Printf("handshake without client cert rejected as expected: %v", err)
	}

	time.Sleep(sleepForFlush)
	end := time.Now()

	events, err := awsservice.GetLogsSince(logGroup, "mtls-stream", &start, &end)
	if err != nil {
		log.Printf("no log stream produced for unauthenticated client (expected): %v", err)
		return
	}
	for _, e := range events {
		assert.NotContains(t, *e.Message, marker,
			"unauthenticated client's data was published")
	}
}

// TestSyslogMTLSRejectsUntrustedClientCert verifies that a well-formed client
// certificate signed by a CA the agent does not trust is still rejected.
func TestSyslogMTLSRejectsUntrustedClientCert(t *testing.T) {
	logGroup := logGroupName("mtls-untrusted")
	defer awsservice.DeleteLogGroupAndStream(logGroup, "mtls-stream")

	installTLSMaterial(t, true)
	startAgentWithConfig(t, "resources/config_mtls.json", map[string]string{
		"{log_group}": logGroup,
	})

	// Generate a self-signed client cert from an unrelated CA at runtime.
	certPath, keyPath := generateUntrustedClientCert(t)

	marker := fmt.Sprintf("untrusted-%d", time.Now().UnixNano())
	start := time.Now()

	conn, err := dialTLS(tlsAddr, "resources/tls/ca.pem", certPath, keyPath)
	if err == nil {
		defer conn.Close()
		_, werr := fmt.Fprintf(conn, "%s\n", rfc5424Msg(1, 6, "rogue", "curl", marker+" untrusted cert"))
		if werr == nil {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			buf := make([]byte, 1)
			_, rerr := conn.Read(buf)
			assert.Error(t, rerr, "server should have rejected the untrusted client certificate")
		}
	} else {
		log.Printf("untrusted client cert rejected as expected: %v", err)
	}

	time.Sleep(sleepForFlush)
	end := time.Now()

	events, err := awsservice.GetLogsSince(logGroup, "mtls-stream", &start, &end)
	if err != nil {
		log.Printf("no log stream produced for untrusted client (expected): %v", err)
		return
	}
	for _, e := range events {
		assert.NotContains(t, *e.Message, marker,
			"untrusted client certificate was accepted")
	}
}
