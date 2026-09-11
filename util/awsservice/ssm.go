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

// throttleRetryAttempts bounds how many times a single invocation read is retried
// when throttled.
const throttleRetryAttempts = 5

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

// commandDeliveryTimeout bounds how long SSM itself will keep trying to hand a command
// to the agent. Without it SendCommand defaults to 3600s, so an undeliverable command sits
// in Pending/Delayed far longer than any test waits and the client-side deadline fires
// first — reporting "did not complete" and hiding SSM's own verdict. At 90s SSM flips the
// invocation to TimedOut/DeliveryTimedOut before WaitForCommandCompletion's 2m default,
// so the failure names the real cause. Healthy commands complete in ~11s.
const commandDeliveryTimeout int32 = 90

func RunSSMDocument(name string, instanceIds []string, parameters map[string][]string) (*ssm.SendCommandOutput, error) {
	out, err := SsmClient.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName:   aws.String(name),
		InstanceIds:    instanceIds,
		Parameters:     parameters,
		TimeoutSeconds: aws.Int32(commandDeliveryTimeout),
	})

	return out, err
}

// RunSSMDocumentAwaitDelivery sends a document and waits for it to complete, resending once
// if SSM never managed to deliver the first attempt. A Pending/Delayed invocation means the
// command never reached the agent ("the system attempted to send the command to the managed
// node but wasn't successful"), which is a transport failure rather than a test failure and
// is worth one retry. Only one resend, and only for undelivered commands, so a genuinely
// failing command still fails on the first attempt.
func RunSSMDocumentAwaitDelivery(name string, instanceIds []string, parameters map[string][]string) (*ssm.SendCommandOutput, *ssm.ListCommandInvocationsOutput, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		out, err := RunSSMDocument(name, instanceIds, parameters)
		if err != nil {
			return nil, nil, err
		}

		result, err := WaitForCommandCompletion(*out.Command.CommandId, instanceIds[0])
		if err == nil {
			return out, result, nil
		}
		lastErr = err

		var undelivered *CommandUndeliveredError
		if !errors.As(err, &undelivered) || attempt == 2 {
			return out, nil, err
		}
		log.Printf("Command %s was never delivered to %s (%s); resending once",
			*out.Command.CommandId, instanceIds[0], undelivered.Details)
	}
	return nil, nil, lastErr
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

			// Check if the instance is online. Note PingStatus==Online only means the
			// agent registered; it does not guarantee SSM can deliver a command to it.
			info := result.InstanceInformationList[0]
			if info.PingStatus != types.PingStatusOnline {
				allReady = false
				break
			}
			agentVersion := "unknown"
			if info.AgentVersion != nil {
				agentVersion = *info.AgentVersion
			}
			lastPing := "unknown"
			if info.LastPingDateTime != nil {
				lastPing = info.LastPingDateTime.Format(time.RFC3339)
			}
			log.Printf("SSM agent on %s: version %s, platform %s, last ping %s",
				instanceId, agentVersion, info.PlatformType, lastPing)
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

// isDeliveryFailure reports whether a StatusDetails value describes SSM failing to hand the
// command to the agent, rather than the command running and failing.
func isDeliveryFailure(details string) bool {
	switch details {
	case "DeliveryTimedOut", "Undeliverable", "Terminated":
		return true
	}
	return false
}

// CommandUndeliveredError is returned when the deadline expires while the invocation is
// still Pending or Delayed, i.e. SSM never handed the command to the agent. This is a
// transport failure, distinct from a command that ran and failed, so callers can retry it.
type CommandUndeliveredError struct {
	CommandId  string
	InstanceId string
	Status     types.CommandInvocationStatus
	Details    string
	Waited     time.Duration
}

