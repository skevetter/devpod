package command

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

func WrapCommandError(stdout []byte, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		stdout: stdout,
		err:    err,
	}
}

type Error struct {
	stdout []byte
	err    error
}

func (e *Error) Error() string {
	message := ""
	if len(e.stdout) > 0 {
		message += string(e.stdout) + "\n"
	}

	var exitError *exec.ExitError
	if errors.As(e.err, &exitError) && len(exitError.Stderr) > 0 {
		message += string(exitError.Stderr) + "\n"
	}

	return message + e.err.Error()
}

func Exists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func ExistsForUser(cmd, user string) bool {
	command := "which " + cmd
	var err error
	if user == "" {
		return Exists(cmd)
	}

	_, err = exec.Command("su", user, "-l", "-c", command).CombinedOutput()
	return err == nil
}

func RefreshPath() {
	path := os.Getenv("PATH")
	commonPaths := []string{"/usr/local/bin", "/usr/bin", "/bin"}
	for _, p := range commonPaths {
		if !strings.Contains(path, p) {
			if path == "" {
				path = p
			} else {
				path = p + ":" + path
			}
		}
	}
	os.Setenv("PATH", path)
}
