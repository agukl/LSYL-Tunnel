package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"lsyltunnel/src/client/mobileprofile"
	"lsyltunnel/src/client/tunnel"
)

const (
	defaultInstallDirName = `LSYL Tunnel Client`
	profileEnvName        = "LSYL_TUNNEL_PROFILE_ROOT"
	installEnvName        = "LSYL_TUNNEL_CLIENT_HOME"
)

type options struct {
	installDir  string
	profilesDir string
}

type profileInfo struct {
	Name     string
	Path     string
	ConfFile string
	CertFile string
	Valid    bool
	Active   bool
}

type pathInfo struct {
	Path      string
	Exists    bool
	Kind      string
	Effective string
}

type switchChange struct {
	Path      string
	Backup    string
	OldTarget string
	Created   bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("lsyl-tunnel-profile", flag.ContinueOnError)
	fs.SetOutput(out)
	installDir := fs.String("install", "", "LSYL Tunnel Client install directory")
	profilesDir := fs.String("profiles", "", "profile root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := resolveOptions(*installDir, *profilesDir)
	if err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return runInteractive(out, opts)
	}

	switch remaining[0] {
	case "current":
		return commandCurrent(out, opts)
	case "list":
		return commandList(out, opts)
	case "show":
		return commandShow(out, opts, remaining[1:])
	case "import":
		return commandImport(out, opts, remaining[1:])
	case "import-current":
		return commandImportCurrent(out, opts, remaining[1:])
	case "export-mobile":
		return commandExportMobile(out, opts, remaining[1:])
	case "use":
		return commandUse(out, opts, remaining[1:])
	case "delete":
		return commandDelete(out, opts, remaining[1:])
	case "help", "-h", "--help", "/?":
		printUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q", remaining[0])
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "LSYL Tunnel profile manager")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] current")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] list")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] show NAME")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] import NAME -conf client.yaml -cert server.crt [-force]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] import-current NAME [-force]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] export-mobile -out mobile.lsylprofile [-profile NAME] [-force]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] use NAME")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] delete NAME -yes")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Defaults:")
	fmt.Fprintln(out, "  -install  uses LSYL_TUNNEL_CLIENT_HOME, a detected client install beside this tool, or Program Files.")
	fmt.Fprintln(out, "  -profiles uses LSYL_TUNNEL_PROFILE_ROOT or ProgramData\\LSYL Tunnel Profiles.")
}

