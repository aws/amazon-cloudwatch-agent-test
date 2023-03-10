package local_workflow

import (
	"fmt"
	"github.com/stretchr/testify/suite"
	"testing"
)

type LocalWorkflowSuite struct {
	suite.Suite
}

func TestLocalWorkflowSuite(t *testing.T) {
	fmt.Println("Hello LocalWorkflowSuite")
}
