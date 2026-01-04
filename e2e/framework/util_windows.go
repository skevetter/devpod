//go:build windows

package framework

import "os"

func getFileOwnership(info os.FileInfo) (string, string) {
	return "?", "?"
}
