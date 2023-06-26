// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package canary

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"
)

const (
	configInputPath = "resources/canary_config.json"
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
	// S3 keys always use backslash, so split on that to get filename.
	installerFilePath := "./" + e.S3Key[strings.LastIndex(e.S3Key, "/")+1:]
	err := awsservice.DownloadFile(e.Bucket, e.S3Key, installerFilePath)
	reportMetric(t, "DownloadFail", err)
	err = common.InstallAgent(installerFilePath)
	reportMetric(t, "InstallFail", err)
	common.CopyFile(configInputPath, common.ConfigOutputPath)
	err = common.StartAgent(common.ConfigOutputPath, false, false)
	reportMetric(t, "StartFail", err)
	actualVersion, _ := os.ReadFile(common.InstallAgentVersionPath)
	expectedVersion, _ := getVersionFromS3(e.Bucket)
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
		log.Printf("error: name %s, err %s", name, err)
		v += 1
	}
	require.NoError(t, awsservice.ReportMetric("CanaryTest", name, v,
		types.StandardUnitCount))
	require.NoError(t, err)
}

func getVersionFromS3(bucket string) (string, error) {
	filename := "./CWAGENT_VERSION"
	// Assuming the release process will create this s3 key.
	key := "release/CWAGENT_VERSION"
	err := awsservice.DownloadFile(bucket, key, filename)
	if err != nil {
		return "", err
	}
	v, err := os.ReadFile(filename)
	return string(v), err
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
