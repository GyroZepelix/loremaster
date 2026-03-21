package provider

import (
	"os"
	"path/filepath"
)

type Claude struct{}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(projectRoot, ".claude", "skills", skillName)
}

func (c *Claude) GlobalSkillDir(skillName string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".claude", "skills", skillName)
	}
	return filepath.Join(home, ".claude", "skills", skillName)
}

func (c *Claude) MarkerDir() string { return ".claude" }
