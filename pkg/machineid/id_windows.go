//go:build windows

package machineid

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

func ID() (string, error) {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE|registry.WOW64_64KEY,
	)
	if err != nil {
		return "", fmt.Errorf("open registry: %w", err)
	}
	defer k.Close()
	val, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return "", fmt.Errorf("read MachineGuid: %w", err)
	}
	return val, nil
}
