package network_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type SetupTestSuite struct {
	suite.Suite
}

func (s *SetupTestSuite) TestSetupWorks() {
	s.True(true, "testify suite is working")
}

func TestSetupTestSuite(t *testing.T) {
	suite.Run(t, new(SetupTestSuite))
}
