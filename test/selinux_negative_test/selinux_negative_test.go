package selinux_negative_test

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
	"math/rand"
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
	// Generate random large numbers
	randomNumber := rand.Int63()

	logGroupName := fmt.Sprintf("/aws/cloudwatch/shadow-%d", randomNumber)
	workingLogGroupName := fmt.Sprintf("/aws/cloudwatch/working-%d", randomNumber)

	configFilePath := filepath.Join("agent_configs", "config.json")

	// Read original config and create a backup
	originalConfigContent, err := os.ReadFile(configFilePath)
	require.NoError(t, err)

	updatedConfigContent := string(originalConfigContent)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${LOG_GROUP_NAME}", logGroupName)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${WORKING_LOG_GROUP}", workingLogGroupName)

	// Write updated config
	err = os.WriteFile(configFilePath, []byte(updatedConfigContent), 0777)
	require.NoError(t, err)

	// Start the agent
	require.NoError(t, common.StartAgent(configFilePath, true, false))
	time.Sleep(10 * time.Second) // Wait for the agent to start properly

	// Restore the original config file after the test
	err = os.WriteFile(configFilePath, originalConfigContent, 0777)
	require.NoError(t, err)

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
