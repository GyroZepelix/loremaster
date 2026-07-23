package provider

import "path/filepath"

type OpenCode struct{}

func (o *OpenCode) Name() string { return "opencode" }

func (o *OpenCode) ConfigRoot(projectRoot string) string {
	return filepath.Join(projectRoot, ".opencode")
}

func (o *OpenCode) ResourceDir(projectRoot string, resource string, item string) string {
	return filepath.Join(o.ConfigRoot(projectRoot), resource, item)
}

func (o *OpenCode) SkillRoot(projectRoot string) string {
	return filepath.Join(o.ConfigRoot(projectRoot), "skills")
}

func (o *OpenCode) SkillDir(projectRoot string, skillName string) string {
	return o.ResourceDir(projectRoot, "skills", skillName)
}

func (o *OpenCode) ConfigDirs() []string { return []string{".opencode"} }

func (o *OpenCode) MarkerDirs() []string { return []string{".opencode"} }

func (o *OpenCode) DefaultConfigDir() string { return ".opencode" }
