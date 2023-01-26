// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// StartLogWrite starts go routines to write logs to each of the logs that are monitored by CW Agent according to
// the config provided
func StartLogWrite(configFilePath string, agentRunDuration time.Duration, dataRate int) error {
	//create wait group so main test thread waits for log writing to finish before stopping agent and collecting data
	var wg sync.WaitGroup

	logPaths, err := getLogFilePaths(configFilePath)
	if err != nil {
		return err
	}

	for _, logPath := range logPaths {
		filePath := logPath //necessary weird golang thing
		wg.Add(1)
		go func() {
			defer wg.Done()
			err = writeToLogs(filePath, agentRunDuration, dataRate)
		}()
	}

	//wait until writing to logs finishes
	wg.Wait()
	return err
}

// writeToLogs opens a file at the specified file path and writes the specified number of lines per second (tps)
// for the specified duration
func writeToLogs(filePath string, durationMinutes time.Duration, dataRate int) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(filePath)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	endTimeout := time.After(durationMinutes)

	//loop until the test duration is reached
	for {
		select {
		case <-ticker.C:
			for i := 0; i < 1000; i++ {
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
