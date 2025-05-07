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
	"github.com/aws/amazon-cloudwatch-agent-test/environment/eksinstallationtype"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	DefaultEC2AgentStartCommand = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c "
)

var metaDataStorage *MetaData = nil
var registeredMetaDataStrings = &(MetaDataStrings{})

type MetaData struct {
	ComputeType                                 computetype.ComputeType
	EcsLaunchType                               ecslaunchtype.ECSLaunchType
	EcsDeploymentStrategy                       ecsdeploymenttype.ECSDeploymentType
	EksDeploymentStrategy                       eksdeploymenttype.EKSDeploymentType
	EcsClusterArn                               string
	EcsClusterName                              string
	CwagentConfigSsmParamName                   string
	EcsServiceName                              string
	EC2PluginTests                              map[string]struct{} // set of EC2 plugin names
	ExcludedTests                               map[string]struct{} // set of excluded names
	Bucket                                      string
	S3Key                                       string
	CwaCommitSha                                string
	CaCertPath                                  string
	EKSClusterName                              string
	ProxyUrl                                    string
	AssumeRoleArn                               string
	InstanceArn                                 string
	InstanceId                                  string
	InstancePlatform                            string
	AgentStartCommand                           string
	EksGpuType                                  string
	AmpWorkspaceId                              string
	Region                                      string
	K8sVersion                                  string
	Destroy                                     bool
	HelmChartsBranch                            string
	CloudwatchAgentRepository                   string
	CloudwatchAgentTag                          string
	CloudwatchAgentRepositoryURL                string
	CloudwatchAgentOperatorRepository           string
	CloudwatchAgentOperatorTag                  string
	CloudwatchAgentOperatorRepositoryURL        string
	CloudwatchAgentTargetAllocatorRepository    string
	CloudwatchAgentTargetAllocatorTag           string
	CloudwatchAgentTargetAllocatorRepositoryURL string
	AgentConfig                                 string
	PrometheusConfig                            string
	OtelConfig                                  string
	SampleApp                                   string
	AccountId                                   string
	EKSInstallationType                         eksinstallationtype.EKSInstallationType
	EnableTargetAllocator                       bool
}

type MetaDataStrings struct {
	ComputeType                                 string
	EcsLaunchType                               string
	EcsDeploymentStrategy                       string
	EksDeploymentStrategy                       string
	EcsClusterArn                               string
	CwagentConfigSsmParamName                   string
	EcsServiceName                              string
	EC2PluginTests                              string // input comma delimited list of plugin names
	ExcludedTests                               string // Exclude specific tests from OS
	Bucket                                      string
	S3Key                                       string
	CwaCommitSha                                string
	CaCertPath                                  string
	EKSClusterName                              string
	ProxyUrl                                    string
	AssumeRoleArn                               string
	InstanceArn                                 string
	InstanceId                                  string
	InstancePlatform                            string
	AgentStartCommand                           string
	EksGpuType                                  string
	AmpWorkspaceId                              string
	Region                                      string
	K8sVersion                                  string
	Destroy                                     bool
	HelmChartsBranch                            string
	CloudwatchAgentRepository                   string
	CloudwatchAgentTag                          string
	CloudwatchAgentRepositoryURL                string
	CloudwatchAgentOperatorRepository           string
	CloudwatchAgentOperatorTag                  string
	CloudwatchAgentOperatorRepositoryURL        string
	CloudwatchAgentTargetAllocatorRepository    string
	CloudwatchAgentTargetAllocatorTag           string
	CloudwatchAgentTargetAllocatorRepositoryURL string
	AgentConfig                                 string
	PrometheusConfig                            string
	OtelConfig                                  string
	SampleApp                                   string
	AccountId                                   string
	EKSInstallationType                         string
	EnableTargetAllocator                       bool
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
	flag.StringVar(&(d.EksGpuType), "eksGpuType", "", "nvidia/inferentia")
}

