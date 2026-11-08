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
	"lsyltunnel/src/internal/configupgrade"
)

type clientConfigCompatOptions struct {
	Check         bool
	Apply         bool
	Rollback      bool
	ConfigPath    string
	ResultFile    string
	SourceVersion string
	BackupFile    string
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
	fs.BoolVar(&opts.Apply, "apply", false, "apply config migration")
	fs.BoolVar(&opts.Rollback, "rollback", false, "restore config migration")
	fs.StringVar(&opts.ConfigPath, "config", "", "installed client config")
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

func runClientConfigCompatCheck(opts clientConfigCompatOptions) error {
	if opts.Rollback {
		return configupgrade.Rollback(opts.BackupFile)
	}
	if opts.SourceVersion == "" && opts.BackupFile == "" && !opts.Apply {
		return tunnel.CheckConfigUpgradeCompatible(opts.ConfigPath)
	}
	if err := configupgrade.CheckInstalledVersion(opts.SourceVersion); err != nil {
		return err
	}
	files, err := clientMigrationFiles(opts.ConfigPath)
	if err != nil || len(files) == 0 {
		return err
	}
	if !opts.Apply {
		return nil
	}
	return configupgrade.Apply(files, opts.BackupFile)
}

func clientMigrationFiles(activePath string) ([]configupgrade.File, error) {
	dir := filepath.Dir(activePath)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]configupgrade.File, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (name != "client.yaml" &&
			!(strings.HasPrefix(name, "client.") && strings.HasSuffix(name, ".yaml"))) {
			continue
		}
		certName := "server.crt"
		if name != "client.yaml" {
			certName = "server." + strings.TrimSuffix(strings.TrimPrefix(name, "client."), ".yaml") + ".crt"
		}
		path := filepath.Join(dir, name)
		cert := filepath.Join(filepath.Dir(dir), "cert", certName)
		file, err := tunnel.PrepareConfigMigration(path, cert)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		files = append(files, file)
	}
	return files, nil
}

func writeClientConfigCompatResult(path string, err error) {
	if strings.TrimSpace(path) == "" || err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(err.Error()), 0o644)
}

func IsNonInteractiveCommand(args []string) bool {
	return IsQuitCommand(args) || isClientConfigCompatCommand(args) || isClientInstallerCleanupCommand(args)
}

func isClientConfigCompatCommand(args []string) bool {
	for _, arg := range args {
		if arg == "-config-compat-check" || strings.HasPrefix(arg, "-config-compat-check=") {
			return true
		}
	}
	return false
}

func isClientInstallerCleanupCommand(args []string) bool {
	for _, arg := range args {
		if arg == "-"+clientInstallerCleanupFlag || strings.HasPrefix(arg, "-"+clientInstallerCleanupFlag+"=") {
			return true
		}
	}
	return false
}
