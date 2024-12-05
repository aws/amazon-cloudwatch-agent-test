// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
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

type ExpectedEntity struct {
	EntityType        string
	Name              string
	Environment       string
	PlatformType      string
	K8sWorkload       string
	K8sNode           string
	K8sNamespace      string
	EksCluster        string
	ServiceNameSource string
	InstanceId        string
}

type EntityValidator struct {
	requiredFields  map[string]bool
	expectedEntity  ExpectedEntity
	platformType    string
	fieldValidators map[string]func(fieldValue string) bool
}

// NewEntityValidator initializes the validator based on the platform type.
func NewEntityValidator(platformType string, expected ExpectedEntity) *EntityValidator {
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
			entityType:        func(v string) bool { return v == ev.expectedEntity.EntityType },
			entityName:        func(v string) bool { return v == ev.expectedEntity.Name },
			entityEnvironment: func(v string) bool { return v == ev.expectedEntity.Environment },
			entityPlatform:    func(v string) bool { return v == ev.expectedEntity.PlatformType },
			entityInstanceId:  func(v string) bool { return v == ev.expectedEntity.InstanceId },
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
			entityType:              func(v string) bool { return v == ev.expectedEntity.EntityType },
			entityName:              func(v string) bool { return v == ev.expectedEntity.Name },
			entityEnvironment:       func(v string) bool { return v == ev.expectedEntity.Environment },
			entityPlatform:          func(v string) bool { return v == ev.expectedEntity.PlatformType },
			entityEKSCluster:        func(v string) bool { return v == ev.expectedEntity.EksCluster },
			entityK8sNode:           func(v string) bool { return v == ev.expectedEntity.K8sNode },
			entityK8sNamespace:      func(v string) bool { return v == ev.expectedEntity.K8sNamespace },
			entityK8sWorkload:       func(v string) bool { return v == ev.expectedEntity.K8sWorkload },
			entityServiceNameSource: func(v string) bool { return v == ev.expectedEntity.ServiceNameSource },
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

// ValidateLogEntity validates the entity data for both EC2 and EKS
func ValidateLogEntity(t *testing.T, logGroup, logStream string, end *time.Time, queryString string, expectedEntity ExpectedEntity, entityPlatformType string) {
	log.Printf("Checking log group/stream: %s/%s", logGroup, logStream)
	if !awsservice.IsLogGroupExists(logGroup) {
		t.Fatalf("application log group used for entity validation doesn't exist: %s", logGroup)
	}

	begin := end.Add(-12 * time.Minute)
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