//go:build windows

package daemon

import (
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func GetSocketAddr(providerName string) string {
	return fmt.Sprintf("\\\\.\\pipe\\devpod.%s", providerName)
}

func Dial(addr string) (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(addr, &timeout)
}

func listen(addr string) (net.Listener, error) {
	return winio.ListenPipe(addr, nil)
}
