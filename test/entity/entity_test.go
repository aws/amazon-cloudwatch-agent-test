package entity

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	sleepForFlush = 180 * time.Second

	entityType        = "@entity.KeyAttributes.Type"
	entityName        = "@entity.KeyAttributes.Name"
	entityEnvironment = "@entity.KeyAttributes.Environment"

	entityPlatform          = "@entity.Attributes.PlatformType"
	entityInstanceId        = "@entity.Attributes.EC2.InstanceId"
	entityEKSCluster        = "@entity.Attributes.EKS.Cluster"
	entityK8sNode           = "@entity.Attributes.K8s.Node"
	entityK8sNamespace      = "@entity.Attributes.K8s.Namespace"
	entityK8sWorkload       = "@entity.Attributes.K8s.Workload"
	entityServiceNameSource = "@entity.Attributes.AWS.ServiceNameSource"
)

type expectedEntity struct {
	entityType        string
	name              string
	environment       string
	platformType      string
	k8sWorkload       string
	k8sNode           string
	k8sNamespace      string
	eksCluster        string
	serviceNameSource string
	instanceId        string
}

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
		expectedEntity  expectedEntity
	}{
		"Entity/K8sWorkloadServiceNameSource": {
			agentConfigPath: filepath.Join("resources", "compass_default_log.json"),
			podName:         "log-generator",
			expectedEntity: expectedEntity{
				entityType:        "Service",
				name:              "log-generator",
				environment:       "eks:" + env.EKSClusterName + "/" + "default",
				platformType:      "AWS::EKS",
				k8sWorkload:       "log-generator",
				k8sNode:           *instancePrivateDNS,
				k8sNamespace:      "default",
				eksCluster:        env.EKSClusterName,
				instanceId:        env.InstanceId,
				serviceNameSource: "K8sWorkload",
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
			ValidateEntity(t, appLogGroup, podApplicationLogStream, &end, queryString, testCase.expectedEntity, string(env.ComputeType))
		})
	}
}

// ValidateEntity queries a given LogGroup/LogStream combination given the start and end times, and executes an
// log query for entity attributes to ensure the entity values are correct
func ValidateEntity(t *testing.T, logGroup, logStream string, end *time.Time, queryString string, expectedEntity expectedEntity, entityPlatformType string) {
	var requiredEntityFields map[string]bool

	log.Printf("Checking log group/stream: %s/%s", logGroup, logStream)

	if !awsservice.IsLogGroupExists(logGroup) {
		t.Fatalf("application log group used for entity validation doesn't exsit: %s", logGroup)
	}
	begin := end.Add(-sleepForFlush * 2)
	log.Printf("Start time is " + begin.String() + " and end time is " + end.String())

	result, err := awsservice.GetLogQueryResults(logGroup, begin.Unix(), end.Unix(), queryString)
	assert.NoError(t, err)
	if !assert.NotZero(t, len(result)) {
		return
	}
	if entityPlatformType == "EC2" {
		requiredEntityFields = map[string]bool{
			entityType:        false,
			entityName:        false,
			entityEnvironment: false,
			entityPlatform:    false,
			entityInstanceId:  false,
		}
	} else if entityPlatformType == "EKS" {
		requiredEntityFields = map[string]bool{
			entityType:              false,
			entityName:              false,
			entityEnvironment:       false,
			entityPlatform:          false,
			entityEKSCluster:        false,
			entityK8sNode:           false,
			entityK8sNamespace:      false,
			entityK8sWorkload:       false,
			entityServiceNameSource: false,
		}
	}
	for _, field := range result[0] {
		if entityPlatformType == "EC2" {
			switch aws.ToString(field.Field) {
			case entityType:
				requiredEntityFields[entityType] = true
				assert.Equal(t, expectedEntity.entityType, aws.ToString(field.Value))
			case entityName:
				requiredEntityFields[entityName] = true
				assert.Equal(t, expectedEntity.name, aws.ToString(field.Value))
			case entityEnvironment:
				requiredEntityFields[entityEnvironment] = true
				assert.Equal(t, expectedEntity.environment, aws.ToString(field.Value))
			case entityPlatform:
				requiredEntityFields[entityPlatform] = true
				assert.Equal(t, expectedEntity.platformType, aws.ToString(field.Value))
			case entityInstanceId:
				requiredEntityFields[entityInstanceId] = true
				assert.Equal(t, expectedEntity.instanceId, aws.ToString(field.Value))
			}
		} else {
			switch aws.ToString(field.Field) {
			case entityType:
				requiredEntityFields[entityType] = true
				assert.Equal(t, expectedEntity.entityType, aws.ToString(field.Value))
			case entityName:
				requiredEntityFields[entityName] = true
				assert.Equal(t, expectedEntity.name, aws.ToString(field.Value))
			case entityEnvironment:
				requiredEntityFields[entityEnvironment] = true
				assert.Equal(t, expectedEntity.environment, aws.ToString(field.Value))
			case entityPlatform:
				requiredEntityFields[entityPlatform] = true
				assert.Equal(t, expectedEntity.platformType, aws.ToString(field.Value))
			case entityEKSCluster:
				requiredEntityFields[entityEKSCluster] = true
				assert.Equal(t, expectedEntity.eksCluster, aws.ToString(field.Value))
			case entityK8sNode:
				requiredEntityFields[entityK8sNode] = true
				assert.Equal(t, expectedEntity.k8sNode, aws.ToString(field.Value))
			case entityK8sNamespace:
				requiredEntityFields[entityK8sNamespace] = true
				assert.Equal(t, expectedEntity.k8sNamespace, aws.ToString(field.Value))
			case entityK8sWorkload:
				requiredEntityFields[entityK8sWorkload] = true
				assert.Equal(t, expectedEntity.k8sWorkload, aws.ToString(field.Value))
			case entityServiceNameSource:
				requiredEntityFields[entityServiceNameSource] = true
				assert.Equal(t, expectedEntity.serviceNameSource, aws.ToString(field.Value))
			}
		}

		fmt.Printf("%s: %s\n", aws.ToString(field.Field), aws.ToString(field.Value))
	}
	allEntityFieldsFound := true
	for _, value := range requiredEntityFields {
		if !value {
			allEntityFieldsFound = false
		}
	}
	assert.True(t, allEntityFieldsFound)
}
