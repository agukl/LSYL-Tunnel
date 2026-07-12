//go:build windows

package gui

import (
	"fmt"
	"net"
	"strings"
	"time"

	"lsyltunnel/src/server/tunnel"
)

const (
	adminIssueError   = "error"
	adminIssueWarning = "warning"
)

type adminConfigIssue struct {
	Level   string `json:"level"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type adminConfigIssues []adminConfigIssue

type adminForwardValidationOptions struct {
	Existing                       tunnel.Config
	ServiceRunning                 bool
	CheckForwardTargetReachability bool
	CheckPassivePortAvailability   bool
	DialTimeout                    time.Duration
}

func (issues adminConfigIssues) hasErrors() bool {
	for _, issue := range issues {
		if issue.Level == adminIssueError {
			return true
		}
	}
	return false
}

func (issues adminConfigIssues) summary() string {
	for _, issue := range issues {
		if issue.Level == adminIssueError {
			if issue.Field != "" {
				return issue.Field + ": " + issue.Message
			}
			return issue.Message
		}
	}
	if len(issues) == 0 {
		return ""
	}
	if issues[0].Field != "" {
		return issues[0].Field + ": " + issues[0].Message
	}
	return issues[0].Message
}

func (issues *adminConfigIssues) addError(field, message string) {
	*issues = append(*issues, adminConfigIssue{Level: adminIssueError, Field: field, Message: message})
}

func validateAdminForwardsForSave(form adminConfig) adminConfigIssues {
	return validateAdminForwardsForSaveWithOptions(form, adminForwardValidationOptions{})
}

func validateAdminForwardsForSaveWithOptions(form adminConfig, opts adminForwardValidationOptions) adminConfigIssues {
	issues := adminConfigIssues{}
	users := adminUserSet(form.Users)
	passiveOwners := map[string]string{}
	existingPassivePorts := existingPassiveForwardPorts(opts.Existing)
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 800 * time.Millisecond
	}
	for i, item := range form.Forwards {
		if adminForwardIsEmpty(item) {
			continue
		}
		fieldPrefix := fmt.Sprintf("forwards[%d]", i)
		direction := strings.TrimSpace(item.Direction)
		if direction == "" {
			direction = tunnel.DirectionClientToServer
		}
		if direction != tunnel.DirectionClientToServer && direction != tunnel.DirectionServerToClient {
			issues.addError(fieldPrefix+".direction", "direction must be client_to_server or server_to_client")
		}

		allowedUsers := cleanAllowedUsers(append(item.AllowedUsers, item.Owner))
		if len(allowedUsers) == 0 {
			issues.addError(fieldPrefix+".allowed_users", "at least one allowed user is required")
			continue
		}
		for _, username := range allowedUsers {
			if !users[username] {
				issues.addError(fieldPrefix+".allowed_users", "allowed user "+username+" does not exist")
			}
		}
		if direction == tunnel.DirectionServerToClient {
			listenAddr := adminForwardListenAddrForValidation(item)
			host, err := adminForwardAddressHost(listenAddr)
			if err != nil || !isAdminLoopbackHost(host) {
				issues.addError(fieldPrefix+".listen_addr", "passive listen address must be a loopback address with port")
				continue
			}
			if len(allowedUsers) != 1 {
				issues.addError(fieldPrefix+".allowed_users", "passive ports must belong to exactly one allowed user")
			} else {
				owner := allowedUsers[0]
				normalized := normalizeAdminHostPort(listenAddr)
				if previousOwner, ok := passiveOwners[normalized]; ok && previousOwner != owner {
					issues.addError(fieldPrefix+".allowed_users", "passive port is already assigned to "+previousOwner)
				} else {
					passiveOwners[normalized] = owner
				}
			}
			if opts.CheckPassivePortAvailability && !shouldSkipPassivePortProbe(opts, existingPassivePorts, listenAddr) {
				if err := canListenOnTCP(listenAddr); err != nil {
					issues.addError(fieldPrefix+".port", "passive port is already in use: "+err.Error())
				}
			}
			continue
		}
		target := adminForwardTargetAddrForValidation(item)
		host, err := adminForwardAddressHost(target)
		if err != nil {
			issues.addError(fieldPrefix+".server_target", "target address must include a host and port")
			continue
		}
		if !isAdminLoopbackHost(host) && !isAdminPrivateIP(host) {
			issues.addError(fieldPrefix+".server_target", "target must be a loopback or private IP address")
			continue
		}
		if direction == tunnel.DirectionClientToServer && opts.CheckForwardTargetReachability {
			if err := canConnectTCP(target, dialTimeout); err != nil {
				issues.addError(fieldPrefix+".server_target", "forward target is not reachable: "+err.Error())
			}
		}
	}
	return issues
}

func adminForwardIsEmpty(item adminForward) bool {
	return strings.TrimSpace(item.Name) == "" &&
		strings.TrimSpace(item.Port) == "" &&
		strings.TrimSpace(item.ListenAddr) == "" &&
		strings.TrimSpace(item.ServerTarget) == "" &&
		len(cleanAllowedUsers(append(item.AllowedUsers, item.Owner))) == 0
}

func adminForwardTargetAddrForValidation(item adminForward) string {
	if target := strings.TrimSpace(item.ServerTarget); target != "" {
		return target
	}
	if port := strings.TrimSpace(item.Port); port != "" {
		return net.JoinHostPort(serverLocalForwardHost, port)
	}
	return strings.TrimSpace(item.ServerTarget)
}

func adminForwardListenAddrForValidation(item adminForward) string {
	if listenAddr := strings.TrimSpace(item.ListenAddr); listenAddr != "" {
		return listenAddr
	}
	if port := strings.TrimSpace(item.Port); port != "" {
		return net.JoinHostPort(serverLocalForwardHost, port)
	}
	return strings.TrimSpace(item.ListenAddr)
}

func adminForwardAddressHost(addr string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil || strings.Trim(host, "[]") == "" {
		return "", fmt.Errorf("invalid address")
	}
	if _, err := parseForwardPort(port); err != nil {
		return "", err
	}
	return strings.Trim(host, "[]"), nil
}

func isAdminLoopbackHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isAdminPrivateIP(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsPrivate()
}

func adminUserSet(users []adminUser) map[string]bool {
	out := map[string]bool{}
	for _, user := range users {
		username := strings.TrimSpace(user.Username)
		if username != "" {
			out[username] = true
		}
	}
	return out
}

func existingPassiveForwardPorts(cfg tunnel.Config) map[string]bool {
	out := map[string]bool{}
	for _, fwd := range cfg.Forwards {
		if forwardDirectionOrDefault(fwd.Direction) != tunnel.DirectionServerToClient {
			continue
		}
		listenAddr := strings.TrimSpace(fwd.ListenAddr)
		if listenAddr != "" {
			out[normalizeAdminHostPort(listenAddr)] = true
		}
	}
	return out
}

func shouldSkipPassivePortProbe(opts adminForwardValidationOptions, existingPassivePorts map[string]bool, listenAddr string) bool {
	if !opts.ServiceRunning {
		return false
	}
	return existingPassivePorts[normalizeAdminHostPort(listenAddr)]
}

func canListenOnTCP(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ln.Close()
}

func canConnectTCP(addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

func normalizeAdminHostPort(addr string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return strings.ToLower(strings.TrimSpace(addr))
	}
	return net.JoinHostPort(strings.ToLower(strings.Trim(host, "[]")), port)
}
