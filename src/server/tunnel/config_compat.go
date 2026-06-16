package tunnel

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckConfigShapeCompatible compares YAML key structure with a reference config.
// Mapping keys must match exactly, but order and scalar values are ignored.
// Empty reference sequences are treated as data lists, so auth users and forwards
// can contain site-specific rows without blocking an upgrade.
func CheckConfigShapeCompatible(configPath, referencePath string) error {
	current, err := loadYAMLRoot(configPath)
	if err != nil {
		return fmt.Errorf("read installed config: %w", err)
	}
	reference, err := loadYAMLRoot(referencePath)
	if err != nil {
		return fmt.Errorf("read package config: %w", err)
	}
	issues := compareYAMLShape("", current, reference)
	if len(issues) > 0 {
		return fmt.Errorf("server config structure is incompatible with this installer: %s", summarizeYAMLShapeIssues(issues))
	}
	return nil
}

func loadYAMLRoot(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("empty yaml")
	}
	return doc.Content[0], nil
}

func compareYAMLShape(path string, current, reference *yaml.Node) []string {
	if current == nil || reference == nil {
		return nil
	}
	if reference.Kind == yaml.MappingNode {
		if current.Kind != yaml.MappingNode {
			return []string{fmt.Sprintf("%s expects mapping", displayYAMLPath(path))}
		}
		return compareYAMLMappingShape(path, current, reference)
	}
	if reference.Kind == yaml.SequenceNode {
		if current.Kind != yaml.SequenceNode {
			return []string{fmt.Sprintf("%s expects sequence", displayYAMLPath(path))}
		}
		itemShape := referenceSequenceItemShape(reference)
		if itemShape == nil {
			return nil
		}
		var issues []string
		for _, item := range current.Content {
			issues = append(issues, compareYAMLShape(path+"[]", item, itemShape)...)
		}
		return issues
	}
	if current.Kind == yaml.MappingNode || current.Kind == yaml.SequenceNode {
		return []string{fmt.Sprintf("%s expects scalar", displayYAMLPath(path))}
	}
	return nil
}

func compareYAMLMappingShape(path string, current, reference *yaml.Node) []string {
	currentMap := yamlMappingChildren(current)
	referenceMap := yamlMappingChildren(reference)
	var issues []string
	for _, key := range sortedYAMLKeys(referenceMap) {
		childPath := joinYAMLPath(path, key)
		currentChild, ok := currentMap[key]
		if !ok {
			issues = append(issues, "missing "+childPath)
			continue
		}
		issues = append(issues, compareYAMLShape(childPath, currentChild, referenceMap[key])...)
	}
	for _, key := range sortedYAMLKeys(currentMap) {
		if _, ok := referenceMap[key]; !ok {
			issues = append(issues, "extra "+joinYAMLPath(path, key))
		}
	}
	return issues
}

func referenceSequenceItemShape(seq *yaml.Node) *yaml.Node {
	for _, item := range seq.Content {
		if item.Kind == yaml.MappingNode || item.Kind == yaml.SequenceNode {
			return item
		}
	}
	return nil
}

func yamlMappingChildren(node *yaml.Node) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind != yaml.ScalarNode || strings.TrimSpace(keyNode.Value) == "" {
			continue
		}
		out[keyNode.Value] = node.Content[i+1]
	}
	return out
}

func sortedYAMLKeys(values map[string]*yaml.Node) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinYAMLPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func displayYAMLPath(path string) string {
	if path == "" {
		return "<root>"
	}
	return path
}

func summarizeYAMLShapeIssues(issues []string) string {
	sort.Strings(issues)
	const limit = 8
	if len(issues) <= limit {
		return strings.Join(issues, "; ")
	}
	return strings.Join(issues[:limit], "; ") + fmt.Sprintf("; and %d more", len(issues)-limit)
}
