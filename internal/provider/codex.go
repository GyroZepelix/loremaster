package provider

import "path/filepath"

type Codex struct{}

func (c *Codex) Name() string { return "codex" }

func (c *Codex) SkillRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".agents", "skills")
}

func (c *Codex) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(c.SkillRoot(projectRoot), skillName)
}

func (c *Codex) ConfigDirs() []string { return []string{".agents", ".codex"} }

func (c *Codex) MarkerDirs() []string { return []string{".agents", ".codex"} }

func (c *Codex) DefaultConfigDir() string { return ".agents" }
