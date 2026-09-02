package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

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
