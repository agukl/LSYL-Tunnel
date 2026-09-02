//go:build windows

package gui

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"lsyltunnel/src/server/tunnel"
)

const serverLocalForwardHost = "127.0.0.1"

type adminForward struct {
	Name         string   `json:"name"`
	Direction    string   `json:"direction"`
	Port         string   `json:"port"`
	ListenAddr   string   `json:"listen_addr"`
	ServerTarget string   `json:"server_target"`
	Owner        string   `json:"owner"`
	AllowedUsers []string `json:"allowed_users,omitempty"`
}

func forwardDirectionOrDefault(direction string) string {
	direction = strings.TrimSpace(direction)
	if direction == tunnel.DirectionServerToClient {
		return tunnel.DirectionServerToClient
	}
	return tunnel.DirectionClientToServer
}

func adminForwardToTunnel(item adminForward) (tunnel.ForwardConfig, bool, error) {
	name := strings.TrimSpace(item.Name)
	direction := forwardDirectionOrDefault(item.Direction)
	portText := strings.TrimSpace(item.Port)
	if name == "" && portText == "" && strings.TrimSpace(item.ListenAddr) == "" && strings.TrimSpace(item.ServerTarget) == "" {
		return tunnel.ForwardConfig{}, true, nil
	}
	fwd := tunnel.ForwardConfig{Name: name, Direction: direction}
	if direction == tunnel.DirectionServerToClient {
		addr, err := adminForwardAddress(strings.TrimSpace(item.ListenAddr), portText)
		if err != nil {
			return tunnel.ForwardConfig{}, false, fmt.Errorf("forward %q listen address is invalid", forwardDisplayName(item))
		}
		_, port, _ := net.SplitHostPort(addr)
		listenPort, _ := strconv.Atoi(port)
		fwd.ListenPort = listenPort
		fwd.ListenAddr = addr
	} else {
		addr, err := adminForwardAddress(strings.TrimSpace(item.ServerTarget), portText)
		if err != nil {
			return tunnel.ForwardConfig{}, false, fmt.Errorf("forward %q target address is invalid", forwardDisplayName(item))
		}
		fwd.ServerTarget = addr
	}
	return fwd, false, nil
}

func adminForwardAddress(addr, portText string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr != "" {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || strings.Trim(host, "[]") == "" {
			return "", fmt.Errorf("invalid address")
		}
		if _, err := parseForwardPort(port); err != nil {
			return "", err
		}
		return addr, nil
	}
	if _, err := parseForwardPort(portText); err != nil {
		return "", err
	}
	return net.JoinHostPort(serverLocalForwardHost, portText), nil
}

func forwardPortText(fwd tunnel.ForwardConfig) string {
	if forwardDirectionOrDefault(fwd.Direction) == tunnel.DirectionServerToClient && fwd.ListenPort > 0 {
		return strconv.Itoa(fwd.ListenPort)
	}
	target := strings.TrimSpace(fwd.ServerTarget)
	if forwardDirectionOrDefault(fwd.Direction) == tunnel.DirectionServerToClient {
		target = strings.TrimSpace(fwd.ListenAddr)
	}
	if target == "" {
		return ""
	}
	_, portText, err := net.SplitHostPort(target)
	if err != nil {
		return ""
	}
	return portText
}

func parseForwardPort(portText string) (int, error) {
	if portText == "" {
		return 0, fmt.Errorf("empty port")
	}
	for _, ch := range portText {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("port must be numeric")
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("port out of range")
	}
	return port, nil
}

func forwardDisplayName(fwd adminForward) string {
	if name := strings.TrimSpace(fwd.Name); name != "" {
		return name
	}
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	if target := strings.TrimSpace(fwd.ServerTarget); target != "" {
		return target
	}
	return "未命名转发"
}
