package setup

import (
	"os"
	"os/exec"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LifecycleHookTestSuite struct {
	suite.Suite
}

func (s *LifecycleHookTestSuite) TestStringCommandWithQuotes() {
	currentUser, err := user.Current()
	s.Require().NoError(err)

	c := []string{`echo "hello world"`}
	args := buildCommandArgs(c, currentUser.Username, currentUser.Username)
	assert.Equal(s.T(), []string{"sh", "-c", `echo "hello world"`}, args)
}

func (s *LifecycleHookTestSuite) TestArrayCommand() {
	currentUser, err := user.Current()
	s.Require().NoError(err)

	c := []string{"echo", "hello", "world"}
	args := buildCommandArgs(c, currentUser.Username, currentUser.Username)
	assert.Equal(s.T(), []string{"echo", "hello", "world"}, args)
}

func (s *LifecycleHookTestSuite) TestArrayCommandWithShellWrapper() {
	currentUser, err := user.Current()
	s.Require().NoError(err)

	c := []string{"sh", "-c", `echo "test"`}
	args := buildCommandArgs(c, currentUser.Username, currentUser.Username)
	assert.Equal(s.T(), []string{"sh", "-c", `echo "test"`}, args)
}

func (s *LifecycleHookTestSuite) TestStringCommandWithUserSwitch() {
	currentUser, err := user.Current()
	s.Require().NoError(err)

	c := []string{`echo "hello"`}
	args := buildCommandArgs(c, "otheruser", currentUser.Username)
	assert.Equal(s.T(), []string{"su", "otheruser", "-c", `echo "hello"`}, args)
}

func (s *LifecycleHookTestSuite) TestArrayCommandWithUserSwitch() {
	currentUser, err := user.Current()
	s.Require().NoError(err)

	c := []string{"echo", "hello"}
	args := buildCommandArgs(c, "otheruser", currentUser.Username)
	assert.Equal(s.T(), []string{"su", "otheruser", "-c", "echo hello"}, args)
}

func (s *LifecycleHookTestSuite) TestSymlinkWithQuotes() {
	if os.Getuid() != 0 {
		s.T().Skip("Requires root")
	}

	testLink := "/tmp/devpod_test_link"
	_ = os.Remove(testLink)
	defer func() { _ = os.Remove(testLink) }()

	cmd := exec.Command("sh", "-c", `ln -sf "$(command -v ls)" `+testLink)
	output, err := cmd.CombinedOutput()
	s.Require().NoError(err, "Output: %s", output)

	target, err := os.Readlink(testLink)
	s.Require().NoError(err)
	s.Require().NotEmpty(target, "symlink target should not be empty")
	assert.NotEqual(s.T(), '"', target[0])
	assert.NotEqual(s.T(), '"', target[len(target)-1])
}

func TestLifecycleHookTestSuite(t *testing.T) {
	suite.Run(t, new(LifecycleHookTestSuite))
}
