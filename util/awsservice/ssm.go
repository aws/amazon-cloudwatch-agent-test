// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"fmt"
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
func WaitForSSMReady(instanceIds []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
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
				allReady = false
				break
			}

			if len(result.InstanceInformationList) == 0 {
				allReady = false
				break
			}

			// Check if the instance is online
			info := result.InstanceInformationList[0]
			if info.PingStatus != types.PingStatusOnline {
				allReady = false
				break
			}
		}

		if allReady {
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
	for i := 0; i < 12; i++ {
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
			switch invocation.Status {
			case types.CommandInvocationStatusSuccess:
				return result, nil
			case types.CommandInvocationStatusFailed,
				types.CommandInvocationStatusCancelled,
				types.CommandInvocationStatusTimedOut:
				// Fail fast on a terminal non-success status instead of looping
				// until the time budget expires and returning the generic
				// "commands did not complete within 1 minute". A fast,
				// deterministic command failure was previously masked as a 60s
				// timeout; surfacing the real status and details here makes every
				// future failure self-diagnosing.
				details := ""
				if invocation.StatusDetails != nil {
					details = *invocation.StatusDetails
				}
				return nil, fmt.Errorf("command %s on instance %s reached terminal status %s: %s",
					commandId, instanceId, invocation.Status, details)
			}
		}
	}
	return nil, errors.New("commands did not complete within 1 minute")
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

// GetParameter fetches a parameter value and returns the underlying error,
// unlike GetStringParameter which swallows the error and returns a sentinel
// string. This is needed for read-after-write checks where the caller must
// distinguish a genuine ParameterNotFound from a real value.
func GetParameter(name string) (string, error) {
	parameter, err := SsmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(name),
	})
	if err != nil {
		return "", err
	}

	return *parameter.Parameter.Value, nil
}

// WaitForParameterAvailable polls Parameter Store until the named parameter is
// readable or the timeout elapses. SSM Parameter Store reads are eventually
// consistent, so a PutParameter is not guaranteed to be immediately visible to
// a subsequent read (e.g. an on-instance agent fetching the config). This
// bounded poll closes that read-after-write window before we issue a command
// that depends on the parameter, and fails closed (returns an error) if the
// parameter is still not readable when the budget is exhausted.
func WaitForParameterAvailable(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := GetParameter(name); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("parameter %q not readable within %v: %v", name, timeout, lastErr)
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
