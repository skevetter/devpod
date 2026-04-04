package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	gotoolsassert "gotest.tools/assert"
)

func TestUserHomeDir(t *testing.T) {
	// Remember to reset environment variables after the test
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("USERPROFILE", origUserProfile)
	})

	type input struct {
		home, userProfile string
	}

	type testCase struct {
		Name   string
		Input  input
		Expect string
	}

	testCases := []testCase{
		{
			// $HOME is preferred on every platform
			Name: "both HOME and USERPROFILE are set",
			Input: input{
				home:        "home",
				userProfile: "userProfile",
			},
			Expect: "home",
		},
	}
	if runtime.GOOS == "windows" {
		// On Windows, after $HOME, %userprofile% value is checked
		testCases = append(testCases, testCase{
			Name: "HOME is unset and USERPROFILE is set",
			Input: input{
				home:        "",
				userProfile: "userProfile",
			},
			Expect: "userProfile",
		})
	}

	for _, test := range testCases {
		t.Run(test.Name, func(t *testing.T) {
			_ = os.Setenv("HOME", test.Input.home)
			_ = os.Setenv("USERPROFILE", test.Input.userProfile)

			got, err := UserHomeDir()
			gotoolsassert.NilError(t, err, test.Name)
			gotoolsassert.Equal(t, test.Expect, got)
		})
	}
}

type ExpandTildeSuite struct {
	suite.Suite
	home string
}

func TestExpandTildeSuite(t *testing.T) {
	suite.Run(t, new(ExpandTildeSuite))
}

func (s *ExpandTildeSuite) SetupSuite() {
	home, err := UserHomeDir()
	require.NoError(s.T(), err)
	s.home = home
}

func (s *ExpandTildeSuite) TestExpandsTildeSlash() {
	got := ExpandTilde("~/foo.sock")
	assert.Equal(s.T(), filepath.Join(s.home, "foo.sock"), got)
}

func (s *ExpandTildeSuite) TestExpandsBareTilde() {
	got := ExpandTilde("~")
	assert.Equal(s.T(), s.home, got)
}

func (s *ExpandTildeSuite) TestNoExpansionForAbsolutePath() {
	got := ExpandTilde("/tmp/ssh-agent.sock")
	assert.Equal(s.T(), "/tmp/ssh-agent.sock", got)
}

func (s *ExpandTildeSuite) TestEmptyReturnsEmpty() {
	got := ExpandTilde("")
	assert.Empty(s.T(), got)
}

func (s *ExpandTildeSuite) TestNoExpansionForRelativePath() {
	got := ExpandTilde("./local.sock")
	assert.Equal(s.T(), "./local.sock", got)
}

func (s *ExpandTildeSuite) TestNoExpansionForTildeUser() {
	got := ExpandTilde("~otheruser/foo")
	assert.Equal(s.T(), "~otheruser/foo", got)
}

func (s *ExpandTildeSuite) TestExpandsHomeEnvVar() {
	s.T().Setenv("HOME", s.home)
	got := ExpandTilde("$HOME/foo.sock")
	assert.Equal(s.T(), filepath.Join(s.home, "foo.sock"), got)
}

func (s *ExpandTildeSuite) TestExpandsBracedHomeEnvVar() {
	s.T().Setenv("HOME", s.home)
	got := ExpandTilde("${HOME}/foo.sock")
	assert.Equal(s.T(), filepath.Join(s.home, "foo.sock"), got)
}
