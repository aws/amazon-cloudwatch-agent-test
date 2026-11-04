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
	"runtime"
	"strings"
	"time"

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

const (
	agentDownloadBaseURL = "https://amazoncloudwatch-agent.s3.amazonaws.com"
	agentInstallDir      = "/opt/aws/amazon-cloudwatch-agent"
	agentPackageName     = "amazon-cloudwatch-agent"
	agentLaunchdPlist    = "/Library/LaunchDaemons/com.amazon.cloudwatch.agent.plist"
	agentSystemdUnit     = "/etc/systemd/system/amazon-cloudwatch-agent.service"
	agentDownloadTries   = 3
	agentDownloadBackoff = 10 * time.Second
	processPollInterval  = 2 * time.Second
)

// InstallAgentVersion downloads a released agent package for this host's
// OS and architecture and installs it. version is a release version such as
// "1.300074.0b1920", or "latest". Installing an older version over a newer
// one is allowed.
func InstallAgentVersion(version string) error {
	url, err := agentDownloadURL(version)
	if err != nil {
		return err
	}
	dest := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s%s", agentPackageName, version, filepath.Ext(url)))
	log.Printf("Installing agent version %s from %s", version, url)
	var out []byte
	for attempt := 1; attempt <= agentDownloadTries; attempt++ {
		out, err = exec.Command("curl", "-fsSL", "-o", dest, url).CombinedOutput()
		if err == nil {
			break
		}
		log.Printf("Download attempt %d of %d failed: %v %s", attempt, agentDownloadTries, err, out)
		if attempt < agentDownloadTries {
			time.Sleep(agentDownloadBackoff)
		}
	}
	if err != nil {
		return fmt.Errorf("unable to download %s: %w", url, err)
	}
	defer os.Remove(dest)
	return installAgentPackage(dest)
}

// InstallAgentUnderTest reinstalls the agent package that was downloaded
// for this test run, for tests that install another version first.
func InstallAgentUnderTest() error {
	path, err := agentUnderTestPackage()
	if err != nil {
		return err
	}
	log.Printf("Reinstalling agent under test from %s", path)
	return installAgentPackage(path)
}

// ResetAgentInstall stops the agent, removes the package and deletes the
// agent's install directory, so the next install starts from a clean host.
func ResetAgentInstall() error {
	log.Printf("Resetting agent install")
	ctl := filepath.Join(agentInstallDir, "bin", "amazon-cloudwatch-agent-ctl")
	if _, err := os.Stat(ctl); err == nil {
		runLogged("sudo", ctl, "-a", "stop")
	}
	if runtime.GOOS == "darwin" {
		runLogged("sudo", "launchctl", "unload", agentLaunchdPlist)
		runLogged("sudo", "rm", "-f", agentLaunchdPlist)
	} else if hostPackageManager() == DEB {
		runLogged("sudo", "dpkg", "-P", agentPackageName)
	} else {
		runLogged("sudo", "rpm", "-e", agentPackageName)
	}
	if out, err := exec.Command("sudo", "rm", "-rf", agentInstallDir).CombinedOutput(); err != nil {
		return fmt.Errorf("unable to remove %s: %w %s", agentInstallDir, err, out)
	}
	return nil
}

// ProcessOwner returns the user that the running process with the given
// name runs as. It returns an error if no such process is running.
func ProcessOwner(name string) (string, error) {
	users, err := processUsers(name)
	if err != nil {
		return "", err
	}
	if len(users) == 0 {
		return "", fmt.Errorf("no running process named %s", name)
	}
	for _, u := range users[1:] {
		if u != users[0] {
			return "", fmt.Errorf("processes named %s run as different users: %v", name, users)
		}
	}
	log.Printf("Process %s runs as %s", name, users[0])
	return users[0], nil
}

