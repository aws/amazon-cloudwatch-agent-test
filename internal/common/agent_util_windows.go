// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package common

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
)

const (
	ConfigOutputPath = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent.json"
	AgentLogFile     = "C:\\ProgramData\\Amazon\\AmazonCloudWatchAgent\\Logs\\amazon-cloudwatch-agent.log"
)

func CopyFile(pathIn string, pathOut string) error {
	ps, err := exec.LookPath("powershell.exe")

	if err != nil {
		return err
	}

	log.Printf("Copy File %s to %s", pathIn, pathOut)
	pathInAbs, err := filepath.Abs(pathIn)

	if err != nil {
		return err
	}

	log.Printf("File %s abs path %s", pathIn, pathInAbs)
	bashArgs := append([]string{"-NoProfile", "-NonInteractive", "-NoExit", "cp " + pathInAbs + " " + pathOut})
	out, err := exec.Command(ps, bashArgs...).Output()

	if err != nil {
		log.Printf("Copy file failed: %v; the output is: %s", err, string(out))
		return err
	}

	log.Printf("File : %s copied to : %s", pathIn, pathOut)
	return nil

}

func StartAgent(configOutputPath string, fatalOnFailure bool, ssm bool) error {
	// @TODO add ssm functionality

	ps, err := exec.LookPath("powershell.exe")

	if err != nil {
		return err
	}

	bashArgs := append([]string{"-NoProfile", "-NonInteractive", "-NoExit", "& \"C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1\" -a fetch-config -m ec2 -s -c file:" + configOutputPath})
	out, err := exec.Command(ps, bashArgs...).Output()

	if err != nil && fatalOnFailure {
		log.Printf("Start agent failed: %v; the output is: %s", err, string(out))
		return err
	} else if err != nil {
		log.Printf(fmt.Sprint(err) + string(out))
	} else {
		log.Printf("Agent has started")
	}

	return err
}

func StopAgent() error {
	ps, err := exec.LookPath("powershell.exe")

	if err != nil {
		return err
	}

	bashArgs := append([]string{"-NoProfile", "-NonInteractive", "-NoExit", "& \"C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1\" -a stop"})
	out, err := exec.Command(ps, bashArgs...).Output()

	if err != nil {
		log.Printf("Stop agent failed: %v; the output is: %s", err, string(out))
		return err
	}

	log.Printf("Agent is stopped")
	return nil
}

func RunShellScript(path string, args ...string) error {
	ps, err := exec.LookPath("powershell.exe")

	if err != nil {
		return err
	}

	bashArgs := append([]string{"-NoProfile", "-NonInteractive", "-NoExit", path}, args...)
	out, err := exec.Command(ps, bashArgs...).Output()

	if err != nil {
		log.Printf("Error occurred when executing %s: %s | %s", path, err.Error(), string(out))
		return err
	}

	return nil
}

// printOutputAndError does nothing if there was no error.
// Else it prints stdout and stderr.
func printOutputAndError(stdout []byte, err error) {
	if err == nil {
		return
	}
	stderr := ""
	ee, ok := err.(*exec.ExitError)
	if ok {
		stderr = string(ee.Stderr)
	}
	log.Printf("failed\n\tstdout:\n%s\n\tstderr:\n%s\n", string(stdout), stderr)
}

func RunCommand(cmd string) (string, error) {
	log.Printf("running cmd, %s", cmd)
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-NoExit", cmd).Output()
	printOutputAndError(out, err)
	return string(out), err
}

func RunAyncCommand(cmd string) error {
	log.Printf("running async cmd, %s", cmd)
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-NoExit", cmd).Start()
}
