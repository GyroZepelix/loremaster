package provider

import "path/filepath"

type OpenCode struct{}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) SkillRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".opencode", "skills")
}

func (o *OpenCode) SkillDir(projectRoot string, skillName string) string {
	return filepath.Join(o.SkillRoot(projectRoot), skillName)
}

func (o *OpenCode) ConfigDirs() []string { return []string{".opencode"} }

func (o *OpenCode) MarkerDirs() []string { return []string{".opencode"} }

func (o *OpenCode) DefaultConfigDir() string { return ".opencode" }
