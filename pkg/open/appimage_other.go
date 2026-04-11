//go:build !linux

package open

func isAppImage() bool {
	return false
}

func openURLSanitized(_ string) error {
	panic("openURLSanitized is only available on Linux")
}
