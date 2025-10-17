// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

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

	archCmd := exec.Command("uname", "-m")
	archOutput, err := archCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to detect architecture: %v", err)
	}

	arch := strings.TrimSpace(string(archOutput))

	var s3Key string
	var javaDir string
	if arch == "aarch64" || arch == "arm64" {
		s3Key = "jdk17/linux/amazon-corretto-17-aarch64-linux-jdk.tar.gz"
		javaDir = "/tmp/amazon-corretto-17.0.16.8.1-linux-aarch64"
	} else {
		s3Key = "jdk17/linux/amazon-corretto-17-x64-linux-jdk.tar.gz"
		javaDir = "/tmp/amazon-corretto-17.0.16.8.1-linux-x64"
	}

	localPath := "/tmp/jdk17.tar.gz"

	if err := awsservice.DownloadFile("cloudwatch-agent-integration-bucket", s3Key, localPath); err != nil {
		return fmt.Errorf("failed to download JDK17 from S3: %v", err)
	}

	cmd = exec.Command("tar", "-xzf", localPath, "-C", "/tmp")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract JDK17: %v, output: %s", err, string(output))
	}

	cmd = exec.Command("sudo", "rm", "-f", "/usr/local/bin/java")
	cmd.Run()
	cmd = exec.Command("sudo", "ln", "-sf", filepath.Join(javaDir, "bin/java"), "/usr/local/bin/java")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create Java symlink: %v, output: %s", err, string(output))
	}

	cmd = exec.Command("sudo", "rm", "-f", "/usr/local/bin/javac")
	cmd.Run()
	cmd = exec.Command("sudo", "ln", "-sf", filepath.Join(javaDir, "bin/javac"), "/usr/local/bin/javac")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create Javac symlink: %v, output: %s", err, string(output))
	}

	cmd = exec.Command("sh", "-c", fmt.Sprintf("echo 'export JAVA_HOME=%s' >> ~/.bashrc", javaDir))
	cmd.Run()
	os.Setenv("JAVA_HOME", javaDir)
	os.Setenv("PATH", javaDir+"/bin:"+os.Getenv("PATH"))
	return nil
}

func GetWorkloads() ([]Workload, error) {
	cmd := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl", "-a", "status-with-workloads")
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
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	z := zip.NewWriter(f)
	defer z.Close()

	manifestFile, err := z.Create("META-INF/MANIFEST.MF")
	if err != nil {
		return err
	}

	var content strings.Builder
	content.WriteString("Manifest-Version: 1.0\n")
	for k, v := range manifestData {
		content.WriteString(k + ": " + v + "\n")
	}

	_, err = manifestFile.Write([]byte(content.String()))
	if err != nil {
		return err
	}

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

	tempDir := "/tmp/jvm-compile"
	os.RemoveAll(tempDir)
	os.MkdirAll(tempDir, 0755)

	javaFile := filepath.Join(tempDir, "Main.java")
	err = os.WriteFile(javaFile, []byte(javaSource), 0644)
	if err != nil {
		return err
	}

	compileCmd := exec.Command("javac", javaFile)
	if err := compileCmd.Run(); err != nil {
		return fmt.Errorf("failed to compile Java: %v", err)
	}

	classFile, err := z.Create("Main.class")
	if err != nil {
		return err
	}

	classBytes, err := os.ReadFile(filepath.Join(tempDir, "Main.class"))
	if err != nil {
		return err
	}

	_, err = classFile.Write(classBytes)
	if err != nil {
		return err
	}

	os.RemoveAll(tempDir)
	return nil
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
	var versionBump int
	switch workloadType {
	case "tomcat":
		versionBump = 7
	case "kafka":
		versionBump = 4
	default:
		return fmt.Errorf("unknown workload type: %s", workloadType)
	}

	s3Key := fmt.Sprintf("%s/%s", workloadType, version)
	localPath := fmt.Sprintf("/tmp/%s", version)

	env := environment.GetEnvironmentMetaData()
	if err := awsservice.DownloadFile(env.Bucket, s3Key, localPath); err != nil {
		return fmt.Errorf("failed to download %s from S3: %v", s3Key, err)
	}

	cmd := exec.Command("tar", "-xzf", localPath, "-C", "/tmp")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to extract %s: %v, output: %s", localPath, err, string(output))
	}

	dir := fmt.Sprintf("/tmp/%s", version[:len(version)-versionBump])
	cmd = exec.Command("sh", "-c", fmt.Sprintf("chmod +x %s/bin/*.sh", dir))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to make scripts executable: %v, output: %s", err, string(output))
	}
	return nil
}
