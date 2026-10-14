package tunnel

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Unknown per-rule fields must fail at runtime as well as during installation.
func (f *ForwardConfig) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return fmt.Errorf("forward must be a mapping")
	}
	type rawForward ForwardConfig
	var raw rawForward
	if err := node.Decode(&raw); err != nil {
		return err
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch field := node.Content[i].Value; field {
		case "name", "direction", "listen_port", "listen_addr", "server_target", "allowed_users":
		default:
			return fmt.Errorf("forward %q contains unknown field %q", raw.Name, field)
		}
	}
	*f = ForwardConfig(raw)
	return nil
}
