package port

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Address struct {
	Protocol string
	Address  string
}

type Mapping struct {
	Host      Address
	Container Address
}

func ParsePortSpec(port string) (Mapping, error) {
	parts, err := splitParts(port)
	if err != nil {
		return Mapping{}, err
	}

	hostAddress, err := toAddress(parts.host, parts.hostPort, addressOptions{
		emptyHostLabel: "listen host",
		requireHost:    parts.explicitHost,
	})
	if err != nil {
		return Mapping{}, fmt.Errorf("parse host address: %w", err)
	}

	containerAddress, err := toAddress(parts.container, parts.containerPort, addressOptions{
		emptyHostLabel: "target host",
		requireHost:    parts.explicitContainer,
		allowHostnames: true,
	})
	if err != nil {
		return Mapping{}, fmt.Errorf("parse container address: %w", err)
	}

	return Mapping{
		Host:      hostAddress,
		Container: containerAddress,
	}, nil
}

type splitResult struct {
	host              string
	hostPort          string
	container         string
	containerPort     string
	explicitHost      bool
	explicitContainer bool
}

type addressOptions struct {
	emptyHostLabel string
	requireHost    bool
	allowHostnames bool
}

func toAddress(ip, port string, opts addressOptions) (Address, error) {
	if isPortNumber(port) {
		return toTCPAddress(ip, port, opts)
	}

	return toUnixAddress(ip, port, opts)
}

func toTCPAddress(ip, port string, opts addressOptions) (Address, error) {
	if ip == "" {
		if opts.requireHost {
			return Address{}, fmt.Errorf("%s is empty", opts.emptyHostLabel)
		}

		ip = "localhost"
	}

	if !opts.allowHostnames && ip != "localhost" && net.ParseIP(ip) == nil {
		return Address{}, fmt.Errorf("not an ip address %s", ip)
	}

	return Address{
		Protocol: "tcp",
		Address:  ip + ":" + port,
	}, nil
}

func toUnixAddress(ip, port string, opts addressOptions) (Address, error) {
	if port == "" {
		return Address{}, fmt.Errorf("%s is empty", opts.emptyHostLabel)
	}

	if opts.requireHost && ip == "" {
		return Address{}, fmt.Errorf("%s is empty", opts.emptyHostLabel)
	}

	if ip != "" {
		return Address{}, fmt.Errorf("unexpected ip address for unix socket: %s", ip)
	}

	return Address{
		Protocol: "unix",
		Address:  port,
	}, nil
}

func isPortNumber(raw string) bool {
	_, err := strconv.Atoi(raw)
	return err == nil
}

func splitParts(rawport string) (splitResult, error) {
	parts := strings.Split(rawport, ":")
	n := len(parts)
	containerport := parts[n-1]

	switch n {
	case 1:
		return splitResult{hostPort: containerport, containerPort: containerport}, nil
	case 2:
		return splitResult{hostPort: parts[0], containerPort: containerport}, nil
	case 3:
		if isPortNumber(parts[0]) {
			return splitResult{
				hostPort:          parts[0],
				container:         parts[1],
				containerPort:     containerport,
				explicitContainer: true,
			}, nil
		}

		return splitResult{
			host:          parts[0],
			hostPort:      parts[1],
			containerPort: containerport,
			explicitHost:  true,
		}, nil
	case 4:
		return splitResult{
			host:              parts[0],
			hostPort:          parts[1],
			container:         parts[2],
			containerPort:     parts[3],
			explicitHost:      true,
			explicitContainer: true,
		}, nil
	default:
		return splitResult{}, fmt.Errorf("unexpected port format: %s", rawport)
	}
}
