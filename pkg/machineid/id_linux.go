//go:build linux

package machineid

import (
	"fmt"
	"os"
	"strings"
)

func ID() (string, error) {
	for _, p := range []string{"/var/lib/dbus/machine-id", "/etc/machine-id"} {
		b, err := os.ReadFile(p) //nolint:gosec
		if err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("failed to read machine-id")
}
