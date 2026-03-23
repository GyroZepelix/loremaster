package provider

import "fmt"

type Provider interface {
	Name() string
	SkillDir(projectRoot string, skillName string) string
	MarkerDir() string
}

var registry = map[string]Provider{
	"claude":   &Claude{},
	"opencode": &OpenCode{},
}

func Get(name string) (Provider, error) {
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q: supported providers are: claude, opencode", name)
	}
	return p, nil
}

func All() []Provider {
	return []Provider{
		registry["claude"],
		registry["opencode"],
	}
}
