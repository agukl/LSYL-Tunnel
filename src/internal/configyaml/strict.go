package configyaml

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func DecodeStrict(data []byte, out any, label string) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s must contain exactly one YAML document", label)
		}
		return err
	}
	return nil
}

func RootMapping(data []byte, label string) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("%s must contain one YAML document", label)
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", label)
	}
	return root, nil
}

func MappingFields(node *yaml.Node, path string) (map[string]*yaml.Node, error) {
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

func SchemaVersion(data []byte, label string) (int, bool, error) {
	root, err := RootMapping(data, label)
	if err != nil {
		return 0, false, err
	}
	fields, err := MappingFields(root, label)
	if err != nil {
		return 0, false, err
	}
	node, ok := fields["config_version"]
	if !ok {
		return 0, false, nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return 0, true, fmt.Errorf("%s config_version must be a number", label)
	}
	var version int
	if err := node.Decode(&version); err != nil {
		return 0, true, fmt.Errorf("%s config_version is invalid: %w", label, err)
	}
	return version, true, nil
}

func RequiredField(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("%s is missing required field %s", path, name)
	}
	return node, nil
}

func RequiredScalar(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, err := RequiredField(fields, path, name)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return nil, fmt.Errorf("%s.%s must be a value", path, name)
	}
	return node, nil
}

func RequireScalar(fields map[string]*yaml.Node, path, name string) error {
	_, err := RequiredScalar(fields, path, name)
	return err
}

func RequireOptionalScalar(fields map[string]*yaml.Node, path, name string) error {
	node, ok := fields[name]
	if !ok || node.Tag == "!!null" {
		return nil
	}
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%s.%s must be a value", path, name)
	}
	return nil
}

func RequireSequence(fields map[string]*yaml.Node, path, name string) (*yaml.Node, error) {
	node, err := RequiredField(fields, path, name)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s.%s must be a list", path, name)
	}
	return node, nil
}

func ScalarMapping(node *yaml.Node, path string, names ...string) error {
	fields, err := MappingFields(node, path)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := RequireScalar(fields, path, name); err != nil {
			return err
		}
	}
	return nil
}
