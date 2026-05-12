// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package common

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

const (
	CatCommand              = "cat "
	AppOwnerCommand         = "ps -u -p "
	Namespace               = "CWAgent"
	Host                    = "host"
	InstallAgentVersionPath = "/opt/aws/amazon-cloudwatch-agent/bin/CWAGENT_VERSION"
)

const (
	ConfigOutputPath      = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	AgentLogFile          = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"
	AgentCommonConfigFile = "/opt/aws/amazon-cloudwatch-agent/etc/common-config.toml"
)

type PackageManager int

const (
	RPM PackageManager = iota
	DEB
)

func CopyFile(pathIn string, pathOut string) {
	log.Printf("Copy File %s to %s", pathIn, pathOut)
	pathInAbs, err := filepath.Abs(pathIn)

	if err != nil {
		log.Fatal(err)
	}

	log.Printf("File %s abs path %s", pathIn, pathInAbs)
	cmd := exec.Command("bash", "-c", "sudo cp "+pathInAbs+" "+pathOut)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		log.Fatalf("Error : %s | Error Code: %s | Out: %s", fmt.Sprint(err), stderr.String(), out.String())
	}

	log.Printf("File : %s copied to : %s", pathIn, pathOut)
}

func WriteFile(filePath string, content string) error {
	log.Printf("Writing file %s", filePath)

	// Use tee with sudo to write content to file
	cmd := fmt.Sprintf("echo '%s' | sudo tee %s > /dev/null", content, filePath)
	out, err := exec.Command("bash", "-c", cmd).Output()

	if err != nil {
		log.Printf(fmt.Sprint(err) + string(out))
		return err
	}

	log.Printf("Wrote file: %s", filePath)
	return nil

}

func MkdirAll(path string) error {
	log.Printf("Creating directory %s", path)
	absPath, err := filepath.Abs(path)

	if err != nil {
		log.Fatal(err)
	}

	out, err := exec.Command("bash", "-c", "sudo mkdir -p "+absPath).Output()

	if err != nil {
		log.Printf(fmt.Sprint(err) + string(out))
		return err
	}

	log.Printf("Created directory: %s", absPath)
	return nil
}

func DeleteFile(filePathAbsolute string) error {
	if _, err := os.Stat(filePathAbsolute); os.IsNotExist(err) {
		return nil
	}

	log.Printf("Delete file %s", filePathAbsolute)
	out, err := exec.Command("bash", "-c", "sudo rm "+filePathAbsolute).Output()

	if err != nil {
		log.Printf(fmt.Sprint(err) + string(out))
		return err
	}

	log.Printf("Removed file: %s", filePathAbsolute)
	return nil
}

