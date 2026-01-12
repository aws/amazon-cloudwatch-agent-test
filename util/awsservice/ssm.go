// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func CreateSSMDocument(name string, content string, documentType types.DocumentType) error {
	_, err := SsmClient.CreateDocument(ctx, &ssm.CreateDocumentInput{
		Name:         aws.String(name),
		Content:      aws.String(content),
		DocumentType: documentType,
	})

	return err
}

func RunSSMDocument(name string, instanceIds []string, parameters map[string][]string) (*ssm.SendCommandOutput, error) {
	out, err := SsmClient.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: aws.String(name),
		InstanceIds:  instanceIds,
		Parameters:   parameters,
	})

	return out, err
}

// WaitForSSMReady waits for instances to be registered and online with SSM.
// This is necessary because there's a delay between EC2 instance launch and SSM agent registration.
// Increased timeout to 10 minutes for newer OS distributions (RHEL 10, Rocky Linux 9) that may take longer.
func WaitForSSMReady(instanceIds []string, timeout time.Duration) error {
	log.Printf("[SSM-READY] Starting SSM readiness check for instances %v with timeout %v", instanceIds, timeout)
	deadline := time.Now().Add(timeout)
	checkCount := 0

	for time.Now().Before(deadline) {
		checkCount++
		allReady := true
		for _, instanceId := range instanceIds {
			result, err := SsmClient.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
				Filters: []types.InstanceInformationStringFilter{
					{
						Key:    aws.String("InstanceIds"),
						Values: []string{instanceId},
					},
				},
			})
			if err != nil {
				log.Printf("[SSM-READY] Check #%d: Error querying SSM for instance %s: %v", checkCount, instanceId, err)
				allReady = false
				break
			}

			if len(result.InstanceInformationList) == 0 {
				log.Printf("[SSM-READY] Check #%d: Instance %s not yet registered with SSM", checkCount, instanceId)
				allReady = false
				break
			}

			// Check if the instance is online
			info := result.InstanceInformationList[0]
			if info.PingStatus != types.PingStatusOnline {
				log.Printf("[SSM-READY] Check #%d: Instance %s registered but PingStatus=%s (waiting for Online)", checkCount, instanceId, info.PingStatus)
				allReady = false
				break
			}
			log.Printf("[SSM-READY] Check #%d: Instance %s is Online", checkCount, instanceId)
		}

		if allReady {
			log.Printf("[SSM-READY] All instances are SSM-ready after %d checks", checkCount)
			return nil
		}

		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("instances %v did not become SSM-ready within %v", instanceIds, timeout)
}

func DeleteSSMDocument(name string) error {
	_, err := SsmClient.DeleteDocument(ctx, &ssm.DeleteDocumentInput{
		Name: aws.String(name),
	})

	return err
}

func WaitForCommandCompletion(commandId, instanceId string) (*ssm.ListCommandInvocationsOutput, error) {
	// Increased from 12 iterations (1 minute) to 60 iterations (5 minutes) to handle slower SSM agent startup
	// Note: Test binaries need to be rebuilt for this change to take effect in CI/CD environments
	for i := 0; i < 60; i++ {
		time.Sleep(5 * time.Second)
		result, err := SsmClient.ListCommandInvocations(ctx, &ssm.ListCommandInvocationsInput{
			CommandId:  aws.String(commandId),
			InstanceId: aws.String(instanceId),
			Details:    true, // This gets the CommandPlugins details
		})
		if err != nil {
			return nil, err
		}

		if len(result.CommandInvocations) > 0 {
			invocation := result.CommandInvocations[0]
			if invocation.Status == types.CommandInvocationStatusSuccess {
				return result, nil
			}
		}
	}
	return nil, errors.New("commands did not complete within 5 minutes")
}

func PutStringParameter(name, value string) error {
	return putParameter(name, value, types.ParameterTypeString)
}

func DeleteParameter(name string) error {
	_, err := SsmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(name),
	})
	return err
}

func GetStringParameter(name string) string {
	parameter, err := SsmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return "Parameter not found"
	}

	return *parameter.Parameter.Value
}

func putParameter(name, value string, paramType types.ParameterType) error {
	isOverwriteAllowed := true

	_, err := SsmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      paramType,
		Overwrite: &isOverwriteAllowed,
	})

	return err
}

// GetCommandInvocationDetails retrieves detailed command output for debugging
func GetCommandInvocationDetails(commandId, instanceId string) string {
	result, err := SsmClient.ListCommandInvocations(ctx, &ssm.ListCommandInvocationsInput{
		CommandId:  aws.String(commandId),
		InstanceId: aws.String(instanceId),
		Details:    true,
	})
	if err != nil {
		return "Failed to retrieve command output: " + err.Error()
	}

	if len(result.CommandInvocations) == 0 {
		return "No command invocations found"
	}

	invocation := result.CommandInvocations[0]
	output := "Command Status: " + string(invocation.Status) + "\n"

	if invocation.StatusDetails != nil {
		output += "Status Details: " + *invocation.StatusDetails + "\n"
	}

	for _, plugin := range invocation.CommandPlugins {
		output += "\nPlugin: " + *plugin.Name + "\n"
		output += "  Status: " + string(plugin.Status) + "\n"
		if plugin.StatusDetails != nil {
			output += "  Status Details: " + *plugin.StatusDetails + "\n"
		}
		if plugin.Output != nil && *plugin.Output != "" {
			output += "  Output:\n" + *plugin.Output + "\n"
		}
		if plugin.ResponseCode != 0 {
			output += fmt.Sprintf("  Response Code: %d\n", plugin.ResponseCode)
		}
	}

	return output
}
