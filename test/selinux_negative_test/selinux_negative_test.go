package selinux_negative_test

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestSelinuxNegativeTest(t *testing.T) {
	logGroupName, workingLogGroupName := startAgent(t)

	time.Sleep(2 * time.Minute)
	verifyLogStreamDoesExist(t, workingLogGroupName) // This should have a log stream
	verifyLogStreamDoesNotExist(t, logGroupName)     // This should not have a log stream
}

func startAgent(t *testing.T) (string, string) {
	logGroupName := fmt.Sprintf("/aws/cloudwatch/shadow-%d", time.Now().UnixNano())         // Log group that shouldn't work
	workingLogGroupName := fmt.Sprintf("/aws/cloudwatch/working-%d", time.Now().UnixNano()) // Log group that should work

	configFilePath := filepath.Join("agent_configs", "config.json")
	configContent, err := os.ReadFile(configFilePath)
	require.NoError(t, err)

	updatedConfigContent := string(configContent)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${LOG_GROUP_NAME}", logGroupName)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${WORKING_LOG_GROUP}", workingLogGroupName)

	// Print updated JSON before writing
	fmt.Println("Updated Config Content (Before Writing to File):")
	fmt.Println(updatedConfigContent)

	// Write back to agent_configs/config.json
	err = os.WriteFile(configFilePath, []byte(updatedConfigContent), 0644)
	require.NoError(t, err)

	// Print the final content to verify correctness
	finalConfigContent, err := os.ReadFile(configFilePath)
	require.NoError(t, err)

	fmt.Println("Final Config Content in agent_configs/config.json:")
	fmt.Println(string(finalConfigContent))

	// Start the agent using the updated config file
	require.NoError(t, common.StartAgent(configFilePath, true, false))
	time.Sleep(10 * time.Second) // Wait for the agent to start properly

	return logGroupName, workingLogGroupName
}

func verifyLogStreamDoesNotExist(t *testing.T, logGroupName string) {
	logStreams := awsservice.GetLogStreamNames(logGroupName)
	require.Len(t, logStreams, 0)
}

func verifyLogStreamDoesExist(t *testing.T, logGroupName string) {
	logStreams := awsservice.GetLogStreamNames(logGroupName)
	require.Greater(t, len(logStreams), 0)
}
