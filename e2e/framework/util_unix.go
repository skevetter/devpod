//go:build !windows

package framework

import (
	"fmt"
	"os"
	"syscall"
)

func getFileOwnership(info os.FileInfo) (string, string) {
	uid, gid := "?", "?"
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = fmt.Sprintf("%d", stat.Uid)
		gid = fmt.Sprintf("%d", stat.Gid)
	}
	return uid, gid
}
