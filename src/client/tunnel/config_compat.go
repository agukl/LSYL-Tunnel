package tunnel

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	appversion "lsyltunnel/src/internal/version"

	"gopkg.in/yaml.v3"
)

func CheckConfigUpgradeCompatible(path string) error {
	return checkConfigUpgradeCompatible(path, "")
}

// CheckConfigUpgradeCompatibleWithCACert validates an archived config against
// its paired certificate instead of the certificate referenced by the active profile.
func CheckConfigUpgradeCompatibleWithCACert(path, caCertFile string) error {
	if strings.TrimSpace(caCertFile) == "" {
		return fmt.Errorf("client config compatibility check requires a CA certificate file")
	}
	return checkConfigUpgradeCompatible(path, caCertFile)
}

func checkConfigUpgradeCompatible(path, caCertFile string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read installed client config: %w", err)
	}
	cfg, err := decodeClientConfig(data)
	if err != nil {
		return fmt.Errorf("installed client config is not compatible with this client version: %w", err)
	}

	ApplyDefaults(&cfg)
	if strings.TrimSpace(caCertFile) != "" {
		cfg.TLS.CACertFile = resolveConfigPath(filepath.Dir(path), caCertFile)
	} else {
		cfg.TLS.CACertFile = resolveConfigPath(filepath.Dir(path), cfg.TLS.CACertFile)
	}
	// Login state is operational data, not part of the install-time schema check.
	cfg.Username = "compatibility-check"
	cfg.Password = "compatibility-check"
	if err := ValidateConfig(cfg); err != nil {
		return fmt.Errorf("installed client config is not valid for this client version: %w", err)
	}
	return nil
}

func decodeClientConfig(data []byte) (Config, error) {
	version, hasVersion, err := clientConfigSchemaVersion(data)
	if err != nil {
		return Config{}, err
	}
	if hasVersion && version != 0 && version != 1 && version != appversion.ClientConfigVersion {
		cfg := Config{ConfigVersion: version}
		return Config{}, ValidateConfigVersion(cfg)
	}

	var cfg Config
	if err := decodeStrictClientYAML(data, &cfg); err != nil {
		return Config{}, err
	}
	if hasVersion && version > 0 {
		if err := validateVersionedClientConfigStructure(data); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func clientConfigSchemaVersion(data []byte) (int, bool, error) {
	root, err := clientYAMLRootMapping(data)
	if err != nil {
		return 0, false, err
	}
	fields, err := clientYAMLMappingFields(root, "client config")
	if err != nil {
		return 0, false, err
	}
	node, ok := fields["config_version"]
	if !ok {
		return 0, false, nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return 0, true, fmt.Errorf("client config config_version must be a number")
	}
	var version int
	if err := node.Decode(&version); err != nil {
		return 0, true, fmt.Errorf("client config config_version is invalid: %w", err)
	}
	return version, true, nil
}

func decodeStrictClientYAML(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("client config must contain exactly one YAML document")
		}
		return err
	}
	return nil
}

func validateVersionedClientConfigStructure(data []byte) error {
	root, err := clientYAMLRootMapping(data)
	if err != nil {
		return err
	}
	top, err := clientYAMLMappingFields(root, "client config")
	if err != nil {
		return err
	}
	for _, name := range []string{
		"config_version", "requires", "server_addr", "username", "password", "password_env",
		"password_file", "saved_credential", "client_id", "log_level", "tls", "connection", "forwards",
	} {
		if _, err := clientYAMLRequiredField(top, "client config", name); err != nil {
			return err
		}
	}
	for _, name := range []string{
		"config_version", "server_addr", "username", "password", "password_env", "password_file", "client_id", "log_level",
	} {
		if err := clientYAMLRequireScalar(top, "client config", name); err != nil {
			return err
		}
	}
	if err := validateClientScalarMapping(top["requires"], "client config.requires", "min_client_version"); err != nil {
		return err
	}
	if err := validateClientSavedCredentialStructure(top["saved_credential"]); err != nil {
		return err
	}
	if err := validateClientScalarMapping(top["tls"], "client config.tls", "ca_cert_file", "server_name", "min_version", "insecure_skip_verify"); err != nil {
		return err
	}
	if err := validateClientScalarMapping(top["connection"], "client config.connection", "dial_timeout_sec"); err != nil {
		return err
	}
	return validateClientForwardsStructure(top["forwards"])
}

func validateClientSavedCredentialStructure(node *yaml.Node) error {
	const path = "client config.saved_credential"
	fields, err := clientYAMLMappingFields(node, path)
	if err != nil {
		return err
	}
	// Release packages and logged-out clients use an empty mapping here.
	if len(fields) == 0 {
		return nil
	}
	for _, name := range []string{"type", "key_id", "expires_at", "ciphertext"} {
		if err := clientYAMLRequireScalar(fields, path, name); err != nil {
			return err
		}
	}
	return nil
}

func validateClientForwardsStructure(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.SequenceNode {
		return fmt.Errorf("client config.forwards must be a list")
	}
	for index, forward := range node.Content {
		path := fmt.Sprintf("client config.forwards[%d]", index)
		fields, err := clientYAMLMappingFields(forward, path)
		if err != nil {
			return err
		}
		for _, name := range []string{"name", "direction", "listen_addr", "server_target"} {
			if err := clientYAMLRequireScalar(fields, path, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateClientScalarMapping(node *yaml.Node, path string, names ...string) error {
	fields, err := clientYAMLMappingFields(node, path)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := clientYAMLRequireScalar(fields, path, name); err != nil {
			return err
		}
	}
	return nil
}

func clientYAMLRootMapping(data []byte) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("client config must contain one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("client config must be a mapping")
	}
	return root, nil
}

func clientYAMLMappingFields(node *yaml.Node, path string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", path)
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s contains an invalid key", path)
		}
		if _, exists := fields[key.Value]; exists {
			return nil, fmt.Errorf("%s contains duplicate field %s", path, key.Value)
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, nil
}

func clientYAMLRequiredField(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("%s is missing required field %s", path, name)
	}
	return node, nil
}

func clientYAMLRequireScalar(fields map[string]*yaml.Node, path, name string) error {
	node, err := clientYAMLRequiredField(fields, path, name)
	if err != nil {
		return err
	}
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return fmt.Errorf("%s.%s must be a value", path, name)
	}
	return nil
}
