package dockerfile

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/suite"
)

//go:embed test_Dockerfile
var testDockerFileContents string

type ParseTestSuite struct {
	suite.Suite
}

func TestParseTestSuite(t *testing.T) {
	suite.Run(t, new(ParseTestSuite))
}

func (s *ParseTestSuite) TestBuildContextFiles() {
	dockerFile, err := Parse(testDockerFileContents)
	s.NoError(err)

	files := dockerFile.BuildContextFiles()
	s.Equal(2, len(files))
	s.Equal("app", files[0])
	s.Equal("files", files[1])
}

func (s *ParseTestSuite) TestFindBaseImage() {
	// Test simple FROM
	dockerfile := `FROM ubuntu:20.04`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu:20.04", baseImage)
}

func (s *ParseTestSuite) TestFindBaseImageMultistage() {
	// Test multistage build with stage reference
	dockerfile := `FROM ubuntu AS base
RUN apt update
FROM base
RUN mkdir /app`
	d, err := Parse(dockerfile)
	s.NoError(err)

	// Should resolve "base" to "ubuntu"
	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu", baseImage)
}

func (s *ParseTestSuite) TestFindUserStatement() {
	// Test USER statement
	dockerfile := `FROM ubuntu
USER testuser`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("testuser", user)
}

func (s *ParseTestSuite) TestFindUserStatementWithVariable() {
	// Test USER with variable
	dockerfile := `FROM ubuntu
ARG USERNAME=defaultuser
USER $USERNAME`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{"USERNAME": "myuser"}, map[string]string{}, "")
	s.Equal("myuser", user)
}

func (s *ParseTestSuite) TestEnsureFinalStageName() {
	// Test adding stage name to final stage
	dockerfile := `FROM ubuntu
RUN echo hello`

	stageName, modifiedDockerfile, err := EnsureFinalStageName(dockerfile, "final")
	s.NoError(err)
	s.Equal("final", stageName)
	s.Contains(modifiedDockerfile, "AS final")
}

func (s *ParseTestSuite) TestEnsureFinalStageNameExisting() {
	// Test when stage already has a name
	dockerfile := `FROM ubuntu AS existing
RUN echo hello`

	stageName, modifiedDockerfile, err := EnsureFinalStageName(dockerfile, "final")
	s.NoError(err)
	s.Equal("existing", stageName)
	s.Equal("", modifiedDockerfile) // No modification needed
}

func (s *ParseTestSuite) TestRemoveSyntaxVersion() {
	// Test removing syntax directive
	dockerfile := `# syntax=docker/dockerfile:1.4
FROM ubuntu
RUN echo hello`

	result := RemoveSyntaxVersion(dockerfile)
	s.NotContains(result, "syntax=")
	s.Contains(result, "FROM ubuntu")
}

func (s *ParseTestSuite) TestParseErrors() {
	// Test Dockerfile with no FROM statement
	dockerfile := `RUN echo hello`
	d, err := Parse(dockerfile)
	s.NoError(err) // Parse succeeds but no stages
	s.Empty(d.Stages)

	// Test empty Dockerfile
	_, err = Parse("")
	s.Error(err)
	s.Contains(err.Error(), "file with no instructions")
}

func (s *ParseTestSuite) TestVariableReplacement() {
	// Test complex variable replacement
	dockerfile := `FROM ubuntu
ARG USERNAME=defaultuser
ENV USER_HOME=/home/${USERNAME}
USER ${USERNAME:-root}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	// Test with build args
	user := d.FindUserStatement(map[string]string{"USERNAME": "testuser"}, map[string]string{}, "")
	s.Equal("testuser", user)

	// Test with default value
	user = d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("defaultuser", user)
}

func (s *ParseTestSuite) TestMultistageUserResolution() {
	// Test user resolution across stages
	dockerfile := `FROM ubuntu AS base
USER baseuser

FROM base AS final
USER finaluser`
	d, err := Parse(dockerfile)
	s.NoError(err)

	// Test finding user for specific stage
	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "base")
	s.Equal("baseuser", user)

	user = d.FindUserStatement(map[string]string{}, map[string]string{}, "final")
	s.Equal("finaluser", user)
}

func (s *ParseTestSuite) TestEnsureDockerfileErrors() {
	// Test no FROM statement
	_, _, err := EnsureFinalStageName("RUN echo hello", "final")
	s.Error(err)
	s.Contains(err.Error(), "no FROM statement")

	// Test malformed FROM
	_, _, err = EnsureFinalStageName("FROM", "final")
	s.Error(err)
	s.Contains(err.Error(), "cannot parse FROM statement")
}

func (s *ParseTestSuite) TestCircularStageReference() {
	// Test circular stage reference prevention
	dockerfile := `FROM stage2 AS stage1
FROM stage1 AS stage2`
	d, err := Parse(dockerfile)
	s.NoError(err)

	// Should return empty string to prevent infinite loop
	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "stage1")
	s.Equal("", user)
}

func (s *ParseTestSuite) TestBaseImageEnvironmentVariables() {
	// Test using base image environment variables
	dockerfile := `FROM ubuntu
USER $USER`
	d, err := Parse(dockerfile)
	s.NoError(err)

	// Test with base image env
	user := d.FindUserStatement(map[string]string{}, map[string]string{"USER": "baseuser"}, "")
	s.Equal("baseuser", user)
}

func (s *ParseTestSuite) TestEmptyUserStatement() {
	// Test empty user statement
	dockerfile := `FROM ubuntu`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("", user)
}

func (s *ParseTestSuite) TestComplexMultistageScenario() {
	// Test complex multistage with variables and inheritance
	dockerfile := `FROM ubuntu AS base
ARG BASE_USER=baseuser
USER $BASE_USER

FROM base AS middle
ENV MIDDLE_USER=middleuser
USER $MIDDLE_USER

FROM middle
ARG FINAL_USER=finaluser
USER $FINAL_USER`
	d, err := Parse(dockerfile)
	s.NoError(err)

	// Test final stage user
	user := d.FindUserStatement(map[string]string{"FINAL_USER": "testfinal"}, map[string]string{}, "")
	s.Equal("testfinal", user)

	// Test middle stage user
	user = d.FindUserStatement(map[string]string{}, map[string]string{}, "middle")
	s.Equal("middleuser", user)

	// Test base stage user
	user = d.FindUserStatement(map[string]string{"BASE_USER": "testbase"}, map[string]string{}, "base")
	s.Equal("testbase", user)
}