func runInteractive(out io.Writer, opts options) error {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "LSYL Tunnel Profile Tool")
		fmt.Fprintf(out, "Install:  %s\n", opts.installDir)
		fmt.Fprintf(out, "Profiles: %s\n", opts.profilesDir)
		if name, ok := activeProfileName(opts); ok {
			fmt.Fprintf(out, "Active:   %s\n", name)
		} else {
			fmt.Fprintln(out, "Active:   <direct install files>")
		}
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "1. View current config and certificate")
		fmt.Fprintln(out, "2. List profiles")
		fmt.Fprintln(out, "3. Show profile")
		fmt.Fprintln(out, "4. Import current client files as profile")
		fmt.Fprintln(out, "5. Import profile from files")
		fmt.Fprintln(out, "6. Switch active profile")
		fmt.Fprintln(out, "7. Delete profile")
		fmt.Fprintln(out, "8. Export mobile profile")
		fmt.Fprintln(out, "9. Help")
		fmt.Fprintln(out, "q. Quit")

		choice, err := promptLine(reader, out, "Select")
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1":
			printInteractiveError(out, commandCurrent(out, opts))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "2":
			printInteractiveError(out, commandList(out, opts))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "3":
			name, err := promptRequired(reader, out, "Profile name")
			if err != nil {
				return err
			}
			printInteractiveError(out, commandShow(out, opts, []string{name}))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "4":
			name, err := promptRequired(reader, out, "New profile name")
			if err != nil {
				return err
			}
			args := []string{name}
			if _, err := getProfile(opts, name); err == nil {
				yes, err := confirm(reader, out, "Profile already exists. Overwrite")
				if err != nil {
					return err
				}
				if yes {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandImportCurrent(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "5":
			name, err := promptRequired(reader, out, "New profile name")
			if err != nil {
				return err
			}
			conf, err := promptRequired(reader, out, "client.yaml path")
			if err != nil {
				return err
			}
			cert, err := promptRequired(reader, out, "server.crt path")
			if err != nil {
				return err
			}
			args := []string{name, "-conf", conf, "-cert", cert}
			if _, err := getProfile(opts, name); err == nil {
				yes, err := confirm(reader, out, "Profile already exists. Overwrite")
				if err != nil {
					return err
				}
				if yes {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandImport(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "6":
			name, err := promptRequired(reader, out, "Profile name")
			if err != nil {
				return err
			}
			printInteractiveError(out, commandUse(out, opts, []string{name}))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "7":
			name, err := promptRequired(reader, out, "Profile name")
			if err != nil {
				return err
			}
			yes, err := confirm(reader, out, "Delete this profile")
			if err != nil {
				return err
			}
			if !yes {
				fmt.Fprintln(out, "Delete cancelled.")
				if err := waitForEnter(reader, out); err != nil {
					return err
				}
				continue
			}
			args := []string{name, "-yes"}
			if p, err := getProfile(opts, name); err == nil && p.Active {
				force, err := confirm(reader, out, "Profile is active. Force delete")
				if err != nil {
					return err
				}
				if force {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandDelete(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "8":
			target, err := promptRequired(reader, out, "Output .lsylprofile path")
			if err != nil {
				return err
			}
			args := []string{"-out", target}
			if fileExists(target) {
				yes, err := confirm(reader, out, "Output already exists. Overwrite")
				if err != nil {
					return err
				}
				if yes {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandExportMobile(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "9", "h", "help", "?":
			printUsage(out)
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "q", "quit", "exit":
			fmt.Fprintln(out, "Bye.")
			return nil
		default:
			fmt.Fprintln(out, "Unknown selection.")
		}
	}
}

func promptLine(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprintf(out, "%s> ", label)
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && strings.TrimSpace(line) != "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptRequired(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	for {
		value, err := promptLine(reader, out, label)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(out, "Value is required.")
	}
}

func confirm(reader *bufio.Reader, out io.Writer, label string) (bool, error) {
	for {
		value, err := promptLine(reader, out, label+" [y/N]")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "n", "no":
			return false, nil
		case "y", "yes":
			return true, nil
		default:
			fmt.Fprintln(out, "Please answer y or n.")
		}
	}
}

func waitForEnter(reader *bufio.Reader, out io.Writer) error {
	fmt.Fprint(out, "\nPress Enter to continue...")
	_, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	fmt.Fprintln(out, "")
	return nil
}

func printInteractiveError(out io.Writer, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(out, "\nERROR: %v\n", err)
	if strings.Contains(strings.ToLower(err.Error()), "access is denied") {
		fmt.Fprintln(out, "Tip: switch operations under Program Files usually need an Administrator CMD.")
	}
}

func resolveOptions(installDir, profilesDir string) (options, error) {
	if strings.TrimSpace(installDir) == "" {
		installDir = defaultInstallDir()
	}
	if strings.TrimSpace(profilesDir) == "" {
		profilesDir = defaultProfilesDir()
	}
	installAbs, err := filepath.Abs(installDir)
	if err != nil {
		return options{}, err
	}
	profilesAbs, err := filepath.Abs(profilesDir)
	if err != nil {
		return options{}, err
	}
	return options{installDir: filepath.Clean(installAbs), profilesDir: filepath.Clean(profilesAbs)}, nil
}

func defaultInstallDir() string {
	if v := strings.TrimSpace(os.Getenv(installEnvName)); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if strings.EqualFold(filepath.Base(dir), "bin") {
			candidate := filepath.Dir(dir)
			if strings.EqualFold(filepath.Base(candidate), defaultInstallDirName) || looksLikeClientInstall(candidate) {
				return candidate
			}
		}
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		return filepath.Join(pf, defaultInstallDirName)
	}
	return filepath.Join(`C:\Program Files`, defaultInstallDirName)
}

func looksLikeClientInstall(path string) bool {
	return fileExists(filepath.Join(path, "conf", "client.yaml")) && fileExists(filepath.Join(path, "cert", "server.crt"))
}

func defaultProfilesDir() string {
	if v := strings.TrimSpace(os.Getenv(profileEnvName)); v != "" {
		return v
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "LSYL Tunnel Profiles")
	}
	return filepath.Join(defaultInstallDir(), "profiles")
}

func commandCurrent(out io.Writer, opts options) error {
	fmt.Fprintf(out, "Install:  %s\n", opts.installDir)
	fmt.Fprintf(out, "Profiles: %s\n", opts.profilesDir)
	fmt.Fprintln(out, "")
	confPath := filepath.Join(opts.installDir, "conf")
	certPath := filepath.Join(opts.installDir, "cert")
	conf := inspectPath(confPath)
	cert := inspectPath(certPath)
	fmt.Fprintf(out, "Active conf dir: %s\n", describePath(conf))
	fmt.Fprintf(out, "Active cert dir: %s\n", describePath(cert))
	if name, ok := activeProfileName(opts); ok {
		fmt.Fprintf(out, "Active profile: %s\n", name)
	} else {
		fmt.Fprintln(out, "Active profile: <direct install files>")
	}
	fmt.Fprintln(out, "")
	printConfigSummary(out, filepath.Join(conf.Effective, "client.yaml"))
	fmt.Fprintln(out, "")
	printCertSummary(out, filepath.Join(cert.Effective, "server.crt"))
	return nil
}

func commandList(out io.Writer, opts options) error {
	profiles, err := listProfiles(opts)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Fprintf(out, "No profiles found under %s\n", opts.profilesDir)
		return nil
	}
	for _, p := range profiles {
		mark := " "
		if p.Active {
			mark = "*"
		}
		state := "ok"
		if !p.Valid {
			state = "incomplete"
		}
		fmt.Fprintf(out, "%s %-24s %s\n", mark, p.Name, state)
	}
	return nil
}

func commandShow(out io.Writer, opts options, args []string) error {
	if len(args) != 1 {
		return errors.New("show requires NAME")
	}
	p, err := getProfile(opts, args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Profile: %s\n", p.Name)
	fmt.Fprintf(out, "Path:    %s\n", p.Path)
	fmt.Fprintf(out, "Active:  %v\n", p.Active)
	fmt.Fprintln(out, "")
	printConfigSummary(out, p.ConfFile)
	fmt.Fprintln(out, "")
	printCertSummary(out, p.CertFile)
	return nil
}

func commandImport(out io.Writer, opts options, args []string) error {
	name, conf, cert, force, err := parseImportArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(conf) == "" || strings.TrimSpace(cert) == "" {
		return errors.New("import requires -conf and -cert")
	}
	if err := importProfile(opts, name, conf, cert, force); err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported profile %s\n", name)
	return nil
}

func commandImportCurrent(out io.Writer, opts options, args []string) error {
	name, force, err := parseNameForceArgs("import-current", args)
	if err != nil {
		return err
	}
	conf := filepath.Join(inspectPath(filepath.Join(opts.installDir, "conf")).Effective, "client.yaml")
	cert := filepath.Join(inspectPath(filepath.Join(opts.installDir, "cert")).Effective, "server.crt")
	if err := importProfile(opts, name, conf, cert, force); err != nil {
		return err
	}
	fmt.Fprintf(out, "Imported current install files as profile %s\n", name)
	return nil
}

func commandExportMobile(out io.Writer, opts options, args []string) error {
	profileName, target, force, err := parseExportMobileArgs(args)
	if err != nil {
		return err
	}
	conf, cert := currentClientFiles(opts)
	if profileName != "" {
		p, err := getProfile(opts, profileName)
		if err != nil {
			return err
		}
		if !p.Valid {
			return fmt.Errorf("profile %s is incomplete; expected conf/client.yaml and cert/server.crt", p.Name)
		}
		conf = p.ConfFile
		cert = p.CertFile
	}
	if err := exportMobileProfile(conf, cert, target, force); err != nil {
		return err
	}
	fmt.Fprintf(out, "Exported mobile profile: %s\n", target)
	return nil
}

func commandUse(out io.Writer, opts options, args []string) error {
	if len(args) != 1 {
		return errors.New("use requires NAME")
	}
	p, err := getProfile(opts, args[0])
	if err != nil {
		return err
	}
	if !p.Valid {
		return fmt.Errorf("profile %s is incomplete; expected conf/client.yaml and cert/server.crt", p.Name)
	}
	if err := useProfile(opts, p.Name); err != nil {
		return err
	}
	fmt.Fprintf(out, "Active profile switched to %s\n", p.Name)
	fmt.Fprintln(out, "Restart LSYL Tunnel Client for the new config and certificate to take effect.")
	return nil
}

func commandDelete(out io.Writer, opts options, args []string) error {
	name, yes, force, err := parseDeleteArgs(args)
	if err != nil {
		return err
	}
	if !yes {
		return errors.New("delete requires -yes")
	}
	p, err := getProfile(opts, name)
	if err != nil {
		return err
	}
	if p.Active && !force {
		return fmt.Errorf("profile %s is active; switch away before deleting or pass -force", name)
	}
	if err := removeProfile(opts, name); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted profile %s\n", name)
	return nil
}

func parseImportArgs(args []string) (name, conf, cert string, force bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-force", "--force":
			force = true
		case "-conf", "--conf":
			i++
			if i >= len(args) {
				return "", "", "", false, errors.New("missing value after -conf")
			}
			conf = args[i]
		case "-cert", "--cert":
			i++
			if i >= len(args) {
				return "", "", "", false, errors.New("missing value after -cert")
			}
			cert = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", "", false, fmt.Errorf("unknown import option %s", arg)
			}
			if name != "" {
				return "", "", "", false, errors.New("import accepts exactly one NAME")
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", "", false, errors.New("import requires NAME")
	}
	return name, conf, cert, force, nil
}

func parseExportMobileArgs(args []string) (profileName, target string, force bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-force", "--force":
			force = true
		case "-profile", "--profile":
			i++
			if i >= len(args) {
				return "", "", false, errors.New("missing value after -profile")
			}
			profileName = strings.TrimSpace(args[i])
		case "-out", "--out":
			i++
			if i >= len(args) {
				return "", "", false, errors.New("missing value after -out")
			}
			target = strings.TrimSpace(args[i])
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, fmt.Errorf("unknown export-mobile option %s", arg)
			}
			return "", "", false, fmt.Errorf("unexpected export-mobile argument %q", arg)
		}
	}
	if target == "" {
		return "", "", false, errors.New("export-mobile requires -out")
	}
	if profileName == "" {
		profileName = ""
	}
	return profileName, target, force, nil
}

func parseNameForceArgs(command string, args []string) (name string, force bool, err error) {
	for _, arg := range args {
		switch arg {
		case "-force", "--force":
			force = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, fmt.Errorf("unknown %s option %s", command, arg)
			}
			if name != "" {
				return "", false, fmt.Errorf("%s accepts exactly one NAME", command)
			}
			name = arg
		}
	}
	if name == "" {
		return "", false, fmt.Errorf("%s requires NAME", command)
	}
	return name, force, nil
}

