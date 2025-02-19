package selinux_negative_test

import (
	"encoding/json"
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
	require.NoError(t, err, "Failed to read config file")

	updatedConfigContent := string(configContent)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${LOG_GROUP_NAME}", logGroupName)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${WORKING_LOG_GROUP}", workingLogGroupName)

	updatedConfigPath := common.ConfigOutputPath

	// Debugging: Print the resolved file path
	fmt.Println("Writing config to:", updatedConfigPath)

	// Ensure the target directory exists
	dir := filepath.Dir(updatedConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		require.NoError(t, err, "Failed to create config directory")
	}

	if !json.Valid([]byte(updatedConfigContent)) {
		t.Fatalf("Invalid JSON detected in config: %s", updatedConfigContent)
	}

	// Write the updated JSON config file
	err = os.WriteFile(updatedConfigPath, []byte(updatedConfigContent), 0644)
	require.NoError(t, err, "Failed to write updated config file")

	// Debugging: Confirm file contents after writing
	writtenContent, err := os.ReadFile(updatedConfigPath)
	require.NoError(t, err, "Failed to verify written config file")
	fmt.Println("Written config content:", string(writtenContent))

	// Start the agent using the updated config
	require.NoError(t, common.StartAgent(updatedConfigPath, true, false), "Failed to start agent")

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
