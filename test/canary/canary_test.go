// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
//go:build !windows

package canary

import (
	"context"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"log"
	"os"
	"testing"
)

const (
	configInputPath           = "resources/canary_config.json"
	configOutputPath          = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	cloudwatchAgentVersionKey = "release/CWAGENT_VERSION"
	downloadAgentVersionPath  = "./CWAGENT_VERSION"
	installAgentVersionPath   = "/opt/aws/amazon-cloudwatch-agent/bin/CWAGENT_VERSION"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// This copies the canary config, starts the agent, and confirm version is correct.
// Sanity test runs earlier in the terraform code execution
func TestCanary(t *testing.T) {
	// Canary set up
	downloadVersionFile()
	common.CopyFile(configInputPath, configOutputPath)
	common.StartAgent(configOutputPath, true)

	// Version validation
	expectedVersion, err := os.ReadFile(downloadAgentVersionPath)
	if err != nil {
		t.Fatalf("Failure reading downloaded version file %s err %v", downloadAgentVersionPath, err)
	}
	actualVersion, err := os.ReadFile(installAgentVersionPath)
	if err != nil {
		t.Fatalf("Failure reading installed version file %s err %v", installAgentVersionPath, err)
	}
	assert.Equal(t, string(expectedVersion), string(actualVersion))
}

func downloadVersionFile() {
	metadata := environment.GetEnvironmentMetaData(envMetaDataStrings)
	ctx := context.Background()
	c, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to create config")
	}
	file, err := os.Create(downloadAgentVersionPath)
	if err != nil {
		log.Fatalf("Failed to create file %s err %v", downloadAgentVersionPath, err)
	}
	defer file.Close()
	s3Client := s3.NewFromConfig(c)
	s3GetObjectInput := s3.GetObjectInput{
		Bucket: aws.String(metadata.Bucket),
		Key:    aws.String(cloudwatchAgentVersionKey),
	}
	downloader := manager.NewDownloader(s3Client)
	_, err = downloader.Download(ctx, file, &s3GetObjectInput)
	if err != nil {
		log.Fatalf("Can't get cloud agent version from s3 err %v", err)
	}
}