func parseDeleteArgs(args []string) (name string, yes, force bool, err error) {
	for _, arg := range args {
		switch arg {
		case "-yes", "--yes", "/yes":
			yes = true
		case "-force", "--force":
			force = true
		default:
			if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "/") {
				return "", false, false, fmt.Errorf("unknown delete option %s", arg)
			}
			if name != "" {
				return "", false, false, errors.New("delete accepts exactly one NAME")
			}
			name = arg
		}
	}
	if name == "" {
		return "", false, false, errors.New("delete requires NAME")
	}
	return name, yes, force, nil
}

func importProfile(opts options, name, confSource, certSource string, force bool) error {
	profileDir, err := profilePath(opts, name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(profileDir); err == nil {
		if !force {
			return fmt.Errorf("profile %s already exists; pass -force to overwrite", name)
		}
		if err := ensureChild(opts.profilesDir, profileDir); err != nil {
			return err
		}
		if err := os.RemoveAll(profileDir); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := requireFile(confSource); err != nil {
		return fmt.Errorf("invalid conf source: %w", err)
	}
	if err := requireFile(certSource); err != nil {
		return fmt.Errorf("invalid cert source: %w", err)
	}
	confDir := filepath.Join(profileDir, "conf")
	certDir := filepath.Join(profileDir, "cert")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(confSource, filepath.Join(confDir, "client.yaml")); err != nil {
		return err
	}
	if err := copyFile(certSource, filepath.Join(certDir, "server.crt")); err != nil {
		return err
	}
	return nil
}

func currentClientFiles(opts options) (conf, cert string) {
	conf = filepath.Join(inspectPath(filepath.Join(opts.installDir, "conf")).Effective, "client.yaml")
	cert = filepath.Join(inspectPath(filepath.Join(opts.installDir, "cert")).Effective, "server.crt")
	return conf, cert
}

type mobileProfileFile struct {
	Version         int                    `json:"version"`
	ProfileID       string                 `json:"profile_id,omitempty"`
	ServerAddr      string                 `json:"server_addr"`
	Username        string                 `json:"username"`
	ClientID        string                 `json:"client_id,omitempty"`
	SavedCredential any                    `json:"saved_credential"`
	TLS             mobileTLSConfig        `json:"tls"`
	Connection      mobileConnectionConfig `json:"connection"`
	Forwards        []mobileForwardConfig  `json:"forwards"`
}

type mobileTLSConfig struct {
	CACertFile         string `json:"ca_cert_file"`
	ServerName         string `json:"server_name"`
	MinVersion         string `json:"min_version"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}

type mobileConnectionConfig struct {
	DialTimeoutSec int `json:"dial_timeout_sec"`
}

type mobileForwardConfig struct {
	Name         string `json:"name"`
	Direction    string `json:"direction"`
	ListenAddr   string `json:"listen_addr"`
	ServerTarget string `json:"server_target"`
}

func exportMobileProfile(confFile, certFile, target string, force bool) error {
	_, err := mobileprofile.Export(confFile, certFile, target, force)
	return err
}

func readClientConfigFile(path string) (tunnel.Config, error) {
	var cfg tunnel.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read client config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse client config: %w", err)
	}
	tunnel.ApplyDefaults(&cfg)
	return cfg, nil
}

func mobileProfileFromConfig(cfg tunnel.Config) (mobileProfileFile, error) {
	tunnel.ApplyDefaults(&cfg)
	if _, _, err := splitMobileHostPort(cfg.ServerAddr); err != nil {
		return mobileProfileFile{}, fmt.Errorf("server_addr is invalid: %w", err)
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return mobileProfileFile{}, errors.New("username is required")
	}
	if strings.TrimSpace(cfg.SavedCredential.Ciphertext) == "" {
		return mobileProfileFile{}, errors.New("saved_credential is required; connect successfully in the client once before exporting")
	}
	if strings.TrimSpace(cfg.SavedCredential.Type) != "server_sealed" {
		return mobileProfileFile{}, errors.New("saved_credential.type must be server_sealed")
	}
	if strings.TrimSpace(cfg.SavedCredential.KeyID) == "" {
		return mobileProfileFile{}, errors.New("saved_credential.key_id is required")
	}
	if strings.TrimSpace(cfg.SavedCredential.ExpiresAt) == "" {
		return mobileProfileFile{}, errors.New("saved_credential.expires_at is required")
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(cfg.SavedCredential.ExpiresAt))
	if err != nil {
		return mobileProfileFile{}, fmt.Errorf("saved_credential.expires_at must be RFC3339: %w", err)
	}
	if !expiresAt.After(time.Now()) {
		return mobileProfileFile{}, errors.New("saved_credential has expired; reconnect in the client before exporting")
	}
	if cfg.TLS.InsecureSkipVerify {
		return mobileProfileFile{}, errors.New("mobile profile cannot use tls.insecure_skip_verify")
	}
	if !mobileTLS13(cfg.TLS.MinVersion) {
		return mobileProfileFile{}, errors.New("mobile profile requires tls.min_version 1.3")
	}
	forwards := make([]mobileForwardConfig, 0, len(cfg.Forwards))
	seenNames := map[string]bool{}
	seenListens := map[string]bool{}
	for _, fwd := range cfg.Forwards {
		mobileFwd, err := mobileForwardFromConfig(fwd)
		if err != nil {
			return mobileProfileFile{}, err
		}
		if seenNames[mobileFwd.Name] {
			return mobileProfileFile{}, fmt.Errorf("duplicate forward name: %s", mobileFwd.Name)
		}
		if seenListens[mobileFwd.ListenAddr] {
			return mobileProfileFile{}, fmt.Errorf("duplicate mobile listen address: %s", mobileFwd.ListenAddr)
		}
		seenNames[mobileFwd.Name] = true
		seenListens[mobileFwd.ListenAddr] = true
		forwards = append(forwards, mobileFwd)
	}
	if len(forwards) == 0 {
		return mobileProfileFile{}, errors.New("at least one client_to_server forward is required")
	}
	timeout := cfg.Connection.DialTimeoutSec
	if timeout <= 0 {
		timeout = 5
	}
	return mobileProfileFile{
		Version:         1,
		ProfileID:       mobileProfileID(cfg),
		ServerAddr:      strings.TrimSpace(cfg.ServerAddr),
		Username:        strings.TrimSpace(cfg.Username),
		ClientID:        strings.TrimSpace(cfg.ClientID),
		SavedCredential: cfg.SavedCredential,
		TLS: mobileTLSConfig{
			CACertFile:         "server.crt",
			ServerName:         strings.TrimSpace(cfg.TLS.ServerName),
			MinVersion:         "1.3",
			InsecureSkipVerify: false,
		},
		Connection: mobileConnectionConfig{DialTimeoutSec: timeout},
		Forwards:   forwards,
	}, nil
}

func mobileForwardFromConfig(fwd tunnel.ForwardConfig) (mobileForwardConfig, error) {
	direction := strings.TrimSpace(fwd.Direction)
	if direction == "" {
		direction = tunnel.DirectionClientToServer
	}
	if direction != tunnel.DirectionClientToServer {
		return mobileForwardConfig{}, fmt.Errorf("forward %q is %s; mobile export only supports client_to_server", forwardNameForError(fwd), direction)
	}
	host, port, err := splitMobileHostPort(fwd.ListenAddr)
	if err != nil {
		return mobileForwardConfig{}, fmt.Errorf("forward %q listen_addr is invalid: %w", forwardNameForError(fwd), err)
	}
	if !isMobileLoopback(host) {
		return mobileForwardConfig{}, fmt.Errorf("forward %q listen_addr must use 127.0.0.1 for mobile", forwardNameForError(fwd))
	}
	if port < 1024 {
		return mobileForwardConfig{}, fmt.Errorf("forward %q listen port %d is below 1024 and cannot be used by Android", forwardNameForError(fwd), port)
	}
	if _, _, err := splitMobileHostPort(fwd.ServerTarget); err != nil {
		return mobileForwardConfig{}, fmt.Errorf("forward %q server_target is invalid: %w", forwardNameForError(fwd), err)
	}
	name := strings.TrimSpace(fwd.Name)
	listenAddr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port))
	if name == "" {
		name = listenAddr
	}
	return mobileForwardConfig{
		Name:         name,
		Direction:    tunnel.DirectionClientToServer,
		ListenAddr:   listenAddr,
		ServerTarget: strings.TrimSpace(fwd.ServerTarget),
	}, nil
}

func writeMobileProfileZip(target string, profileJSON, certPEM []byte, force bool) error {
	if _, err := os.Stat(target); err == nil && !force {
		return fmt.Errorf("output already exists: %s; pass -force to overwrite", target)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := writeMobileProfileZipTo(tmp, profileJSON, certPEM); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if force {
		_ = os.Remove(target)
	}
	return os.Rename(tmpName, target)
}

func writeMobileProfileZipTo(w io.Writer, profileJSON, certPEM []byte) error {
	zw := zip.NewWriter(w)
	if err := writeZipFile(zw, "profile.json", profileJSON); err != nil {
		_ = zw.Close()
		return err
	}
	if err := writeZipFile(zw, "server.crt", certPEM); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(time.Now())
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func splitMobileHostPort(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port")
	}
	return strings.Trim(host, "[]"), port, nil
}

func isMobileLoopback(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func mobileTLS13(version string) bool {
	switch strings.TrimSpace(strings.ToLower(version)) {
	case "", "1.3", "tls1.3":
		return true
	default:
		return false
	}
}

func mobileProfileID(cfg tunnel.Config) string {
	id := strings.TrimSpace(cfg.ClientID)
	if id == "" {
		id = strings.TrimSpace(cfg.Username)
	}
	id = strings.ToLower(id)
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	id = strings.Trim(b.String(), "-_")
	if id == "" {
		id = "client"
	}
	return "mobile-" + id
}

func forwardNameForError(fwd tunnel.ForwardConfig) string {
	if name := strings.TrimSpace(fwd.Name); name != "" {
		return name
	}
	if addr := strings.TrimSpace(fwd.ListenAddr); addr != "" {
		return addr
	}
	return strings.TrimSpace(fwd.ServerTarget)
}

func useProfile(opts options, name string) error {
	p, err := getProfile(opts, name)
	if err != nil {
		return err
	}
	changes := make([]switchChange, 0, 2)
	change, err := switchDirectoryToLink(filepath.Join(opts.installDir, "conf"), filepath.Join(p.Path, "conf"))
	if err != nil {
		rollbackSwitch(changes)
		return err
	}
	changes = append(changes, change)
	change, err = switchDirectoryToLink(filepath.Join(opts.installDir, "cert"), filepath.Join(p.Path, "cert"))
	if err != nil {
		rollbackSwitch(changes)
		return err
	}
	return nil
}

func switchDirectoryToLink(path, target string) (switchChange, error) {
	var change switchChange
	path = filepath.Clean(path)
	target = filepath.Clean(target)
	if st, err := os.Stat(target); err != nil {
		return change, fmt.Errorf("link target not accessible %s: %w", target, err)
	} else if !st.IsDir() {
		return change, fmt.Errorf("link target is not a directory: %s", target)
	}
	change.Path = path
	info := inspectPath(path)
	if info.Exists {
		if info.Kind == "link" {
			change.OldTarget = info.Effective
			if err := os.Remove(path); err != nil {
				return change, fmt.Errorf("remove existing link %s: %w", path, err)
			}
		} else {
			backup, err := nextBackupPath(path)
			if err != nil {
				return change, err
			}
			if err := os.Rename(path, backup); err != nil {
				return change, fmt.Errorf("backup existing %s to %s: %w", path, backup, err)
			}
			change.Backup = backup
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		restoreChange(change)
		return change, err
	}
	if err := createDirectoryLink(path, target); err != nil {
		restoreChange(change)
		return change, err
	}
	change.Created = true
	return change, nil
}

func rollbackSwitch(changes []switchChange) {
	for i := len(changes) - 1; i >= 0; i-- {
		restoreChange(changes[i])
	}
}

func restoreChange(change switchChange) {
	if change.Created {
		_ = os.Remove(change.Path)
	}
	if change.Backup != "" {
		if _, err := os.Stat(change.Path); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(change.Backup, change.Path)
		}
		return
	}
	if change.OldTarget != "" {
		if _, err := os.Stat(change.Path); errors.Is(err, os.ErrNotExist) {
			_ = createDirectoryLink(change.Path, change.OldTarget)
		}
	}
}

func createDirectoryLink(linkPath, targetPath string) error {
	if runtime.GOOS != "windows" {
		return os.Symlink(targetPath, linkPath)
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", linkPath, targetPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeProfile(opts options, name string) error {
	profileDir, err := profilePath(opts, name)
	if err != nil {
		return err
	}
	if err := ensureChild(opts.profilesDir, profileDir); err != nil {
		return err
	}
	return os.RemoveAll(profileDir)
}

func listProfiles(opts options) ([]profileInfo, error) {
	entries, err := os.ReadDir(opts.profilesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]profileInfo, 0, len(entries))
	active, activeOK := activeProfileName(opts)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		p, err := getProfile(opts, entry.Name())
		if err != nil {
			continue
		}
		p.Active = activeOK && p.Name == active
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool { return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name) })
	return profiles, nil
}

func getProfile(opts options, name string) (profileInfo, error) {
	profileDir, err := profilePath(opts, name)
	if err != nil {
		return profileInfo{}, err
	}
	conf := filepath.Join(profileDir, "conf", "client.yaml")
	cert := filepath.Join(profileDir, "cert", "server.crt")
	p := profileInfo{
		Name:     strings.TrimSpace(name),
		Path:     profileDir,
		ConfFile: conf,
		CertFile: cert,
		Valid:    fileExists(conf) && fileExists(cert),
	}
	active, ok := activeProfileName(opts)
	p.Active = ok && active == p.Name
	if _, err := os.Stat(profileDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, fmt.Errorf("profile %s does not exist", name)
		}
		return p, err
	}
	return p, nil
}

func profilePath(opts options, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `\/:*?"<>|`) {
		return "", fmt.Errorf("invalid profile name %q", name)
	}
	root, err := filepath.Abs(opts.profilesDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, name)
	if err := ensureChild(root, path); err != nil {
		return "", err
	}
	return path, nil
}

func activeProfileName(opts options) (string, bool) {
	confTarget := inspectPath(filepath.Join(opts.installDir, "conf")).Effective
	certTarget := inspectPath(filepath.Join(opts.installDir, "cert")).Effective
	entries, err := os.ReadDir(opts.profilesDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profileDir := filepath.Join(opts.profilesDir, entry.Name())
		if samePath(confTarget, filepath.Join(profileDir, "conf")) && samePath(certTarget, filepath.Join(profileDir, "cert")) {
			return entry.Name(), true
		}
	}
	return "", false
}

func inspectPath(path string) pathInfo {
	clean := filepath.Clean(path)
	pi := pathInfo{Path: clean, Effective: clean}
	st, err := os.Lstat(clean)
	if err != nil {
		pi.Exists = false
		pi.Kind = "missing"
		return pi
	}
	pi.Exists = true
	if st.Mode()&os.ModeSymlink != 0 {
		pi.Kind = "link"
	} else if st.IsDir() {
		pi.Kind = "dir"
	} else {
		pi.Kind = "file"
	}
	if target, err := os.Readlink(clean); err == nil && strings.TrimSpace(target) != "" {
		pi.Kind = "link"
		if filepath.IsAbs(target) {
			pi.Effective = filepath.Clean(target)
		} else {
			pi.Effective = filepath.Clean(filepath.Join(filepath.Dir(clean), target))
		}
		return pi
	}
	if effective, err := filepath.EvalSymlinks(clean); err == nil {
		pi.Effective = filepath.Clean(effective)
		if !samePath(pi.Effective, clean) {
			pi.Kind = "link"
		}
	}
	return pi
}

func describePath(info pathInfo) string {
	if !info.Exists {
		return info.Path + " (missing)"
	}
	if info.Kind == "link" {
		return fmt.Sprintf("%s -> %s", info.Path, info.Effective)
	}
	return fmt.Sprintf("%s (%s)", info.Path, info.Kind)
}

func printConfigSummary(out io.Writer, configFile string) {
	fmt.Fprintf(out, "Config file: %s\n", configFile)
	data, err := os.ReadFile(configFile)
	if err != nil {
		fmt.Fprintf(out, "Config:      not readable: %v\n", err)
		return
	}
	var cfg tunnel.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(out, "Config:      invalid yaml: %v\n", err)
		return
	}
	fmt.Fprintf(out, "Server:      %s\n", cfg.ServerAddr)
	fmt.Fprintf(out, "Username:    %s\n", cfg.Username)
	fmt.Fprintf(out, "TLS name:    %s\n", cfg.TLS.ServerName)
	fmt.Fprintf(out, "Credential:  %s\n", credentialState(cfg))
	fmt.Fprintf(out, "Forwards:    %d\n", len(cfg.Forwards))
	for _, fwd := range cfg.Forwards {
		name := strings.TrimSpace(fwd.Name)
		if name == "" {
			name = "forward"
		}
		fmt.Fprintf(out, "  - %s %s %s -> %s\n", name, fwd.Direction, fwd.ListenAddr, fwd.ServerTarget)
	}
}