// WaitForProcess waits until a process with the given name is running, or
// until none is when running is false. It returns an error on timeout.
func WaitForProcess(name string, running bool, timeout time.Duration) error {
	state := "running"
	if !running {
		state = "stopped"
	}
	log.Printf("Waiting up to %s for %s to be %s", timeout, name, state)
	deadline := time.Now().Add(timeout)
	for {
		users, err := processUsers(name)
		if err == nil && (len(users) > 0) == running {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s not %s after %s", name, state, timeout)
		}
		time.Sleep(processPollInterval)
	}
}

// processUsers returns the user of each running process whose executable
// name is name. It reads the full command line because Linux truncates the
// short process name to 15 characters.
func processUsers(name string) ([]string, error) {
	out, err := exec.Command("ps", "-A", "-o", "user=", "-o", "args=").Output()
	if err != nil {
		return nil, fmt.Errorf("unable to list processes: %w", err)
	}
	var users []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && filepath.Base(fields[1]) == name {
			users = append(users, fields[0])
		}
	}
	return users, nil
}

func agentDownloadURL(version string) (string, error) {
	var arch string
	switch runtime.GOARCH {
	case "amd64", "arm64":
		arch = runtime.GOARCH
	default:
		return "", fmt.Errorf("unsupported architecture %s", runtime.GOARCH)
	}
	osDir, ext := "amazon_linux", ".rpm"
	if runtime.GOOS == "darwin" {
		osDir, ext = "darwin", ".pkg"
	} else if hostPackageManager() == DEB {
		osDir, ext = "ubuntu", ".deb"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s%s", agentDownloadBaseURL, osDir, arch, version, agentPackageName, ext), nil
}

// agentUnderTestPackage finds the package downloaded for this test run. Linux
// hosts keep it in the test repo root, macOS hosts in the directory above it.
func agentUnderTestPackage() (string, error) {
	root, err := testRepoRoot()
	if err != nil {
		return "", err
	}
	ext := ".rpm"
	if runtime.GOOS == "darwin" {
		ext = ".pkg"
	} else if hostPackageManager() == DEB {
		ext = ".deb"
	}
	for _, dir := range []string{root, filepath.Dir(root)} {
		path := filepath.Join(dir, agentPackageName+ext)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no %s%s found in %s or its parent", agentPackageName, ext, root)
}

func testRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above the working directory")
		}
		dir = parent
	}
}

func hostPackageManager() PackageManager {
	if _, err := exec.LookPath("dpkg"); err == nil {
		if _, err := exec.LookPath("apt-get"); err == nil {
			return DEB
		}
	}
	return RPM
}

func installAgentPackage(path string) error {
	var cmd *exec.Cmd
	switch filepath.Ext(path) {
	case ".rpm":
		cmd = exec.Command("sudo", "rpm", "-Uvh", "--oldpackage", "--replacepkgs", path)
	case ".deb":
		cmd = exec.Command("sudo", "dpkg", "-i", "-E", path)
	case ".pkg":
		cmd = exec.Command("sudo", "installer", "-pkg", path, "-target", "/")
	default:
		return fmt.Errorf("unsupported package %s", path)
	}
	out, err := cmd.CombinedOutput()
	log.Printf("%s\n%s", cmd.String(), out)
	if err != nil {
		return fmt.Errorf("unable to install %s: %w", path, err)
	}
	relabelAgentFiles()
	return nil
}

// relabelAgentFiles restores SELinux file labels after an install, so the
// agent keeps the labels from its SELinux policy.
func relabelAgentFiles() {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath("restorecon"); err != nil {
		return
	}
	if out, err := exec.Command("getenforce").Output(); err != nil || strings.TrimSpace(string(out)) == "Disabled" {
		return
	}
	runLogged("sudo", "restorecon", "-RF", agentInstallDir, agentSystemdUnit)
}

// runLogged runs a command and logs its output. Failures are logged, not
// returned, for cleanup steps that may have nothing to do.
func runLogged(name string, args ...string) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("%s: %v %s", cmd.String(), err, out)
	}
}
