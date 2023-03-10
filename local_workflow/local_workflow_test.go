package local_workflow

import (
	"fmt"
	"testing"
)

func TestLocalWorkflowSuite(t *testing.T) {
	config := FetchConfig()
	varsFilepath := WriteVarsFile(config)
	fmt.Println(varsFilepath)
}
