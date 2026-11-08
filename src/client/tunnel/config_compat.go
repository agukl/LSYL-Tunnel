package tunnel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lsyltunnel/src/internal/configupgrade"
	"lsyltunnel/src/internal/configyaml"
	appversion "lsyltunnel/src/internal/version"

	"gopkg.in/yaml.v3"
)

func CheckConfigUpgradeCompatible(path string) error {
	return checkConfigUpgradeCompatible(path, "")
}

// PrepareConfigMigration materializes the current schema without persisting
// credentials synthesized solely for install-time validation.
func PrepareConfigMigration(path, caCertFile string) (configupgrade.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return configupgrade.File{}, err
	}
	version, present, err := configyaml.SchemaVersion(data, "client config")
	if err != nil {
		return configupgrade.File{}, err
	}
	if !present || version != appversion.ClientConfigVersion {
		return configupgrade.File{}, fmt.Errorf("client config_version %d cannot be migrated from an installed 3.x version", version)
	}
	root, err := configyaml.RootMapping(data, "client config")
	if err != nil {
		return configupgrade.File{}, err
	}
	fields, err := configyaml.MappingFields(root, "client config")
	if err != nil {
		return configupgrade.File{}, err
	}
	for _, name := range []string{"requires", "saved_credential", "tls", "connection"} {
		if node, ok := fields[name]; ok && node.Kind != yaml.MappingNode {
			return configupgrade.File{}, fmt.Errorf("client config.%s must be a mapping", name)
		}
	}
	if node, ok := fields["forwards"]; ok && node.Kind != yaml.SequenceNode {
		return configupgrade.File{}, fmt.Errorf("client config.forwards must be a list")
	}
	if _, err := configyaml.RequiredField(fields, "client config", "username"); err != nil {
		return configupgrade.File{}, err
	}
	if node, ok := fields["saved_credential"]; ok {
		if err := validateClientSavedCredentialStructure(node); err != nil {
			return configupgrade.File{}, err
		}
	}
	var cfg Config
	if err := configyaml.DecodeStrict(data, &cfg, "client config"); err != nil {
		return configupgrade.File{}, err
	}
	originalID := cfg.ClientID
	ApplyDefaults(&cfg)
	if cfg.Forwards == nil {
		cfg.Forwards = []ForwardConfig{}
	}
	validation := cfg
	if caCertFile != "" {
		validation.TLS.CACertFile = resolveConfigPath(filepath.Dir(path), caCertFile)
	} else {
		validation.TLS.CACertFile = resolveConfigPath(filepath.Dir(path), cfg.TLS.CACertFile)
	}
	validation.Username = "compatibility-check"
	validation.Password = "compatibility-check"
	if err := ValidateConfig(validation); err != nil {
		return configupgrade.File{}, err
	}
	cfg.ClientID = originalID
	output, err := yaml.Marshal(cfg)
	if err != nil {
		return configupgrade.File{}, err
	}
	if err := validateVersionedClientConfigStructure(output); err != nil {
		return configupgrade.File{}, err
	}
	return configupgrade.File{Path: path, Data: output}, nil
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
	version, hasVersion, err := configyaml.SchemaVersion(data, "client config")
	if err != nil {
		return Config{}, err
	}
	if !hasVersion || version < appversion.ClientConfigVersion {
		return Config{}, fmt.Errorf("client config_version %d is no longer supported; migrate an installed 3.x config before starting", version)
	}
	if version > appversion.ClientConfigVersion {
		return Config{}, ValidateConfigVersion(Config{ConfigVersion: version})
	}

	var cfg Config
	if err := configyaml.DecodeStrict(data, &cfg, "client config"); err != nil {
		return Config{}, err
	}
	if err := validateVersionedClientConfigStructure(data); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateVersionedClientConfigStructure(data []byte) error {
	root, err := configyaml.RootMapping(data, "client config")
	if err != nil {
		return err
	}
	top, err := configyaml.MappingFields(root, "client config")
	if err != nil {
		return err
	}
	for _, name := range []string{
		"config_version", "requires", "server_addr", "username", "password", "password_env",
		"password_file", "saved_credential", "client_id", "tls", "connection", "forwards",
	} {
		if _, err := configyaml.RequiredField(top, "client config", name); err != nil {
			return err
		}
	}
	for _, name := range []string{
		"config_version", "server_addr", "username", "password", "password_env", "password_file", "client_id",
	} {
		if err := configyaml.RequireScalar(top, "client config", name); err != nil {
			return err
		}
	}
	if err := configyaml.RequireOptionalScalar(top, "client config", "log_level"); err != nil {
		return err
	}
	if err := configyaml.RequireOptionalScalar(top, "client config", "remark"); err != nil {
		return err
	}
	if err := configyaml.ScalarMapping(top["requires"], "client config.requires", "min_client_version"); err != nil {
		return err
	}
	if err := validateClientSavedCredentialStructure(top["saved_credential"]); err != nil {
		return err
	}
	if err := configyaml.ScalarMapping(top["tls"], "client config.tls", "ca_cert_file", "server_name", "min_version", "insecure_skip_verify"); err != nil {
		return err
	}
	if err := configyaml.ScalarMapping(top["connection"], "client config.connection", "dial_timeout_sec"); err != nil {
		return err
	}
	return validateClientForwardsStructure(top["forwards"])
}

func validateClientSavedCredentialStructure(node *yaml.Node) error {
	const path = "client config.saved_credential"
	fields, err := configyaml.MappingFields(node, path)
	if err != nil {
		return err
	}
	// Release packages and logged-out clients use an empty mapping here.
	if len(fields) == 0 {
		return nil
	}
	for _, name := range []string{"type", "key_id", "expires_at", "ciphertext"} {
		if err := configyaml.RequireScalar(fields, path, name); err != nil {
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
		fields, err := configyaml.MappingFields(forward, path)
		if err != nil {
			return err
		}
		for _, name := range []string{"name", "direction", "listen_addr", "server_target"} {
			if err := configyaml.RequireScalar(fields, path, name); err != nil {
				return err
			}
		}
	}
	return nil
}
