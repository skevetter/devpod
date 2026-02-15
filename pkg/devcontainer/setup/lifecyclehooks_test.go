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
	currentUser *user.User
}

func (s *LifecycleHookTestSuite) SetupTest() {
	var err error
	s.currentUser, err = user.Current()
	s.Require().NoError(err)
}

func (s *LifecycleHookTestSuite) TestStringCommandWithQuotes() {
	c := []string{`echo "hello world"`}
	args := s.buildArgs(c)
	assert.Equal(s.T(), []string{"sh", "-c", `echo "hello world"`}, args)
}

func (s *LifecycleHookTestSuite) TestArrayCommand() {
	c := []string{"echo", "hello", "world"}
	args := s.buildArgs(c)
	assert.Equal(s.T(), []string{"echo", "hello", "world"}, args)
}

func (s *LifecycleHookTestSuite) TestArrayCommandWithShellWrapper() {
	c := []string{"sh", "-c", `echo "test"`}
	args := s.buildArgs(c)
	assert.Equal(s.T(), []string{"sh", "-c", `echo "test"`}, args)
}

func (s *LifecycleHookTestSuite) TestSymlinkWithQuotes() {
	if os.Getuid() != 0 {
		s.T().Skip("Requires root")
	}

	testLink := "/tmp/devpod_test_link"
	_ = os.Remove(testLink)
	defer func() { _ = os.Remove(testLink) }()

	cmd := exec.Command("sh", "-c", `sudo ln -sf "$(command -v ls)" `+testLink)
	output, err := cmd.CombinedOutput()
	s.Require().NoError(err, "Output: %s", output)

	target, err := os.Readlink(testLink)
	s.Require().NoError(err)
	assert.NotEqual(s.T(), '"', target[0])
	assert.NotEqual(s.T(), '"', target[len(target)-1])
}

func (s *LifecycleHookTestSuite) buildArgs(c []string) []string {
	var args []string
	if len(c) == 1 {
		args = append(args, "sh", "-c", c[0])
	} else {
		args = c
	}
	return args
}

func TestLifecycleHookTestSuite(t *testing.T) {
	suite.Run(t, new(LifecycleHookTestSuite))
}
