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

// getExitAfterTimeout calculates the timeout value based on configuration
func getExitAfterTimeout(devPodConfig *config.Config) time.Duration {
	if devPodConfig.ContextOption(config.ContextOptionExitAfterTimeout) != "true" {
		return 0
	}
	return time.Second * 5
}

// createForwarder creates a port forwarder if port forwarding is enabled
func createForwarder(opts RunServicesOptions, forwardedPorts []string) netstat.Forwarder {
	if !opts.ForwardPorts {
		return nil
	}
	ports := append([]string{}, forwardedPorts...)
	ports = append(ports, fmt.Sprintf("%d", openvscode.DefaultVSCodePort))
	return newForwarder(opts.ContainerClient, ports, opts.Log)
}

// runTunnelServer runs the tunnel server in a goroutine
func runTunnelServer(ctx context.Context, opts RunServicesOptions, stdoutReader *os.File, stdinWriter *os.File, forwarder netstat.Forwarder, errChan chan error, cancel context.CancelFunc) {
	defer cancel()
	defer func() { _ = stdinWriter.Close() }()
	err := tunnelserver.RunServicesServer(
		ctx,
		stdoutReader,
		stdinWriter,
		opts.ConfigureGitCredentials,
		opts.ConfigureDockerCredentials,
		forwarder,
		opts.Workspace,
		opts.Log,
		tunnelserver.WithPlatformOptions(opts.PlatformOptions),
	)
	if err != nil {
		errChan <- fmt.Errorf("run tunnel server %w", err)
	}
	close(errChan)
}

// addGitSSHSigningKey adds SSH signing key to command if configured
func addGitSSHSigningKey(command string) string {
	format, userSigningKey, err := gitsshsigning.ExtractGitConfiguration()
	if err == nil && format == gitsshsigning.GPGFormatSSH && userSigningKey != "" {
		encodedKey := base64.StdEncoding.EncodeToString([]byte(userSigningKey))
		command += fmt.Sprintf(" --git-user-signing-key %s", encodedKey)
	}
	return command
}

// buildCredentialsCommand builds the credentials server command
func buildCredentialsCommand(opts RunServicesOptions) string {
	command := fmt.Sprintf(
		"'%s' agent container credentials-server --user '%s'",
		agent.ContainerDevPodHelperLocation,
		opts.User,
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
		command += " --debug"
	}
	return command
}

// runServicesIteration performs one iteration of the retry loop
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

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	forwarder := createForwarder(opts, forwardedPorts)

	errChan := make(chan error, 1)
	go runTunnelServer(cancelCtx, opts, stdoutReader, stdinWriter, forwarder, errChan, cancel)

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

	forwardedPorts, err := forwardDevContainerPorts(ctx, opts.ContainerClient, opts.ExtraPorts, exitAfterTimeout, opts.Log)
	if err != nil {
		return fmt.Errorf("forward ports %w", err)
	}

	return retry.OnError(wait.Backoff{
		Steps:    math.MaxInt,
		Duration: 500 * time.Millisecond,
		Factor:   1,
		Jitter:   0.1,
	}, func(err error) bool {
		return true
	}, func() error {
		return runServicesIteration(ctx, opts, forwardedPorts)
	})
}

// forwardDevContainerPorts forwards all the ports defined in the devcontainer.json
func forwardDevContainerPorts(ctx context.Context, containerClient *ssh.Client, extraPorts []string, exitAfterTimeout time.Duration, log log.Logger) ([]string, error) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := devssh.Run(ctx, containerClient, "cat "+setup.ResultLocation, nil, stdout, stderr, nil)
	if err != nil {
		return nil, fmt.Errorf("retrieve container result: %s\n%s%w", stdout.String(), stderr.String(), err)
	}

	// parse result
	result := &config2.Result{}
	err = json.Unmarshal(stdout.Bytes(), result)
	if err != nil {
		return nil, fmt.Errorf("error parsing container result %s %w", stdout.String(), err)
	}
	log.WithFields(logrus.Fields{
		"location": setup.ResultLocation,
	}).Debug("parsed container result")

	// return forwarded ports
	forwardedPorts := []string{}

	// extra ports
	for _, port := range extraPorts {
		forwardedPorts = append(forwardedPorts, forwardPort(ctx, containerClient, port, exitAfterTimeout, log)...)
	}

	// app ports
	for _, port := range result.MergedConfig.AppPort {
		forwardedPorts = append(forwardedPorts, forwardPort(ctx, containerClient, port, 0, log)...)
	}

	// forward ports
	for _, port := range result.MergedConfig.ForwardPorts {
		// convert port
		host, portNumber, err := parseForwardPort(port)
		if err != nil {
			log.Debugf("Error parsing forwardPort %s: %v", port, err)
		}

		// try to forward
		go func(port string) {
			log.Debugf("Forward port %s", port)
			err = devssh.PortForward(
				ctx,
				containerClient,
				"tcp",
				fmt.Sprintf("localhost:%d", portNumber),
				"tcp",
				fmt.Sprintf("%s:%d", host, portNumber),
				0,
				log,
			)
			if err != nil {
				log.Errorf("Error port forwarding %s: %v", port, err)
			}
		}(port)

		forwardedPorts = append(forwardedPorts, port)
	}

	return forwardedPorts, nil
}

func forwardPort(ctx context.Context, containerClient *ssh.Client, port string, exitAfterTimeout time.Duration, log log.Logger) []string {
	parsed, err := nat.ParsePortSpec(port)
	if err != nil {
		log.Debugf("Error parsing appPort %s: %v", port, err)
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
			log.Debugf("Forward port %s:%s", parsedPort.Binding.HostIP+":"+parsedPort.Binding.HostPort, "localhost:"+parsedPort.Port.Port())
			err = devssh.PortForward(ctx, containerClient, "tcp", parsedPort.Binding.HostIP+":"+parsedPort.Binding.HostPort, "tcp", "localhost:"+parsedPort.Port.Port(), exitAfterTimeout, log)
			if err != nil {
				log.Errorf("Error port forwarding %s:%s:%s: %v", parsedPort.Binding.HostIP, parsedPort.Binding.HostPort, parsedPort.Port.Port(), err)
			}
		}(parsedPort)

		forwardedPorts = append(forwardedPorts, parsedPort.Binding.HostPort)
	}

	return forwardedPorts
}

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
