package xray_selinux_restrictions

import (
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/stretchr/testify/require"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func startAgent(t *testing.T) {
	common.CopyFile(filepath.Join("agent_configs", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))
	time.Sleep(10 * time.Second) // Wait for the agent to start properly
}

// This test verifies that a customer can restrict there selinux policy
// to only work with what there policy allows. In this case we have a policy that
// allows xray to work but something like run_as_user will not work since
// that requires some extra more permissive permissions like dac_override which
// allows a process to change its roles. Having this test, tests the application of
// SELinux is correct and the agent is being denied due to not having permissions.
func TestXraySelinuxRestrictions(t *testing.T) {
	verifyXrayTestPasses(t)
	verifyRunAsUserTestFails(t)

}

func verifyXrayTestPasses(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("go", "test", "../test/xray", "-p", "1", "-timeout", "1h", "-computeType=EC2", "-bucket="+env.Bucket,
		"cwaCommitSha="+env.CwaCommitSha, "-caCertPath="+env.CaCertPath, "-instanceId="+env.InstanceId)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed xray test %s", output)
	time.Sleep(10 * time.Second) // Wait for the agent to apply the new configuration
}

func verifyRunAsUserTestFails(t *testing.T) {

	env := environment.GetEnvironmentMetaData()
	cmd := exec.Command("go", "test", "../test/run_as_user", "-p", "1", "-timeout", "1h", "-computeType=EC2", "-bucket="+env.Bucket,
		"cwaCommitSha="+env.CwaCommitSha, "-caCertPath="+env.CaCertPath, "-instanceId="+env.InstanceId)
	_, err := cmd.CombinedOutput()
	require.Error(t, err)
}
