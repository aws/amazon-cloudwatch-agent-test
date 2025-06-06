// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/logs"
)

type GeneratorStarter interface {
	Init(configFilePath string) error
	Start(ctx context.Context, wg *sync.WaitGroup)
}

func Validate(gs GeneratorStarter, testConfigPath string) error {
	instanceID := awsservice.GetInstanceId()

	common.CopyFile(testConfigPath, common.ConfigOutputPath)
	log.Print("Starting agent...")
	if err := common.StartAgent(common.ConfigOutputPath, true, false); err != nil {
		return fmt.Errorf("could not start agent: %v", err)
	}
	time.Sleep(5 * time.Second)

	if err := gs.Init(common.ConfigOutputPath); err != nil {
		return fmt.Errorf("could not create logs generator: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	log.Print("Generating windows events...")
	startTime := time.Now()
	gs.Start(ctx, &wg)
	time.Sleep(30 * time.Second)

	log.Print("Shutting down agent")
	common.StopAgent()
	time.Sleep(10 * time.Second)

	logStateFiles()

	log.Print("Restarting agent. Resuming log collection...")
	if err := common.StartAgent(common.ConfigOutputPath, true, false); err != nil {
		return fmt.Errorf("could not restart agent: %v", err)
	}
	time.Sleep(30 * time.Second)

	log.Print("Stopping event generator")
	cancel()
	wg.Wait()
	time.Sleep(10 * time.Second)

	log.Print("Shutting down agent")
	common.StopAgent()

	endTime := time.Now()
	logStateFiles()
	return awsservice.ValidateLogs(
		instanceID,
		instanceID,
		&startTime,
		&endTime,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertNoDuplicateLogs(),
		logs.AssertNoMissingLogs,
	)
}

func logStateFiles() {
	files, err := os.ReadDir(common.AgentLogStateFileDir)
	if err != nil {
		log.Printf("Could not read agent log state directory: %v", err)
		return
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		var content []byte
		content, err = os.ReadFile(filepath.Join(common.AgentLogStateFileDir, file.Name()))
		if err != nil {
			log.Printf("Error reading state file %s: %v", file.Name(), err)
			continue
		}
		log.Printf("File: %s\nContent:\n%s\n", file.Name(), string(content))
	}
}
