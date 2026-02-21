package docker

import (
	"os/user"
	"testing"

	"github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/stretchr/testify/suite"
)

type DockerDriverTestSuite struct {
	suite.Suite
	driver *dockerDriver
}

func TestDockerDriverSuite(t *testing.T) {
	suite.Run(t, new(DockerDriverTestSuite))
}

func (s *DockerDriverTestSuite) SetupTest() {
	s.driver = &dockerDriver{}
}

func (s *DockerDriverTestSuite) TestShouldSkipUpdate_RootContainerUser() {
	localUser := &user.User{Uid: "1000", Gid: "1000"}
	info := &user.User{Uid: "0", Gid: "0"}

	result := shouldSkipUpdate(localUser, info)

	s.True(result, "should skip when container user is root")
}

func (s *DockerDriverTestSuite) TestShouldSkipUpdate_MatchingUIDs() {
	localUser := &user.User{Uid: "1000", Gid: "1000"}
	info := &user.User{Uid: "1000", Gid: "1000"}

	result := shouldSkipUpdate(localUser, info)

	s.True(result, "should skip when UIDs and GIDs match")
}

func (s *DockerDriverTestSuite) TestShouldSkipUpdate_DifferentUIDs() {
	localUser := &user.User{Uid: "1000", Gid: "1000"}
	info := &user.User{Uid: "1001", Gid: "1001"}

	result := shouldSkipUpdate(localUser, info)

	s.False(result, "should not skip when UIDs differ")
}

func (s *DockerDriverTestSuite) TestShouldSkipUpdate_UIDMatch_GIDDifferent() {
	localUser := &user.User{Uid: "1000", Gid: "1000"}
	info := &user.User{Uid: "1000", Gid: "1001"}

	result := shouldSkipUpdate(localUser, info)

	s.False(result, "should not skip when UID matches but GID differs")
}

func (s *DockerDriverTestSuite) TestShouldSkipUpdate_UIDDifferent_GIDMatch() {
	localUser := &user.User{Uid: "1000", Gid: "1000"}
	info := &user.User{Uid: "1001", Gid: "1000"}

	result := shouldSkipUpdate(localUser, info)

	s.False(result, "should not skip when GID matches but UID differs")
}

func (s *DockerDriverTestSuite) TestShouldSkipUpdate_RootWithDifferentGID() {
	localUser := &user.User{Uid: "1000", Gid: "1000"}
	info := &user.User{Uid: "0", Gid: "1001"}

	result := shouldSkipUpdate(localUser, info)

	s.True(result, "should skip when container user is root regardless of GID")
}

func (s *DockerDriverTestSuite) TestGetContainerUser_RemoteUserPriority() {
	cfg := &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			RemoteUser: "remote",
		},
		NonComposeBase: config.NonComposeBase{
			ContainerUser: "container",
		},
	}

	result := s.driver.getContainerUser(cfg)

	s.Equal("remote", result, "should prioritize RemoteUser")
}

func (s *DockerDriverTestSuite) TestGetContainerUser_ContainerUserFallback() {
	cfg := &config.DevContainerConfig{
		NonComposeBase: config.NonComposeBase{
			ContainerUser: "container",
		},
	}

	result := s.driver.getContainerUser(cfg)

	s.Equal("container", result, "should use ContainerUser when RemoteUser is empty")
}

func (s *DockerDriverTestSuite) TestGetContainerUser_BothEmpty() {
	cfg := &config.DevContainerConfig{}

	result := s.driver.getContainerUser(cfg)

	s.Equal("", result, "should return empty when both are empty")
}

func (s *DockerDriverTestSuite) TestGatherUpdateRequirements_WithRemoteUser() {
	cfg := &config.DevContainerConfig{
		DevContainerConfigBase: config.DevContainerConfigBase{
			RemoteUser: "testuser",
		},
	}

	localUser, containerUser, err := s.driver.gatherUpdateRequirements(cfg)

	s.NoError(err)
	s.NotNil(localUser)
	s.Equal("testuser", containerUser)
}

func (s *DockerDriverTestSuite) TestGatherUpdateRequirements_WithContainerUser() {
	cfg := &config.DevContainerConfig{
		NonComposeBase: config.NonComposeBase{
			ContainerUser: "container",
		},
	}

	localUser, containerUser, err := s.driver.gatherUpdateRequirements(cfg)

	s.NoError(err)
	s.NotNil(localUser)
	s.Equal("container", containerUser)
}
