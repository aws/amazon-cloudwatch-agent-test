// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package syslog

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// generateUntrustedClientCert creates a self-signed client certificate from a
// CA that the agent does not trust, and writes the cert/key to a temp dir.
// It returns the cert and key paths.
//
// This exists so the mTLS rejection test exercises a genuinely untrusted chain
// rather than a malformed certificate — the server must reject it because it
// cannot verify the issuer, not because the material failed to parse.
func generateUntrustedClientCert(t *testing.T) (string, string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generate untrusted client key")

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "untrusted-syslog-client",
			Organization: []string{"Untrusted Test CA"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Self-signed: the certificate is its own issuer, so no CA in the agent's
	// client_ca_file can validate it.
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err, "create untrusted client certificate")

	dir := t.TempDir()
	certPath := filepath.Join(dir, "untrusted-client.pem")
	keyPath := filepath.Join(dir, "untrusted-client-key.pem")

	certOut, err := os.Create(certPath)
	require.NoError(t, err)
	defer certOut.Close()
	require.NoError(t, pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyOut, err := os.Create(keyPath)
	require.NoError(t, err)
	defer keyOut.Close()
	require.NoError(t, pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	return certPath, keyPath
}
