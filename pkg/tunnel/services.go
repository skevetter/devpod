package tunnel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/kballard/go-shellquote"
	"github.com/loft-sh/api/v4/pkg/devpod"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent"
	"github.com/skevetter/devpod/pkg/agent/tunnelserver"
	"github.com/skevetter/devpod/pkg/config"
	config2 "github.com/skevetter/devpod/pkg/devcontainer/config"
	"github.com/skevetter/devpod/pkg/devcontainer/setup"
	"github.com/skevetter/devpod/pkg/gitsshsigning"
	"github.com/skevetter/devpod/pkg/ide/openvscode"
	"github.com/skevetter/devpod/pkg/netstat"
	"github.com/skevetter/devpod/pkg/provider"
	devssh "github.com/skevetter/devpod/pkg/ssh"
	"github.com/skevetter/log"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

const (
	debugFlag          = " --debug"
	defaultExitTimeout = time.Second * 5
	maxRetrySteps      = math.MaxInt
	retryDuration      = 500 * time.Millisecond
	retryFactor        = 1.0
	retryJitter        = 0.1
)

// RunServicesOptions contains all options for running services.
type RunServicesOptions struct {
	DevPodConfig                   *config.Config
	ContainerClient                *ssh.Client
	User                           string
	ForwardPorts                   bool
	ExtraPorts                     []string
	PlatformOptions                *devpod.PlatformOptions
	Workspace                      *provider.Workspace
	ConfigureDockerCredentials     bool
	ConfigureGitCredentials        bool
	ConfigureGitSSHSignatureHelper bool
	Log                            log.Logger
}

// getExitAfterTimeout calculates the timeout value based on configuration.
func getExitAfterTimeout(devPodConfig *config.Config) time.Duration {
	if devPodConfig.ContextOption(config.ContextOptionExitAfterTimeout) != "true" {
		return 0
	}
	return defaultExitTimeout
}

// createForwarder creates a port forwarder if port forwarding is enabled.
func createForwarder(opts RunServicesOptions, forwardedPorts []string) netstat.Forwarder {
	if !opts.ForwardPorts {
		return nil
	}
	ports := append([]string{}, forwardedPorts...)
	ports = append(ports, fmt.Sprintf("%d", openvscode.DefaultVSCodePort))
	return newForwarder(opts.ContainerClient, ports, opts.Log)
}

// tunnelServerParams contains parameters for running the tunnel server.
type tunnelServerParams struct {
	ctx          context.Context
	opts         RunServicesOptions
	stdoutReader *os.File
	stdinWriter  *os.File
	forwarder    netstat.Forwarder
	errChan      chan error
	cancel       context.CancelFunc
}

// runTunnelServer runs the tunnel server in a goroutine.
func runTunnelServer(p tunnelServerParams) {
	defer p.cancel()
	defer func() { _ = p.stdoutReader.Close() }()
	defer func() { _ = p.stdinWriter.Close() }()
	err := tunnelserver.RunServicesServer(
		p.ctx,
		p.stdoutReader,
		p.stdinWriter,
		p.opts.ConfigureGitCredentials,
		p.opts.ConfigureDockerCredentials,
		p.forwarder,
		p.opts.Workspace,
		p.opts.Log,
		tunnelserver.WithPlatformOptions(p.opts.PlatformOptions),
	)
	if err != nil {
		p.errChan <- fmt.Errorf("run tunnel server %w", err)
	}
	close(p.errChan)
}

// addGitSSHSigningKey adds SSH signing key to command if configured.
func addGitSSHSigningKey(command string) string {
	format, userSigningKey, err := gitsshsigning.ExtractGitConfiguration()
	if err == nil && format == gitsshsigning.GPGFormatSSH && userSigningKey != "" {
		encodedKey := base64.StdEncoding.EncodeToString([]byte(userSigningKey))
		command += fmt.Sprintf(" --git-user-signing-key %s", encodedKey)
	}
	return command
}

