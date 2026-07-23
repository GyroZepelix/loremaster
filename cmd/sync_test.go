package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/manifest"
	loresync "github.com/GyroZepelix/loremaster/internal/sync"
)

func TestReconcileRemovedProviders_SymlinkRemoval(t *testing.T) {
	projectDir := t.TempDir()
	for _, dir := range []string{".claude/skills", ".opencode/skills"} {
		skillsDir := filepath.Join(projectDir, dir)
		os.MkdirAll(skillsDir, 0755)
		targetDir := filepath.Join(t.TempDir(), "target", filepath.Base(dir))
		os.MkdirAll(targetDir, 0755)
		os.WriteFile(filepath.Join(targetDir, "workflow.md"), []byte("# test"), 0644)
		os.Symlink(targetDir, filepath.Join(skillsDir, "foo"))
	}

	items := map[string]manifest.Item{
		".claude/skills/foo":   {Path: ".claude/skills/foo", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory"},
		".opencode/skills/foo": {Path: ".opencode/skills/foo", Provider: "opencode", Resource: "skills", Mode: "soft", Kind: "directory"},
	}
	warnings, _ := reconcileRemovedProviderItems(items, config.ProviderList{"claude"}, projectDir, nil, "default", false)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, ok := items[".opencode/skills/foo"]; ok {
		t.Fatal("removed provider item remains owned")
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".opencode", "skills", "foo")); err == nil {
		t.Fatal("removed provider symlink still exists")
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "foo")); err != nil {
		t.Fatal("configured provider symlink was removed")
	}
}

func TestReconcileRemovedProviders_HardCopyModifiedPreserved(t *testing.T) {
	projectDir := t.TempDir()
	skillDir := filepath.Join(projectDir, ".opencode", "skills", "foo")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# original"), 0644)
	checksum, err := loresync.ComputeDirChecksum(skillDir)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# MODIFIED"), 0644)
	items := map[string]manifest.Item{
		".opencode/skills/foo": {Path: ".opencode/skills/foo", Provider: "opencode", Resource: "skills", Mode: "hard", Kind: "directory", Checksum: checksum},
	}
	warnings, _ := reconcileRemovedProviderItems(items, config.ProviderList{"claude"}, projectDir, nil, "default", false)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "local modifications") {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, ok := items[".opencode/skills/foo"]; !ok {
		t.Fatal("modified hard copy ownership was dropped")
	}
	if _, err := os.Stat(skillDir); err != nil {
		t.Fatal("modified hard copy was removed")
	}
}

func TestReconcileRemovedProviders_CodexPreservesUnmanagedSibling(t *testing.T) {
	projectDir := t.TempDir()
	skillsDir := filepath.Join(projectDir, ".agents", "skills")
	os.MkdirAll(skillsDir, 0755)
	targetDir := filepath.Join(t.TempDir(), "target")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "workflow.md"), []byte("# test"), 0644)
	os.Symlink(targetDir, filepath.Join(skillsDir, "foo"))
	manualDir := filepath.Join(skillsDir, "manual")
	os.MkdirAll(manualDir, 0755)
	os.WriteFile(filepath.Join(manualDir, "SKILL.md"), []byte("# manual"), 0644)

	items := map[string]manifest.Item{
		".agents/skills/foo": {Path: ".agents/skills/foo", Provider: "codex", Resource: "skills", Mode: "soft", Kind: "directory"},
	}
	warnings, _ := reconcileRemovedProviderItems(items, config.ProviderList{"claude"}, projectDir, nil, "default", false)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, err := os.Lstat(filepath.Join(skillsDir, "foo")); err == nil {
		t.Fatal("codex item still exists")
	}
	if _, err := os.Stat(manualDir); err != nil {
		t.Fatal("unmanaged sibling was removed")
	}
	if _, err := os.Stat(skillsDir); err != nil {
		t.Fatal("skills root was removed")
	}
}

func TestResolveProjectRoot_ProviderConfigDirs(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name      string
		configSub string
	}{
		{"root", "lore.yml"},
		{"claude", filepath.Join(".claude", "lore.yml")},
		{"opencode", filepath.Join(".opencode", "lore.yml")},
		{"pi", filepath.Join(".pi", "lore.yml")},
		{"pi agent", filepath.Join(".pi", "agent", "lore.yml")},
		{"agents", filepath.Join(".agents", "lore.yml")},
		{"codex", filepath.Join(".codex", "lore.yml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveProjectRoot(filepath.Join(root, tt.configSub))
			if got != root {
				t.Errorf("resolveProjectRoot = %q, want %q", got, root)
			}
		})
	}
}

func TestRunSync_DiscoversPiConfigFromHome(t *testing.T) {
	tests := []struct {
		name      string
		configDir string
		profile   string
	}{
		{"default in .pi", ".pi", ""},
		{"default in .pi agent", filepath.Join(".pi", "agent"), ""},
		{"profile in .pi", ".pi", "dev"},
		{"profile in .pi agent", filepath.Join(".pi", "agent"), "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			sourceDir := filepath.Join(t.TempDir(), "source")
			skillDir := filepath.Join(sourceDir, "foo")
			if err := os.MkdirAll(skillDir, 0755); err != nil {
				t.Fatalf("create skill dir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Foo\n"), 0644); err != nil {
				t.Fatalf("write skill: %v", err)
			}

			configName := "lore.yml"
			if tt.profile != "" {
				configName = fmt.Sprintf("lore-%s.yml", tt.profile)
			}
			configPath := filepath.Join(home, tt.configDir, configName)
			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				t.Fatalf("create config dir: %v", err)
			}
			configContent := fmt.Sprintf("provider: pi\nskills:\n  - source: %q\n    include: [foo]\n", sourceDir)
			if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			t.Setenv("HOME", home)
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

			oldWd, err := os.Getwd()
			if err != nil {
				t.Fatalf("get wd: %v", err)
			}
			oldProfile, oldPrune := profileFlag, pruneFlag
			t.Cleanup(func() {
				profileFlag = oldProfile
				pruneFlag = oldPrune
				if err := os.Chdir(oldWd); err != nil {
					t.Fatalf("restore wd: %v", err)
				}
			})

			if err := os.Chdir(home); err != nil {
				t.Fatalf("chdir home: %v", err)
			}
			profileFlag = tt.profile
			pruneFlag = false

			if err := runSync(nil, nil); err != nil {
				t.Fatalf("runSync: %v", err)
			}

			skillPath := filepath.Join(home, ".pi", "agent", "skills", "foo")
			if _, err := os.Lstat(skillPath); err != nil {
				t.Fatalf("expected synced skill at %s: %v", skillPath, err)
			}
		})
	}
}
