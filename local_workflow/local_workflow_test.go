package local_workflow

import (
	"github.com/stretchr/testify/suite"
	"testing"
)

type LocalWorkflowSuite struct {
	suite.Suite
}

func TestLocalWorkflowSuite(t *testing.T) {
	config := FetchConfig()
	WriteVarsFile(config)
}
