package restart

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func TestAgentStatusAfterRestart(t *testing.T) {
	RestartCheck(t)
}
