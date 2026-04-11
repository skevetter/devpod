//go:build !linux

package open

import "errors"

func isAppImage() bool {
	return false
}

func openURLSanitized(_ string) error {
	return errors.New("openURLSanitized is only available on Linux")
}