func TouchFile(filePathAbsolute string) error {
	log.Printf("Touch file %s", filePathAbsolute)
	out, err := exec.Command("bash", "-c", "sudo touch "+filePathAbsolute).Output()

	if err != nil {
		log.Printf(fmt.Sprint(err) + string(out))
		return err
	}

	log.Printf("Touched file: %s", filePathAbsolute)
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

func UninstallAgent(pm PackageManager) error {
	log.Printf("Uninstalling Agent...")
	var c *exec.Cmd
	switch pm {
	case RPM:
		c = exec.Command("bash", "-c", "sudo rpm -e amazon-cloudwatch-agent")
	case DEB:
		c = exec.Command("bash", "-c", "sudo dpkg -r amazon-cloudwatch-agent")
	default:
		log.Fatalf("unsupported package manager, %v", pm)
	}
	out, err := c.Output()
	printOutputAndError(out, err)
	return err
}

// InstallAgent can determine the package manager based on the installer suffix.
func InstallAgent(installerFilePath string) error {
	log.Printf("Installing Agent...")
	var c *exec.Cmd
	// Assuming lower case
	if strings.HasSuffix(installerFilePath, ".rpm") {
		c = exec.Command("bash", "-c", "sudo rpm -Uvh "+installerFilePath)
	} else {
		c = exec.Command("bash", "-c", "sudo dpkg -i -E "+installerFilePath)
	}
	out, err := c.Output()
	printOutputAndError(out, err)
	return err
}

func StartAgent(configOutputPath string, fatalOnFailure bool, ssm bool) error {
	agentStartCommand := environment.GetEnvironmentMetaData().AgentStartCommand
	return StartAgentWithCommand(configOutputPath, fatalOnFailure, ssm, agentStartCommand)
}

func StartAgentWithCommand(configOutputPath string, fatalOnFailure bool, ssm bool, agentStartCommand string) error {
	path := "file:"
	if ssm {
		path = "ssm:"
	}
	completedAgentStartCommand := agentStartCommand + path + configOutputPath
	log.Printf("Starting agent with command %s", completedAgentStartCommand)
	out, err := exec.
		Command("bash", "-c", completedAgentStartCommand).
		Output()

	if err != nil && fatalOnFailure {
		log.Fatal(fmt.Sprint(err) + string(out))
	} else if err != nil {
		log.Printf(fmt.Sprint(err) + string(out))
	} else {
		log.Printf("Agent has started")
	}

	return err
}

func StopAgent() {
	out, err := exec.
		Command("bash", "-c", "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a stop").
		Output()

	if err != nil {
		log.Fatal(fmt.Sprint(err) + string(out))
	}

	log.Printf("Agent is stopped")
}

func ReadAgentLogfile(logfile string) string {
	out, err := os.ReadFile(logfile)
	if err != nil {
		log.Fatal(fmt.Sprint(err) + string(out))
	}
	return string(out)
}

func RecreateAgentLogfile(logfile string) {
	if _, err := os.Stat(logfile); os.IsNotExist(err) {
		return
	}

	out, err := exec.Command("bash", "-c",
		fmt.Sprintf("sudo rm %s", logfile)).
		Output()

	if err != nil {
		log.Fatal(fmt.Sprint(err) + string(out))
	}
}

func RunShellScript(path string, args ...string) (string, error) {
	out, err := exec.Command("bash", "-c", "sudo chmod +x "+path).Output()

	if err != nil {
		log.Printf("Error occurred when attempting to chmod %s: %s | %s", path, err.Error(), string(out))
		return "", err
	}

	bashArgs := []string{"-c", "sudo ./" + path}
	bashArgs = append(bashArgs, args...)

	//out, err = exec.Command("bash", "-c", "sudo ./"+path, args).Output()
	out, err = exec.Command("bash", bashArgs...).Output()

	if err != nil {
		log.Printf("Error occurred when executing %s: %s | %s", path, err.Error(), string(out))
		return "", err
	}

	return string(out), nil
}

func RunCommand(cmd string) (string, error) {
	log.Printf("running cmd, %s", cmd)
	out, err := exec.Command("bash", "-c", cmd).Output()
	printOutputAndError(out, err)
	return string(out), err
}

func RunAsyncCommand(cmd string) error {
	log.Printf("running async cmd, %s", cmd)
	return exec.Command("nohup", "bash", "-c", cmd).Start()
}

func RunCommands(commands []string) error {
	for _, cmd := range commands {
		_, err := RunCommand(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func ReplaceLocalStackHostName(pathIn string) {
	out, err := exec.Command("bash", "-c", "sed -i 's/localhost.localstack.cloud/'\"$LOCAL_STACK_HOST_NAME\"'/g' "+pathIn).Output()

	if err != nil {
		log.Fatal(fmt.Sprint(err) + string(out))
	}
}

func DownloadFromS3(bucket string, key string, destPath string) error {
	sess := session.Must(session.NewSession())
	s3Client := s3.New(sess)

	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to download from S3 bucket %s, key %s: %v", bucket, key, err)
	}
	defer result.Body.Close()

	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file at %s: %v", destPath, err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, result.Body)
	if err != nil {
		return fmt.Errorf("failed to copy content to %s: %v", destPath, err)
	}

	return nil
}

// ReplacePlaceholder replaces a placeholder string with a value in the specified file
// using sed command. This centralizes placeholder replacement functionality used across tests.
func ReplacePlaceholder(filePath, placeholder, value string) error {
	// Escape special characters in the value for sed
	escapedValue := strings.ReplaceAll(value, "/", "\\/")
	escapedValue = strings.ReplaceAll(escapedValue, "&", "\\&")

	sedCmd := fmt.Sprintf("sudo sed -i 's|%s|%s|g' %s", placeholder, escapedValue, filePath)
	out, err := exec.Command("bash", "-c", sedCmd).Output()

	if err != nil {
		return fmt.Errorf("error replacing placeholder (%s) in %s: %s | %s", placeholder, filePath, err.Error(), string(out))
	}

	return nil
}

// ReplacePlaceholders replaces multiple placeholders in a single file
// placeholders is a map where key is the placeholder and value is the replacement
func ReplacePlaceholders(filePath string, placeholders map[string]string) error {
	for placeholder, value := range placeholders {
		if err := ReplacePlaceholder(filePath, placeholder, value); err != nil {
			return fmt.Errorf("failed to replace placeholder '%s': %w", placeholder, err)
		}
	}

	return nil
}

func SELinuxEnforced() (bool, error) {
	status, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(status)) == "1" {
		return true, nil
	}
	return false, nil
}
