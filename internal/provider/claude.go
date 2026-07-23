package provider

import "path/filepath"

type Claude struct{}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) ConfigRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".claude")
}

func (c *Claude) ResourceDir(projectRoot string, resource string, item string) string {
	return filepath.Join(c.ConfigRoot(projectRoot), resource, item)
}

func (c *Claude) SkillRoot(projectRoot string) string {
	return filepath.Join(c.ConfigRoot(projectRoot), "skills")
}

func (c *Claude) SkillDir(projectRoot string, skillName string) string {
	return c.ResourceDir(projectRoot, "skills", skillName)
}

func (c *Claude) ConfigDirs() []string { return []string{".claude"} }

func (c *Claude) MarkerDirs() []string { return []string{".claude"} }

func (c *Claude) DefaultConfigDir() string { return ".claude" }
