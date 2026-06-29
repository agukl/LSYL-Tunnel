package tunnel

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func CheckConfigUpgradeCompatible(path string) error {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read installed server config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse installed server config: %w", err)
	}
	ApplyDefaults(&cfg)
	if err := ValidateConfigVersion(cfg); err != nil {
		return fmt.Errorf("installed server config is not compatible with this server version: %w", err)
	}
	return nil
}
