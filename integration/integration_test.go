package integration

import (
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
	rootDir := GetRootDir()
	terraformPath := suite.Config["terraformPath"].(string)
	apply := BuildTerraformCommand(rootDir, terraformPath, suite.VarsFilepath)
	PrettyPrint(apply)
}

func TestLocalWorkflowSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
