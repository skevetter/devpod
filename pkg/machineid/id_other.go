//go:build !linux && !darwin && !windows

package machineid

import "fmt"

func ID() (string, error) { return "", fmt.Errorf("unsupported platform") }