func (e *CommandUndeliveredError) Error() string {
	return fmt.Sprintf("command %s was never delivered to instance %s within %s (status %s, details %q) - SSM could not hand the command to the agent",
		e.CommandId, e.InstanceId, e.Waited, e.Status, e.Details)
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
	var lastStatus, loggedStatus types.CommandInvocationStatus
	var lastDetails, loggedDetails string
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
				time.Sleep(pollBackoff())
				continue
			}
			return nil, err
		}

		if len(result.CommandInvocations) > 0 {
			invocation := result.CommandInvocations[0]
			lastStatus = invocation.Status
			lastDetails = ""
			if invocation.StatusDetails != nil {
				lastDetails = *invocation.StatusDetails
			}
			if lastStatus != loggedStatus || lastDetails != loggedDetails {
				log.Printf("Command %s on instance %s: status %s (%s)", commandId, instanceId, lastStatus, lastDetails)
				loggedStatus, loggedDetails = lastStatus, lastDetails
			}
			switch invocation.Status {
			case types.CommandInvocationStatusSuccess:
				return result, nil
			case types.CommandInvocationStatusFailed,
				types.CommandInvocationStatusCancelled,
				types.CommandInvocationStatusTimedOut:
				if invocation.Status == types.CommandInvocationStatusTimedOut && isDeliveryFailure(lastDetails) {
					return nil, &CommandUndeliveredError{
						CommandId: commandId, InstanceId: instanceId,
						Status: invocation.Status, Details: lastDetails, Waited: time.Since(deadline.Add(-wait)),
					}
				}
				details := ""
				if invocation.StatusDetails != nil {
					details = ": " + *invocation.StatusDetails
				}
				return nil, &CommandTerminalError{CommandId: commandId, InstanceId: instanceId, Status: invocation.Status, Details: details}
			}
		}

		if time.Now().After(deadline) {
			if lastStatus == types.CommandInvocationStatusPending || lastStatus == types.CommandInvocationStatusDelayed {
				return nil, &CommandUndeliveredError{
					CommandId: commandId, InstanceId: instanceId,
					Status: lastStatus, Details: lastDetails, Waited: wait,
				}
			}
			return nil, fmt.Errorf("command %s on instance %s did not complete within %s (last status %s, details %q)", commandId, instanceId, wait, lastStatus, lastDetails)
		}
		time.Sleep(pollBackoff())
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
// pollBackoff is the shared inter-poll pause. The jitter keeps concurrent matrix jobs
// from synchronising into a herd against the same rate limit.
func pollBackoff() time.Duration {
	return 5*time.Second + time.Duration(rand.Int63n(int64(time.Second)))
}

// listCommandInvocations re-reads an invocation, tolerating throttling. Every read of an
// invocation has to do this: the SSM client uses the SDK default of 3 attempts with ~1.5s
// of total backoff, which is not enough when the whole test matrix polls concurrently.
// A non-throttling error is returned immediately.
func listCommandInvocations(commandId, instanceId string, attempts int) (*ssm.ListCommandInvocationsOutput, error) {
	var err error
	for i := 1; i <= attempts; i++ {
		var out *ssm.ListCommandInvocationsOutput
		out, err = SsmClient.ListCommandInvocations(ctx, &ssm.ListCommandInvocationsInput{
			CommandId:  aws.String(commandId),
			InstanceId: aws.String(instanceId),
			Details:    true,
		})
		if err == nil {
			return out, nil
		}
		if !isThrottlingError(err) {
			return nil, err
		}
		log.Printf("ListCommandInvocations throttled reading command %s (attempt %d/%d); retrying", commandId, i, attempts)
		if i < attempts {
			time.Sleep(pollBackoff())
		}
	}
	return nil, err
}

// GetCommandInvocationResult re-reads a command invocation. SSM makes an invocation's
// terminal Status visible before it populates CommandPlugins[].Output, so a caller that
// needs the output has to re-read after WaitForCommandCompletion returns.
func GetCommandInvocationResult(commandId, instanceId string) (*ssm.ListCommandInvocationsOutput, error) {
	return listCommandInvocations(commandId, instanceId, throttleRetryAttempts)
}

// GetCommandInvocationDetails renders a command invocation for diagnostics.
func GetCommandInvocationDetails(commandId, instanceId string) string {
	result, err := listCommandInvocations(commandId, instanceId, throttleRetryAttempts)
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
