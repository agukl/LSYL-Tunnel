//go:build windows && !386

package tunnel

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	virtualRedirectHelperFlag    = "virtual-redirect-helper"
	virtualRedirectHelperSession = "session"
	virtualRedirectHelperAnchor  = "anchor"
)

type virtualRedirectHelperOptions struct {
	Action       string
	Rules        []virtualRedirectRule
	ControlAddr  string
	ControlToken string
	AnchorPID    uint32
}

func HandleVirtualRedirectHelperArgs(args []string) (bool, error) {
	opts, handled, err := parseVirtualRedirectHelperArgs(args)
	if !handled || err != nil {
		return handled, err
	}
	if opts.Action == virtualRedirectHelperAnchor {
		_, err := io.Copy(io.Discard, os.Stdin)
		return true, err
	}
	return true, runVirtualRedirectSessionHelper(opts)
}

func parseVirtualRedirectHelperArgs(args []string) (virtualRedirectHelperOptions, bool, error) {
	var opts virtualRedirectHelperOptions
	handled := false
	for _, arg := range args {
		name := strings.TrimLeft(strings.TrimSpace(arg), "-")
		if name == virtualRedirectHelperFlag || strings.HasPrefix(name, virtualRedirectHelperFlag+"=") {
			handled = true
			break
		}
	}
	if !handled {
		return opts, false, nil
	}
	fs := flag.NewFlagSet("virtual-redirect-helper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	action := fs.String(virtualRedirectHelperFlag, "", "")
	rulesText := fs.String("virtual-redirect-rules", "", "")
	controlAddr := fs.String("virtual-redirect-control", "", "")
	controlToken := fs.String("virtual-redirect-token", "", "")
	anchorPID := fs.Uint("virtual-redirect-anchor-pid", 0, "")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	if fs.NArg() != 0 {
		return opts, true, fmt.Errorf("unexpected virtual redirect helper arguments")
	}
	opts.Action = strings.ToLower(strings.TrimSpace(*action))
	if opts.Action != virtualRedirectHelperSession && opts.Action != virtualRedirectHelperAnchor {
		return opts, true, fmt.Errorf("virtual redirect helper action must be session or anchor")
	}
	if opts.Action == virtualRedirectHelperSession {
		var err error
		opts.Rules, err = decodeVirtualRedirectRules(*rulesText)
		if err != nil {
			return opts, true, err
		}
		opts.ControlAddr, err = normalizeVirtualRedirectControlAddress(*controlAddr)
		if err != nil {
			return opts, true, err
		}
		opts.ControlToken = strings.ToLower(strings.TrimSpace(*controlToken))
		if !validVirtualRedirectToken(opts.ControlToken) {
			return opts, true, fmt.Errorf("invalid virtual redirect helper control token")
		}
		if *anchorPID == 0 || *anchorPID > uint(^uint32(0)) {
			return opts, true, fmt.Errorf("virtual redirect helper requires a valid anchor PID")
		}
		opts.AnchorPID = uint32(*anchorPID)
	}
	return opts, true, nil
}

func runVirtualRedirectSessionHelper(opts virtualRedirectHelperOptions) error {
	control, err := connectVirtualRedirectControl(opts.ControlAddr, opts.ControlToken)
	if err != nil {
		return err
	}
	defer control.Close()
	anchor, err := windows.OpenProcess(windows.SYNCHRONIZE, false, opts.AnchorPID)
	if err != nil {
		return finishVirtualRedirectHelper(control, false, fmt.Errorf("open virtual redirect session anchor: %w", err))
	}
	defer windows.CloseHandle(anchor)
	if status, err := windows.WaitForSingleObject(anchor, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		if err == nil {
			err = fmt.Errorf("virtual redirect session ended before setup completed")
		}
		return finishVirtualRedirectHelper(control, false, err)
	}

	redirect, err := openWinDivertRedirect(opts.Rules)
	if err != nil {
		return finishVirtualRedirectHelper(control, false, err)
	}
	if err := control.Send(virtualRedirectHelperResult{Ready: true}); err != nil {
		redirect.Close()
		redirect.Release()
		return err
	}
	runErr := redirect.Run(anchor)
	return finishVirtualRedirectHelper(control, true, runErr)
}

func finishVirtualRedirectHelper(control *virtualRedirectControlClient, ready bool, actionErr error) error {
	result := virtualRedirectHelperResult{Ready: ready, Stopped: true}
	if actionErr != nil {
		result.Error = actionErr.Error()
	}
	sendErr := control.Send(result)
	return errors.Join(actionErr, sendErr)
}

func encodeVirtualRedirectRules(rules []virtualRedirectRule) (string, error) {
	rules, err := normalizeVirtualRedirectRules(rules)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeVirtualRedirectRules(value string) ([]virtualRedirectRule, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("decode virtual redirect rules: %w", err)
	}
	var rules []virtualRedirectRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse virtual redirect rules: %w", err)
	}
	return normalizeVirtualRedirectRules(rules)
}
