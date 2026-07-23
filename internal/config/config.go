package config

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/GyroZepelix/loremaster/internal/provider"
	"gopkg.in/yaml.v3"
)

var scpPortPattern = regexp.MustCompile(`^git@([^:]+):(\d+)/`)

type ProviderList []string

func (p *ProviderList) UnmarshalYAML(value *yaml.Node) error {
	if value.Tag == "!!null" {
		return fmt.Errorf("missing required field: provider")
	}
	if value.Kind == yaml.ScalarNode {
		if value.Value == "" {
			return fmt.Errorf("missing required field: provider")
		}
		*p = []string{value.Value}
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("invalid provider type: must be a string or list")
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*p = list
	return nil
}

type Config struct {
	Providers ProviderList
	Resources []Resource
	// Skills is the compatibility view of the resource named "skills".
	Skills []SkillSource
}

type Resource struct {
	Name    string
	Sources []SkillSource
}

type SkillSource struct {
	Source         string         `yaml:"source"`
	Ref            string         `yaml:"ref,omitempty"`
	Include        []string       `yaml:"include"`
	Type           string         `yaml:"type,omitempty"`
	ParsedIncludes []IncludeEntry `yaml:"-"`
}

func (c *Config) AllResources() []Resource {
	if len(c.Resources) > 0 {
		return c.Resources
	}
	if c.Skills != nil {
		return []Resource{{Name: "skills", Sources: c.Skills}}
	}
	return nil
}

func (c *Config) AllSources() []SkillSource {
	var sources []SkillSource
	for _, resource := range c.AllResources() {
		sources = append(sources, resource.Sources...)
	}
	return sources
}

func Parse(r io.Reader) (*Config, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid YAML: top level must be a mapping")
	}

	root := doc.Content[0]
	cfg := &Config{}
	seen := make(map[string]bool)
	for i := 0; i < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valueNode := root.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Value == "" {
			return nil, fmt.Errorf("invalid YAML: top-level keys must be non-empty strings")
		}
		key := keyNode.Value
		if seen[key] {
			return nil, fmt.Errorf("duplicate top-level key %q", key)
		}
		seen[key] = true

		if key == "provider" {
			if err := valueNode.Decode(&cfg.Providers); err != nil {
				return nil, fmt.Errorf("invalid YAML: %w", err)
			}
			continue
		}

		name, err := ValidateResourceName(key)
		if err != nil {
			return nil, err
		}
		sources, err := decodeSources(valueNode, name)
		if err != nil {
			return nil, err
		}
		cfg.Resources = append(cfg.Resources, Resource{Name: name, Sources: sources})
		if name == "skills" {
			cfg.Skills = sources
		}
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func decodeSources(node *yaml.Node, resource string) ([]SkillSource, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("resource %q must be a list of sources", resource)
	}
	if len(node.Content) == 0 {
		if resource == "skills" {
			return nil, fmt.Errorf("skills list must have at least one entry (edit lore.yml to add skill sources)")
		}
		return nil, fmt.Errorf("resource %q must have at least one source", resource)
	}

	allowed := map[string]bool{"source": true, "ref": true, "include": true, "type": true}
	sources := make([]SkillSource, 0, len(node.Content))
	for i, sourceNode := range node.Content {
		if sourceNode.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s[%d]: source entry must be a mapping", resource, i)
		}
		for j := 0; j < len(sourceNode.Content); j += 2 {
			field := sourceNode.Content[j].Value
			if !allowed[field] {
				return nil, fmt.Errorf("%s[%d]: field %s not found in source", resource, i, field)
			}
		}
		var source SkillSource
		if err := sourceNode.Decode(&source); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", resource, i, err)
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func validateConfig(cfg *Config) error {
	if len(cfg.Providers) == 0 {
		return fmt.Errorf("missing required field: provider")
	}
	seenProviders := make(map[string]bool)
	for _, p := range cfg.Providers {
		if !provider.IsSupported(p) {
			return fmt.Errorf("invalid provider %q: must be one of: %s", p, provider.SupportedNames())
		}
		if seenProviders[p] {
			return fmt.Errorf("duplicate provider %q", p)
		}
		seenProviders[p] = true
	}
	if len(cfg.Resources) == 0 {
		return fmt.Errorf("configuration must declare at least one resource")
	}

	type sourceRef struct {
		label  string
		source SkillSource
	}
	var refs []sourceRef
	var destinations []IncludeEntry
	for resourceIndex := range cfg.Resources {
		resource := &cfg.Resources[resourceIndex]
		for sourceIndex := range resource.Sources {
			source := &resource.Sources[sourceIndex]
			label := fmt.Sprintf("%s[%d]", resource.Name, sourceIndex)
			if source.Source == "" {
				return fmt.Errorf("%s: missing required field: source", label)
			}
			if len(source.Include) == 0 {
				return fmt.Errorf("%s: missing required field: include", label)
			}
			for _, raw := range source.Include {
				entry, err := ParseIncludeEntry(raw)
				if err != nil {
					return fmt.Errorf("%s: %w", label, err)
				}
				source.ParsedIncludes = append(source.ParsedIncludes, entry)
				destinations = append(destinations, IncludeEntry{Src: entry.Src, Dst: joinResourcePath(resource.Name, entry.Dst)})
			}
			if err := ValidateOverlaps(source.ParsedIncludes); err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			if match := scpPortPattern.FindStringSubmatch(source.Source); match != nil {
				rest := strings.TrimPrefix(source.Source, match[0])
				return fmt.Errorf("%s: source %q looks like it contains a port number - SCP-style URLs (git@host:path) don't support ports; use ssh://git@%s:%s/%s instead", label, source.Source, match[1], match[2], rest)
			}
			if source.Type == "" {
				source.Type = "soft"
			}
			if source.Type != "soft" && source.Type != "hard" {
				return fmt.Errorf("%s: invalid type %q: must be \"soft\" or \"hard\"", label, source.Type)
			}
			refs = append(refs, sourceRef{label: label, source: *source})
		}
		if resource.Name == "skills" {
			cfg.Skills = resource.Sources
		}
	}

	if err := ValidateOverlaps(destinations); err != nil {
		return fmt.Errorf("overlapping resource destinations: %w", err)
	}

	for i := 0; i < len(refs); i++ {
		if !IsGitSource(refs[i].source.Source) {
			continue
		}
		for j := i + 1; j < len(refs); j++ {
			if !IsGitSource(refs[j].source.Source) {
				continue
			}
			if refs[i].source.Source == refs[j].source.Source && refs[i].source.Ref != refs[j].source.Ref {
				return fmt.Errorf("%s and %s reference the same source %q with different refs (%q vs %q) - consolidate into a single source entry or use different URLs", refs[i].label, refs[j].label, refs[i].source.Source, refs[i].source.Ref, refs[j].source.Ref)
			}
		}
	}
	return nil
}

func IsGitSource(source string) bool {
	return strings.Contains(source, "://") || strings.HasPrefix(source, "git@") || strings.HasSuffix(source, ".git")
}
