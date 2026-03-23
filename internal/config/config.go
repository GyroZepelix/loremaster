package config

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// scpPortPattern matches SCP-style SSH URLs where a port number is mistakenly
// embedded in the path (e.g. git@host:2222/path). The SCP format doesn't
// support ports — the user should use ssh://git@host:2222/path instead.
var scpPortPattern = regexp.MustCompile(`^git@([^:]+):(\d+)/`)

type Config struct {
	Provider string        `yaml:"provider"`
	Skills   []SkillSource `yaml:"skills"`
}

type SkillSource struct {
	Source  string   `yaml:"source"`
	Ref     string   `yaml:"ref,omitempty"`
	Include []string `yaml:"include"`
	Type    string   `yaml:"type,omitempty"`
}

var validProviders = map[string]bool{
	"claude":   true,
	"opencode": true,
}

func Parse(r io.Reader) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if cfg.Provider == "" {
		return nil, fmt.Errorf("missing required field: provider")
	}
	if !validProviders[cfg.Provider] {
		return nil, fmt.Errorf("invalid provider %q: must be one of: claude, opencode", cfg.Provider)
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
		for _, skill := range s.Include {
			if err := validateSkillName(skill); err != nil {
				return nil, fmt.Errorf("skills[%d]: %w", i, err)
			}
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

	return &cfg, nil
}

func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("invalid skill name: must not be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid skill name %q: must be a simple directory name", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid skill name %q: must not contain path separators", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid skill name %q: must not contain '..'", name)
	}
	return nil
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
