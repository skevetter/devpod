package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/skevetter/devpod/cmd/flags"
	"github.com/skevetter/devpod/pkg/gitsshsigning"
	"github.com/skevetter/log"
	"github.com/spf13/cobra"
)

// NewGitSSHSignatureCmd creates new git-ssh-signature command
// This agent command can be used as git ssh program by setting
//
//	> git config --global gpg.ssh.program "devpod agent git-ssh-signature"
//
// Git by default uses ssh-keygen for signing commits with ssh. This CLI command is a drop-in
// replacement for ssh-keygen and hence needs to support ssh-keygen interface that git uses.
//
//	custom-ssh-signature-handler -Y sign -n git -f /Users/johndoe/.ssh/my-key.pub /tmp/.git_signing_buffer_tmp4Euk6d
func NewGitSSHSignatureCmd(flags *flags.GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use: "git-ssh-signature",
		// This command implements the ssh-keygen protocol used by git for commit
		// signing. Disable cobra's flag parsing so we can handle the ssh-keygen
		// argument format directly, including boolean flags like -U (ssh-agent
		// mode) that cobra cannot distinguish from flags that take a value.
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			logger := log.GetInstance()

			parsed := parseSSHKeygenArgs(args)

			// For non-sign operations (verify, find-principals, check-novalidate),
			// delegate command to system ssh-keygen since op does not require the tunnel.
			if parsed.command != "sign" {
				return delegateToSSHKeygen(args, logger)
			}

			if parsed.certPath == "" {
				return fmt.Errorf("key file (-f) is required")
			}
			if parsed.namespace == "" {
				return fmt.Errorf("namespace (-n) is required")
			}
			if parsed.bufferFile == "" {
				return fmt.Errorf("buffer file is required")
			}

			return gitsshsigning.HandleGitSSHProgramCall(
				parsed.certPath, parsed.namespace, parsed.bufferFile, logger)
		},
	}
}

// sshKeygenArgs holds the parsed result of a git ssh-keygen invocation.
type sshKeygenArgs struct {
	command    string // -Y value (sign, verify, find-principals, etc.)
	certPath   string // -f value (path to public key)
	namespace  string // -n value (always "git" for commit signing)
	bufferFile string // last non-flag argument
}

// parseSSHKeygenArgs parses the ssh-keygen argument format used by git:
//
//	-Y <command> -n <namespace> -f <key> [flags...] <buffer-file>
//
// The buffer file is always the last argument and is never a flag.
// Unknown flags (e.g. -U for ssh-agent mode) are ignored.
func parseSSHKeygenArgs(args []string) sshKeygenArgs {
	result := sshKeygenArgs{
		command: "sign",
	} // git only ever calls with sign, but default defensively
	consumed := make(map[int]bool)
	for i := 0; i < len(args); i++ {
		if next := consumeFlag(&result, args, i); next > i {
			consumed[i] = true
			consumed[next] = true
			i = next
		}
	}
	// The buffer file is always the last argument and is never a flag.
	lastIdx := len(args) - 1
	if lastIdx >= 0 && !consumed[lastIdx] && !strings.HasPrefix(args[lastIdx], "-") {
		result.bufferFile = args[lastIdx]
	}
	return result
}

// consumeFlag processes a single known flag from args at position i.
// Returns the index of the consumed value if a known flag-value pair is found,
// or i if no value was consumed.
func consumeFlag(result *sshKeygenArgs, args []string, i int) int {
	if i+1 >= len(args) {
		return i
	}
	next := args[i+1]

	switch args[i] {
	case "-Y":
		if strings.HasPrefix(next, "-") {
			return i
		}
		result.command = next
		return i + 1
	case "-f":
		if strings.HasPrefix(next, "-") {
			return i
		}
		result.certPath = next
		return i + 1
	case "-n":
		if strings.HasPrefix(next, "-") {
			return i
		}
		result.namespace = next
		return i + 1
	}
	return i
}

// delegateToSSHKeygen forwards args to the system ssh-keygen binary.
func delegateToSSHKeygen(args []string, logger log.Logger) error {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return fmt.Errorf("find ssh-keygen: %w", err)
	}

	logger.Debugf("delegating to ssh-keygen: %s %v", sshKeygen, args)

	c := exec.Command(sshKeygen, args...) // #nosec G204,G304,G702
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c.Run()
}
