// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package canary

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configInputPath = "resources/canary_config.json"
	// todo: expand testing to include regional S3 links, SSM, ECR.
	downloadLink = "https://amazoncloudwatch-agent.s3.amazonaws.com/amazon_linux/amd64/latest/amazon-cloudwatch-agent.rpm"
	versionLink  = "https://amazoncloudwatch-agent.s3.amazonaws.com/info/latest/CWAGENT_VERSION"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestCanary verifies downloading, installing, and starting the agent.
// Reports metrics for each failure type.
func TestCanary(t *testing.T) {
	//defer setupCron()
	// Don't care if uninstall fails. Agent might not be installed anyways.
	_ = common.UninstallAgent(common.RPM)
	// S3 keys always use backslash, so split on that to get filename.
	installerFilePath := "./" + downloadLink[strings.LastIndex(downloadLink, "/")+1:]
	err := downloadFile(installerFilePath, downloadLink)
	reportMetric(t, "DownloadFail", err)
	err = common.InstallAgent(installerFilePath)
	reportMetric(t, "InstallFail", err)
	common.CopyFile(configInputPath, common.ConfigOutputPath)
	err = common.StartAgent(common.ConfigOutputPath, false, false)
	reportMetric(t, "StartFail", err)
	data, _ := os.ReadFile(common.InstallAgentVersionPath)
	actualVersion := strings.TrimSpace(string(data))
	expectedVersion, _ := getVersionFromS3()
	expectedVersion = strings.TrimSpace(expectedVersion)
	log.Printf("expected version: %s, actual version: %s", expectedVersion, actualVersion)
	if expectedVersion != actualVersion {
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

func getVersionFromS3() (string, error) {
	filename := "./CWAGENT_VERSION"
	err := downloadFile(filename, versionLink)
	if err != nil {
		return "", err
	}
	v, err := os.ReadFile(filename)
	return string(v), err
}

func setupCron() {
	// default to us-west-2
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}
	// Need to create a temporary file at low privilege.
	// Then use sudo to copy it to the CRON directory.
	src := "resources/canary_test_cron"
	updateCron(src, region)
	dst := "/etc/cron.d/canary_test_cron"
	common.CopyFile(src, dst)
}

func updateCron(filepath, region string) {
	// cwd will be something like .../amazon-cloudwatch-agent-test/test/canary/
	cwd, err := os.Getwd()
	log.Printf("cwd %s", cwd)
	if err != nil {
		log.Fatalf("error: Getwd(), %s", err)
	}
	s := fmt.Sprintf("MAILTO=\"\"\n*/5 * * * * ec2-user cd %s && AWS_REGION=%s go test ./ -count=1 -computeType=EC2 > ./cron_run.log\n",
		cwd, region)
	b := []byte(s)
	err = os.WriteFile(filepath, b, 0644)
	if err != nil {
		log.Println("error: creating temp cron file")
	}
}

// downloadFile will download from a given url to a file. It will
// write as it downloads (useful for large files).
func downloadFile(filepath string, url string) error {
	log.Printf("downloading from %s to %s", url, filepath)
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}