// buildCredentialsCommand builds the credentials server command.
func buildCredentialsCommand(opts RunServicesOptions) string {
	command := fmt.Sprintf(
		"%s agent container credentials-server --user %s",
		shellquote.Join(agent.ContainerDevPodHelperLocation),
		shellquote.Join(opts.User),
	)
	if opts.ConfigureGitCredentials {
		command += " --configure-git-helper"
	}
	if opts.ConfigureGitSSHSignatureHelper {
		command = addGitSSHSigningKey(command)
	}
	if opts.ConfigureDockerCredentials {
		command += " --configure-docker-helper"
	}
	if opts.ForwardPorts {
		command += " --forward-ports"
	}
	if opts.Log.GetLevel() == logrus.DebugLevel {
		command += debugFlag
	}
	return command
}

// runServicesIteration performs one iteration of the retry loop.
func runServicesIteration(ctx context.Context, opts RunServicesOptions, forwardedPorts []string) error {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = stdoutWriter.Close() }()

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = stdinReader.Close() }()

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	forwarder := createForwarder(opts, forwardedPorts)

	errChan := make(chan error, 1)
	go runTunnelServer(tunnelServerParams{
		ctx:          cancelCtx,
		opts:         opts,
		stdoutReader: stdoutReader,
		stdinWriter:  stdinWriter,
		forwarder:    forwarder,
		errChan:      errChan,
		cancel:       cancel,
	})

	writer := opts.Log.ErrorStreamOnly().Writer(logrus.DebugLevel, false)
	defer func() { _ = writer.Close() }()

	command := buildCredentialsCommand(opts)

	err = devssh.Run(cancelCtx, opts.ContainerClient, command, stdinReader, stdoutWriter, writer, nil)
	if err != nil {
		return err
	}
	return <-errChan
}

// RunServices forwards the ports for a given workspace and uses it's SSH client to run the credentials server remotely and the services server locally to communicate with the container
func RunServices(ctx context.Context, opts RunServicesOptions) error {
	exitAfterTimeout := getExitAfterTimeout(opts.DevPodConfig)

	forwardedPorts, err := forwardDevContainerPorts(portForwardParams{
		ctx:              ctx,
		containerClient:  opts.ContainerClient,
		extraPorts:       opts.ExtraPorts,
		exitAfterTimeout: exitAfterTimeout,
		log:              opts.Log,
	})
	if err != nil {
		return fmt.Errorf("forward ports %w", err)
	}

	return retry.OnError(wait.Backoff{
		Steps:    maxRetrySteps,
		Duration: retryDuration,
		Factor:   retryFactor,
		Jitter:   retryJitter,
	}, func(err error) bool {
		// Do not retry on context cancellation or deadline exceeded
		if ctx.Err() != nil {
			return false
		}
		return true
	}, func() error {
		return runServicesIteration(ctx, opts, forwardedPorts)
	})
}

// portForwardParams contains parameters for port forwarding.
type portForwardParams struct {
	ctx              context.Context
	containerClient  *ssh.Client
	extraPorts       []string
	exitAfterTimeout time.Duration
	log              log.Logger
}

// forwardDevContainerPorts forwards all the ports defined in the devcontainer.json.
func forwardDevContainerPorts(p portForwardParams) ([]string, error) {
	result, err := getContainerResult(p)
	if err != nil {
		return nil, err
	}

	forwardedPorts := []string{}
	forwardedPorts = append(forwardedPorts, forwardExtraPorts(p)...)
	forwardedPorts = append(forwardedPorts, forwardAppPorts(p, result)...)
	forwardedPorts = append(forwardedPorts, forwardConfigPorts(p, result)...)

	return forwardedPorts, nil
}

// getContainerResult retrieves and parses the container result.
func getContainerResult(p portForwardParams) (*config2.Result, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := devssh.Run(p.ctx, p.containerClient, "cat "+setup.ResultLocation, nil, stdout, stderr, nil)
	if err != nil {
		return nil, fmt.Errorf("retrieve container result: %s\n%s%w", stdout.String(), stderr.String(), err)
	}

	result := &config2.Result{}
	err = json.Unmarshal(stdout.Bytes(), result)
	if err != nil {
		return nil, fmt.Errorf("error parsing container result %s %w", stdout.String(), err)
	}
	p.log.Debugf("parsed container result from %s", setup.ResultLocation)

	return result, nil
}

