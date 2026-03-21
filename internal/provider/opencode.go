package provider

import (
	"os"
	"path/filepath"
)

type OpenCode struct{}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(projectRoot, ".opencode", "skills", skillName)
}

func (o *OpenCode) GlobalSkillDir(skillName string) string {
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return filepath.Join(".config", "opencode", "skills", skillName)
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "opencode", "skills", skillName)
}

func (o *OpenCode) MarkerDir() string { return ".opencode" }
