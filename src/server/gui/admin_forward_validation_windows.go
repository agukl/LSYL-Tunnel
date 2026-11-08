//go:build windows

package gui

import (
	"fmt"
	"net"
	"strings"

	"lsyltunnel/src/internal/netaddr"
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
	Existing                     tunnel.Config
	ServiceRunning               bool
	CheckPassivePortAvailability bool
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
	passiveOwners := map[int]string{}
	existingPassivePorts := existingPassiveForwardPorts(opts.Existing)
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
		fwd, empty, err := adminForwardToTunnel(item)
		if err != nil || empty {
			if direction == tunnel.DirectionServerToClient {
				issues.addError(fieldPrefix+".listen_addr", "反向监听地址必须填写为 127.0.0.1:端口")
			} else {
				issues.addError(fieldPrefix+".server_target", "target address must include a host and port")
			}
			continue
		}
		if direction == tunnel.DirectionServerToClient {
			host, _, _ := net.SplitHostPort(fwd.ListenAddr)
			if host != serverLocalForwardHost {
				issues.addError(fieldPrefix+".listen_addr", "反向监听地址必须填写为 127.0.0.1:端口")
				continue
			}
			if len(allowedUsers) != 1 {
				issues.addError(fieldPrefix+".allowed_users", "passive ports must belong to exactly one allowed user")
			} else {
				owner := allowedUsers[0]
				if previousOwner, ok := passiveOwners[fwd.ListenPort]; ok {
					issues.addError(fieldPrefix+".port", "passive port is already assigned to "+previousOwner)
				} else {
					passiveOwners[fwd.ListenPort] = owner
				}
			}
			if opts.CheckPassivePortAvailability && !shouldSkipPassivePortProbe(opts, existingPassivePorts, fwd.ListenPort) {
				if err := canListenOnTCP(fwd.ListenAddr); err != nil {
					issues.addError(fieldPrefix+".port", "passive port is already in use: "+err.Error())
				}
			}
			continue
		}
		host, _, _ := net.SplitHostPort(fwd.ServerTarget)
		if !netaddr.IsForwardTargetHost(host) {
			issues.addError(fieldPrefix+".server_target", "目标地址必须使用 127.0.0.1 或私有 IP")
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

func existingPassiveForwardPorts(cfg tunnel.Config) map[int]bool {
	out := map[int]bool{}
	for _, fwd := range cfg.Forwards {
		if forwardDirectionOrDefault(fwd.Direction) != tunnel.DirectionServerToClient {
			continue
		}
		if port, err := parseForwardPort(forwardPortText(fwd)); err == nil {
			out[port] = true
		}
	}
	return out
}

func shouldSkipPassivePortProbe(opts adminForwardValidationOptions, existingPassivePorts map[int]bool, port int) bool {
	if !opts.ServiceRunning {
		return false
	}
	return existingPassivePorts[port]
}

func canListenOnTCP(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return ln.Close()
}
