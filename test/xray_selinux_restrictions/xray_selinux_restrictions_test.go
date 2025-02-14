package xray_selinux_restrictions

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/stretchr/testify/require"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// This test ensures that SELinux correctly enforces its policy by allowing X-Ray tracing
// but preventing unauthorized operations like changing user privileges.
func TestXraySelinuxRestrictions(t *testing.T) {
	t.Run("XRay test should pass", verifyXrayTestPasses)
	t.Run("RunAsUser test should fail with SELinux denial", verifyRunAsUserTestFails)
}

// Verifies that X-Ray tests pass as expected under SELinux constraints.
func verifyXrayTestPasses(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("go", "test", "../xray", "-p", "1", "-timeout", "1h",
		"-computeType=EC2", "-bucket="+env.Bucket,
		"-cwaCommitSha="+env.CwaCommitSha, "-caCertPath="+env.CaCertPath, "-instanceId="+env.InstanceId)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "X-Ray test failed: %s", string(output))

	// Poll for SELinux policy application instead of arbitrary sleep
	require.Eventually(t, func() bool {
		return checkAVCLog("xray")
	}, 30*time.Second, 5*time.Second, "Expected SELinux log entry for X-Ray but found none")
}

// Verifies that trying to run the process as a different user fails due to SELinux.
func verifyRunAsUserTestFails(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("go", "test", "../test/run_as_user", "-p", "1", "-timeout", "1h",
		"-computeType=EC2", "-bucket="+env.Bucket,
		"-cwaCommitSha="+env.CwaCommitSha, "-caCertPath="+env.CaCertPath, "-instanceId="+env.InstanceId)

	output, err := cmd.CombinedOutput()
	require.Error(t, err, "RunAsUser test unexpectedly passed! Output: %s", string(output))

	// Check that SELinux denied the action
	require.Eventually(t, func() bool {
		return checkAVCLog("denied")
	}, 30*time.Second, 5*time.Second, "Expected SELinux denial, but no AVC log found")
}

// Checks SELinux logs for a specific keyword.
func checkAVCLog(keyword string) bool {
	avcCheckCmd := exec.Command("sudo", "ausearch", "-m", "AVC,USER_AVC", "-ts", "recent")
	output, err := avcCheckCmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), keyword)
}
