package provider

import "path/filepath"

type Claude struct{}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(projectRoot, ".claude", "skills", skillName)
}

func (c *Claude) MarkerDir() string { return ".claude" }
