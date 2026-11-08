package configupgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"lsyltunnel/src/internal/atomicfile"
	appversion "lsyltunnel/src/internal/version"
)

type File struct {
	Path string
	Data []byte
}

type backup struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

// CheckInstalledVersion uses the installed binary's file version, not config_version:
// the latter was already used by 2.x and cannot prove eligibility for migration.
func CheckInstalledVersion(fileVersion string) error {
	parts := strings.Split(strings.TrimSpace(fileVersion), ".")
	if len(parts) != 4 {
		return fmt.Errorf("installed executable version is unavailable; automatic migration requires an installed 3.x client/server")
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return fmt.Errorf("invalid installed executable version %q", fileVersion)
		}
	}
	if parts[0] != "3" {
		return fmt.Errorf("automatic config migration only supports installed 3.x, found %s", fileVersion)
	}
	installed := strings.Join(parts[:3], ".")
	newer, err := appversion.Greater(installed, appversion.AppVersion)
	if err != nil || newer {
		return fmt.Errorf("installed version %s is newer than this installer %s", fileVersion, appversion.AppVersion)
	}
	return nil
}

// Apply keeps a transaction backup until the installer reports success. Rollback
// also handles a failed write in the middle of a multi-profile migration.
func Apply(files []File, backupFile string) error {
	if len(files) == 0 {
		return nil
	}
	backups := make([]backup, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil {
			return fmt.Errorf("backup %s: %w", file.Path, err)
		}
		backups = append(backups, backup{Path: file.Path, Data: data})
	}
	encoded, err := json.Marshal(backups)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(backupFile, encoded, 0o600); err != nil {
		return fmt.Errorf("write config migration backup: %w", err)
	}
	for _, file := range files {
		if err := atomicfile.WriteFile(file.Path, file.Data, 0o644); err != nil {
			if rollbackErr := Rollback(backupFile); rollbackErr != nil {
				return fmt.Errorf("migrate %s: %w; rollback failed: %v", file.Path, err, rollbackErr)
			}
			return fmt.Errorf("migrate %s: %w", file.Path, err)
		}
	}
	return nil
}

func Rollback(backupFile string) error {
	data, err := os.ReadFile(backupFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var backups []backup
	if err := json.Unmarshal(data, &backups); err != nil {
		return fmt.Errorf("read config migration backup: %w", err)
	}
	for _, file := range backups {
		if !filepath.IsAbs(file.Path) {
			return fmt.Errorf("config migration backup contains a relative path")
		}
		if err := atomicfile.WriteFile(file.Path, file.Data, 0o644); err != nil {
			return fmt.Errorf("restore %s: %w", file.Path, err)
		}
	}
	return nil
}
