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
	
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configInputPath = "resources/canary_config.json"
	versionLink     = "https://amazoncloudwatch-agent.s3.amazonaws.com/info/latest/CWAGENT_VERSION"
	rpm             = "https://amazoncloudwatch-agent.s3.amazonaws.com/amazon_linux/amd64/latest/amazon-cloudwatch-agent.rpm"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestCanary verifies downloading, installing, and starting the agent.
// Reports metrics for each failure type.
func TestCanary(t *testing.T) {
	e := environment.GetEnvironmentMetaData()
	defer setupCron(e.Bucket, e.S3Key)
	// Don't care if uninstall fails. Agent might not be installed anyways.
	_ = common.UninstallAgent(common.RPM)
	installerFilePath := "./amazon-cloudwatch-agent.rpm"
	err := common.DownloadFile(installerFilePath, rpm)
	reportMetric(t, "DownloadFail", err)
	err = common.InstallAgent(installerFilePath)
	reportMetric(t, "InstallFail", err)
	common.CopyFile(configInputPath, common.ConfigOutputPath)
	err = common.StartAgent(common.ConfigOutputPath, false, false)
	reportMetric(t, "StartFail", err)
	actualVersion, _ := os.ReadFile(common.InstallAgentVersionPath)
	versionPath := "./CWAGENT_VERSION"
	err = common.DownloadFile(versionPath, versionLink)
	reportMetric(t, "DownloadFail for version", err)
	expectedVersion, _ := os.ReadFile(versionPath)

	if string(expectedVersion) != string(actualVersion) {
		err = errors.New("agent version mismatch")
	}
	reportMetric(t, "VersionFail", err)
}

// reportMetric is just a helper to report a metric and conditionally fail
// the test based on the error.
func reportMetric(t *testing.T, name string, err error) {
	var v float64 = 0
	if err != nil {
		log.Printf("error: name %s, err %s", name, err)
		v += 1
	}
	require.NoError(t, awsservice.ReportMetric("CanaryTest", name, v,
		types.StandardUnitCount))
	require.NoError(t, err)
}

func setupCron(bucket, key string) {
	// default to us-west-2
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}

	// Need to create a temporary file at low privilege.
	// Then use sudo to copy it to the CRON directory.
	src := "resources/canary_test_cron"
	updateCron(src, region, bucket, key)
	dst := "/etc/cron.d/canary_test_cron"
	common.CopyFile(src, dst)
}

func updateCron(filepath, region, bucket, s3Key string) {
	// cwd will be something like .../amazon-cloudwatch-agent-test/test/canary/
	cwd, err := os.Getwd()
	log.Printf("cwd %s", cwd)
	if err != nil {
		log.Fatalf("error: Getwd(), %s", err)
	}
	s := fmt.Sprintf("MAILTO=\"\"\n*/5 * * * * ec2-user cd %s && AWS_REGION=%s go test ./ -count=1 -computeType=EC2 -bucket=%s -s3key=%s > ./cron_run.log\n",
		cwd, region, bucket, s3Key)
	b := []byte(s)
	err = os.WriteFile(filepath, b, 0644)
	if err != nil {
		log.Println("error: creating temp cron file")
	}
}
