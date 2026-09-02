package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"lsyltunnel/src/client/mobileprofile"
)

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

func exportMobileProfile(confFile, certFile, target string, force bool) error {
	_, err := mobileprofile.Export(confFile, certFile, target, force)
	return err
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
