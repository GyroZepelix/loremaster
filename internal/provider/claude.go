package provider

import "path/filepath"

type Claude struct{}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) SkillRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".claude", "skills")
}

func (c *Claude) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(c.SkillRoot(projectRoot), skillName)
}

func (c *Claude) ConfigDirs() []string { return []string{".claude"} }

func (c *Claude) MarkerDirs() []string { return []string{".claude"} }

func (c *Claude) DefaultConfigDir() string { return ".claude" }
