package multi_config

import (
	"testing"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestMultipleConfig(t *testing.T) {
	err := Validate()
	if err != nil {
		t.Fatalf("append-config validation failed: %s", err)
	}
}
