// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"go.uber.org/multierr"
)

// StartLogWrite starts go routines to write logs to each of the logs that are monitored by CW Agent according to
// the config provided
func StartLogWrite(configFilePath string, agentRunDuration time.Duration, dataRate int) error {
	//create wait group so main test thread waits for log writing to finish before stopping agent and collecting data
	var (
		multiErr error
		wg       sync.WaitGroup
	)

	logPaths, err := getLogFilePaths(configFilePath)
	if err != nil {
		return err
	}

	for _, logPath := range logPaths {
		wg.Add(1)
		go func(logPath string) {
			defer wg.Done()
			err = writeToLogs(logPath, agentRunDuration, dataRate)
			multiErr = multierr.Append(multiErr, err)
		}(logPath)
	}

	wg.Wait()
	return multiErr
}

// writeToLogs opens a file at the specified file path and writes the specified number of lines per second (tps)
// for the specified duration
func writeToLogs(filePath string, duration time.Duration, dataRate int) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(filePath)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	//loop until the test duration is reached
	for {
		select {
		case <-ticker.C:
			for i := 0; i < dataRate; i++ {
				_, err = f.WriteString(fmt.Sprintln(ticker, " - #", i, " This is a log line."))
				if err != nil {
					return err
				}
			}

		case <-endTimeout:
			return nil
		}
	}
}

// getLogFilePaths parses the cloudwatch agent config at the specified path and returns a list of the log files that the
// agent will monitor when using that config file
func getLogFilePaths(configPath string) ([]string, error) {
	file, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfgFileData map[string]interface{}
	err = json.Unmarshal(file, &cfgFileData)
	if err != nil {
		return nil, err
	}

	logFiles := cfgFileData["logs"].(map[string]interface{})["logs_collected"].(map[string]interface{})["files"].(map[string]interface{})["collect_list"].([]interface{})
	var filePaths []string
	for _, process := range logFiles {
		filePaths = append(filePaths, process.(map[string]interface{})["file_path"].(string))
	}

	return filePaths, nil
}

// GenerateLogConfig takes the number of logs to be monitored and applies it to the supplied config,
// It writes logs to be monitored of the form /tmp/testNUM.log where NUM is from 1 to number of logs requested to
// the supplied configuration
// DEFAULT CONFIG MUST BE SUPPLIED WITH AT LEAST ONE LOG BEING MONITORED
// (log being monitored will be overwritten - it is needed for json structure)
// returns the path of the config generated and a list of log stream names
func GenerateLogConfig(numberMonitoredLogs int, filePath string) error {
	type LogInfo struct {
		FilePath      string `json:"file_path"`
		LogGroupName  string `json:"log_group_name"`
		LogStreamName string `json:"log_stream_name"`
		Timezone      string `json:"timezone"`
	}

	var cfgFileData map[string]interface{}
	// For metrics and traces, we will keep the default config while log will be appended dynamically
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	err = json.Unmarshal(fileBytes, &cfgFileData)
	if err != nil {
		return err
	}

	var logFiles []LogInfo

	for i := 0; i < numberMonitoredLogs; i++ {
		logFiles = append(logFiles, LogInfo{
			FilePath:      fmt.Sprintf("/tmp/test%d.log", i+1),
			LogGroupName:  "{instance_id}",
			LogStreamName: fmt.Sprintf("{instance_id}/tmp%d", i+1),
			Timezone:      "UTC",
		})
	}

	log.Printf("Writing config file with %d logs to %v", numberMonitoredLogs, filePath)

	cfgFileData["logs"].(map[string]interface{})["logs_collected"].(map[string]interface{})["files"].(map[string]interface{})["collect_list"] = logFiles

	finalConfig, err := json.MarshalIndent(cfgFileData, "", " ")
	if err != nil {
		return err
	}

	_, err = file.WriteAt(finalConfig, 0)
	if err != nil {
		return err
	}

	return nil
}
