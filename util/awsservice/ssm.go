// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// isThrottlingError reports whether err is a rate-limit response rather than a
// real failure. Throttling is retryable at the call site; the SDK's own retryer
// gives up after 3 attempts, which is not enough when the full test matrix polls
// the same API concurrently.
func isThrottlingError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "ThrottlingException", "Throttling", "RequestLimitExceeded", "TooManyUpdates":
		return true
	}
	return false
}

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

// CommandTerminalError is returned by WaitForCommandCompletion when the command invocation
// reaches a terminal failure state (Failed, Cancelled, or TimedOut). Callers can use
// errors.As to inspect the terminal Status without parsing the error string.
type CommandTerminalError struct {
	CommandId  string
	InstanceId string
	Status     types.CommandInvocationStatus
	Details    string // includes leading ": " when present, matching current format
}

func (e *CommandTerminalError) Error() string {
	return fmt.Sprintf("command %s on instance %s reached terminal status %s%s", e.CommandId, e.InstanceId, e.Status, e.Details)
}

// WaitForCommandCompletion polls an SSM command invocation until it reaches a terminal
// state. It returns immediately on Success, and returns an error immediately on the terminal
// failure states (Failed, Cancelled, TimedOut) surfacing StatusDetails. The optional timeout
// defaults to 2m; only a genuinely hung command waits that long. The deadline is checked
// between polls, so the effective wait may exceed the timeout by up to one ~5s poll interval.
func WaitForCommandCompletion(commandId, instanceId string, timeout ...time.Duration) (*ssm.ListCommandInvocationsOutput, error) {
	wait := 2 * time.Minute
	if len(timeout) > 0 && timeout[0] > 0 {
		wait = timeout[0]
	}
	deadline := time.Now().Add(wait)
	for {
		result, err := SsmClient.ListCommandInvocations(ctx, &ssm.ListCommandInvocationsInput{
			CommandId:  aws.String(commandId),
			InstanceId: aws.String(instanceId),
			Details:    true, // This gets the CommandPlugins details
		})
		if err != nil {
			// A throttled poll says nothing about the command itself, which is very
			// likely still running. The SSM client uses the SDK default of 3 attempts
			// with ~1.5s of total backoff, which is not enough when the whole test
			// matrix polls this API concurrently. Keep polling until the deadline
			// instead of failing the test on a transient rate limit.
			if isThrottlingError(err) {
				log.Printf("ListCommandInvocations throttled for command %s on instance %s; retrying", commandId, instanceId)
				if time.Now().After(deadline) {
					return nil, fmt.Errorf("command %s on instance %s: still throttled at deadline %s: %w", commandId, instanceId, wait, err)
				}
				time.Sleep(5*time.Second + time.Duration(rand.Int63n(int64(time.Second))))
				continue
			}
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
				details := ""
				if invocation.StatusDetails != nil {
					details = ": " + *invocation.StatusDetails
				}
				return nil, &CommandTerminalError{CommandId: commandId, InstanceId: instanceId, Status: invocation.Status, Details: details}
			}
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("command %s on instance %s did not complete within %s", commandId, instanceId, wait)
		}
		time.Sleep(5*time.Second + time.Duration(rand.Int63n(int64(time.Second))))
	}
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
