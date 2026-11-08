package tunnel

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/netaddr"

	"gopkg.in/yaml.v3"
)

type ForwardConfig struct {
	Name         string `yaml:"name"`
	Direction    string `yaml:"direction"`
	ListenAddr   string `yaml:"listen_addr,omitempty"`
	ServerTarget string `yaml:"server_target"`
}

func (f *ForwardConfig) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return fmt.Errorf("forward must be a mapping")
	}
	type rawForwardConfig ForwardConfig
	var raw rawForwardConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}
	direction := strings.TrimSpace(raw.Direction)
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = "<unnamed>"
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		field := node.Content[i].Value
		switch field {
		case "name", "direction", "listen_addr", "server_target":
			continue
		case "listen_port":
			if direction == DirectionServerToClient {
				return fmt.Errorf("forward %q server_to_client client config must not include listen_port; use listen_addr and server_target", name)
			}
			fallthrough
		default:
			return fmt.Errorf("forward %q contains unknown field %q", name, field)
		}
	}
	*f = ForwardConfig(raw)
	return nil
}

const (
	DirectionClientToServer = "client_to_server"
	DirectionServerToClient = "server_to_client"
	DirectionVirtual        = "virtual"
)

func validateReverseForwardFields(forwards []ForwardConfig) error {
	for _, fwd := range forwards {
		if strings.TrimSpace(fwd.Direction) != DirectionServerToClient {
			continue
		}
		listenAddr := strings.TrimSpace(fwd.ListenAddr)
		if listenAddr == "" {
			return fmt.Errorf("forward %q server_to_client requires listen_addr", fwd.Name)
		}
		host, portText, err := net.SplitHostPort(listenAddr)
		if err != nil || strings.Trim(strings.TrimSpace(host), "[]") == "" {
			return fmt.Errorf("forward %q reverse listen_addr must include a loopback host and port (127.0.0.1:port)", fwd.Name)
		}
		if port, err := strconv.Atoi(portText); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("forward %q reverse listen_addr port must be between 1 and 65535", fwd.Name)
		}
		if host != netaddr.LoopbackIPv4 {
			return fmt.Errorf("forward %q reverse listen_addr must use a loopback address (127.0.0.1)", fwd.Name)
		}
	}
	return nil
}
