package selinux_negative_test

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
	"log"
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
	randomNumber := rand.Int63()
	log.Printf("Generated random number: %d", randomNumber)

	logGroupName := fmt.Sprintf("/aws/cloudwatch/shadow-%d", randomNumber)
	workingLogGroupName := fmt.Sprintf("/aws/cloudwatch/working-%d", randomNumber)
	log.Printf("Log group names: logGroupName=%s, workingLogGroupName=%s", logGroupName, workingLogGroupName)

	configFilePath := filepath.Join("agent_configs", "config.json")
	log.Printf("Config file path: %s", configFilePath)

	originalConfigContent, err := os.ReadFile(configFilePath)
	require.NoError(t, err)
	log.Printf("Original config content:\n%s", string(originalConfigContent))

	updatedConfigContent := strings.ReplaceAll(string(originalConfigContent), "${LOG_GROUP_NAME}", logGroupName)
	updatedConfigContent = strings.ReplaceAll(updatedConfigContent, "${WORKING_LOG_GROUP}", workingLogGroupName)
	log.Printf("Updated config content:\n%s", updatedConfigContent)

	err = os.WriteFile(configFilePath, []byte(updatedConfigContent), os.ModePerm)
	require.NoError(t, err)
	log.Println("Updated config file written successfully")

	require.NoError(t, common.StartAgent(configFilePath, true, false))
	log.Println("Agent started successfully")

	err = os.WriteFile(configFilePath, originalConfigContent, os.ModePerm)
	require.NoError(t, err)
	log.Println("Restored original config file")

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
