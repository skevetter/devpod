package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func writeGitConfig(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(content), 0o600)
	assert.NoError(t, err)
}

func TestGpgSigningKey_GPGFormat(t *testing.T) {
	writeGitConfig(t, "[user]\n\tsigningKey = TESTKEY123\n")
	result := gpgSigningKey(log.Discard)
	assert.Equal(t, "TESTKEY123", result)
}

func TestGpgSigningKey_SSHFormat_Skipped(t *testing.T) {
	writeGitConfig(
		t,
		"[gpg]\n\tformat = ssh\n[user]\n\tsigningKey = /home/user/.ssh/id_ed25519.pub\n",
	)
	result := gpgSigningKey(log.Discard)
	assert.Empty(t, result)
}

func TestGpgSigningKey_NoKeyConfigured(t *testing.T) {
	writeGitConfig(t, "[user]\n\tname = Test\n")
	result := gpgSigningKey(log.Discard)
	assert.Empty(t, result)
}

func TestGpgSigningKey_X509Format_Returned(t *testing.T) {
	writeGitConfig(t, "[gpg]\n\tformat = x509\n[user]\n\tsigningKey = /path/to/cert\n")
	result := gpgSigningKey(log.Discard)
	assert.Equal(t, "/path/to/cert", result)
}

func TestGpgSigningKey_SSHKeyPath_Skipped(t *testing.T) {
	writeGitConfig(t, "[user]\n\tsigningKey = /home/user/.ssh/id_ed25519.pub\n")
	result := gpgSigningKey(log.Discard)
	assert.Empty(t, result)
}

func TestGpgSigningKey_TildeKeyPath_Skipped(t *testing.T) {
	writeGitConfig(t, "[user]\n\tsigningKey = ~/.ssh/id_ed25519.pub\n")
	result := gpgSigningKey(log.Discard)
	assert.Empty(t, result)
}

func TestForwardTimeout_UsesParsedDuration(t *testing.T) {
	cmd := &SSHCmd{ForwardPortsTimeout: "90s"}

	timeout, err := cmd.forwardTimeout(log.Discard)
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, timeout)
}

func runPortForwardsForTest(
	t *testing.T,
	cmd *SSHCmd,
	mappings []string,
	forwardFn portForwardFunc,
) (int32, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var calls atomic.Int32
	err := cmd.runPortForwards(ctx, nil, portForwardConfig{
		mappings:    mappings,
		logTemplate: "test %s/%s %s/%s",
		forwardFn: func(
			ctx context.Context,
			client *ssh.Client,
			localNetwork string,
			localAddr string,
			remoteNetwork string,
			remoteAddr string,
			timeout time.Duration,
			logger log.Logger,
		) error {
			calls.Add(1)
			return forwardFn(
				ctx,
				client,
				localNetwork,
				localAddr,
				remoteNetwork,
				remoteAddr,
				timeout,
				logger,
			)
		},
	}, log.Discard)

	return calls.Load(), err
}

func TestRunPortForwards_CleanExit(t *testing.T) {
	calls, err := runPortForwardsForTest(t, &SSHCmd{}, []string{"8080:80"}, func(
		context.Context,
		*ssh.Client,
		string,
		string,
		string,
		string,
		time.Duration,
		log.Logger,
	) error {
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls)
}

func TestRunPortForwards_EOFExit(t *testing.T) {
	calls, err := runPortForwardsForTest(t, &SSHCmd{}, []string{"8080:80"}, func(
		context.Context,
		*ssh.Client,
		string,
		string,
		string,
		string,
		time.Duration,
		log.Logger,
	) error {
		return io.EOF
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls)
}

func TestRunPortForwards_ForwardError(t *testing.T) {
	calls, err := runPortForwardsForTest(t, &SSHCmd{}, []string{"8080:80"}, func(
		context.Context,
		*ssh.Client,
		string,
		string,
		string,
		string,
		time.Duration,
		log.Logger,
	) error {
		return errors.New("boom")
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "error forwarding 8080:80: boom")
	assert.Equal(t, int32(1), calls)
}

func TestRunPortForwards_UsesConfiguredTimeout(t *testing.T) {
	cmd := &SSHCmd{ForwardPortsTimeout: "90s"}
	var gotTimeout time.Duration

	calls, err := runPortForwardsForTest(t, cmd, []string{"8080:80"}, func(
		_ context.Context,
		_ *ssh.Client,
		_ string,
		_ string,
		_ string,
		_ string,
		timeout time.Duration,
		_ log.Logger,
	) error {
		gotTimeout = timeout
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls)
	assert.Equal(t, 90*time.Second, gotTimeout)
}

func TestRunPortForwards_ParseErrorStopsBeforeLaunch(t *testing.T) {
	calls, err := runPortForwardsForTest(t, &SSHCmd{}, []string{"8080:80", ""}, func(
		context.Context,
		*ssh.Client,
		string,
		string,
		string,
		string,
		time.Duration,
		log.Logger,
	) error {
		return nil
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "parse port mapping")
	assert.Equal(t, int32(0), calls)
}

func TestRunPortForwards_MultipleMappingsReturnError(t *testing.T) {
	var started atomic.Int32
	ready := make(chan struct{})

	calls, err := runPortForwardsForTest(t, &SSHCmd{}, []string{"8080:80", "8081:81"}, func(
		_ context.Context,
		_ *ssh.Client,
		_ string,
		localAddr string,
		_ string,
		_ string,
		_ time.Duration,
		_ log.Logger,
	) error {
		if started.Add(1) == 2 {
			close(ready)
		} else {
			<-ready
		}

		if localAddr == "localhost:8081" {
			return errors.New("boom")
		}

		return nil
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "error forwarding 8081:81: boom")
	assert.Equal(t, int32(2), calls)
}
