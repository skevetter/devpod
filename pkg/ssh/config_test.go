package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SSHConfigTestSuite struct {
	suite.Suite
}

func TestSSHConfigSuite(t *testing.T) {
	suite.Run(t, new(SSHConfigTestSuite))
}

func (s *SSHConfigTestSuite) TestAddHostSection() {
	tests := []struct {
		name       string
		config     string
		execPath   string
		host       string
		user       string
		context    string
		workspace  string
		workdir    string
		command    string
		gpgagent   bool
		devPodHome string
		provider   string
		expected   string
	}{
		{
			name:       "Basic host addition",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "AWS provider with ConnectTimeout",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "aws",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ConnectTimeout 60
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Basic host addition with DEVPOD_HOME",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "C:\\\\White Space\\devpod\\test",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace --devpod-home "C:\\White Space\devpod\test"
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Host addition with workdir",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "/path/to/workdir",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace --workdir "/path/to/workdir"
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Host addition with gpg agent",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   true,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace --gpg-agent-forwarding
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Host addition with custom command",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "ssh -W %h:%p bastion",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "ssh -W %h:%p bastion"
  User testuser
# DevPod End testhost`,
		},
		{
			name: "Host addition to existing config",
			config: `Host existinghost
  User existinguser`,
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost
Host existinghost
  User existinguser`,
		},
		{
			name: "Host addition to existing config with DevPod host",
			config: `# DevPod Start existingtesthost
Host existingtesthost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost

Host existinghost
  User existinguser`,
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost
# DevPod Start existingtesthost
Host existingtesthost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost

Host existinghost
  User existinguser`,
		},
		{
			name: "Host addition after top level includes",
			config: `Include ~/config1

Include ~/config2



Include ~/config3`,
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `Include ~/config1

Include ~/config2



Include ~/config3
# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost`,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result, err := addHostSection(tt.config, tt.execPath, addHostParams{
				path:       "",
				host:       tt.host,
				user:       tt.user,
				context:    tt.context,
				workspace:  tt.workspace,
				workdir:    tt.workdir,
				command:    tt.command,
				gpgagent:   tt.gpgagent,
				devPodHome: tt.devPodHome,
				provider:   tt.provider,
			})

			assert.NoError(s.T(), err)
			assert.Equal(s.T(), tt.expected, result)
			assert.Contains(s.T(), result, MarkerEndPrefix+tt.host)
			assert.Contains(s.T(), result, "Host "+tt.host)
			assert.Contains(s.T(), result, "User "+tt.user)

			if tt.command != "" {
				assert.Contains(s.T(), result, "ProxyCommand \""+tt.command+"\"")
			}

			if tt.workdir != "" {
				assert.Contains(s.T(), result, "--workdir \""+tt.workdir+"\"")
			}

			if tt.gpgagent {
				assert.Contains(s.T(), result, "--gpg-agent-forwarding")
			}

			if tt.config != "" {
				assert.Contains(s.T(), result, tt.config)
			}
		})
	}
}
