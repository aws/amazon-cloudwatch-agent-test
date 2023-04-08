// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package canary

import (
	"errors"
	"log"
	"os"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"
)

const (
	configInputPath           = "resources/canary_config.json"
	configOutputPath          = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	installAgentVersionPath   = "/opt/aws/amazon-cloudwatch-agent/bin/CWAGENT_VERSION"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// TestCanary verifies customers can download the agent, install it, and it "works".
// Reports metrics on the number of download failures, install failures, and start failures.
func TestCanary(t *testing.T) {
	installerFilePath := "/tmp/downloaded_cwa.rpm"
	err := downloadInstaller(installerFilePath)
	reportMetric(t, "DownloadFail", err)

	// Don't care if uninstall fails. Agent might not be installed anyways.
	_ = common.UninstallAgent()

	err = common.InstallAgent(installerFilePath)
	reportMetric(t, "InstallFail", err)

	common.CopyFile(configInputPath, configOutputPath)
	err = common.StartAgent(configOutputPath, false)
	reportMetric(t, "StartFail", err)

	actualVersion, _ := os.ReadFile(installAgentVersionPath)
	expectedVersion, _ := getVersionFromS3()
	if expectedVersion != string(actualVersion) {
		err = errors.New("agent version mismatch")
	}
	reportMetric(t, "VersionFail", err)
}

// reportMetric is just a helper to report a metric and conditionally fail
// the test based on the error.
func reportMetric(t *testing.T, name string, err error) {
	var v float64 = 0
	if err != nil {
		log.Printf("error: name %v, err %v", name, err)
		v += 1
	}
	awsservice.ReportMetric("CanaryTest", name, v, types.StandardUnitCount)
	require.NoError(t, err)
}

func downloadInstaller(filepath string) error {
	bucket := environment.GetEnvironmentMetaData(envMetaDataStrings).Bucket
	key := "release/amazon_linux/amd64/latest/amazon-cloudwatch-agent.rpm"
	return awsservice.DownloadFile(bucket, key, filepath)
}

func getVersionFromS3() (string, error) {
	filename := "./CWAGENT_VERSION"
	bucket := environment.GetEnvironmentMetaData(envMetaDataStrings).Bucket
	key := "release/CWAGENT_VERSION"
	err := awsservice.DownloadFile(bucket, key, filename)
	if err != nil {
		return "", err
	}
	v, err := os.ReadFile(filename)
	return string(v), err
}