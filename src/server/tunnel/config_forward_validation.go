package tunnel

import (
	"fmt"
	"strings"

	"lsyltunnel/src/internal/netaddr"
)

type ForwardValidationIssue struct {
	Index   int
	Field   string
	Message string
}

func (issue *ForwardValidationIssue) Error() string { return issue.Message }

// ValidateForwardRules is shared by the YAML loader and the server editor.
func ValidateForwardRules(forwards []ForwardConfig, knownUsers map[string]bool) *ForwardValidationIssue {
	seenReversePorts := map[int]string{}
	for index, forward := range forwards {
		issue := func(field, format string, args ...any) *ForwardValidationIssue {
			return &ForwardValidationIssue{Index: index, Field: field, Message: fmt.Sprintf(format, args...)}
		}
		direction := strings.TrimSpace(forward.Direction)
		if direction == "" {
			direction = DirectionClientToServer
		}
		reversePort := 0
		switch direction {
		case DirectionClientToServer:
			host, _, err := splitHostPort(forward.ServerTarget)
			if err != nil {
				return issue("server_target", "forward %q server_target is invalid", forward.Name)
			}
			if !netaddr.IsForwardTargetHost(host) {
				return issue("server_target", "forward %q server_target must be a loopback or private IP (127.0.0.1 or private IP)", forward.Name)
			}
		case DirectionServerToClient:
			if err := validateReverseListenFields(forward); err != nil {
				return &ForwardValidationIssue{Index: index, Field: "listen_addr", Message: err.Error()}
			}
			port := forward.ListenPort
			if port <= 0 {
				host, legacyPort, err := splitHostPort(forward.ListenAddr)
				if err != nil || host != "127.0.0.1" {
					return issue("listen_port", "forward %q listen_port is required and legacy listen_addr must be a loopback address (127.0.0.1)", forward.Name)
				}
				port = legacyPort
			}
			if port < 1 || port > 65535 {
				return issue("listen_port", "forward %q listen_port must be between 1 and 65535", forward.Name)
			}
			reversePort = port
			if strings.TrimSpace(forward.ListenAddr) != "" {
				host, _, err := splitHostPort(forward.ListenAddr)
				if err != nil || host != "127.0.0.1" {
					return issue("listen_addr", "forward %q listen_addr must be a loopback address (127.0.0.1)", forward.Name)
				}
			}
		default:
			return issue("direction", "forward %q has unsupported direction", forward.Name)
		}

		allowedUsers := map[string]bool{}
		for _, username := range forward.AllowedUsers {
			username = strings.TrimSpace(username)
			if username == "" {
				return issue("allowed_users", "forward %q allowed_users contains an empty username", forward.Name)
			}
			if allowedUsers[username] {
				return issue("allowed_users", "forward %q has duplicate allowed user %s", forward.Name, username)
			}
			if !knownUsers[username] {
				return issue("allowed_users", "forward %q allowed user %s does not exist", forward.Name, username)
			}
			allowedUsers[username] = true
		}
		if len(allowedUsers) == 0 {
			return issue("allowed_users", "forward %q requires at least one allowed user", forward.Name)
		}
		if direction == DirectionServerToClient {
			if len(allowedUsers) != 1 {
				return issue("allowed_users", "forward %q reverse port must belong to exactly one allowed user", forward.Name)
			}
			if existing, ok := seenReversePorts[reversePort]; ok {
				return issue("listen_port", "forward %q reverse listen_port %d duplicates forward %q", forward.Name, reversePort, existing)
			}
			seenReversePorts[reversePort] = forward.Name
		}
	}
	return nil
}
