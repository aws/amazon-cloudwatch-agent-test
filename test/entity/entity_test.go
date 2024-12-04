// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package entity

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	sleepForFlush = 240 * time.Second

	// Constants for possible values for entity attributes
	eksServiceEntityType                   = "Service"
	entityEKSPlatform                      = "AWS::EKS"
	k8sDefaultNamespace                    = "default"
	entityServiceNameSourceInstrumentation = "Instrumentation"
	entityServiceNameSourceK8sWorkload     = "K8sWorkload"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// TestPutLogEventEntityEKS checks if entity is emitted correctly in EKS
// through FluentBit
func TestPutLogEventEntityEKS(t *testing.T) {
	var instancePrivateDNS *string

	env := environment.GetEnvironmentMetaData()
	assert.NotEmpty(t, env.InstanceId)
	assert.NotEmpty(t, env.EKSClusterName)
	assert.NotEmpty(t, env.ComputeType)
	if env.InstanceId != "" {
		var err error
		instancePrivateDNS, err = awsservice.GetInstancePrivateIpDns(env.InstanceId)
		assert.NoError(t, err)
		assert.NotEmpty(t, instancePrivateDNS)
	}

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the StartQuery call
	log.Printf("Sleeping for %f seconds to ensure log group is ready for query", sleepForFlush.Seconds())
	time.Sleep(sleepForFlush)
	end := time.Now()

	testCases := map[string]struct {
		agentConfigPath string
		podName         string
		useEC2Tag       bool
		expectedEntity  common.ExpectedEntity
	}{
		"Entity/K8sWorkloadServiceNameSource": {
			agentConfigPath: filepath.Join("resources", "compass_default_log.json"),
			podName:         "log-generator",
			expectedEntity: common.ExpectedEntity{
				EntityType:        eksServiceEntityType,
				Name:              "log-generator",
				Environment:       "eks:" + env.EKSClusterName + "/" + k8sDefaultNamespace,
				PlatformType:      entityEKSPlatform,
				K8sWorkload:       "log-generator",
				K8sNode:           *instancePrivateDNS,
				K8sNamespace:      k8sDefaultNamespace,
				EksCluster:        env.EKSClusterName,
				InstanceId:        env.InstanceId,
				ServiceNameSource: entityServiceNameSourceK8sWorkload,
			},
		},
		"Entity/InstrumentationServiceNameSource": {
			agentConfigPath: filepath.Join("resources", "compass_default_log.json"),
			podName:         "petclinic-instrumentation-default-env",
			expectedEntity: common.ExpectedEntity{
				EntityType: eksServiceEntityType,
				// This service name comes from OTEL_SERVICE_NAME which is
				// customized in the terraform code when creating the pod
				Name:              "petclinic-custom-service-name",
				Environment:       "eks:" + env.EKSClusterName + "/" + k8sDefaultNamespace,
				PlatformType:      entityEKSPlatform,
				K8sWorkload:       "petclinic-instrumentation-default-env",
				K8sNode:           *instancePrivateDNS,
				K8sNamespace:      k8sDefaultNamespace,
				EksCluster:        env.EKSClusterName,
				InstanceId:        env.InstanceId,
				ServiceNameSource: entityServiceNameSourceInstrumentation,
			},
		},
		"Entity/InstrumentationServiceNameSourceCustomEnvironment": {
			agentConfigPath: filepath.Join("resources", "compass_default_log.json"),
			podName:         "petclinic-instrumentation-custom-env",
			expectedEntity: common.ExpectedEntity{
				EntityType: eksServiceEntityType,
				// This service name comes from OTEL_SERVICE_NAME which is
				// customized in the terraform code when creating the pod
				Name:              "petclinic-custom-service-name",
				Environment:       "petclinic-custom-environment",
				PlatformType:      entityEKSPlatform,
				K8sWorkload:       "petclinic-instrumentation-custom-env",
				K8sNode:           *instancePrivateDNS,
				K8sNamespace:      k8sDefaultNamespace,
				EksCluster:        env.EKSClusterName,
				InstanceId:        env.InstanceId,
				ServiceNameSource: entityServiceNameSourceInstrumentation,
			},
		},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			var podApplicationLogStream string

			appLogGroup := fmt.Sprintf("/aws/containerinsights/%s/%s", env.EKSClusterName, "application")
			logStreamNames := awsservice.GetLogStreamNames(appLogGroup)
			assert.NotZero(t, len(logStreamNames))
			for _, streamName := range logStreamNames {
				if strings.Contains(streamName, testCase.podName) {
					podApplicationLogStream = streamName
					log.Printf("Found log stream %s that matches pattern %s", streamName, testCase.podName)
				}
			}
			assert.NotEmpty(t, podApplicationLogStream)
			// check CWL to ensure we got the expected entities in the log group
			queryString := fmt.Sprintf("fields @message, @entity.KeyAttributes.Type, @entity.KeyAttributes.Name, @entity.KeyAttributes.Environment, @entity.Attributes.PlatformType, @entity.Attributes.EKS.Cluster, @entity.Attributes.K8s.Node, @entity.Attributes.K8s.Namespace, @entity.Attributes.K8s.Workload, @entity.Attributes.AWS.ServiceNameSource, @entity.Attributes.EC2.InstanceId | filter @logStream == \"%s\"", podApplicationLogStream)
			common.ValidateLogEntity(t, appLogGroup, podApplicationLogStream, &end, queryString, testCase.expectedEntity, string(env.ComputeType))
		})
	}
}