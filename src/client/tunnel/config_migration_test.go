package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareClientConfigMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.yaml")
	input := `config_version: 2
server_addr: 127.0.0.1:3443
username: alice
client_id: ""
tls:
  ca_cert_file: ../cert/server.crt
forwards:
  - name: rdp
    listen_addr: 127.0.0.1:3388
    server_target: 127.0.0.1:3389
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := PrepareConfigMigration(path, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"requires:", "saved_credential:", "connection:", "client_id: \"\"", "direction: client_to_server"} {
		if !strings.Contains(string(file.Data), want) {
			t.Fatalf("missing %q in migrated config: %s", want, file.Data)
		}
	}
	if strings.Contains(string(file.Data), "compatibility-check") {
		t.Fatal("temporary login credential was persisted")
	}
	if err := validateVersionedClientConfigStructure(file.Data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, file.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigRaw(path); err != nil {
		t.Fatalf("migrated client config cannot be loaded: %v", err)
	}
	if err := os.WriteFile(path, []byte(input+"unknown_setting: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareConfigMigration(path, ""); err == nil {
		t.Fatal("unknown field should prevent migration")
	}
	if err := os.WriteFile(path, []byte(input+"saved_credential:\n  type: server_sealed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareConfigMigration(path, ""); err == nil {
		t.Fatal("partial saved credential should prevent migration")
	}
	if err := os.WriteFile(path, []byte(strings.Replace(input, "username: alice\n", "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareConfigMigration(path, ""); err == nil {
		t.Fatal("missing account field should prevent migration")
	}
}
