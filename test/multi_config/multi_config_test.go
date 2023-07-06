package multi_config

import (
	"log"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func AppendConfigs(config []string, configOutputPath string) {
	for index, agentConfig := range config {
		common.CopyFile(agentConfig, configOutputPath)

		log.Printf(configOutputPath)
		if index == 0 {
			common.StartAgent(configOutputPath, true, false)
		} else {
			common.StartAgentWithMultiConfig(configOutputPath, true, false)
		}
		time.Sleep(10 * time.Second)
	}
}

func TestMultipleConfig(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("append-config validation failed: %s", err)
	}
}
