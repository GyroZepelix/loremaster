package provider

import (
	"os"
	"path/filepath"
)

type Pi struct{}

func (p *Pi) Name() string { return "pi" }

func (p *Pi) SkillRoot(projectRoot string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && filepath.Clean(projectRoot) == filepath.Clean(home) {
		return filepath.Join(projectRoot, ".pi", "agent", "skills")
	}
	return filepath.Join(projectRoot, ".pi", "skills")
}

func (p *Pi) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(p.SkillRoot(projectRoot), skillName)
}

func (p *Pi) ConfigDirs() []string {
	return []string{".pi", filepath.Join(".pi", "agent")}
}

func (p *Pi) MarkerDirs() []string {
	return []string{".pi", filepath.Join(".pi", "agent")}
}

func (p *Pi) DefaultConfigDir() string { return ".pi" }
