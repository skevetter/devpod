package dockerinstall

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type ValidatorTestSuite struct {
	suite.Suite
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	opts   *InstallOptions
}

func (s *ValidatorTestSuite) SetupTest() {
	s.stdout = &bytes.Buffer{}
	s.stderr = &bytes.Buffer{}
	s.opts = &InstallOptions{
		stdout: s.stdout,
		stderr: s.stderr,
		dryRun: false,
	}
}

func TestValidatorSuite(t *testing.T) {
	suite.Run(t, new(ValidatorTestSuite))
}

func (s *ValidatorTestSuite) TestValidateOS_Darwin() {
	validator := NewValidator(s.opts)
	err := validator.ValidateOS("darwin")
	s.Error(err)
	s.Contains(s.stderr.String(), "ERROR: Unsupported operating system 'macOS'")
}

func (s *ValidatorTestSuite) TestValidateOS_Linux() {
	validator := NewValidator(s.opts)
	err := validator.ValidateOS("linux")
	s.NoError(err)
}

func (s *ValidatorTestSuite) TestValidateOS_Unsupported() {
	validator := NewValidator(s.opts)
	err := validator.ValidateOS("windows")
	s.Error(err)
	s.Contains(err.Error(), "only supported on Linux")
}

func (s *ValidatorTestSuite) TestCheckWSL_NotWSL() {
	validator := NewValidator(s.opts)
	validator.CheckWSL(false)
	s.Empty(s.stdout.String())
	s.Empty(s.stderr.String())
}

func (s *ValidatorTestSuite) TestCheckWSL_IsWSL_DryRun() {
	s.opts.dryRun = true
	validator := NewValidator(s.opts)
	validator.CheckWSL(true)
	s.Contains(s.stdout.String(), "WSL DETECTED")
	s.Contains(s.stderr.String(), "Ctrl+C")
}

func (s *ValidatorTestSuite) TestCheckDeprecation_NotDeprecated() {
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "ubuntu", Version: "22.04"}
	validator.CheckDeprecation(distro)
	s.Empty(s.stdout.String())
}

func (s *ValidatorTestSuite) TestCheckDeprecation_DeprecatedUbuntu_DryRun() {
	s.opts.dryRun = true
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "ubuntu", Version: "xenial"}
	validator.CheckDeprecation(distro)
	s.Contains(s.stdout.String(), "DEPRECATION WARNING")
	s.Contains(s.stdout.String(), "ubuntu xenial")
}

func (s *ValidatorTestSuite) TestCheckDeprecation_DeprecatedDebian() {
	s.opts.dryRun = true
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "debian", Version: "stretch"}
	validator.CheckDeprecation(distro)
	s.Contains(s.stdout.String(), "DEPRECATION WARNING")
	s.Contains(s.stdout.String(), "debian stretch")
}

func (s *ValidatorTestSuite) TestCheckDeprecation_DeprecatedFedora() {
	s.opts.dryRun = true
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "fedora", Version: "32"}
	validator.CheckDeprecation(distro)
	s.Contains(s.stdout.String(), "DEPRECATION WARNING")
}

func (s *ValidatorTestSuite) TestCheckDeprecation_CurrentFedora() {
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "fedora", Version: "39"}
	validator.CheckDeprecation(distro)
	s.Empty(s.stdout.String())
}

func (s *ValidatorTestSuite) TestValidateDistro_EmptyID() {
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "", Version: "22.04"}
	err := validator.ValidateDistro(distro)
	s.Error(err)
	s.Contains(s.stderr.String(), "Unable to detect distribution")
}

func (s *ValidatorTestSuite) TestValidateDistro_Supported() {
	supported := []string{"ubuntu", "debian", "raspbian", "centos", "fedora", "rhel", "sles"}
	for _, id := range supported {
		s.SetupTest()
		validator := NewValidator(s.opts)
		distro := &Distro{ID: id, Version: "1.0"}
		err := validator.ValidateDistro(distro)
		s.NoError(err, "Expected %s to be supported", id)
	}
}

func (s *ValidatorTestSuite) TestValidateDistro_Unsupported() {
	validator := NewValidator(s.opts)
	distro := &Distro{ID: "arch", Version: "rolling"}
	err := validator.ValidateDistro(distro)
	s.Error(err)
	s.Contains(s.stderr.String(), "Unsupported distribution 'arch'")
}

func (s *ValidatorTestSuite) TestSleep_DryRun() {
	s.opts.dryRun = true
	validator := NewValidator(s.opts)
	start := time.Now()
	validator.sleep(1 * time.Second)
	elapsed := time.Since(start)
	s.Less(elapsed, 100*time.Millisecond, "Should not actually sleep in dry-run")
}

func (s *ValidatorTestSuite) TestSleep_NotDryRun() {
	validator := NewValidator(s.opts)
	start := time.Now()
	validator.sleep(50 * time.Millisecond)
	elapsed := time.Since(start)
	s.GreaterOrEqual(elapsed, 50*time.Millisecond, "Should actually sleep")
}
