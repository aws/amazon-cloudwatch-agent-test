// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"go.uber.org/multierr"

	"github.com/aws/amazon-cloudwatch-agent-test/validator/models"
)

const logLine = "# %d - This is a log line. \n"

func GetEventLogServicePid() (string, error) {
	out, err := RunShellScript("gcim -ClassName Win32_Service -Filter \"name like 'EventLog' or displayname like 'EventLog'\" | Select-Object -ExpandProperty 'ProcessId'")
	return out, err
}

func KillEventLogService() error {
	out, err := GetEventLogServicePid()
	if err != nil {
		log.Printf("Error getting Windows event log service PID: %v; the output is %s", err, out)
		return err
	}
	log.Printf("Killing Windows EventLog Service PID: %s", out)
	_, _ = RunShellScript("Stop-Process -Force " + out)

	return nil
}

func StartEventLogService() error {
	out, err := RunShellScript("Start-Service EventLog")
	if err != nil {
		log.Printf("Error starting Windows event log service: %v; the output is %s", err, out)
		return err
	}
	pid, _ := GetEventLogServicePid()
	log.Printf("Started Windows EventLog Service PID: %s", pid)

	return nil
}

func ContainsWindowsEventLog(validationLog []models.LogValidation) bool {
	for _, vLog := range validationLog {
		if isWindowsEventLog(vLog) {
			return true
		}
	}
	return false
}

func GenerateLogs(configFilePath string, duration time.Duration, sendingInterval time.Duration, logLinesPerMinute int, validationLog []models.LogValidation) error {
	var multiErr error
	if err := StartLogWrite(configFilePath, duration, sendingInterval, logLinesPerMinute); err != nil {
		multiErr = multierr.Append(multiErr, err)
	}
	if err := GenerateWindowsEvents(validationLog); err != nil {
		multiErr = multierr.Append(multiErr, err)
	}
	return multiErr
}

func GenerateWindowsEvents(validationLog []models.LogValidation) error {
	var multiErr error
	for _, vLog := range validationLog {
		if isWindowsEventLog(vLog) {
			err := CreateWindowsEvent(vLog.LogStream, vLog.LogLevel, vLog.LogValue)
			if err != nil {
				multiErr = multierr.Append(multiErr, err)
			}
		}
	}
	return multiErr
}

func CreateWindowsEvent(eventLogName string, eventLogLevel string, msg string) error {
	out, err := exec.Command("eventcreate", "/ID", "1", "/L", eventLogName, "/T", eventLogLevel, "/SO", "MYEVENTSOURCE"+eventLogName, "/D", msg).Output()

	if err != nil {
		log.Printf("Windows event creation failed: %v; the output is: %s", err, string(out))
		return err
	}

	log.Printf("Windows Event is successfully created for logname: %s, loglevel: %s, logmsg: %s", eventLogName, eventLogLevel, msg)
	return nil
}

// StartLogWrite starts go routines to write logs to each of the logs that are monitored by CW Agent according to
// the config provided
func StartLogWrite(configFilePath string, duration time.Duration, sendingInterval time.Duration, logLinesPerMinute int) error {
	var multiErr error

	logPaths, err := getLogFilePaths(configFilePath)
	if err != nil {
		return err
	}

	for _, logPath := range logPaths {
		go func(logPath string) {
			if err := writeToLogs(logPath, duration, sendingInterval, logLinesPerMinute); err != nil {
				multiErr = multierr.Append(multiErr, err)
			}
		}(logPath)
	}

	return multiErr
}

func isWindowsEventLog(vLog models.LogValidation) bool {
	return vLog.LogSource == "WindowsEvents" && vLog.LogLevel != ""
}

// writeToLogs opens a file at the specified file path and writes the specified number of lines per second (tps)
// for the specified duration
func writeToLogs(filePath string, duration, sendingInterval time.Duration, logLinesPerMinute int) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(filePath)

	ticker := time.NewTicker(sendingInterval)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	// Sending the logs within the first minute before the ticker kicks in the next minute
	for i := 0; i < logLinesPerMinute; i++ {
		_, err := f.WriteString(fmt.Sprintf(logLine, i))
		if err != nil {
			return err
		}
	}

	for {
		select {
		case <-ticker.C:
			for i := 0; i < logLinesPerMinute; i++ {
				f.WriteString(fmt.Sprintf(logLine, i))
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
	if numberMonitoredLogs == 0 || filePath == "" {
		return errors.New("number of monitored logs or file path is empty")
	}

	type LogInfo struct {
		FilePath        string `json:"file_path"`
		LogGroupName    string `json:"log_group_name"`
		LogStreamName   string `json:"log_stream_name"`
		RetentionInDays int    `json:"retention_in_days"`
		Timezone        string `json:"timezone"`
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
	tempFolder := getTempFolder()

	for i := 0; i < numberMonitoredLogs; i++ {
		logFiles = append(logFiles, LogInfo{
			FilePath:        fmt.Sprintf("%s/test%d.log", tempFolder, i+1),
			LogGroupName:    "{instance_id}",
			LogStreamName:   fmt.Sprintf("test%d.log", i+1),
			RetentionInDays: 1,
			Timezone:        "UTC",
		})
	}

	log.Printf("Writing config file with %d logs to %v", numberMonitoredLogs, filePath)

	cfgFileData["logs"].(map[string]interface{})["logs_collected"].(map[string]interface{})["files"].(map[string]interface{})["collect_list"] = logFiles

	finalConfig, err := json.MarshalIndent(cfgFileData, "", " ")
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, finalConfig, 0644)
	if err != nil {
		return err
	}

	return nil
}

// getTempFolder gets the temp folder for generate logs
// depends on the operating system
func getTempFolder() string {
	if runtime.GOOS == "windows" {
		return "C:/Users/Administrator/AppData/Local/Temp"
	}
	return "/tmp"
}
