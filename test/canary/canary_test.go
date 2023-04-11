// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package canary

import (
	"errors"
	"fmt"
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
	defer setupCron()

	installerFilePath := "./downloaded_cwa.rpm"
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
	require.NoError(t, awsservice.ReportMetric("CanaryTest", name, v, types.StandardUnitCount))
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

func setupCron() {
	// default to us-west-2
	region := os.Getenv("AWS_REGION")
	if  region == "" {
		region = "us-west-2"
	}
	bucket := environment.GetEnvironmentMetaData(envMetaDataStrings).Bucket
	// Need to create a temporary file at low privilege.
	// Then use sudo to copy it to the CRON directory.
	src := "resources/canary_test_cron"
	updateCron(src, region, bucket)
	dst := "/etc/cron.d/canary_test_cron"
	common.CopyFile(src, dst)
}

func updateCron(filepath, region, bucket string) {
	s := fmt.Sprintf("MAILTO=\"\"\n*/5 * * * * ec2-user cd /home/ec2-user/amazon-cloudwatch-agent-test && AWS_REGION=%s go test ./test/canary/ -v -p 1 -count=1 -computeType=EC2 -bucket=%s > ./cron_run.log\n", region, bucket)
	b := []byte(s)
	err := os.WriteFile(filepath, b, 0644)
	if err != nil {
		log.Println("error: creating temp cron file")
	}
}