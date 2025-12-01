package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: server <port>")
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", ":"+os.Args[1])
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("Listening on port %s\n", os.Args[1])

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			_, _ = c.Write([]byte("PONG\n"))
		}(conn)
	}
}