// forwardExtraPorts forwards extra ports specified by the user.
func forwardExtraPorts(p portForwardParams) []string {
	forwardedPorts := []string{}
	for _, port := range p.extraPorts {
		forwardedPorts = append(forwardedPorts, forwardPort(singlePortForwardParams{
			ctx:              p.ctx,
			containerClient:  p.containerClient,
			port:             port,
			exitAfterTimeout: p.exitAfterTimeout,
			log:              p.log,
		})...)
	}
	return forwardedPorts
}

// forwardAppPorts forwards application ports from the devcontainer config.
func forwardAppPorts(p portForwardParams, result *config2.Result) []string {
	forwardedPorts := []string{}
	for _, port := range result.MergedConfig.AppPort {
		forwardedPorts = append(forwardedPorts, forwardPort(singlePortForwardParams{
			ctx:              p.ctx,
			containerClient:  p.containerClient,
			port:             port,
			exitAfterTimeout: 0,
			log:              p.log,
		})...)
	}
	return forwardedPorts
}

// forwardConfigPorts forwards ports from the forwardPorts configuration.
func forwardConfigPorts(p portForwardParams, result *config2.Result) []string {
	forwardedPorts := []string{}
	for _, port := range result.MergedConfig.ForwardPorts {
		host, portNumber, err := parseForwardPort(port)
		if err != nil {
			p.log.Debugf("error parsing forwardPort %s: %v", port, err)
			continue
		}

		// Forward port asynchronously to avoid blocking
		go func(port string) {
			p.log.Debugf("forward port %s", port)
			err = devssh.PortForward(
				p.ctx,
				p.containerClient,
				"tcp",
				fmt.Sprintf("localhost:%d", portNumber),
				"tcp",
				fmt.Sprintf("%s:%d", host, portNumber),
				0,
				p.log,
			)
			if err != nil {
				p.log.Errorf("error port forwarding %s: %v", port, err)
			}
		}(port)

		forwardedPorts = append(forwardedPorts, port)
	}
	return forwardedPorts
}

// singlePortForwardParams contains parameters for forwarding a single port.
type singlePortForwardParams struct {
	ctx              context.Context
	containerClient  *ssh.Client
	port             string
	exitAfterTimeout time.Duration
	log              log.Logger
}

// forwardPort forwards a single port specification.
func forwardPort(p singlePortForwardParams) []string {
	parsed, err := nat.ParsePortSpec(p.port)
	if err != nil {
		p.log.Debugf("error parsing appPort %s: %v", p.port, err)
		return nil
	}

	// try to forward
	forwardedPorts := []string{}
	for _, parsedPort := range parsed {
		if parsedPort.Binding.HostIP == "" {
			parsedPort.Binding.HostIP = "localhost"
		}
		if parsedPort.Binding.HostPort == "" {
			parsedPort.Binding.HostPort = parsedPort.Port.Port()
		}
		go func(parsedPort nat.PortMapping) {
			// do the forward
			hostAddr := parsedPort.Binding.HostIP + ":" + parsedPort.Binding.HostPort
			containerAddr := "localhost:" + parsedPort.Port.Port()
			p.log.Debugf("forward port %s to %s", hostAddr, containerAddr)
			err = devssh.PortForward(
				p.ctx,
				p.containerClient,
				"tcp",
				hostAddr,
				"tcp",
				containerAddr,
				p.exitAfterTimeout,
				p.log,
			)
			if err != nil {
				p.log.Errorf(
					"error port forwarding %s:%s to %s: %v",
					parsedPort.Binding.HostIP,
					parsedPort.Binding.HostPort,
					parsedPort.Port.Port(),
					err,
				)
			}
		}(parsedPort)

		forwardedPorts = append(forwardedPorts, parsedPort.Binding.HostPort)
	}

	return forwardedPorts
}

// parseForwardPort parses a port specification into host and port number.
func parseForwardPort(port string) (string, int64, error) {
	tokens := strings.Split(port, ":")

	if len(tokens) == 1 {
		port, err := strconv.ParseInt(tokens[0], 10, 64)
		if err != nil {
			return "", 0, err
		}
		return "localhost", port, nil
	}

	if len(tokens) == 2 {
		port, err := strconv.ParseInt(tokens[1], 10, 64)
		if err != nil {
			return "", 0, err
		}
		return tokens[0], port, nil
	}

	return "", 0, fmt.Errorf("invalid forwardPorts port")
}
