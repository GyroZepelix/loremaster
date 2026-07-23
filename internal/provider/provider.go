package provider

import (
	"fmt"
	"strings"
)

type Provider interface {
	Name() string
	ConfigRoot(projectRoot string) string
	ResourceDir(projectRoot string, resource string, item string) string
	SkillRoot(projectRoot string) string
	SkillDir(projectRoot string, skillName string) string
	ConfigDirs() []string
	MarkerDirs() []string
	DefaultConfigDir() string
}

var registry = map[string]Provider{
	"claude":   &Claude{},
	"opencode": &OpenCode{},
	"pi":       &Pi{},
	"codex":    &Codex{},
}

var order = []string{"claude", "opencode", "pi", "codex"}

func IsSupported(name string) bool {
	_, ok := registry[name]
	return ok
}

func Get(name string) (Provider, error) {
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q: supported providers are: %s", name, SupportedNames())
	}
	return p, nil
}

func All() []Provider {
	providers := make([]Provider, 0, len(order))
	for _, name := range order {
		providers = append(providers, registry[name])
	}
	return providers
}

func Names() []string {
	names := make([]string, len(order))
	copy(names, order)
	return names
}

func SupportedNames() string {
	return strings.Join(order, ", ")
}

func ConfigDirs() []string {
	var dirs []string
	seen := make(map[string]bool)
	for _, p := range All() {
		for _, dir := range p.ConfigDirs() {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
