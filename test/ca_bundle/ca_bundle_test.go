// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package ca_bundle

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configOutputPath       = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	commonConfigOutputPath = "/opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"
	configJSON             = "/config.json"
	commonConfigTOML       = "/common-config.toml"
	targetString           = "x509: certificate signed by unknown authority"

	localstackS3Key      = "integration-test/ls_tmp/%s"
	keyDelimiter         = "/"
	localstackConfigPath = "../../localstack/ls_tmp/"
	originalPem          = "original.pem"
	combinePem           = "combine.pem"
	snakeOilPem          = "snakeoil.pem"
	tmpDirectory         = "/tmp/"
	runEMF               = "sudo bash resources/emf.sh"
	logfile              = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
)

type input struct {
	findTarget        bool
	commonConfigInput string
	agentConfigInput  string
	testType          string
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// Must run this test with parallel 1 since this will fail if more than one test is running at the same time
// This test uses a pem file created for the local stack endpoint to be able to connect via ssl
func TestBundle(t *testing.T) {
	metadata := environment.GetEnvironmentMetaData()
	t.Logf("metadata required for test cwa sha %s bucket %s ca cert path %s", metadata.CwaCommitSha, metadata.Bucket, metadata.CaCertPath)
	setUpLocalstackConfig(metadata)

	parameters := []input{
		//Use the system pem ca bundle  + local stack pem file ssl should connect thus target string not found
		{commonConfigInput: "resources/with/combine/", agentConfigInput: "resources/https/", findTarget: false, testType: "metric"},
		{commonConfigInput: "resources/with/combine/", agentConfigInput: "resources/https/", findTarget: false, testType: "emf"},
		//Do not look for ca bundle with http connection should connect thus target string not found
		{commonConfigInput: "resources/without/", agentConfigInput: "resources/http/", findTarget: false, testType: "metric"},
		{commonConfigInput: "resources/without/", agentConfigInput: "resources/http/", findTarget: false, testType: "emf"},
		//Use the system pem ca bundle ssl should not connect thus target string found
		{commonConfigInput: "resources/with/original/", agentConfigInput: "resources/https/", findTarget: true, testType: "metric"},
		{commonConfigInput: "resources/with/original/", agentConfigInput: "resources/https/", findTarget: true, testType: "emf"},
		//Do not look for ca bundle should not connect thus target string found
		{commonConfigInput: "resources/without/", agentConfigInput: "resources/https/", findTarget: true, testType: "metric"},
		{commonConfigInput: "resources/without/", agentConfigInput: "resources/https/", findTarget: true, testType: "emf"},
	}

	for _, parameter := range parameters {
		//before test run
		configFile := parameter.agentConfigInput + parameter.testType + configJSON
		commonConfigFile := parameter.commonConfigInput + commonConfigTOML
		log.Printf("common config file location %s agent config file %s find target %t", commonConfigFile, configFile, parameter.findTarget)
		t.Run(fmt.Sprintf("common config file location %s agent config file %s find target %t", commonConfigFile, configFile, parameter.findTarget), func(t *testing.T) {
			common.RecreateAgentLogfile(logfile)
			common.ReplaceLocalStackHostName(configFile)
			t.Logf("config file after localstack host replace %s", string(readFile(configFile)))
			common.CopyFile(configFile, configOutputPath)
			common.CopyFile(commonConfigFile, commonConfigOutputPath)
			common.StartAgent(configOutputPath, true, false)
			// this command will take 5 seconds time 12 = 1 minute
			common.RunCommand(runEMF)
			log.Printf("Agent has been running for : %s", time.Minute)
			common.StopAgent()
			output := common.ReadAgentLogfile(logfile)
			containsTarget := outputLogContainsTarget(output)
			if (parameter.findTarget && !containsTarget) || (!parameter.findTarget && containsTarget) {
				t.Errorf("Find target is %t contains target is %t", parameter.findTarget, containsTarget)
			}
		})
	}
}

func outputLogContainsTarget(output string) bool {
	log.Printf("Log file %s", output)
	contains := strings.Contains(output, targetString)
	log.Printf("Log file contains target string %t", contains)
	return contains
}

// Get localstack pem files
func setUpLocalstackConfig(metadata *environment.MetaData) {
	// Download localstack config files
	prefix := fmt.Sprintf(localstackS3Key, metadata.CwaCommitSha)
	cxt := context.Background()
	cfg, err := config.LoadDefaultConfig(cxt)
	if err != nil {
		log.Fatalf("Can't get config error: %v", err)
	}
	client := s3.NewFromConfig(cfg)
	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(metadata.Bucket),
		Prefix: aws.String(prefix),
	}
	listObjectsOutput, err := client.ListObjectsV2(cxt, listObjectsInput)
	if err != nil {
		log.Fatalf("Got error retrieving list of objects %v", err)
	}
	downloader := manager.NewDownloader(client)
	for _, object := range listObjectsOutput.Contents {
		key := *object.Key
		log.Printf("Download object %s", key)
		keySplit := strings.Split(key, keyDelimiter)
		fileName := keySplit[len(keySplit)-1]
		file, err := os.Create(localstackConfigPath + fileName)
		if err != nil {
			log.Println(err)
		}
		defer file.Close()
		_, err = downloader.Download(cxt, file, &s3.GetObjectInput{
			Bucket: aws.String(metadata.Bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			log.Printf("Error downing file %s error %v", key, err)
		}
	}

	// generate localstack crt files
	writeFile(localstackConfigPath+originalPem, readFile(metadata.CaCertPath))
	writeFile(localstackConfigPath+combinePem, readFile(metadata.CaCertPath))
	writeFile(localstackConfigPath+combinePem, readFile(localstackConfigPath+snakeOilPem))

	// copy crt files to agent directory
	writeFile(tmpDirectory+originalPem, readFile(localstackConfigPath+originalPem))
	writeFile(tmpDirectory+combinePem, readFile(localstackConfigPath+combinePem))
}

func readFile(path string) []byte {
	file, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read file %s error %v", path, err)
	}
	return file
}

func writeFile(path string, output []byte) {
	err := os.WriteFile(path, output, 0644)
	if err != nil {
		log.Fatalf("Error writting file %s, error %v,", path, err)
	}
}
