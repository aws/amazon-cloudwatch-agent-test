// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package environment

import (
	"flag"
	"log"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecsdeploymenttype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/ecslaunchtype"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/eksdeploymenttype"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

type MetaData struct {
	ComputeType               computetype.ComputeType
	EcsLaunchType             ecslaunchtype.ECSLaunchType
	EcsDeploymentStrategy     ecsdeploymenttype.ECSDeploymentType
	EksDeploymentStrategy     eksdeploymenttype.EKSDeploymentType
	EcsClusterArn             string
	EcsClusterName            string
	CwagentConfigSsmParamName string
	EcsServiceName            string
	EC2PluginTests            map[string]struct{} // set of EC2 plugin names
	ExcludedTests             map[string]struct{} // set of excluded names
	Bucket                    string
	S3Key                     string
	CwaCommitSha              string
	CaCertPath                string
	EKSClusterName            string
	ProxyUrl                  string
	AssumeRoleArn             string
	InstanceId                string
}

type MetaDataStrings struct {
	ComputeType               string
	EcsLaunchType             string
	EcsDeploymentStrategy     string
	EksDeploymentStrategy     string
	EcsClusterArn             string
	CwagentConfigSsmParamName string
	EcsServiceName            string
	EC2PluginTests            string // input comma delimited list of plugin names
	ExcludedTests             string // Exclude specific tests from OS
	Bucket                    string
	S3Key                     string
	CwaCommitSha              string
	CaCertPath                string
	EKSClusterName            string
	ProxyUrl                  string
	AssumeRoleArn             string
	InstanceId                string
}

func registerComputeType(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.ComputeType), "computeType", "", "EC2/ECS/EKS")
}
func registerBucket(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.Bucket), "bucket", "", "s3 bucket ex cloudwatch-agent-integration-bucket")
}
func registerS3Key(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.S3Key), "s3key", "release/amazon_linux/amd64/latest/amazon-cloudwatch-agent.rpm",
		"s3 key ex cloudwatch-agent-integration-bucket")
}
func registerCwaCommitSha(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.CwaCommitSha), "cwaCommitSha", "", "agent commit hash ex 0b81ac79ee13f5248b860bbda3afc4ee57f5b8b6")
}
func registerCaCertPath(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.CaCertPath), "caCertPath", "", "ec2 path to crts ex /etc/ssl/certs/ca-certificates.crt")
}
func registerECSData(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.EcsLaunchType), "ecsLaunchType", "", "EC2 or Fargate")
	flag.StringVar(&(dataString.EcsDeploymentStrategy), "ecsDeploymentStrategy", "", "Daemon/Replica/Sidecar")
	flag.StringVar(&(dataString.EcsClusterArn), "clusterArn", "", "Used to restart ecs task to apply new agent config")
	flag.StringVar(&(dataString.CwagentConfigSsmParamName), "cwagentConfigSsmParamName", "", "Used to set new cwa config")
	flag.StringVar(&(dataString.EcsServiceName), "cwagentECSServiceName", "", "Used to restart ecs task to apply new agent config")
}

func registerEKSData(d *MetaDataStrings) {
	flag.StringVar(&(d.EKSClusterName), "eksClusterName", "", "EKS cluster name")
	flag.StringVar(&(d.EksDeploymentStrategy), "eksDeploymentStrategy", "", "Daemon/Replica/Sidecar")
}

func registerPluginTestsToExecute(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.EC2PluginTests), "plugins", "", "Comma-delimited list of plugins to test. Default is empty, which tests all")
}

func registerExcludedTests(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.ExcludedTests), "excludedTests", "", "Comma-delimited list of test to exclude. Default is empty, which tests all")
}

func registerProxyUrl(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.ProxyUrl), "proxyUrl", "", "Public IP address of a proxy instance. Default is empty with no proxy instance being used")
}

func fillComputeType(e *MetaData, data *MetaDataStrings) {
	computeType, ok := computetype.FromString(data.ComputeType)
	if !ok {
		log.Panic("Invalid compute type. Needs to be EC2/ECS/EKS. Compute Type is a required flag. :" + data.ComputeType)
	}
	e.ComputeType = computeType
}

func registerAssumeRoleArn(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.AssumeRoleArn), "assumeRoleArn", "", "Arn for assume role to be used")
}

