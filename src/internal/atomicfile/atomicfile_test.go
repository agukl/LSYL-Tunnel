package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileReplacesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	for _, value := range []string{"old", "new"} {
		if err := WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("file = %q, %v", got, err)
	}
}

func TestWriteFileDoesNotReplaceDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("data"), 0o600); err == nil {
		t.Fatal("replaced an existing directory")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("existing directory changed: %v, %v", info, err)
	}
}