func registerEKSE2ETestData(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.Region), "region", "", "AWS region")
	flag.StringVar(&(dataString.K8sVersion), "k8s_version", "", "Kubernetes version")
	flag.BoolVar(&(dataString.Destroy), "destroy", false, "Whether to run in destroy mode (true/false)")
	flag.StringVar(&(dataString.HelmChartsBranch), "helm_charts_branch", "", "Helm charts branch")
	flag.StringVar(&(dataString.CloudwatchAgentRepository), "cloudwatch_agent_repository", "", "CloudWatch Agent repository")
	flag.StringVar(&(dataString.CloudwatchAgentTag), "cloudwatch_agent_tag", "", "CloudWatch Agent tag")
	flag.StringVar(&(dataString.CloudwatchAgentRepositoryURL), "cloudwatch_agent_repository_url", "", "CloudWatch Agent repository URL")
	flag.StringVar(&(dataString.CloudwatchAgentOperatorRepository), "cloudwatch_agent_operator_repository", "", "CloudWatch Agent Operator repository")
	flag.StringVar(&(dataString.CloudwatchAgentOperatorTag), "cloudwatch_agent_operator_tag", "", "CloudWatch Agent Operator tag")
	flag.StringVar(&(dataString.CloudwatchAgentOperatorRepositoryURL), "cloudwatch_agent_operator_repository_url", "", "CloudWatch Agent Operator repository URL")
	flag.StringVar(&(dataString.CloudwatchAgentTargetAllocatorRepository), "cloudwatch_agent_target_allocator_repository", "", "CloudWatch Agent Target Allocator repository")
	flag.StringVar(&(dataString.CloudwatchAgentTargetAllocatorTag), "cloudwatch_agent_target_allocator_tag", "", "CloudWatch Agent Target Allocator tag")
	flag.StringVar(&(dataString.CloudwatchAgentTargetAllocatorRepositoryURL), "cloudwatch_agent_target_allocator_repository_url", "", "CloudWatch Agent Target Allocator repository URL")
	flag.StringVar(&(dataString.AgentConfig), "agent_config", "", "Agent configuration file path")
	flag.StringVar(&(dataString.PrometheusConfig), "prometheus_config", "", "Prometheus configuration file path")
	flag.StringVar(&(dataString.OtelConfig), "otel_config", "", "OpenTelemetry configuration file path")
	flag.StringVar(&(dataString.SampleApp), "sample_app", "", "Sample application manifest file path")
	flag.StringVar(&(dataString.EKSInstallationType), "eks_installation_type", "HELM_CHART", "Installation type (HELM_CHART or EKS_ADDON)")
	flag.BoolVar(&(dataString.EnableTargetAllocator), "enable_target_allocator", false, "Whether to enable target allocator (true/false")

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

func registerInstanceArn(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.InstanceArn), "instanceArn", "", "ec2 instance ARN that is being used by a test")
}

func registerInstanceId(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.InstanceId), "instanceId", "", "ec2 instance ID that is being used by a test")
}

func registerInstancePlatform(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.InstancePlatform), "instancePlatform", "linux", "ec2 instance OS that is being used for a test")
}

func registerAgentStartCommand(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.AgentStartCommand), "agentStartCommand",
		DefaultEC2AgentStartCommand,
		"Start command is different ec2 vs onprem, linux vs windows. Default set above is for EC2 with Linux")
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
	e.EksGpuType = data.EksGpuType
}

func fillEKSInstallationType(e *MetaData, data *MetaDataStrings) {
	if e.ComputeType != computetype.EKS {
		return
	}

	installationType, ok := eksinstallationtype.FromString(data.EKSInstallationType)
	if !ok {
		log.Printf("Invalid installation type %s. Defaulting to HELM_CHART", data.EKSInstallationType)
		e.EKSInstallationType = eksinstallationtype.HELM_CHART
	} else {
		e.EKSInstallationType = installationType
	}
}

func registerAmpWorkspaceId(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.AmpWorkspaceId), "ampWorkspaceId", "", "workspace Id for Amazon Managed Prometheus (AMP)")
}

func registerAccountId(dataString *MetaDataStrings) {
	flag.StringVar(&(dataString.AccountId), "accountId", "", "AWS account Id")
}

func RegisterEnvironmentMetaDataFlags() *MetaDataStrings {
	registerComputeType(registeredMetaDataStrings)
	registerECSData(registeredMetaDataStrings)
	registerEKSData(registeredMetaDataStrings)
	registerEKSE2ETestData(registeredMetaDataStrings)
	registerBucket(registeredMetaDataStrings)
	registerS3Key(registeredMetaDataStrings)
	registerCwaCommitSha(registeredMetaDataStrings)
	registerCaCertPath(registeredMetaDataStrings)
	registerPluginTestsToExecute(registeredMetaDataStrings)
	registerExcludedTests(registeredMetaDataStrings)
	registerProxyUrl(registeredMetaDataStrings)
	registerAssumeRoleArn(registeredMetaDataStrings)
	registerInstanceArn(registeredMetaDataStrings)
	registerInstanceId(registeredMetaDataStrings)
	registerInstancePlatform(registeredMetaDataStrings)
	registerAgentStartCommand(registeredMetaDataStrings)
	registerAmpWorkspaceId(registeredMetaDataStrings)
	registerAccountId(registeredMetaDataStrings)

	return registeredMetaDataStrings
}

