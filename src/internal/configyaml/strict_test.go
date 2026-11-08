package configyaml

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSchemaVersionRejectsDuplicateFields(t *testing.T) {
	_, _, err := SchemaVersion([]byte("config_version: 1\nconfig_version: 2\n"), "server config")
	if err == nil || !strings.Contains(err.Error(), "duplicate field config_version") {
		t.Fatalf("duplicate version error = %v", err)
	}
}

func TestDecodeStrictRejectsSecondDocument(t *testing.T) {
	var out map[string]string
	err := DecodeStrict([]byte("name: first\n---\nname: second\n"), &out, "client config")
	if err == nil || err.Error() != "client config must contain exactly one YAML document" {
		t.Fatalf("second document error = %v", err)
	}
}

func TestMappingHelpersPreserveFieldErrors(t *testing.T) {
	root, err := RootMapping([]byte("tls:\n  cert_file: null\n"), "server config")
	if err != nil {
		t.Fatal(err)
	}
	top, err := MappingFields(root, "server config")
	if err != nil {
		t.Fatal(err)
	}
	if err := ScalarMapping(top["tls"], "server config.tls", "cert_file"); err == nil ||
		err.Error() != "server config.tls.cert_file must be a value" {
		t.Fatalf("null required value error = %v", err)
	}
	if err := ScalarMapping(top["tls"], "server config.tls", "key_file"); err == nil ||
		err.Error() != "server config.tls is missing required field key_file" {
		t.Fatalf("missing field error = %v", err)
	}
}

func TestRequireOptionalScalarAllowsNull(t *testing.T) {
	fields := map[string]*yaml.Node{"remark": {Kind: yaml.ScalarNode, Tag: "!!null"}}
	if err := RequireOptionalScalar(fields, "client config", "remark"); err != nil {
		t.Fatal(err)
	}
}
