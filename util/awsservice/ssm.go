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
// in Pending far longer than any test waits. Note what this does NOT do: observed
// invocations stayed "Pending (Delayed)" well past this timeout without SSM ever flipping
// them to DeliveryTimedOut, so do not rely on it to produce a terminal status — the
// client-side deadline in WaitForCommandCompletion is what actually ends the wait, and
// CommandUndeliveredError carries the diagnosis. Healthy commands complete in ~11s.
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

// RunSSMDocumentAwaitDelivery sends a document and waits for it to complete. A
// Pending/Delayed invocation means the command never reached the agent ("the system
// attempted to send the command to the managed node but wasn't successful") and comes
// back as CommandUndeliveredError. One resend, but only after the agent has produced
// a health ping NEWER than the failed send — the shutdown watchdog restarts a wedged
// agent and the restarted worker pings immediately, so a post-send ping is proof of
// recovery, whereas any merely "recent" ping could predate the wedge. Against an
// agent that stays wedged the recovery wait times out and we surface the delivery
// error without doubling time-to-failure.
func RunSSMDocumentAwaitDelivery(name string, instanceIds []string, parameters map[string][]string) (*ssm.SendCommandOutput, *ssm.ListCommandInvocationsOutput, error) {
	sentAt := time.Now()
	out, err := RunSSMDocument(name, instanceIds, parameters)
	if err != nil {
		return nil, nil, err
	}

	result, err := WaitForCommandCompletion(*out.Command.CommandId, instanceIds[0])
	if err == nil {
		return out, result, nil
	}

	var undelivered *CommandUndeliveredError
	if !errors.As(err, &undelivered) {
		return out, nil, err
	}
	if readyErr := waitForSSMPingAfter(instanceIds[0], sentAt, 90*time.Second); readyErr != nil {
		return out, nil, fmt.Errorf("%w (agent did not recover: %v)", err, readyErr)
	}
	log.Printf("Command %s was never delivered to %s (%s); agent pinged after the send, resending once",
		*out.Command.CommandId, instanceIds[0], undelivered.Details)

	retryOut, err := RunSSMDocument(name, instanceIds, parameters)
	if err != nil {
		// Return the first attempt's output so callers still see the original command id.
		return out, nil, fmt.Errorf("resend after undelivered command %s failed: %w", *out.Command.CommandId, err)
	}
	result, err = WaitForCommandCompletion(*retryOut.Command.CommandId, instanceIds[0])
	if err != nil {
		return retryOut, nil, err
	}
	return retryOut, result, nil
}

// waitForSSMPingAfter waits until the instance's LastPingDateTime is strictly newer
// than the given time. A ping that merely falls inside the staleness window is not
// proof of anything: it can predate the failure being recovered from.
func waitForSSMPingAfter(instanceId string, after time.Time, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		result, err := SsmClient.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
			Filters: []types.InstanceInformationStringFilter{
				{
					Key:    aws.String("InstanceIds"),
					Values: []string{instanceId},
				},
			},
		})
		if err != nil {
			log.Printf("DescribeInstanceInformation for %s failed: %v", instanceId, err)
		} else if len(result.InstanceInformationList) > 0 {
			info := result.InstanceInformationList[0]
			if info.PingStatus == types.PingStatusOnline && info.LastPingDateTime != nil && info.LastPingDateTime.After(after) {
				return nil
			}
		}
		time.Sleep(10 * time.Second)
	}
	return fmt.Errorf("instance %s produced no SSM health ping after %s within %v",
		instanceId, after.Format(time.RFC3339), timeout)
}

// The SSM agent sends a health ping every five minutes (HealthFrequencyMinutes,
// default and floor 5), so a healthy agent's LastPingDateTime is never more than
// ~300s old; seven minutes is one interval plus margin for ping jitter. Older than
// that means the agent has stopped checking in even though PingStatus still reports
// Online — observed on hosts where mandatory patching scheduled a reboot: the agent
// stops polling for work in anticipation of the reboot, the shutdown watchdog
// cancels the reboot, and every subsequent SendCommand sits Pending/Delayed. This
// gate is a best-effort detector, not the primary remediation (that is the watchdog
// in terraform/ec2/linux/main.tf): the health ping runs on a timer that can survive
// the wedge, in which case staleness never fires and the delivery failure is caught
// by RunSSMDocumentAwaitDelivery instead.
const maxSSMPingStaleness = 7 * time.Minute

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
				// Log rather than silently retry: an IAM denial, wrong-region client,
				// or sustained throttling would otherwise present as an unexplained
				// stall ending in a generic timeout.
				log.Printf("DescribeInstanceInformation for %s failed: %v", instanceId, err)
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

			// Require a fresh ping, not just Online: a wedged agent keeps its Online
			// status while no longer polling for work, and dispatching a command to it
			// produces an undeliverable Pending/Delayed invocation. A healthy agent
			// pings within the staleness window, so this loop either observes a fresh
			// ping on a later poll or times out with a truthful diagnostic.
			if info.LastPingDateTime == nil || time.Since(*info.LastPingDateTime) > maxSSMPingStaleness {
				log.Printf("SSM agent on %s reports Online but last ping was %s (older than %v); waiting for a fresh ping",
					instanceId, lastPing, maxSSMPingStaleness)
				allReady = false
				break
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
