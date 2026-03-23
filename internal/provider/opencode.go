package provider

import "path/filepath"

type OpenCode struct{}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(projectRoot, ".opencode", "skills", skillName)
}

func (o *OpenCode) MarkerDir() string { return ".opencode" }
