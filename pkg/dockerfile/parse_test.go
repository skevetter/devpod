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
	dockerfile := `FROM ubuntu:20.04`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu:20.04", baseImage)
}

func (s *ParseTestSuite) TestFindBaseImageWithArgs() {
	dockerfile := `ARG BASE_IMAGE=ubuntu
ARG BASE_VERSION=20.04
FROM ${BASE_IMAGE}:${BASE_VERSION}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu", baseImage)

	baseImage = d.FindBaseImage(map[string]string{"BASE_IMAGE": "debian", "BASE_VERSION": "11"}, "")
	s.Equal("debian:11", baseImage)
}

func (s *ParseTestSuite) TestFindBaseImageWithArgsNoDefaults() {
	dockerfile := `ARG BASE_IMAGE
ARG BASE_VERSION
FROM ${BASE_IMAGE}:${BASE_VERSION}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{"BASE_IMAGE": "alpine", "BASE_VERSION": "3.18"}, "")
	s.Equal("alpine:3.18", baseImage)

	baseImage = d.FindBaseImage(map[string]string{}, "")
	s.Equal(":", baseImage)
}

func (s *ParseTestSuite) TestFindBaseImageMultistage() {
	dockerfile := `FROM ubuntu AS base
RUN apt update
FROM base
RUN mkdir /app`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu", baseImage)
}

func (s *ParseTestSuite) TestFindBaseImageMultistageWithArgs() {
	dockerfile := `ARG BASE_IMAGE=golang
ARG BASE_VERSION=1.21
FROM ${BASE_IMAGE}:${BASE_VERSION} AS builder
RUN go build

FROM ${BASE_IMAGE}:${BASE_VERSION}
COPY --from=builder /app /app`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("golang", baseImage)

	baseImage = d.FindBaseImage(map[string]string{}, "builder")
	s.Equal("golang", baseImage)

	baseImage = d.FindBaseImage(map[string]string{"BASE_IMAGE": "node", "BASE_VERSION": "18"}, "")
	s.Equal("node:18", baseImage)
}

func (s *ParseTestSuite) TestFindUserStatement() {
	dockerfile := `FROM ubuntu
USER testuser`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("testuser", user)
}

func (s *ParseTestSuite) TestFindUserStatementWithVariable() {
	dockerfile := `FROM ubuntu
ARG USERNAME=defaultuser
USER $USERNAME`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{"USERNAME": "myuser"}, map[string]string{}, "")
	s.Equal("myuser", user)
}

func (s *ParseTestSuite) TestEnsureFinalStageName() {
	dockerfile := `FROM ubuntu
RUN echo hello`

	stageName, modifiedDockerfile, err := EnsureFinalStageName(dockerfile, "final")
	s.NoError(err)
	s.Equal("final", stageName)
	s.Contains(modifiedDockerfile, "AS final")
}

func (s *ParseTestSuite) TestEnsureFinalStageNameExisting() {
	dockerfile := `FROM ubuntu AS existing
RUN echo hello`

	stageName, modifiedDockerfile, err := EnsureFinalStageName(dockerfile, "final")
	s.NoError(err)
	s.Equal("existing", stageName)
	s.Equal("", modifiedDockerfile)
}

func (s *ParseTestSuite) TestRemoveSyntaxVersion() {
	dockerfile := `# syntax=docker/dockerfile:1.4
FROM ubuntu
RUN echo hello`

	result := RemoveSyntaxVersion(dockerfile)
	s.NotContains(result, "syntax=")
	s.Contains(result, "FROM ubuntu")
}

func (s *ParseTestSuite) TestParseErrors() {
	dockerfile := `RUN echo hello`
	d, err := Parse(dockerfile)
	s.NoError(err)
	s.Empty(d.Stages)

	_, err = Parse("")
	s.Error(err)
	s.Contains(err.Error(), "file with no instructions")
}

