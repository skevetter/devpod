package upgrade

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type UpgradeTestSuite struct {
	suite.Suite
}

func TestUpgradeSuite(t *testing.T) {
	suite.Run(t, new(UpgradeTestSuite))
}

func (s *UpgradeTestSuite) TestCheckerIsNewerAvailable() {
	checker := newChecker()
	checker.currentVer = "0.0.1" // Set to very old version
	ctx := context.Background()

	newerVersion, err := checker.isNewerAvailable(ctx)
	s.NoError(err)
	s.NotEmpty(newerVersion, "should find a newer version than 0.0.1")
}
