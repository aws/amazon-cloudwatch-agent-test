package local_workflow

import (
	"fmt"
	"github.com/stretchr/testify/suite"
	"testing"
)

type LocalWorkflowTestSuite struct {
	suite.Suite
	Config       Config
	VarsFilepath string
}

func (suite *LocalWorkflowTestSuite) SetupTest() {
	suite.Config = FetchConfig()
	suite.VarsFilepath = WriteVarsFile(suite.Config)
}

func (suite *LocalWorkflowTestSuite) TestLocalWorkflow() {
	fmt.Println(suite.VarsFilepath)
}

func TestLocalWorkflowSuite(t *testing.T) {
	suite.Run(t, new(LocalWorkflowTestSuite))
}