func (s *ParseTestSuite) TestVariableReplacement() {
	dockerfile := `FROM ubuntu
ARG USERNAME=defaultuser
ENV USER_HOME=/home/${USERNAME}
USER ${USERNAME:-root}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{"USERNAME": "testuser"}, map[string]string{}, "")
	s.Equal("testuser", user)

	user = d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("defaultuser", user)
}

func (s *ParseTestSuite) TestMultistageUserResolution() {
	dockerfile := `FROM ubuntu AS base
USER baseuser

FROM base AS final
USER finaluser`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "base")
	s.Equal("baseuser", user)

	user = d.FindUserStatement(map[string]string{}, map[string]string{}, "final")
	s.Equal("finaluser", user)
}

func (s *ParseTestSuite) TestEnsureDockerfileErrors() {
	_, _, err := EnsureFinalStageName("RUN echo hello", "final")
	s.Error(err)
	s.Contains(err.Error(), "no FROM statement")

	_, _, err = EnsureFinalStageName("FROM", "final")
	s.Error(err)
	s.Contains(err.Error(), "cannot parse FROM statement")
}

func (s *ParseTestSuite) TestCircularStageReference() {
	dockerfile := `FROM stage2 AS stage1
FROM stage1 AS stage2`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "stage1")
	s.Equal("", user)
}

func (s *ParseTestSuite) TestBaseImageEnvironmentVariables() {
	dockerfile := `FROM ubuntu
USER $USER`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{"USER": "baseuser"}, "")
	s.Equal("baseuser", user)
}

func (s *ParseTestSuite) TestEmptyUserStatement() {
	dockerfile := `FROM ubuntu`
	d, err := Parse(dockerfile)
	s.NoError(err)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("", user)
}

func (s *ParseTestSuite) TestComplexMultistageScenario() {
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

	user := d.FindUserStatement(map[string]string{"FINAL_USER": "testfinal"}, map[string]string{}, "")
	s.Equal("testfinal", user)

	user = d.FindUserStatement(map[string]string{}, map[string]string{}, "middle")
	s.Equal("middleuser", user)

	user = d.FindUserStatement(map[string]string{"BASE_USER": "testbase"}, map[string]string{}, "base")
	s.Equal("testbase", user)
}

func (s *ParseTestSuite) TestArgScopeAcrossStages() {
	dockerfile := `ARG GLOBAL_ARG=global
FROM ubuntu AS stage1
ARG STAGE_ARG=stage1val
RUN echo $GLOBAL_ARG

FROM ubuntu AS stage2
RUN echo $STAGE_ARG`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "stage1")
	s.Equal("ubuntu", baseImage)

	baseImage = d.FindBaseImage(map[string]string{}, "stage2")
	s.Equal("ubuntu", baseImage)
}

func (s *ParseTestSuite) TestArgRedeclarationAfterFrom() {
	dockerfile := `ARG VERSION=1.0
FROM ubuntu:${VERSION}
ARG VERSION
RUN echo ${VERSION}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu:1.0", baseImage)

	baseImage = d.FindBaseImage(map[string]string{"VERSION": "2.0"}, "")
	s.Equal("ubuntu:2.0", baseImage)
}

func (s *ParseTestSuite) TestArgNotAvailableAfterFromWithoutRedeclaration() {
	dockerfile := `ARG VERSION=1.0
FROM ubuntu:${VERSION}
USER ${VERSION}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu:1.0", baseImage)

	user := d.FindUserStatement(map[string]string{}, map[string]string{}, "")
	s.Equal("", user)
}

func (s *ParseTestSuite) TestArgEmptyValue() {
	dockerfile := `ARG EMPTY_ARG=
FROM ubuntu:${EMPTY_ARG}latest`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu:latest", baseImage)
}

func (s *ParseTestSuite) TestArgOverriding() {
	dockerfile := `ARG VERSION=1.0
ARG VERSION=2.0
FROM ubuntu:${VERSION}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{}, "")
	s.Equal("ubuntu:2.0", baseImage)
}

func (s *ParseTestSuite) TestMultipleArgsInSingleExpression() {
	dockerfile := `ARG REGISTRY=docker.io
ARG IMAGE=library/ubuntu
ARG TAG=20.04
FROM ${REGISTRY}/${IMAGE}:${TAG}`
	d, err := Parse(dockerfile)
	s.NoError(err)

	baseImage := d.FindBaseImage(map[string]string{"REGISTRY": "gcr.io", "IMAGE": "my/image", "TAG": "latest"}, "")
	s.Equal("gcr.io/my/image:latest", baseImage)
}
