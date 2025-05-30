// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package logfile_state

import (
	"context"
	"fmt"
	"log"
	"os"
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

	common.CopyFile(testConfigJSON, common.ConfigOutputPath)
	log.Print("Starting agent...")
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(5 * time.Second)

	paths, err := common.GetLogFilePaths(common.ConfigOutputPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	log.Print("Generating log lines...")
	startTime := time.Now()
	for _, path := range paths {
		writer, err := newFileWriter(path)
		require.NoError(t, err)
		generator, err := logs.NewGenerator(&logs.GeneratorConfig{
			LinesPerSecond:  1,
			LineLength:      50,
			TimestampFormat: "2006-01-02T15:04:05.000",
		}, writer)
		require.NoError(t, err)
		wg.Add(1)
		go generator.Generate(ctx, &wg)
	}
	time.Sleep(30 * time.Second)

	log.Print("Shutting down agent")
	common.StopAgent()
	time.Sleep(5 * time.Second)

	log.Print("Restarting agent. Resuming log collection...")
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(30 * time.Second)

	log.Print("Stopping log generator")
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

type fileWriter struct {
	file *os.File
	mu   sync.Mutex
}

var _ logs.EntryWriter = (*fileWriter)(nil)

func newFileWriter(filePath string) (*fileWriter, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return &fileWriter{file: f}, nil
}

func (w *fileWriter) Write(entry string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.file.WriteString(entry)
	return err
}

func (w *fileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
