package tunnel

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	appversion "lsyltunnel/src/internal/version"

	"gopkg.in/yaml.v3"
)

// legacyServerConfigV0 is the unversioned schema shipped before config_version.
// It remains installation-compatible, but only its known fields are accepted.
type legacyServerConfigV0 struct {
	ConfigVersion  int                  `yaml:"config_version"`
	ListenAddr     string               `yaml:"listen_addr"`
	MonitorAddr    string               `yaml:"monitor_addr"`
	LogLevel       string               `yaml:"log_level"`
	TLS            TLSConfig            `yaml:"tls"`
	Auth           AuthConfig           `yaml:"auth"`
	Forwards       []legacyForwardV0    `yaml:"forwards"`
	Security       SecurityConfig       `yaml:"security"`
	CredentialSeal CredentialSealConfig `yaml:"credential_seal"`
	Runtime        RuntimeConfig        `yaml:"runtime"`
}

type legacyForwardV0 struct {
	Name         string   `yaml:"name"`
	Direction    string   `yaml:"direction"`
	ListenAddr   string   `yaml:"listen_addr"`
	ServerTarget string   `yaml:"server_target"`
	AllowedUsers []string `yaml:"allowed_users,omitempty"`
}

func CheckConfigUpgradeCompatible(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read installed server config: %w", err)
	}

	version, hasVersion, err := serverConfigSchemaVersion(data)
	if err != nil {
		return fmt.Errorf("parse installed server config structure: %w", err)
	}

	var cfg Config
	switch {
	case !hasVersion || version == 0:
		cfg, err = decodeLegacyServerConfigV0(data)
	case version == appversion.ServerConfigVersion:
		if err = decodeStrictYAML(data, &cfg); err == nil {
			err = validateServerConfigV1Structure(data)
		}
	default:
		cfg.ConfigVersion = version
		err = ValidateConfigVersion(cfg)
	}
	if err != nil {
		return fmt.Errorf("installed server config is not compatible with this server version: %w", err)
	}

	ApplyDefaults(&cfg)
	if err := ValidateConfig(cfg); err != nil {
		return fmt.Errorf("installed server config is not valid for this server version: %w", err)
	}
	return nil
}

func decodeLegacyServerConfigV0(data []byte) (Config, error) {
	var legacy legacyServerConfigV0
	if err := decodeStrictYAML(data, &legacy); err != nil {
		return Config{}, err
	}
	cfg := Config{
		ConfigVersion:  legacy.ConfigVersion,
		ListenAddr:     legacy.ListenAddr,
		MonitorAddr:    legacy.MonitorAddr,
		LogLevel:       legacy.LogLevel,
		TLS:            legacy.TLS,
		Auth:           legacy.Auth,
		Security:       legacy.Security,
		CredentialSeal: legacy.CredentialSeal,
		Runtime:        legacy.Runtime,
		Forwards:       make([]ForwardConfig, 0, len(legacy.Forwards)),
	}
	for _, forward := range legacy.Forwards {
		cfg.Forwards = append(cfg.Forwards, ForwardConfig{
			Name:         forward.Name,
			Direction:    forward.Direction,
			ListenAddr:   forward.ListenAddr,
			ServerTarget: forward.ServerTarget,
			AllowedUsers: forward.AllowedUsers,
		})
	}
	return cfg, nil
}

func serverConfigSchemaVersion(data []byte) (int, bool, error) {
	root, err := yamlRootMapping(data)
	if err != nil {
		return 0, false, err
	}
	fields, err := yamlMappingFields(root, "server config")
	if err != nil {
		return 0, false, err
	}
	node, ok := fields["config_version"]
	if !ok {
		return 0, false, nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return 0, true, fmt.Errorf("server config config_version must be a number")
	}
	var version int
	if err := node.Decode(&version); err != nil {
		return 0, true, fmt.Errorf("server config config_version is invalid: %w", err)
	}
	return version, true, nil
}

func decodeStrictYAML(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("config must contain exactly one YAML document")
		}
		return err
	}
	return nil
}

