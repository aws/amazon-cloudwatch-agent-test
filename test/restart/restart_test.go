package restart

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

func TestAgentStatusAfterRestart(t *testing.T) {
	var before, after string
	var err error
	before, err = common.RunCommand(fmt.Sprintf("%s %s 2>/dev/null | wc -l", common.CatCommand, common.AgentLogFile))
	if err != nil {
		t.Fatalf("Running restarts test failed")
	}

	time.Sleep(30 * time.Second)

	after, err = common.RunCommand(fmt.Sprintf("%s %s 2>/dev/null | wc -l", common.CatCommand, common.AgentLogFile))
	if err != nil {
		t.Fatalf("Running restarts test failed")
	}

	if before != after {
		t.Fatalf("Logs are flowing, first time the log size is %s, while second time it become %s.", before, after)
	}
}
