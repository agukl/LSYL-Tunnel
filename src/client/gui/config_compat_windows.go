//go:build windows

package gui

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"lsyltunnel/src/client/tunnel"
)

type clientConfigCompatOptions struct {
	Check      bool
	ConfigPath string
	ResultFile string
}

func parseClientConfigCompatArgs(args []string) (clientConfigCompatOptions, bool, error) {
	hasCompatCheck := false
	for _, arg := range args {
		if arg == "-config-compat-check" || strings.HasPrefix(arg, "-config-compat-check=") {
			hasCompatCheck = true
			break
		}
	}
	if !hasCompatCheck {
		return clientConfigCompatOptions{}, false, nil
	}

	opts := clientConfigCompatOptions{}
	fs := flag.NewFlagSet("lsyl-tunnel-client-gui-config-compat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.Check, "config-compat-check", false, "check client config compatibility")
	fs.StringVar(&opts.ConfigPath, "config", "", "installed client config")
	fs.StringVar(&opts.ResultFile, "result-file", "", "write compatibility error to file")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return opts, true, fmt.Errorf("missing -config")
	}
	return opts, true, nil
}

func runClientConfigCompatCheck(opts clientConfigCompatOptions) error {
	return tunnel.CheckConfigUpgradeCompatible(opts.ConfigPath)
}

func writeClientConfigCompatResult(path string, err error) {
	if strings.TrimSpace(path) == "" || err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(err.Error()), 0o644)
}

func IsNonInteractiveCommand(args []string) bool {
	return IsQuitCommand(args) || isClientConfigCompatCommand(args)
}

func isClientConfigCompatCommand(args []string) bool {
	for _, arg := range args {
		if arg == "-config-compat-check" || strings.HasPrefix(arg, "-config-compat-check=") {
			return true
		}
	}
	return false
}