func validateServerConfigV1Structure(data []byte) error {
	root, err := yamlRootMapping(data)
	if err != nil {
		return err
	}
	top, err := yamlMappingFields(root, "server config")
	if err != nil {
		return err
	}
	for _, name := range []string{
		"config_version", "requires", "compatibility", "listen_addr", "monitor_addr", "log_level",
		"tls", "auth", "forwards", "security", "credential_seal", "runtime",
	} {
		if _, err := yamlRequiredField(top, "server config", name); err != nil {
			return err
		}
	}
	for _, name := range []string{"config_version", "listen_addr", "monitor_addr", "log_level"} {
		if err := yamlRequireScalar(top, "server config", name); err != nil {
			return err
		}
	}

	if err := validateScalarMapping(top["requires"], "server config.requires", "min_server_version"); err != nil {
		return err
	}
	if err := validateScalarMapping(top["compatibility"], "server config.compatibility", "min_client_version", "max_client_version", "protocol_version"); err != nil {
		return err
	}
	if err := validateScalarMapping(top["tls"], "server config.tls", "cert_file", "key_file", "min_version"); err != nil {
		return err
	}
	if err := validateAuthUsers(top["auth"]); err != nil {
		return err
	}
	if err := validateForwardsV1(top["forwards"]); err != nil {
		return err
	}
	if err := validateScalarMapping(top["security"], "server config.security",
		"handshake_timeout_sec", "dial_timeout_sec", "max_handshake_bytes",
		"max_concurrent_connections", "max_concurrent_connections_per_ip",
		"connection_rate_window_sec", "max_new_connections_per_ip_window",
		"max_concurrent_streams_per_user", "stream_rate_limit_bytes_per_sec",
		"auth_fail_window_sec", "auth_fail_threshold", "auth_fail_block_sec"); err != nil {
		return err
	}
	if err := validateCredentialSealKeys(top["credential_seal"]); err != nil {
		return err
	}
	return validateScalarMapping(top["runtime"], "server config.runtime",
		"state_file", "permanent_block_file", "request_log_file", "business_log_file",
		"entry_traffic_log_file", "flow_traffic_log_file", "recent_events")
}

func validateAuthUsers(node *yaml.Node) error {
	fields, err := yamlMappingFields(node, "server config.auth")
	if err != nil {
		return err
	}
	users, err := yamlRequireSequence(fields, "server config.auth", "users")
	if err != nil {
		return err
	}
	for i, user := range users.Content {
		if err := validateScalarMapping(user, fmt.Sprintf("server config.auth.users[%d]", i), "username", "password_hash", "disabled"); err != nil {
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
		fields, err := yamlMappingFields(forward, path)
		if err != nil {
			return err
		}
		for _, name := range []string{"name", "direction", "allowed_users"} {
			if _, err := yamlRequiredField(fields, path, name); err != nil {
				return err
			}
		}
		if err := yamlRequireScalar(fields, path, "name"); err != nil {
			return err
		}
		direction, err := yamlRequiredScalar(fields, path, "direction")
		if err != nil {
			return err
		}
		users, err := yamlRequireSequence(fields, path, "allowed_users")
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
			if err := yamlRequireScalar(fields, path, "server_target"); err != nil {
				return err
			}
		case DirectionServerToClient:
			if _, hasPort := fields["listen_port"]; !hasPort {
				if err := yamlRequireScalar(fields, path, "listen_addr"); err != nil {
					return err
				}
			} else if err := yamlRequireScalar(fields, path, "listen_port"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCredentialSealKeys(node *yaml.Node) error {
	fields, err := yamlMappingFields(node, "server config.credential_seal")
	if err != nil {
		return err
	}
	keys, err := yamlRequireSequence(fields, "server config.credential_seal", "keys")
	if err != nil {
		return err
	}
	for i, key := range keys.Content {
		if err := validateScalarMapping(key, fmt.Sprintf("server config.credential_seal.keys[%d]", i),
			"key_id", "private_key_file", "public_key_file", "expires_at", "active"); err != nil {
			return err
		}
	}
	return nil
}

func validateScalarMapping(node *yaml.Node, path string, names ...string) error {
	fields, err := yamlMappingFields(node, path)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := yamlRequireScalar(fields, path, name); err != nil {
			return err
		}
	}
	return nil
}

func yamlRootMapping(data []byte) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("server config must contain one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("server config must be a mapping")
	}
	return root, nil
}

func yamlMappingFields(node *yaml.Node, path string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", path)
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s contains an invalid key", path)
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, nil
}

func yamlRequiredField(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("%s is missing required field %s", path, name)
	}
	return node, nil
}

func yamlRequireScalar(fields map[string]*yaml.Node, path, name string) error {
	_, err := yamlRequiredScalar(fields, path, name)
	return err
}

func yamlRequiredScalar(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, err := yamlRequiredField(fields, path, name)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return nil, fmt.Errorf("%s.%s must be a value", path, name)
	}
	return node, nil
}

func yamlRequireSequence(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, err := yamlRequiredField(fields, path, name)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s.%s must be a list", path, name)
	}
	return node, nil
}
