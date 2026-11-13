package tunnel

import (
	"fmt"
	"os"
	"strings"

	"lsyltunnel/src/internal/configupgrade"
	"lsyltunnel/src/internal/configyaml"
	appversion "lsyltunnel/src/internal/version"

	"gopkg.in/yaml.v3"
)

func CheckConfigUpgradeCompatible(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read installed server config: %w", err)
	}
	cfg, err := decodeServerConfig(data)
	if err != nil {
		return fmt.Errorf("installed server config is not compatible with this server version: %w", err)
	}
	ApplyDefaults(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return fmt.Errorf("installed server config is not valid for this server version: %w", err)
	}
	return nil
}

// PrepareConfigMigration keeps only the installed business configuration and
// takes all other settings from the new installer's template.
func PrepareConfigMigration(path, templatePath string) (configupgrade.File, error) {
	cfg, err := LoadConfigRaw(templatePath)
	if err != nil {
		return configupgrade.File{}, fmt.Errorf("read new server config template: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return configupgrade.File{}, err
	}
	var document yaml.Node
	if err := configyaml.DecodeStrict(data, &document, "installed server config"); err != nil {
		return configupgrade.File{}, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return configupgrade.File{}, fmt.Errorf("installed server config must be a mapping")
	}
	fields, err := configyaml.MappingFields(document.Content[0], "installed server config")
	if err != nil {
		return configupgrade.File{}, err
	}
	listen, err := configyaml.RequiredScalar(fields, "installed server config", "listen_addr")
	if err != nil {
		return configupgrade.File{}, err
	}
	if _, _, err := splitHostPort(listen.Value); err != nil {
		return configupgrade.File{}, fmt.Errorf("installed server listen_addr: %w", err)
	}
	authNode, err := configyaml.RequiredField(fields, "installed server config", "auth")
	if err != nil {
		return configupgrade.File{}, err
	}
	authFields, err := configyaml.MappingFields(authNode, "installed server config.auth")
	if err != nil {
		return configupgrade.File{}, err
	}
	if _, err := configyaml.RequireSequence(authFields, "installed server config.auth", "users"); err != nil {
		return configupgrade.File{}, err
	}
	authData, err := yaml.Marshal(authNode)
	if err != nil {
		return configupgrade.File{}, err
	}
	if err := configyaml.DecodeStrict(authData, &cfg.Auth, "installed server users"); err != nil {
		return configupgrade.File{}, err
	}
	forwardsNode, err := configyaml.RequireSequence(fields, "installed server config", "forwards")
	if err != nil {
		return configupgrade.File{}, err
	}
	forwardsData, err := yaml.Marshal(forwardsNode)
	if err != nil {
		return configupgrade.File{}, err
	}
	if err := configyaml.DecodeStrict(forwardsData, &cfg.Forwards, "installed server forwards"); err != nil {
		return configupgrade.File{}, err
	}
	if entriesNode, ok := fields["business_entries"]; ok {
		if entriesNode.Kind != yaml.SequenceNode {
			return configupgrade.File{}, fmt.Errorf("installed server business_entries must be a list")
		}
		entriesData, err := yaml.Marshal(entriesNode)
		if err != nil {
			return configupgrade.File{}, err
		}
		if err := configyaml.DecodeStrict(entriesData, &cfg.BusinessEntries, "installed server business entries"); err != nil {
			return configupgrade.File{}, err
		}
	}
	cfg.ListenAddr = listen.Value
	ApplyDefaults(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return configupgrade.File{}, err
	}
	output, err := yaml.Marshal(configForSave(cfg, path))
	if err != nil {
		return configupgrade.File{}, err
	}
	if err := validateServerConfigV1Structure(output); err != nil {
		return configupgrade.File{}, err
	}
	return configupgrade.File{Path: path, Data: output}, nil
}

func decodeServerConfig(data []byte) (Config, error) {
	version, hasVersion, err := configyaml.SchemaVersion(data, "server config")
	if err != nil {
		return Config{}, fmt.Errorf("parse server config structure: %w", err)
	}
	if !hasVersion || version < appversion.ServerConfigVersion {
		return Config{}, fmt.Errorf("server config_version %d is no longer supported; migrate an installed 3.x config before starting", version)
	}
	if version > appversion.ServerConfigVersion {
		return Config{}, ValidateConfigVersion(Config{ConfigVersion: version})
	}
	var cfg Config
	if err := configyaml.DecodeStrict(data, &cfg, "config"); err != nil {
		return Config{}, err
	}
	if err := validateServerConfigV1Structure(data); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateServerConfigV1Structure(data []byte) error {
	root, err := configyaml.RootMapping(data, "server config")
	if err != nil {
		return err
	}
	top, err := configyaml.MappingFields(root, "server config")
	if err != nil {
		return err
	}
	for _, name := range []string{
		"config_version", "compatibility", "listen_addr", "monitor_addr",
		"tls", "auth", "forwards", "security", "credential_seal",
	} {
		if _, err := configyaml.RequiredField(top, "server config", name); err != nil {
			return err
		}
	}
	for _, name := range []string{"config_version", "listen_addr", "monitor_addr"} {
		if err := configyaml.RequireScalar(top, "server config", name); err != nil {
			return err
		}
	}
	if _, ok := top["log_level"]; ok {
		if err := configyaml.RequireScalar(top, "server config", "log_level"); err != nil {
			return err
		}
	}

	if requires, ok := top["requires"]; ok {
		if err := configyaml.ScalarMapping(requires, "server config.requires", "min_server_version"); err != nil {
			return err
		}
	}
	if err := configyaml.ScalarMapping(top["compatibility"], "server config.compatibility", "min_client_version", "max_client_version"); err != nil {
		return err
	}
	if err := configyaml.ScalarMapping(top["tls"], "server config.tls", "cert_file", "key_file", "min_version"); err != nil {
		return err
	}
	if entries, ok := top["business_entries"]; ok {
		if entries.Kind != yaml.SequenceNode {
			return fmt.Errorf("server config.business_entries must be a list")
		}
		for i, entry := range entries.Content {
			if err := configyaml.ScalarMapping(entry, fmt.Sprintf("server config.business_entries[%d]", i), "listen_addr", "advertise_addr"); err != nil {
				return err
			}
		}
	}
	if err := validateAuthUsers(top["auth"]); err != nil {
		return err
	}
	if err := validateForwardsV1(top["forwards"]); err != nil {
		return err
	}
	if err := configyaml.ScalarMapping(top["security"], "server config.security",
		"handshake_timeout_sec", "dial_timeout_sec",
		"max_concurrent_connections", "max_concurrent_connections_per_ip",
		"max_new_connections_per_ip_window",
		"max_concurrent_streams_per_user", "stream_rate_limit_bytes_per_sec",
		"auth_fail_window_sec", "auth_fail_threshold", "auth_fail_block_sec"); err != nil {
		return err
	}
	if err := validateCredentialSealKeys(top["credential_seal"]); err != nil {
		return err
	}
	if runtime, ok := top["runtime"]; ok {
		_, err := configyaml.MappingFields(runtime, "server config.runtime")
		return err
	}
	return nil
}

func validateAuthUsers(node *yaml.Node) error {
	fields, err := configyaml.MappingFields(node, "server config.auth")
	if err != nil {
		return err
	}
	users, err := configyaml.RequireSequence(fields, "server config.auth", "users")
	if err != nil {
		return err
	}
	for i, user := range users.Content {
		if err := configyaml.ScalarMapping(user, fmt.Sprintf("server config.auth.users[%d]", i), "username", "password_hash", "disabled"); err != nil {
			return err
		}
	}
	return nil
}

func validateForwardsV1(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("server config.forwards must be a list")
	}
	for i, forward := range node.Content {
		path := fmt.Sprintf("server config.forwards[%d]", i)
		fields, err := configyaml.MappingFields(forward, path)
		if err != nil {
			return err
		}
		for _, name := range []string{"name", "direction", "allowed_users"} {
			if _, err := configyaml.RequiredField(fields, path, name); err != nil {
				return err
			}
		}
		if err := configyaml.RequireScalar(fields, path, "name"); err != nil {
			return err
		}
		direction, err := configyaml.RequiredScalar(fields, path, "direction")
		if err != nil {
			return err
		}
		users, err := configyaml.RequireSequence(fields, path, "allowed_users")
		if err != nil {
			return err
		}
		for userIndex, user := range users.Content {
			if user.Kind != yaml.ScalarNode || user.Tag == "!!null" {
				return fmt.Errorf("%s.allowed_users[%d] must be a value", path, userIndex)
			}
		}
		switch strings.TrimSpace(direction.Value) {
		case DirectionClientToServer:
			if err := configyaml.RequireScalar(fields, path, "server_target"); err != nil {
				return err
			}
		case DirectionServerToClient:
			if _, hasPort := fields["listen_port"]; !hasPort {
				if err := configyaml.RequireScalar(fields, path, "listen_addr"); err != nil {
					return err
				}
			} else if err := configyaml.RequireScalar(fields, path, "listen_port"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCredentialSealKeys(node *yaml.Node) error {
	fields, err := configyaml.MappingFields(node, "server config.credential_seal")
	if err != nil {
		return err
	}
	keys, err := configyaml.RequireSequence(fields, "server config.credential_seal", "keys")
	if err != nil {
		return err
	}
	for i, key := range keys.Content {
		if err := configyaml.ScalarMapping(key, fmt.Sprintf("server config.credential_seal.keys[%d]", i),
			"key_id", "private_key_file", "public_key_file", "expires_at", "active"); err != nil {
			return err
		}
	}
	return nil
}
