//go:build windows

package gui

import (
	"fmt"
	"net"
	"strings"

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
	forwards := make([]tunnel.ForwardConfig, 0, len(form.Forwards))
	indices := make([]int, 0, len(form.Forwards))
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
			continue
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
		fwd.AllowedUsers = cleanAllowedUsers(append(item.AllowedUsers, item.Owner))
		indices = append(indices, i)
		forwards = append(forwards, fwd)
	}
	if issues.hasErrors() {
		return issues
	}
	if issue := tunnel.ValidateForwardRules(forwards, users); issue != nil {
		field := issue.Field
		if field == "listen_port" {
			field = "port"
		}
		message := issue.Message
		if strings.Contains(message, "server_target must be a loopback or private IP") {
			message = "目标地址必须使用 127.0.0.1 或私有 IP"
		}
		issues.addError(fmt.Sprintf("forwards[%d].%s", indices[issue.Index], field), message)
		return issues
	}
	if opts.CheckPassivePortAvailability {
		existingPassivePorts := existingPassiveForwardPorts(opts.Existing)
		for j, fwd := range forwards {
			if fwd.Direction != tunnel.DirectionServerToClient || shouldSkipPassivePortProbe(opts, existingPassivePorts, fwd.ListenPort) {
				continue
			}
			if err := canListenOnTCP(fwd.ListenAddr); err != nil {
				issues.addError(fmt.Sprintf("forwards[%d].port", indices[j]), "passive port is already in use: "+err.Error())
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
