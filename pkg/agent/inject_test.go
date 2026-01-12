package agent

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/suite"
)

type InjectTestSuite struct {
	suite.Suite
	ctx    context.Context
	logger log.Logger
}

func TestInjectTestSuite(t *testing.T) {
	suite.Run(t, new(InjectTestSuite))
}

func (s *InjectTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.logger = log.Discard
}

func (s *InjectTestSuite) TestLocalInjection() {
	opts := &InjectOptions{
		Ctx:     s.ctx,
		Exec:    (&MockExecFunc{}).Exec,
		Log:     s.logger,
		IsLocal: true,
		Command: "echo hello",
	}

	err := opts.Validate()
	s.NoError(err, "Validation of local injection options should succeed")
}

func (s *InjectTestSuite) TestOptionsDefaults() {
	opts := &InjectOptions{}
	opts.ApplyDefaults()

	s.NotZero(opts.Timeout, "Timeout should be set by defaults")
	s.NotEmpty(opts.DownloadURL, "DownloadURL should be set by defaults")
	s.NotEmpty(opts.LocalVersion, "LocalVersion should be set by defaults")
	s.Equal(opts.LocalVersion, opts.RemoteVersion, "RemoteVersion should default to LocalVersion")
}

func (s *InjectTestSuite) TestVersionChecker() {
	s.Run("Matches", func() {
		vc := &versionChecker{
			remoteVersion: "v1.0.0",
			skipCheck:     false,
		}
		mockExec := &MockExecFunc{Output: "v1.0.0\n"}

		detected, err := vc.validateRemoteAgent(s.ctx, mockExec.Exec, "/path", s.logger)
		s.NoError(err)
		s.Equal("v1.0.0", detected)
	})

	s.Run("Mismatch", func() {
		vc := &versionChecker{
			remoteVersion: "v1.0.0",
			skipCheck:     false,
		}
		mockExec := &MockExecFunc{Output: "v0.9.0\n"}

		detected, err := vc.validateRemoteAgent(s.ctx, mockExec.Exec, "/path", s.logger)
		s.Error(err)
		s.Equal("v0.9.0", detected)
	})

	s.Run("Skip", func() {
		vc := &versionChecker{skipCheck: true}
		mockExec := &MockExecFunc{Output: "irrelevant"}

		detected, err := vc.validateRemoteAgent(s.ctx, mockExec.Exec, "/path", s.logger)
		s.NoError(err)
		s.Empty(detected)
	})
}

func (s *InjectTestSuite) TestRetryStrategy() {
	rs := &RetryStrategy{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond * 10,
		Timeout:      time.Second,
	}

	attempts := 0
	err := rs.WithRetry(s.ctx, s.logger, func(a int) error {
		attempts = a
		return errors.New("fail")
	})

	s.Error(err)
	s.Equal(3, attempts, "Should retry up to MaxAttempts")
}

// MockExecFunc is a helper for testing
type MockExecFunc struct {
	CapturedCmd string
	Output      string
	Err         error
}

func (m *MockExecFunc) Exec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	m.CapturedCmd = cmd
	if stdout != nil {
		stdout.Write([]byte(m.Output))
	}
	return m.Err
}
