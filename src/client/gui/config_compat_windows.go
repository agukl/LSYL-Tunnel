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

	"gopkg.in/yaml.v3"
)

type clientConfigCompatOptions struct {
	Check            bool
	Apply            bool
	Rollback         bool
	PreserveAccount  bool
	ArchiveSuffix    bool
	ConfigPath       string
	SourceConfigPath string
	ResultFile       string
	SourceVersion    string
	BackupFile       string
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
	fs.BoolVar(&opts.PreserveAccount, "preserve-account", false, "keep the installed account name")
	fs.BoolVar(&opts.ArchiveSuffix, "archive-suffix", false, "derive the installed profile archive suffix")
	fs.StringVar(&opts.ConfigPath, "config", "", "installed client config")
	fs.StringVar(&opts.SourceConfigPath, "source-config", "", "previous client config backup")
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
	if opts.PreserveAccount && strings.TrimSpace(opts.SourceConfigPath) == "" {
		return opts, true, fmt.Errorf("missing -source-config")
	}
	if opts.ArchiveSuffix && strings.TrimSpace(opts.ResultFile) == "" {
		return opts, true, fmt.Errorf("missing -result-file")
	}
	return opts, true, nil
}

func runClientConfigCompatCheck(opts clientConfigCompatOptions) error {
	if opts.Rollback {
		return configupgrade.Rollback(opts.BackupFile)
	}
	if opts.ArchiveSuffix {
		return os.WriteFile(opts.ResultFile, []byte(clientArchiveSuffixFromConfig(opts.ConfigPath)), 0o644)
	}
	if opts.PreserveAccount {
		return tunnel.PreserveInstalledUsername(opts.SourceConfigPath, opts.ConfigPath)
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

func clientArchiveSuffixFromConfig(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	var profile struct {
		ServerAddr string `yaml:"server_addr"`
	}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return "unknown"
	}
	if suffix := clientProfileFileSuffix(profile.ServerAddr); suffix != "" {
		return suffix
	}
	return "unknown"
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
			// Keep incompatible old profiles unchanged; the package supplies the active profile.
			continue
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
