package tunnel

import (
	"fmt"
	"os"
	"strings"

	"lsyltunnel/src/internal/atomicfile"
	"lsyltunnel/src/internal/configyaml"

	"gopkg.in/yaml.v3"
)

// PreserveInstalledUsername carries the old account name into the newly
// installed profile without copying credentials or generating a client ID.
func PreserveInstalledUsername(oldPath, newPath string) error {
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("read previous client config: %w", err)
	}
	var oldDocument yaml.Node
	if err := configyaml.DecodeStrict(oldData, &oldDocument, "previous client config"); err != nil {
		return nil
	}
	if oldDocument.Kind != yaml.DocumentNode || len(oldDocument.Content) != 1 {
		return nil
	}
	oldFields, err := configyaml.MappingFields(oldDocument.Content[0], "previous client config")
	if err != nil {
		return nil
	}
	oldUsername, err := configyaml.RequiredScalar(oldFields, "previous client config", "username")
	if err != nil {
		return nil
	}
	if strings.TrimSpace(oldUsername.Value) == "" {
		return nil
	}

	newData, err := os.ReadFile(newPath)
	if err != nil {
		return fmt.Errorf("read installed client config: %w", err)
	}
	if _, err := decodeClientConfig(newData); err != nil {
		return fmt.Errorf("installed client config is invalid: %w", err)
	}
	var newDocument yaml.Node
	if err := configyaml.DecodeStrict(newData, &newDocument, "installed client config"); err != nil {
		return err
	}
	newFields, err := configyaml.MappingFields(newDocument.Content[0], "installed client config")
	if err != nil {
		return err
	}
	newUsername, err := configyaml.RequiredScalar(newFields, "installed client config", "username")
	if err != nil {
		return err
	}
	if newUsername.Value == oldUsername.Value {
		return nil
	}
	newUsername.Value = oldUsername.Value
	newUsername.Tag = "!!str"
	output, err := yaml.Marshal(&newDocument)
	if err != nil {
		return err
	}
	if _, err := decodeClientConfig(output); err != nil {
		return fmt.Errorf("installed client config after account migration is invalid: %w", err)
	}
	return atomicfile.WriteFile(newPath, output, 0o644)
}
