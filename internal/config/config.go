package config

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/GyroZepelix/loremaster/internal/provider"
	"gopkg.in/yaml.v3"
)

// scpPortPattern matches SCP-style SSH URLs where a port number is mistakenly
// embedded in the path (e.g. git@host:2222/path). The SCP format doesn't
// support ports — the user should use ssh://git@host:2222/path instead.
var scpPortPattern = regexp.MustCompile(`^git@([^:]+):(\d+)/`)

// ProviderList is a []string that accepts both scalar and sequence YAML values.
// This allows `provider: claude` and `provider: [claude, opencode]` in lore.yml.
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
	Providers ProviderList  `yaml:"provider"`
	Skills    []SkillSource `yaml:"skills"`
}

type SkillSource struct {
	Source         string         `yaml:"source"`
	Ref            string         `yaml:"ref,omitempty"`
	Include        []string       `yaml:"include"`
	Type           string         `yaml:"type,omitempty"`
	ParsedIncludes []IncludeEntry `yaml:"-"`
}

func Parse(r io.Reader) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("missing required field: provider")
	}
	seen := make(map[string]bool)
	for _, p := range cfg.Providers {
		if !provider.IsSupported(p) {
			return nil, fmt.Errorf("invalid provider %q: must be one of: %s", p, provider.SupportedNames())
		}
		if seen[p] {
			return nil, fmt.Errorf("duplicate provider %q", p)
		}
		seen[p] = true
	}
	if len(cfg.Skills) == 0 {
		return nil, fmt.Errorf("skills list must have at least one entry (edit lore.yml to add skill sources)")
	}

	for i := range cfg.Skills {
		s := &cfg.Skills[i]
		if s.Source == "" {
			return nil, fmt.Errorf("skills[%d]: missing required field: source", i)
		}
		if len(s.Include) == 0 {
			return nil, fmt.Errorf("skills[%d]: missing required field: include", i)
		}
		for _, raw := range s.Include {
			entry, err := ParseIncludeEntry(raw)
			if err != nil {
				return nil, fmt.Errorf("skills[%d]: %w", i, err)
			}
			s.ParsedIncludes = append(s.ParsedIncludes, entry)
		}
		if err := ValidateOverlaps(s.ParsedIncludes); err != nil {
			return nil, fmt.Errorf("skills[%d]: %w", i, err)
		}
		if m := scpPortPattern.FindStringSubmatch(s.Source); m != nil {
			// Extract the remaining path after the port match
			rest := strings.TrimPrefix(s.Source, m[0])
			return nil, fmt.Errorf("skills[%d]: source %q looks like it contains a port number — SCP-style URLs (git@host:path) don't support ports; use ssh://git@%s:%s/%s instead", i, s.Source, m[1], m[2], rest)
		}
		if s.Type == "" {
			s.Type = "soft"
		}
		if s.Type != "soft" && s.Type != "hard" {
			return nil, fmt.Errorf("skills[%d]: invalid type %q: must be \"soft\" or \"hard\"", i, s.Type)
		}
	}

	// Check for git sources with same URL but different refs
	for i := 0; i < len(cfg.Skills); i++ {
		if !IsGitSource(cfg.Skills[i].Source) {
			continue
		}
		for j := i + 1; j < len(cfg.Skills); j++ {
			if !IsGitSource(cfg.Skills[j].Source) {
				continue
			}
			if cfg.Skills[i].Source == cfg.Skills[j].Source && cfg.Skills[i].Ref != cfg.Skills[j].Ref {
				return nil, fmt.Errorf("skills[%d] and skills[%d] reference the same source %q with different refs (%q vs %q) — consolidate into a single source entry or use different URLs", i, j, cfg.Skills[i].Source, cfg.Skills[i].Ref, cfg.Skills[j].Ref)
			}
		}
	}

	return &cfg, nil
}

func IsGitSource(source string) bool {
	if strings.Contains(source, "://") {
		return true
	}
	if strings.HasPrefix(source, "git@") {
		return true
	}
	if strings.HasSuffix(source, ".git") {
		return true
	}
	return false
}
