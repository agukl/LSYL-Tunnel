package configupgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckInstalledVersion(t *testing.T) {
	for _, version := range []string{"3.0.0.0", "3.2.5.0", "3.3.0.0", "3.4.0.0"} {
		if err := CheckInstalledVersion(version); err != nil {
			t.Errorf("%s: %v", version, err)
		}
	}
	for _, version := range []string{"", "2.9.0.0", "3.5.0.0", "3.3", "bad"} {
		if err := CheckInstalledVersion(version); err == nil {
			t.Errorf("%q should be rejected", version)
		}
	}
}

func TestApplyAndRollbackProfiles(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "migration.json")
	files := make([]File, 0, 2)
	for _, name := range []string{"client.yaml", "client.archive.yaml"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("original:"+name), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, File{Path: path, Data: []byte("updated:" + name)})
	}
	if err := Apply(files, backup); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil || string(data) != string(file.Data) {
			t.Fatalf("updated %s = %q, %v", file.Path, data, err)
		}
	}
	if err := Rollback(backup); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file.Path)
		if err != nil || !strings.HasPrefix(string(data), "original:") {
			t.Fatalf("restored %s = %q, %v", file.Path, data, err)
		}
	}
}
