package integration

import (
	"fmt"
	"github.com/stretchr/testify/suite"
	"testing"
)

type IntegrationTestSuite struct {
	suite.Suite
	Config       Config
	VarsFilepath string
}

func (suite *IntegrationTestSuite) SetupTest() {
	suite.Config = FetchConfig()
	suite.VarsFilepath = WriteVarsFile(suite.Config)
}

func (suite *IntegrationTestSuite) TestLocalWorkflow() {
	fmt.Println(suite.VarsFilepath)
	PrettyPrint(suite.Config)
}

func TestLocalWorkflowSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
