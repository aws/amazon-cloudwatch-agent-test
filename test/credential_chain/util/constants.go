// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package util

import "time"

const (
	PlaceholderNamespace      = "PLACEHOLDER_NAMESPACE"
	PlaceholderProfile        = "PLACEHOLDER_PROFILE"
	PlaceholderCredentialFile = "PLACEHOLDER_CREDENTIAL_FILE"
	PlaceholderAccessKey      = "PLACEHOLDER_ACCESS_KEY"
	PlaceholderSecretKey      = "PLACEHOLDER_SECRET_KEY"
	PlaceholderSessionToken   = "PLACEHOLDER_SESSION_TOKEN"
	PlaceholderTestName       = "PLACEHOLDER_TEST_NAME"
	PlaceholderUser           = "PLACEHOLDER_USER"
)

const (
	SharedTestNamespace      = "CredentialChainTest"
	MetricNameCpuUsageActive = "cpu_usage_active"

	DefaultProfile         = "default"
	DefaultOverrideHomeDir = "/tmp/test-home"
	DefaultAgentRunTime    = 2 * time.Minute

	UserRoot           = "root"
	UserRootHomeDir    = "/root"
	UserCWAgent        = "cwagent"
	UserCWAgentHomeDir = "/home/cwagent"
	AwsCredentialsPath = ".aws/credentials"

	// ProviderNameSharedCredentials is logged by agents built on aws-sdk-go v1, where shared credentials
	// resolve via credentials.SharedCredentialsProvider.
	ProviderNameSharedCredentials = "SharedCredentialsProvider"
	// ProviderNameSharedConfig is logged by agents built on aws-sdk-go-v2, where shared credentials
	// resolve via config.LoadSharedConfigProfile, and by the SDK default chain's shared config resolution.
	ProviderNameSharedConfig = "SharedConfigCredentials"
)

var (
	MeasuredMetrics = []string{MetricNameCpuUsageActive}
)
