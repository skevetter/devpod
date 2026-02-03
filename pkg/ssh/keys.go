package ssh

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/skevetter/devpod/pkg/provider"
	"github.com/skevetter/devpod/pkg/util"

	"golang.org/x/crypto/ssh"
)

var (
	DevPodSSHHostKeyFile    = "id_devpod_rsa_host"
	DevPodSSHPrivateKeyFile = "id_devpod_rsa"
	DevPodSSHPublicKeyFile  = "id_devpod_rsa.pub"
)

var keyLock sync.Mutex

func rsaKeyGen() (privateKey string, publicKey string, err error) {
	privateKeyRaw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate private key: %w", err)
	}

	return generateKeys(pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKeyRaw),
	}, privateKeyRaw)
}

func generateKeys(block pem.Block, cp crypto.Signer) (privateKey string, publicKey string, err error) {
	pkBytes := pem.EncodeToMemory(&block)
	privateKey = string(pkBytes)

	publicKeyRaw := cp.Public()
	p, err := ssh.NewPublicKey(publicKeyRaw)
	if err != nil {
		return "", "", err
	}
	publicKey = string(ssh.MarshalAuthorizedKey(p))

	return privateKey, publicKey, nil
}

func makeHostKey() (string, error) {
	privKey, _, err := rsaKeyGen()
	if err != nil {
		return "", err
	}

	return privKey, err
}

func makeSSHKeyPair() (string, string, error) {
	privKey, pubKey, err := rsaKeyGen()
	if err != nil {
		return "", "", err
	}

	return pubKey, privKey, err
}

func GetPrivateKeyRaw(context, workspaceID string) ([]byte, error) {
	workspaceDir, err := provider.GetWorkspaceDir(context, workspaceID)
	if err != nil {
		return nil, err
	}

	return GetPrivateKeyRawBase(workspaceDir)
}

func GetDevPodKeysDir() string {
	dir, err := util.UserHomeDir()
	if err == nil {
		tempDir := filepath.Join(dir, ".devpod", "keys")
		err = os.MkdirAll(tempDir, 0755)
		if err == nil {
			return tempDir
		}
	}

	tempDir := os.TempDir()
	return filepath.Join(tempDir, "devpod-ssh")
}

func GetDevPodHostKey() (string, error) {
	tempDir := GetDevPodKeysDir()
	return GetHostKeyBase(tempDir)
}

func GetDevPodPublicKey() (string, error) {
	tempDir := GetDevPodKeysDir()
	return GetPublicKeyBase(tempDir)
}

func GetDevPodPrivateKeyRaw() ([]byte, error) {
	tempDir := GetDevPodKeysDir()
	return GetPrivateKeyRawBase(tempDir)
}

func GetHostKey(context, workspaceID string) (string, error) {
	workspaceDir, err := provider.GetWorkspaceDir(context, workspaceID)
	if err != nil {
		return "", err
	}

	return GetHostKeyBase(workspaceDir)
}

func GetPrivateKeyRawBase(dir string) ([]byte, error) {
	keyLock.Lock()
	defer keyLock.Unlock()

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}

	// check if key pair exists
	privateKeyFile := filepath.Join(dir, DevPodSSHPrivateKeyFile)
	publicKeyFile := filepath.Join(dir, DevPodSSHPublicKeyFile)
	_, err = os.Stat(privateKeyFile)
	if err != nil {
		pubKey, privateKey, err := makeSSHKeyPair()
		if err != nil {
			return nil, fmt.Errorf("generate key pair: %w", err)
		}

		err = os.WriteFile(publicKeyFile, []byte(pubKey), 0644)
		if err != nil {
			return nil, fmt.Errorf("write public ssh key: %w", err)
		}

		err = os.WriteFile(privateKeyFile, []byte(privateKey), 0600)
		if err != nil {
			return nil, fmt.Errorf("write private ssh key: %w", err)
		}
	}

	// read private key
	out, err := os.ReadFile(privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read private ssh key: %w", err)
	}

	return out, nil
}

func GetHostKeyBase(dir string) (string, error) {
	keyLock.Lock()
	defer keyLock.Unlock()

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return "", err
	}

	// check if key pair exists
	hostKeyFile := filepath.Join(dir, DevPodSSHHostKeyFile)
	_, err = os.Stat(hostKeyFile)
	if err != nil {
		privateKey, err := makeHostKey()
		if err != nil {
			return "", fmt.Errorf("generate host key: %w", err)
		}

		err = os.WriteFile(hostKeyFile, []byte(privateKey), 0600)
		if err != nil {
			return "", fmt.Errorf("write host key: %w", err)
		}
	}

	// read public key
	out, err := os.ReadFile(hostKeyFile)
	if err != nil {
		return "", fmt.Errorf("read host ssh key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(out), nil
}

func GetPublicKeyBase(dir string) (string, error) {
	keyLock.Lock()
	defer keyLock.Unlock()

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return "", err
	}

	// check if key pair exists
	privateKeyFile := filepath.Join(dir, DevPodSSHPrivateKeyFile)
	publicKeyFile := filepath.Join(dir, DevPodSSHPublicKeyFile)
	_, err = os.Stat(privateKeyFile)
	if err != nil {
		pubKey, privateKey, err := makeSSHKeyPair()
		if err != nil {
			return "", fmt.Errorf("generate key pair: %w", err)
		}

		err = os.WriteFile(publicKeyFile, []byte(pubKey), 0644)
		if err != nil {
			return "", fmt.Errorf("write public ssh key: %w", err)
		}

		err = os.WriteFile(privateKeyFile, []byte(privateKey), 0600)
		if err != nil {
			return "", fmt.Errorf("write private ssh key: %w", err)
		}
	}

	// read public key
	out, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return "", fmt.Errorf("read public ssh key: %w", err)
	}

	return base64.StdEncoding.EncodeToString(out), nil
}

func GetPublicKey(context, workspaceID string) (string, error) {
	workspaceDir, err := provider.GetWorkspaceDir(context, workspaceID)
	if err != nil {
		return "", err
	}

	return GetPublicKeyBase(workspaceDir)
}