func registerInstanceId(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.InstanceId), "instanceId", "", "ec2 instance ID that is being used by a test")
}

func fillECSData(e *MetaData, data *MetaDataStrings) {
	if e.ComputeType != computetype.ECS {
		return
	}

	ecsLaunchType, ok := ecslaunchtype.FromString(data.EcsLaunchType)
	if !ok {
		log.Printf("Invalid launch type %s. This might be because it wasn't provided for non-ECS tests", data.ComputeType)
	} else {
		e.EcsLaunchType = ecsLaunchType
	}

	ecsDeploymentStrategy, ok := ecsdeploymenttype.FromString(data.EcsDeploymentStrategy)
	if !ok {
		log.Printf("Invalid deployment strategy %s. This might be because it wasn't provided for non-ECS tests", data.ComputeType)
	} else {
		e.EcsDeploymentStrategy = ecsDeploymentStrategy
	}

	e.EcsClusterArn = data.EcsClusterArn
	e.CwagentConfigSsmParamName = data.CwagentConfigSsmParamName
	e.EcsServiceName = data.EcsServiceName
	e.EcsClusterName = awsservice.GetClusterName(data.EcsClusterArn)
}

func fillEC2PluginTests(e *MetaData, data *MetaDataStrings) {
	if e.ComputeType != computetype.EC2 {
		return
	}

	if len(data.EC2PluginTests) == 0 {
		log.Println("Testing all EC2 plugins")
		return
	}

	plugins := strings.Split(strings.ReplaceAll(data.EC2PluginTests, " ", ""), ",")
	log.Printf("Executing subset of plugin tests: %v", plugins)
	m := make(map[string]struct{}, len(plugins))
	for _, p := range plugins {
		m[strings.ToLower(p)] = struct{}{}
	}
	e.EC2PluginTests = m
}

func fillExcludedTests(e *MetaData, data *MetaDataStrings) {
	if e.ComputeType != computetype.EC2 {
		return
	}

	if len(data.ExcludedTests) == 0 {
		log.Println("Testing all EC2 plugins")
		return
	}

	plugins := strings.Split(strings.ReplaceAll(data.ExcludedTests, " ", ""), ",")
	log.Printf("Excluding subset of tests: %v", plugins)
	m := make(map[string]struct{}, len(plugins))
	for _, p := range plugins {
		m[strings.ToLower(p)] = struct{}{}
	}
	e.ExcludedTests = m
}

func fillEKSData(e *MetaData, data *MetaDataStrings) {
	if e.ComputeType != computetype.EKS {
		return
	}

	eksDeploymentStrategy, ok := eksdeploymenttype.FromString(data.EksDeploymentStrategy)
	if !ok {
		log.Printf("Invalid deployment strategy %s. This might be because it wasn't provided for non-EKS tests", data.ComputeType)
	} else {
		e.EksDeploymentStrategy = eksDeploymentStrategy
	}

	e.EKSClusterName = data.EKSClusterName
}

func RegisterEnvironmentMetaDataFlags(metaDataStrings *MetaDataStrings) *MetaDataStrings {
	registerComputeType(metaDataStrings)
	registerECSData(metaDataStrings)
	registerEKSData(metaDataStrings)
	registerBucket(metaDataStrings)
	registerS3Key(metaDataStrings)
	registerCwaCommitSha(metaDataStrings)
	registerCaCertPath(metaDataStrings)
	registerPluginTestsToExecute(metaDataStrings)
	registerExcludedTests(metaDataStrings)
	registerProxyUrl(metaDataStrings)
	registerAssumeRoleArn(metaDataStrings)
	registerInstanceId(metaDataStrings)
	return metaDataStrings
}

func GetEnvironmentMetaData(data *MetaDataStrings) *MetaData {
	metaData := &(MetaData{})
	fillComputeType(metaData, data)
	fillECSData(metaData, data)
	fillEKSData(metaData, data)
	fillEC2PluginTests(metaData, data)
	fillExcludedTests(metaData, data)
	metaData.Bucket = data.Bucket
	metaData.S3Key = data.S3Key
	metaData.CwaCommitSha = data.CwaCommitSha
	metaData.CaCertPath = data.CaCertPath
	metaData.ProxyUrl = data.ProxyUrl
	metaData.AssumeRoleArn = data.AssumeRoleArn
	metaData.InstanceId = data.InstanceId
	return metaData
}
