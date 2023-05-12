// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"log"
	"os"
	"os/exec"
	"time"
)

const (
	retryNumber       = 10
	retryTime         = 30 * time.Second
	debInstall        = "deb"
	rpmInstall        = "rpm"
)

func main() {
	installType := os.Args[1]
	installCommand := ""
	
	debInstallCommand := "sudo dpkg -i -E ./amazon-cloudwatch-agent.deb"
	rpmInstallCommand := "sudo rpm -U ./amazon-cloudwatch-agent.rpm"
	if os.Geteuid() == 0 {
		debInstallCommand = "dpkg -i -E ./amazon-cloudwatch-agent.deb"
		rpmInstallCommand = "rpm -U ./amazon-cloudwatch-agent.rpm"
	}
	if installType == debInstall {
		installCommand = debInstallCommand
	} else if installType == rpmInstall {
		installCommand = rpmInstallCommand
	} else {
		log.Fatal("No valid package to install")
	}
	for i := 0; i < retryNumber; i++ {
		out, err := exec.Command("bash", "-c", installCommand).Output()
		log.Printf("Install command output %s, err %s", string(out), err)
		if err == nil {
			break
		}
		time.Sleep(retryTime)
	}
}
