package gitsshsigning

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

type GitSSHSignatureRequest struct {
	Content   string
	KeyPath   string
	PublicKey string // Public key content; when set, written to a temp file for ssh-keygen
}

type GitSSHSignatureResponse struct {
	Signature []byte
}

// Sign signs the content using the private key and returns the signature.
// This is intended to be a drop-in replacement for gpg.ssh.program for git,
// so we simply execute ssh-keygen in the same way as git would do locally.
//
// When PublicKey is set, it is written to a temporary file that ssh-keygen
// can read. This is necessary because the original KeyPath comes from
// inside the container and does not exist on the host where Sign() runs.
func (req *GitSSHSignatureRequest) Sign() (*GitSSHSignatureResponse, error) {
	keyFile, cleanup, err := req.resolveKeyFile()
	if err != nil {
		return nil, fmt.Errorf("resolve signing key: %w", err)
	}
	defer cleanup()

	var commitBuffer bytes.Buffer
	commitBuffer.WriteString(req.Content)

	//nolint:gosec // keyFile is a controlled temp path or validated KeyPath
	cmd := exec.Command(
		"ssh-keygen",
		"-Y",
		"sign",
		"-f",
		keyFile,
		"-n",
		"git",
	)
	cmd.Stdin = &commitBuffer

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to sign commit: %w, stderr: %s", err, stderr.String())
	}

	return &GitSSHSignatureResponse{
		Signature: out.Bytes(),
	}, nil
}

// resolveKeyFile returns the path to use for ssh-keygen -f and a cleanup function.
// When PublicKey content is available, it writes a temp file. Otherwise falls back to KeyPath.
func (req *GitSSHSignatureRequest) resolveKeyFile() (string, func(), error) {
	noop := func() {}

	if req.PublicKey == "" {
		return req.KeyPath, noop, nil
	}

	// ssh-keygen -Y sign -f <key> looks for <key>.pub to identify the agent key.
	// We write the public key to a temp file with a .pub suffix and pass the
	// path without the .pub suffix to -f.
	tmpFile, err := os.CreateTemp("", ".git_signing_key_*.pub")
	if err != nil {
		return "", noop, fmt.Errorf("create temp key file: %w", err)
	}
	pubPath := tmpFile.Name()
	// keyPath is the path without .pub, passed to ssh-keygen -f
	keyPath := pubPath[:len(pubPath)-len(".pub")]

	if _, err := tmpFile.WriteString(req.PublicKey); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(pubPath)
		return "", noop, fmt.Errorf("write public key: %w", err)
	}
	_ = tmpFile.Close()

	cleanup := func() {
		_ = os.Remove(pubPath)
	}

	return keyPath, cleanup, nil
}
