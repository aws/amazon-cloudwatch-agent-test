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

	ProviderNameSharedCredentials = "SharedCredentialsProvider"
	ProviderNameSharedConfig      = "SharedConfigCredentials"
)

var (
	MeasuredMetrics = []string{MetricNameCpuUsageActive}
)
