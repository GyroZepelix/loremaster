package provider

import "path/filepath"

type Codex struct{}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) ConfigRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".agents")
}

func (c *Codex) ResourceDir(projectRoot string, resource string, item string) string {
	return filepath.Join(c.ConfigRoot(projectRoot), resource, item)
}

func (c *Codex) SkillRoot(projectRoot string) string {
	return filepath.Join(c.ConfigRoot(projectRoot), "skills")
}

func (c *Codex) SkillDir(projectRoot string, skillName string) string {
	return c.ResourceDir(projectRoot, "skills", skillName)
}

func (c *Codex) ConfigDirs() []string { return []string{".agents", ".codex"} }

func (c *Codex) MarkerDirs() []string { return []string{".agents", ".codex"} }

func (c *Codex) DefaultConfigDir() string { return ".agents" }
