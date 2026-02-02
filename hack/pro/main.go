package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed provider.yaml
var provider string

func main() {
	if len(os.Args) < 2 {
		panic("usage: go run main.go <version> [base_path]")
	}

	basePath := "./bin"
	if len(os.Args) > 2 {
		basePath = os.Args[2]
	}

	checksumMap := map[string]string{
		filepath.Join(basePath, "devpod-linux-amd64"):       "##CHECKSUM_LINUX_AMD64##",
		filepath.Join(basePath, "devpod-linux-arm64"):       "##CHECKSUM_LINUX_ARM64##",
		filepath.Join(basePath, "devpod-darwin-amd64"):      "##CHECKSUM_DARWIN_AMD64##",
		filepath.Join(basePath, "devpod-darwin-arm64"):      "##CHECKSUM_DARWIN_ARM64##",
		filepath.Join(basePath, "devpod-windows-amd64.exe"): "##CHECKSUM_WINDOWS_AMD64##",
	}

	partial := os.Getenv("PARTIAL") == "true"
	sourceFile, ok := os.LookupEnv("SOURCE_FILE")
	absPath := ""

	if ok {
		var err error

		absPath, err = filepath.Abs(sourceFile)
		if err != nil {
			panic(err)
		}

		providerBytes, err := os.ReadFile(absPath)
		if err != nil {
			panic(err)
		}

		provider = string(providerBytes)
	}

	replaced := strings.ReplaceAll(provider, "##VERSION##", os.Args[1])
	for k, v := range checksumMap {
		checksum, err := File(k)
		if err != nil {
			if partial {
				continue
			}

			panic(fmt.Errorf("generate checksum for %s: %w", k, err))
		}

		replaced = strings.ReplaceAll(replaced, v, checksum)
	}

	if ok {
		err := os.WriteFile(absPath, []byte(replaced), 0644)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println(replaced)
	}
}

// File hashes a given file to a sha256 string
func File(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()

	_, err = io.Copy(hash, file)
	if err != nil {
		return "", err
	}

	return strings.ToLower(hex.EncodeToString(hash.Sum(nil))), nil
}