func credentialState(cfg tunnel.Config) string {
	switch {
	case strings.TrimSpace(cfg.SavedCredential.Ciphertext) != "":
		return "saved_credential"
	case strings.TrimSpace(cfg.PasswordFile) != "":
		return "password_file"
	case strings.TrimSpace(cfg.PasswordEnv) != "":
		return "password_env"
	case cfg.Password != "":
		return "inline_password"
	default:
		return "none"
	}
}

func printCertSummary(out io.Writer, certFile string) {
	fmt.Fprintf(out, "Cert file:   %s\n", certFile)
	cert, raw, err := readFirstCert(certFile)
	if err != nil {
		fmt.Fprintf(out, "Cert:        not readable: %v\n", err)
		return
	}
	fp := sha256.Sum256(raw)
	fmt.Fprintf(out, "Subject:     %s\n", cert.Subject.String())
	fmt.Fprintf(out, "Not before:  %s\n", cert.NotBefore.Format(time.RFC3339))
	fmt.Fprintf(out, "Not after:   %s\n", cert.NotAfter.Format(time.RFC3339))
	fmt.Fprintf(out, "DNS names:   %s\n", strings.Join(cert.DNSNames, ","))
	ips := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}
	fmt.Fprintf(out, "IP addrs:    %s\n", strings.Join(ips, ","))
	fmt.Fprintf(out, "SHA256:      %s\n", strings.ToUpper(hex.EncodeToString(fp[:])))
}

func readFirstCert(path string) (*x509.Certificate, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, errors.New("no PEM certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, block.Bytes, nil
}

func requireFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return errors.New("is a directory")
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err == nil {
		a = aa
	}
	bb, err := filepath.Abs(b)
	if err == nil {
		b = bb
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func ensureChild(root, child string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	childAbs, err := filepath.Abs(child)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, childAbs)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes root: %s", child)
	}
	return nil
}

func nextBackupPath(path string) (string, error) {
	base := filepath.Clean(path) + ".profile-backup-" + time.Now().Format("20060102-150405")
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%02d", base, i)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("cannot allocate backup path for %s", path)
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

var _ = isLoopbackAddr
