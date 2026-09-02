package main

import (
	"os"
	"path/filepath"
	"strings"
)

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
