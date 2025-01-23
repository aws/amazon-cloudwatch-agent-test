// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package entity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	sleepForFlush = 240 * time.Second

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

	// Constants for possible vaues for entity attributes
	eksServiceEntityType                   = "Service"
	entityEKSPlatform                      = "AWS::EKS"
	k8sDefaultNamespace                    = "default"
	entityServiceNameSourceInstrumentation = "Instrumentation"
	entityServiceNameSourceK8sWorkload     = "K8sWorkload"
)

type EntityValidator struct {
	requiredFields  map[string]bool
	expectedEntity  expectedEntity
	platformType    string
	fieldValidators map[string]func(fieldValue string) bool
}

// NewEntityValidator initializes the validator based on the platform type.
func NewEntityValidator(platformType string, expected expectedEntity) *EntityValidator {
	ev := &EntityValidator{
		expectedEntity:  expected,
		platformType:    platformType,
		requiredFields:  make(map[string]bool),
		fieldValidators: make(map[string]func(fieldValue string) bool),
	}

	// Define platform-specific required fields and validators
	if platformType == "EC2" {
		ev.requiredFields = map[string]bool{
			entityType:        false,
			entityName:        false,
			entityEnvironment: false,
			entityPlatform:    false,
			entityInstanceId:  false,
		}
		ev.fieldValidators = map[string]func(fieldValue string) bool{
			entityType:        func(v string) bool { return v == ev.expectedEntity.entityType },
			entityName:        func(v string) bool { return v == ev.expectedEntity.name },
			entityEnvironment: func(v string) bool { return v == ev.expectedEntity.environment },
			entityPlatform:    func(v string) bool { return v == ev.expectedEntity.platformType },
			entityInstanceId:  func(v string) bool { return v == ev.expectedEntity.instanceId },
		}
	} else if platformType == "EKS" {
		ev.requiredFields = map[string]bool{
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
		ev.fieldValidators = map[string]func(fieldValue string) bool{
			entityType:              func(v string) bool { return v == ev.expectedEntity.entityType },
			entityName:              func(v string) bool { return v == ev.expectedEntity.name },
			entityEnvironment:       func(v string) bool { return v == ev.expectedEntity.environment },
			entityPlatform:          func(v string) bool { return v == ev.expectedEntity.platformType },
			entityEKSCluster:        func(v string) bool { return v == ev.expectedEntity.eksCluster },
			entityK8sNode:           func(v string) bool { return v == ev.expectedEntity.k8sNode },
			entityK8sNamespace:      func(v string) bool { return v == ev.expectedEntity.k8sNamespace },
			entityK8sWorkload:       func(v string) bool { return v == ev.expectedEntity.k8sWorkload },
			entityServiceNameSource: func(v string) bool { return v == ev.expectedEntity.serviceNameSource },
		}
	}
	return ev
}

// ValidateField checks if a field is expected and matches the expected value.
func (ev *EntityValidator) ValidateField(field, value string, t *testing.T) {
	if validator, ok := ev.fieldValidators[field]; ok {
		ev.requiredFields[field] = true
		assert.True(t, validator(value), "Validation failed for field %s", field)
	}
}

// AllFieldsPresent ensures all required fields are found.
func (ev *EntityValidator) AllFieldsPresent() bool {
	for _, present := range ev.requiredFields {
		if !present {
			return false
		}
	}
	return true
}

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
				entityType:        eksServiceEntityType,
				name:              "log-generator",
				environment:       "eks:" + env.EKSClusterName + "/" + k8sDefaultNamespace,
				platformType:      entityEKSPlatform,
				k8sWorkload:       "log-generator",
				k8sNode:           *instancePrivateDNS,
				k8sNamespace:      k8sDefaultNamespace,
				eksCluster:        env.EKSClusterName,
				instanceId:        env.InstanceId,
				serviceNameSource: entityServiceNameSourceK8sWorkload,
			},
		},
		"Entity/InstrumentationServiceNameSource": {
			agentConfigPath: filepath.Join("resources", "compass_default_log.json"),
			podName:         "petclinic-instrumentation-default-env",
			expectedEntity: expectedEntity{
				entityType: eksServiceEntityType,
				// This service name comes from OTEL_SERVICE_NAME which is
				// customized in the terraform code when creating the pod
				name:              "petclinic-custom-service-name",
				environment:       "eks:" + env.EKSClusterName + "/" + k8sDefaultNamespace,
				platformType:      entityEKSPlatform,
				k8sWorkload:       "petclinic-instrumentation-default-env",
				k8sNode:           *instancePrivateDNS,
				k8sNamespace:      k8sDefaultNamespace,
				eksCluster:        env.EKSClusterName,
				instanceId:        env.InstanceId,
				serviceNameSource: entityServiceNameSourceInstrumentation,
			},
		},
		"Entity/InstrumentationServiceNameSourceCustomEnvironment": {
			agentConfigPath: filepath.Join("resources", "compass_default_log.json"),
			podName:         "petclinic-instrumentation-custom-env",
			expectedEntity: expectedEntity{
				entityType: eksServiceEntityType,
				// This service name comes from OTEL_SERVICE_NAME which is
				// customized in the terraform code when creating the pod
				name:              "petclinic-custom-service-name",
				environment:       "petclinic-custom-environment",
				platformType:      entityEKSPlatform,
				k8sWorkload:       "petclinic-instrumentation-custom-env",
				k8sNode:           *instancePrivateDNS,
				k8sNamespace:      k8sDefaultNamespace,
				eksCluster:        env.EKSClusterName,
				instanceId:        env.InstanceId,
				serviceNameSource: entityServiceNameSourceInstrumentation,
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
			ValidateLogEntity(t, appLogGroup, podApplicationLogStream, &end, queryString, testCase.expectedEntity, string(env.ComputeType))
		})
	}
}

