//go:build windows

package cloudwatchlogs

import (
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestWindowsEventLog(t *testing.T) {
	cfgFilePath := "resources/config_windows_event_log.json"

	instanceId := awsservice.GetInstanceId()
	log.Printf("Found instance id %s", instanceId)
	logGroup := "CloudWatchAgent"
	logStream := instanceId

	start := time.Now()
	common.CopyFile(cfgFilePath, configOutputPath)

	common.StartAgent(configOutputPath, true)

	// ensure that there is enough time from the "start" time and the first log line,
	// so we don't miss it in the GetLogEvents call
	time.Sleep(agentRuntime)
	t.Log("Writing logs from windows event log plugin")
	time.Sleep(agentRuntime)
	common.StopAgent()

	lines := []string{
		fmt.Sprintf("{\"Metric\": \"%s\"}", strings.Repeat("12345", 10)),
		fmt.Sprintf("{\"Metric\": \"%s\"}", strings.Repeat("09876", 10)),
		fmt.Sprintf("{\"Metric\": \"%s\"}", strings.Repeat("1234567890", 10)),
	}

	end := time.Now()

	ok, err := awsservice.ValidateLogs(logGroup, logStream, &start, &end, func(logs []string) bool {
		log.Printf("logs length: %s ", len(logs))
		if len(logs) != len(lines) {
			return false
		}

		for i := 0; i < len(logs); i++ {
			log.Printf("lines[%s] is %s", i, lines[i])
			log.Printf("logs[%s] is %s", i, logs[i])
			expected := strings.ReplaceAll(lines[i], "'", "\"")
			actual := strings.ReplaceAll(logs[i], "'", "\"")
			if expected != actual {
				return false
			}
		}

		return true
	})
	assert.NoError(t, err)
	assert.True(t, ok)
}