func GetEnvironmentMetaData() *MetaData {
	if metaDataStorage != nil {
		return metaDataStorage
	}

	metaDataStorage := &(MetaData{})
	fillComputeType(metaDataStorage, registeredMetaDataStrings)
	fillECSData(metaDataStorage, registeredMetaDataStrings)
	fillEKSData(metaDataStorage, registeredMetaDataStrings)
	fillEC2PluginTests(metaDataStorage, registeredMetaDataStrings)
	fillExcludedTests(metaDataStorage, registeredMetaDataStrings)
	metaDataStorage.Bucket = registeredMetaDataStrings.Bucket
	metaDataStorage.S3Key = registeredMetaDataStrings.S3Key
	metaDataStorage.CwaCommitSha = registeredMetaDataStrings.CwaCommitSha
	metaDataStorage.CaCertPath = registeredMetaDataStrings.CaCertPath
	metaDataStorage.ProxyUrl = registeredMetaDataStrings.ProxyUrl
	metaDataStorage.AssumeRoleArn = registeredMetaDataStrings.AssumeRoleArn
	metaDataStorage.InstanceArn = registeredMetaDataStrings.InstanceArn
	metaDataStorage.InstanceId = registeredMetaDataStrings.InstanceId
	metaDataStorage.InstancePlatform = registeredMetaDataStrings.InstancePlatform
	metaDataStorage.AgentStartCommand = registeredMetaDataStrings.AgentStartCommand
	metaDataStorage.EksGpuType = registeredMetaDataStrings.EksGpuType
	metaDataStorage.AmpWorkspaceId = registeredMetaDataStrings.AmpWorkspaceId
	metaDataStorage.Region = registeredMetaDataStrings.Region
	metaDataStorage.K8sVersion = registeredMetaDataStrings.K8sVersion
	metaDataStorage.Destroy = registeredMetaDataStrings.Destroy
	metaDataStorage.HelmChartsBranch = registeredMetaDataStrings.HelmChartsBranch
	metaDataStorage.CloudwatchAgentRepository = registeredMetaDataStrings.CloudwatchAgentRepository
	metaDataStorage.CloudwatchAgentTag = registeredMetaDataStrings.CloudwatchAgentTag
	metaDataStorage.CloudwatchAgentRepositoryURL = registeredMetaDataStrings.CloudwatchAgentRepositoryURL
	metaDataStorage.CloudwatchAgentOperatorRepository = registeredMetaDataStrings.CloudwatchAgentOperatorRepository
	metaDataStorage.CloudwatchAgentOperatorTag = registeredMetaDataStrings.CloudwatchAgentOperatorTag
	metaDataStorage.CloudwatchAgentOperatorRepositoryURL = registeredMetaDataStrings.CloudwatchAgentOperatorRepositoryURL
	metaDataStorage.CloudwatchAgentTargetAllocatorRepository = registeredMetaDataStrings.CloudwatchAgentTargetAllocatorRepository
	metaDataStorage.CloudwatchAgentTargetAllocatorTag = registeredMetaDataStrings.CloudwatchAgentTargetAllocatorTag
	metaDataStorage.CloudwatchAgentTargetAllocatorRepositoryURL = registeredMetaDataStrings.CloudwatchAgentTargetAllocatorRepositoryURL
	metaDataStorage.AgentConfig = registeredMetaDataStrings.AgentConfig
	metaDataStorage.PrometheusConfig = registeredMetaDataStrings.PrometheusConfig
	metaDataStorage.OtelConfig = registeredMetaDataStrings.OtelConfig
	metaDataStorage.SampleApp = registeredMetaDataStrings.SampleApp
	metaDataStorage.AccountId = registeredMetaDataStrings.AccountId
	metaDataStorage.EnableTargetAllocator = registeredMetaDataStrings.EnableTargetAllocator
	fillEKSInstallationType(metaDataStorage, registeredMetaDataStrings)

	return metaDataStorage
}
