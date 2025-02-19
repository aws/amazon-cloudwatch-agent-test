package selinux_negative_test

import (
	"bytes"
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
	require.NoError(t, err)

	updatedConfigContent := string(configContent)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${LOG_GROUP_NAME}", logGroupName)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${WORKING_LOG_GROUP}", workingLogGroupName)

	// Pretty print JSON
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, []byte(updatedConfigContent), "", "  ")
	require.NoError(t, err)

	fmt.Println("Updated Config Content:")
	fmt.Println(prettyJSON.String())

	// Copy the updated config file
	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)

	// Read and print the config from the output path
	finalConfigContent, err := os.ReadFile(common.ConfigOutputPath)
	require.NoError(t, err)

	fmt.Println("Final Config Content in Output Path:")
	fmt.Println(string(finalConfigContent))

	fmt.Println()
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
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
