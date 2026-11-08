//go:build windows

package gui

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"lsyltunnel/src/internal/configupgrade"
	"lsyltunnel/src/server/tunnel"
)

type configCompatOptions struct {
	Check         bool
	Apply         bool
	Rollback      bool
	ConfigPath    string
	ResultFile    string
	SourceVersion string
	BackupFile    string
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
	fs.BoolVar(&opts.Apply, "apply", false, "apply config migration")
	fs.BoolVar(&opts.Rollback, "rollback", false, "restore config migration")
	fs.StringVar(&opts.ConfigPath, "config", "", "installed server config")
	fs.StringVar(&opts.ResultFile, "result-file", "", "write compatibility error to file")
	fs.StringVar(&opts.SourceVersion, "source-version", "", "installed executable file version")
	fs.StringVar(&opts.BackupFile, "backup-file", "", "config migration backup")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	if !opts.Rollback && strings.TrimSpace(opts.ConfigPath) == "" {
		return opts, true, fmt.Errorf("missing -config")
	}
	if (opts.Apply || opts.Rollback) && strings.TrimSpace(opts.BackupFile) == "" {
		return opts, true, fmt.Errorf("missing -backup-file")
	}
	return opts, true, nil
}

func runConfigCompatCheck(opts configCompatOptions) error {
	if opts.Rollback {
		return configupgrade.Rollback(opts.BackupFile)
	}
	if opts.SourceVersion == "" && opts.BackupFile == "" && !opts.Apply {
		return tunnel.CheckConfigUpgradeCompatible(opts.ConfigPath)
	}
	if err := configupgrade.CheckInstalledVersion(opts.SourceVersion); err != nil {
		return err
	}
	file, err := tunnel.PrepareConfigMigration(opts.ConfigPath)
	if err != nil || !opts.Apply {
		return err
	}
	return configupgrade.Apply([]configupgrade.File{file}, opts.BackupFile)
}

func writeConfigCompatResult(path string, err error) {
	if strings.TrimSpace(path) == "" || err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(err.Error()), 0o644)
}
