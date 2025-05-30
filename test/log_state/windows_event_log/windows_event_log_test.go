// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package windows_event_log

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/logs"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestStateFile(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	common.CopyFile(filepath.Join("resources", "config.json"), common.ConfigOutputPath)
	log.Print("Starting agent...")
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(5 * time.Second)

	writer := &windowsEventWriter{
		eventLogName:  "Application",
		eventLogLevel: "INFORMATION",
	}
	generator, err := logs.NewGenerator(&logs.GeneratorConfig{
		LinesPerSecond:  1,
		LineLength:      50,
		TimestampFormat: "2006-01-02T15:04:05.000",
	}, writer)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	log.Print("Generating windows events...")
	startTime := time.Now()
	wg.Add(1)
	go generator.Generate(ctx, &wg)
	time.Sleep(30 * time.Second)

	log.Print("Shutting down agent")
	common.StopAgent()
	time.Sleep(10 * time.Second)

	log.Print("Restarting agent. Resuming log collection...")
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(30 * time.Second)

	log.Print("Stopping event generator")
	cancel()
	wg.Wait()
	time.Sleep(10 * time.Second)

	log.Print("Shutting down agent")
	common.StopAgent()
	endTime := time.Now()
	assert.NoError(t, awsservice.ValidateLogs(
		env.InstanceId,
		env.InstanceId,
		&startTime,
		&endTime,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertNoDuplicateLogs(),
		logs.AssertNoMissingLogs,
	))
}

type windowsEventWriter struct {
	eventLogName  string
	eventLogLevel string
}

var _ logs.EntryWriter = (*windowsEventWriter)(nil)

func (w windowsEventWriter) Write(entry string) error {
	return common.CreateWindowsEvent(w.eventLogName, w.eventLogLevel, entry)
}
