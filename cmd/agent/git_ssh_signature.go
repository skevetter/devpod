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

type GitSSHSignatureCmd struct {
	*flags.GlobalFlags

	CertPath   string
	Namespace  string
	BufferFile string
	Command    string
}

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
	cmd := &GitSSHSignatureCmd{
		GlobalFlags: flags,
	}

	gitSSHSignatureCmd := &cobra.Command{
		Use: "git-ssh-signature",
		// Allow unknown flags so that git can pass any ssh-keygen flags
		// (e.g. -U for stdin input) without cobra rejecting them.
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			UnknownFlags: true,
		},
		RunE: func(_ *cobra.Command, args []string) error {
			logger := log.GetInstance()

			// For non-sign operations (verify, find-principals, check-novalidate),
			// delegate command to system ssh-keygen since op does not require the tunnel.
			if cmd.Command != "sign" {
				return delegateToSSHKeygen(logger)
			}

			// Sign operation requires a buffer file
			if len(args) < 1 {
				return fmt.Errorf(
					"buffer file is required (received %d positional args: %v)",
					len(args), args,
				)
			}

			// The last argument is the buffer file
			cmd.BufferFile = args[len(args)-1]

			return gitsshsigning.HandleGitSSHProgramCall(
				cmd.CertPath, cmd.Namespace, cmd.BufferFile, logger)
		},
	}

	gitSSHSignatureCmd.Flags().StringVarP(&cmd.CertPath, "file", "f", "", "Path to the private key")
	gitSSHSignatureCmd.Flags().StringVarP(&cmd.Namespace, "namespace", "n", "", "Namespace")
	gitSSHSignatureCmd.Flags().
		StringVarP(&cmd.Command, "command", "Y", "sign", "Command - should be 'sign'")

	return gitSSHSignatureCmd
}

// delegateToSSHKeygen forwards the original arguments to the system ssh-keygen binary.
func delegateToSSHKeygen(logger log.Logger) error {
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return fmt.Errorf("find ssh-keygen: %w", err)
	}

	// Extract the arguments that were originally passed to this command.
	// Find "git-ssh-signature" in os.Args and take everything after it.
	var sshArgs []string
	for i, arg := range os.Args {
		if strings.HasSuffix(arg, "git-ssh-signature") {
			sshArgs = os.Args[i+1:]
			break
		}
	}
	if sshArgs == nil {
		return fmt.Errorf("git-ssh-signature not found in process arguments")
	}

	logger.Debugf("delegating to ssh-keygen: %s %v", sshKeygen, sshArgs)

	c := exec.Command(sshKeygen, sshArgs...) // #nosec G204,G304,G702
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c.Run()
}
