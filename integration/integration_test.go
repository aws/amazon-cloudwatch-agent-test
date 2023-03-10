package integration

import (
	"github.com/stretchr/testify/suite"
	"log"
	"testing"
)

type IntegrationTestSuite struct {
	suite.Suite
	Config       Config
	VarsFilepath string
	RootDir      string
}

func (suite *IntegrationTestSuite) SetupTest() {
	suite.Config = FetchConfig()
	suite.VarsFilepath = WriteVarsFile(suite.Config)
	suite.RootDir = GetRootDir()

}

func (suite *IntegrationTestSuite) TestLocalWorkflow() {
	if terraformPath, ok := suite.Config["terraformPath"].(string); ok {
		RunIntegrationTest(suite.RootDir, terraformPath, suite.VarsFilepath)
	} else {
		log.Fatal("Error: terraformPath was not provided in config.json")
	}
}

func TestLocalWorkflowSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}
