package provider

import (
	"os"
	"path/filepath"
)

type Pi struct{}

func (p *Pi) Name() string { return "pi" }

func (p *Pi) ConfigRoot(projectRoot string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && samePath(projectRoot, home) {
		return filepath.Join(projectRoot, ".pi", "agent")
	}
	return filepath.Join(projectRoot, ".pi")
}

func (p *Pi) ResourceDir(projectRoot string, resource string, item string) string {
	return filepath.Join(p.ConfigRoot(projectRoot), resource, item)
}

func (p *Pi) SkillRoot(projectRoot string) string {
	return filepath.Join(p.ConfigRoot(projectRoot), "skills")
}

func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	realA, errA := filepath.EvalSymlinks(a)
	realB, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && filepath.Clean(realA) == filepath.Clean(realB)
}

func (p *Pi) SkillDir(projectRoot string, skillName string) string {
	return p.ResourceDir(projectRoot, "skills", skillName)
}

func (p *Pi) ConfigDirs() []string {
	return []string{".pi", filepath.Join(".pi", "agent")}
}

func (p *Pi) MarkerDirs() []string {
	return []string{".pi", filepath.Join(".pi", "agent")}
}

func (p *Pi) DefaultConfigDir() string { return ".pi" }
