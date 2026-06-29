//go:build windows

package gui

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"lsyltunnel/src/server/tunnel"
)

type configCompatOptions struct {
	Check      bool
	ConfigPath string
	ResultFile string
}

func parseConfigCompatArgs(args []string) (configCompatOptions, bool, error) {
	hasCompatCheck := false
	for _, arg := range args {
		if arg == "-config-compat-check" || strings.HasPrefix(arg, "-config-compat-check=") {
			hasCompatCheck = true
			break
		}
	}
	if !hasCompatCheck {
		return configCompatOptions{}, false, nil
	}

	opts := configCompatOptions{}
	fs := flag.NewFlagSet("lsyl-tunnel-server-gui-config-compat", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.Check, "config-compat-check", false, "check server config compatibility")
	fs.StringVar(&opts.ConfigPath, "config", "", "installed server config")
	fs.StringVar(&opts.ResultFile, "result-file", "", "write compatibility error to file")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return opts, true, fmt.Errorf("missing -config")
	}
	return opts, true, nil
}

func runConfigCompatCheck(opts configCompatOptions) error {
	return tunnel.CheckConfigUpgradeCompatible(opts.ConfigPath)
}

func writeConfigCompatResult(path string, err error) {
	if strings.TrimSpace(path) == "" || err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(err.Error()), 0o644)
}
