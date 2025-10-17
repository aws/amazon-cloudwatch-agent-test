// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package util

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	Sleep = 20 * time.Second
)

type WorkloadStatus struct {
	Status    string     `json:"status"`
	StartTime string     `json:"starttime"`
	Config    string     `json:"configstatus"`
	Version   string     `json:"version"`
	Workloads []Workload `json:"workloads"`
}

type Workload struct {
	Categories    []string `json:"categories"`
	Name          string   `json:"name"`
	TelemetryPort int      `json:"telemetry_port,omitempty"`
	Status        string   `json:"status"`
}

func InstallJava17() error {
	cmd := exec.Command("java", "-version")
	output, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(output), "17.") {
		javacCmd := exec.Command("javac", "-version")
		if javacOutput, javacErr := javacCmd.CombinedOutput(); javacErr == nil && strings.Contains(string(javacOutput), "17.") {
			return nil
		}
	}

	s3Key := "jdk17/windows/microsoft-jdk-17-windows-x64.zip"
	localPath := "C:\\temp\\jdk17.zip"

	if err := awsservice.DownloadFile("cloudwatch-agent-integration-bucket", s3Key, localPath); err != nil {
		return fmt.Errorf("failed to download JDK17 from S3: %v", err)
	}

	cmd = exec.Command("powershell", "-Command",
		"Expand-Archive -Path 'C:\\temp\\jdk17.zip' -DestinationPath 'C:\\temp\\jdk17' -Force")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract JDK17: %v, output: %s", err, string(output))
	}

	findCmd := exec.Command("powershell", "-Command",
		"Get-ChildItem -Path 'C:\\temp\\jdk17' -Directory | Select-Object -First 1 -ExpandProperty FullName")
	jdkPathOutput, err := findCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to find JDK directory: %v", err)
	}

	jdkPath := strings.TrimSpace(string(jdkPathOutput))

	os.Setenv("JAVA_HOME", jdkPath)
	os.Setenv("PATH", jdkPath+"\\bin;"+os.Getenv("PATH"))
	return nil
}

func GetWorkloads() ([]Workload, error) {
	cmd := exec.Command("powershell", "-File", "C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1", "-a", "status-with-workloads")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload status: %v", err)
	}
	fmt.Printf("CloudWatch Agent status output: %s\n", string(output))

	var status WorkloadStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse workloads status: %v", err)
	}

	return status.Workloads, nil
}

func VerifyWorkloadsEmpty() error {
	workloads, err := GetWorkloads()
	if err != nil {
		return fmt.Errorf("failed to get workloads: %v", err)
	}

	if len(workloads) != 0 {
		return fmt.Errorf("workloads are not empty")
	}

	return nil
}

func CreateTestJAR(path string, manifestData map[string]string) error {
	tempDir := "C:\\temp\\jvm-compile"
	os.RemoveAll(tempDir)
	os.MkdirAll(tempDir, 0755)

	javaSource := `
public class Main {
    public static void main(String[] args) {
        System.out.println("JVM Test Application Started");
        try {
            // Keep running for test duration
            while (true) {
                Thread.sleep(5000);
                System.out.println("JVM Test Application Running...");
            }
        } catch (InterruptedException e) {
            System.out.println("JVM Test Application Interrupted");
            System.exit(0);
        }
    }
}
`

	javaFile := filepath.Join(tempDir, "Main.java")
	err := os.WriteFile(javaFile, []byte(javaSource), 0644)
	if err != nil {
		return fmt.Errorf("failed to write Java source: %v", err)
	}

	javaHome := "C:\\temp\\jdk17\\jdk-17.0.16+8"
	javacPath := filepath.Join(javaHome, "bin", "javac.exe")
	compileCmd := exec.Command(javacPath, javaFile)
	compileCmd.Dir = tempDir
	if err := compileCmd.Run(); err != nil {
		return fmt.Errorf("failed to compile Java: %v", err)
	}

	classPath := filepath.Join(tempDir, "Main.class")
	if _, err := os.Stat(classPath); err != nil {
		return fmt.Errorf("compiled class file not found: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create JAR file: %v", err)
	}

	defer f.Close()

	z := zip.NewWriter(f)

	manifestFile, err := z.Create("META-INF/MANIFEST.MF")
	if err != nil {
		z.Close()
		return fmt.Errorf("failed to create manifest entry: %v", err)
	}

	var content strings.Builder
	content.WriteString("Manifest-Version: 1.0\r\n")
	for k, v := range manifestData {
		content.WriteString(k + ": " + v + "\r\n")
	}
	content.WriteString("\r\n")

	_, err = manifestFile.Write([]byte(content.String()))
	if err != nil {
		z.Close()
		return fmt.Errorf("failed to write manifest: %v", err)
	}

	classFile, err := z.Create("Main.class")
	if err != nil {
		z.Close()
		return fmt.Errorf("failed to create class entry: %v", err)
	}

	classBytes, err := os.ReadFile(classPath)
	if err != nil {
		z.Close()
		return fmt.Errorf("failed to read class file: %v", err)
	}

	_, err = classFile.Write(classBytes)
	if err != nil {
		z.Close()
		return fmt.Errorf("failed to write class file: %v", err)
	}

	if err := z.Close(); err != nil {
		return fmt.Errorf("failed to close ZIP writer: %v", err)
	}

	os.RemoveAll(tempDir)

	if stat, err := os.Stat(path); err != nil {
		return fmt.Errorf("JAR file verification failed: %v", err)
	} else if stat.Size() == 0 {
		return fmt.Errorf("JAR file is empty")
	}

	return nil
}

func IsWindows2016() bool {
	cmd := exec.Command("powershell", "-Command", "(Get-WmiObject -Class Win32_OperatingSystem).Caption")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	osName := strings.ToLower(string(output))
	return strings.Contains(osName, "2016")
}

func IsWindows2019() bool {
	cmd := exec.Command("powershell", "-Command", "(Get-WmiObject -Class Win32_OperatingSystem).Caption")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	osName := strings.ToLower(string(output))
	return strings.Contains(osName, "2019")
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func CheckJavaStatus(expectedStatus string, expectedName string, workloadType string, port int) error {
	workloads, err := GetWorkloads()
	if err != nil {
		return fmt.Errorf("failed to get workloads: %v", err)
	}

	for _, workload := range workloads {
		if strings.Contains(workload.Name, expectedName) && workload.Status == expectedStatus {
			if expectedStatus == "READY" {
				if workload.TelemetryPort != port {
					return fmt.Errorf("%s workload has wrong telemetry port, expected %d, got %d", workloadType, port, workload.TelemetryPort)
				}
				if !Contains(workload.Categories, workloadType) {
					return fmt.Errorf("missing %s category, got: %v", workloadType, workload.Categories)
				}
			}
			return nil
		}
	}

	return fmt.Errorf("workload %s with status %s not found", expectedName, expectedStatus)
}

func SetupJavaWorkload(version string, workloadType string) error {
	s3Key := fmt.Sprintf("%s/%s", workloadType, version)
	localPath := fmt.Sprintf("C:\\temp\\%s", version)

	env := environment.GetEnvironmentMetaData()
	if err := awsservice.DownloadFile(env.Bucket, s3Key, localPath); err != nil {
		return fmt.Errorf("failed to download %s from S3: %v", s3Key, err)
	}

	cmd := exec.Command("tar", "-xzf", localPath, "-C", "C:\\temp")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %v", err)
	}

	return nil
}