func TestResourceMetrics(t *testing.T) {
	instanceId := awsservice.GetInstanceId()
	configPath := "resources/config_metrics_resource.json"
	configOutputPath := "resources/config_metrics_resource.json" // can remove?

	// start agent and write metrics
	common.CopyFile(configPath, configOutputPath)
	common.StartAgent(configOutputPath, true, false)
	time.Sleep(4 * time.Minute)
	common.StopAgent()

	// this section builds, signs, and sends the request
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-west-2"))
	assert.NoError(t, err)
	signer := v4.NewSigner()

	body := []byte(fmt.Sprintf(`{
        "Namespace": "CWAgent",
        "MetricName": "cpu_usage_idle",
        "Dimensions": [
            {"Name": "InstanceId", "Value": "%s"},
            {"Name": "cpu", "Value": "cpu-total"}
        ]
    }`, instanceId))

	h := sha256.New()
	h.Write(body)
	payloadHash := hex.EncodeToString(h.Sum(nil))

	// essentially trying to convert this curl command:

	// curl -i -X POST monitoring.us-west-2.amazonaws.com -H 'Content-Type: application/json' \
	//   -H 'Content-Encoding: amz-1.0' \
	//   --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
	//   -H "x-amz-security-token: $AWS_SESSION_TOKEN" \
	//   --aws-sigv4 "aws:amz:us-west-2:monitoring" \
	//   -H 'X-Amz-Target: com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric' \
	//   -d '{
	//     "Namespace": "CWAgent",
	//     "MetricName": "cpu_usage_idle",
	//     "Dimensions": [{"Name": "InstanceId", "Value": "i-0123456789012"}, { "Name": "cpu", "Value": "cpu-total"}]
	//   }'

	// build the request
	req, err := http.NewRequest("POST", "https://monitoring.us-west-2.amazonaws.com/", bytes.NewReader(body))
	assert.NoError(t, err, "Error creating request")

	// set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Target", "com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric")
	req.Header.Set("Content-Encoding", "amz-1.0")

	// set creds
	credentials, err := cfg.Credentials.Retrieve(context.TODO())
	assert.NoError(t, err, "Error getting credentials")

	req.Header.Set("x-amz-security-token", credentials.SessionToken)

	// sign the request
	err = signer.SignHTTP(context.TODO(), credentials, req, payloadHash, "monitoring", "us-west-2", time.Now())
	assert.NoError(t, err, "Error signing the request")

	// send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err, "Error sending the request")
	defer resp.Body.Close()

	// parse and verify the response
	var response struct {
		Entities []struct {
			KeyAttributes struct {
				Type         string `json:"Type"`
				ResourceType string `json:"ResourceType"`
				Identifier   string `json:"Identifier"`
			} `json:"KeyAttributes"`
		} `json:"Entities"`
	}

	err = json.NewDecoder(resp.Body).Decode(&response)
	assert.NoError(t, err, "Error parsing JSON response")

	// Verify the KeyAttributes
	assert.NotEmpty(t, response.Entities, "No entities found in the response")
	entity := response.Entities[0]
	assert.Equal(t, "AWS::Resource", entity.KeyAttributes.Type)
	assert.Equal(t, "AWS::EC2::Instance", entity.KeyAttributes.ResourceType)
	assert.Equal(t, instanceId, entity.KeyAttributes.Identifier)
}

// ValidateLogEntity performs the entity validation for PutLogEvents.
func ValidateLogEntity(t *testing.T, logGroup, logStream string, end *time.Time, queryString string, expectedEntity expectedEntity, entityPlatformType string) {
	log.Printf("Checking log group/stream: %s/%s", logGroup, logStream)
	if !awsservice.IsLogGroupExists(logGroup) {
		t.Fatalf("application log group used for entity validation doesn't exist: %s", logGroup)
	}

	begin := end.Add(-2 * time.Minute)
	log.Printf("Start time is %s and end time is %s", begin.String(), end.String())

	result, err := awsservice.GetLogQueryResults(logGroup, begin.Unix(), end.Unix(), queryString)
	assert.NoError(t, err)
	if !assert.NotZero(t, len(result)) {
		return
	}

	validator := NewEntityValidator(entityPlatformType, expectedEntity)
	for _, field := range result[0] {
		fieldName := aws.ToString(field.Field)
		fieldValue := aws.ToString(field.Value)
		validator.ValidateField(fieldName, fieldValue, t)
		fmt.Printf("%s: %s\n", fieldName, fieldValue)
	}

	assert.True(t, validator.AllFieldsPresent(), "Not all required fields were found")
}
