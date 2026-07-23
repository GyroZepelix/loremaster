package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/manifest"
	"github.com/GyroZepelix/loremaster/internal/provider"
)

func TestSyncResourcesFilesAndDirectories(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	os.WriteFile(filepath.Join(source, "review.md"), []byte("review"), 0644)
	os.WriteFile(filepath.Join(source, "tool.json"), []byte(`{"tool":true}`), 0644)
	os.MkdirAll(filepath.Join(source, "templates", "nested"), 0755)
	os.WriteFile(filepath.Join(source, "templates", ".hidden"), []byte("hidden"), 0644)
	os.WriteFile(filepath.Join(source, "templates", "nested", "body.md"), []byte("body"), 0644)

	cfg, err := config.Parse(strings.NewReader(fmt.Sprintf(`
provider: claude
prompts:
  - source: %q
    include: [review.md, templates]
hooks/tools:
  - source: %q
    include: [tool.json:check.json]
`, source, source)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	baseDirs, fetchErrs := FetchSources(&mockFetcher{}, cfg.AllSources())
	if len(fetchErrs) != 0 {
		t.Fatalf("FetchSources: %v", fetchErrs)
	}
	prov, _ := provider.Get("claude")
	syncer := &Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}
	result, err := syncer.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 3 {
		t.Fatalf("Synced = %d, want 3", result.Synced)
	}

	checks := map[string]string{
		filepath.Join(project, ".claude", "prompts", "review.md"):                      "review",
		filepath.Join(project, ".claude", "prompts", "templates", ".hidden"):           "hidden",
		filepath.Join(project, ".claude", "prompts", "templates", "nested", "body.md"): "body",
		filepath.Join(project, ".claude", "hooks", "tools", "check.json"):              `{"tool":true}`,
	}
	for path, want := range checks {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != want {
			t.Errorf("%s = %q, err = %v, want %q", path, content, err, want)
		}
	}
}

func TestSyncRejectsSymlinkedDestinationParent(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	source := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	os.WriteFile(filepath.Join(source, "review.md"), []byte("review"), 0644)
	os.Symlink(outside, filepath.Join(project, ".claude"))
	cfg, err := config.Parse(strings.NewReader(fmt.Sprintf("provider: claude\nprompts:\n  - source: %q\n    include: [review.md]\n", source)))
	if err != nil {
		t.Fatal(err)
	}
	prov, _ := provider.Get("claude")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(cfg, map[string]string{source: source})
	if err == nil || !strings.Contains(strings.Join(result.Errors, "\n"), "is a symlink") {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "prompts", "review.md")); !os.IsNotExist(err) {
		t.Fatalf("item escaped through symlinked parent: %v", err)
	}
}

func TestRemoveManagedItemRejectsSymlinkedDestinationParent(t *testing.T) {
	project := t.TempDir()
	outside := t.TempDir()
	os.MkdirAll(filepath.Join(outside, "prompts"), 0755)
	outsideFile := filepath.Join(outside, "prompts", "review.md")
	os.WriteFile(outsideFile, []byte("outside"), 0644)
	os.Symlink(outside, filepath.Join(project, ".claude"))
	checksum, _ := ComputeFileChecksum(outsideFile)
	item := manifest.Item{Path: ".claude/prompts/review.md", Provider: "claude", Resource: "prompts", Mode: "hard", Kind: "file", Checksum: checksum}
	if err := RemoveManagedItem(project, item); err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("error = %v", err)
	}
	if content, _ := os.ReadFile(outsideFile); string(content) != "outside" {
		t.Fatalf("outside file changed: %q", content)
	}
}

func TestSyncSkillsRemainDirectoryOnly(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	os.WriteFile(filepath.Join(source, "skill.md"), []byte("skill"), 0644)
	cfg, err := config.Parse(strings.NewReader(fmt.Sprintf("provider: claude\nskills:\n  - source: %q\n    include: [skill.md]\n", source)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	prov, _ := provider.Get("claude")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project}).Sync(cfg, map[string]string{source: source})
	if err == nil || result.Synced != 0 || !strings.Contains(strings.Join(result.Errors, "\n"), "expected skill directory") {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}

func TestSyncRefusesCrossProfileOwnership(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	os.WriteFile(filepath.Join(source, "review.md"), []byte("new"), 0644)
	target := filepath.Join(project, ".claude", "prompts", "review.md")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.WriteFile(target, []byte("staging"), 0644)

	cfg, err := config.Parse(strings.NewReader(fmt.Sprintf("provider: claude\nprompts:\n  - source: %q\n    include: [review.md]\n", source)))
	if err != nil {
		t.Fatal(err)
	}
	mf := manifest.New()
	mf.SetProfileItems("staging", []manifest.Item{{Path: ".claude/prompts/review.md", Provider: "claude", Resource: "prompts", Mode: "hard", Kind: "file", Checksum: "unused"}})
	prov, _ := provider.Get("claude")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "dev", Manifest: mf}).Sync(cfg, map[string]string{source: source})
	if err == nil || !strings.Contains(strings.Join(result.Errors, "\n"), "owned by profile") {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "staging" {
		t.Fatalf("cross-profile target changed: %q", content)
	}
}

func TestStaleSharedOwnershipReleasesWithoutDeleting(t *testing.T) {
	project := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	targetDir := t.TempDir()
	target := filepath.Join(project, ".claude", "skills", "review")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.Symlink(targetDir, target)
	item := manifest.Item{Path: ".claude/skills/review", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory"}
	mf := manifest.New()
	mf.SetProfileItems("dev", []manifest.Item{item})
	mf.SetProfileItems("staging", []manifest.Item{item})
	prov, _ := provider.Get("claude")
	cfg := &config.Config{Providers: config.ProviderList{"claude"}, Skills: []config.SkillSource{}}
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "dev", Manifest: mf}).Sync(cfg, nil)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.Removed) != 1 || result.Removed[0] != item.Path {
		t.Fatalf("released paths = %v", result.Removed)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("shared target was deleted: %v", err)
	}
}

func TestRemoveManagedItemRejectsResourceRoot(t *testing.T) {
	project := t.TempDir()
	root := filepath.Join(project, ".claude", "prompts")
	os.MkdirAll(root, 0755)
	checksum, err := ComputeDirChecksum(root)
	if err != nil {
		t.Fatal(err)
	}
	item := manifest.Item{Path: ".claude/prompts", Provider: "claude", Resource: "prompts", Mode: "hard", Kind: "directory", Checksum: checksum}
	if err := RemoveManagedItem(project, item); err == nil || !strings.Contains(err.Error(), "outside its provider resource root") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("resource root was deleted: %v", err)
	}
}

func TestInspectLegacyHardDirectoryUpgradesChecksum(t *testing.T) {
	project := t.TempDir()
	path := ".claude/skills/review"
	absolute := filepath.Join(project, path)
	os.MkdirAll(absolute, 0755)
	os.WriteFile(filepath.Join(absolute, "SKILL.md"), []byte("review"), 0644)
	legacyChecksum, err := computeLegacyDirChecksum(absolute)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(absolute, ".lore-checksum"), []byte(legacyChecksum), 0644)
	item, err := InspectLegacyItem(project, manifest.Item{Path: path, Provider: "claude", Resource: "skills", Legacy: true})
	if err != nil {
		t.Fatalf("InspectLegacyItem: %v", err)
	}
	if item.ChecksumVersion != currentChecksumVersion || item.Checksum == legacyChecksum || item.Legacy {
		t.Fatalf("upgraded item = %#v", item)
	}
	os.Mkdir(filepath.Join(absolute, "local-empty-dir"), 0755)
	if err := RemoveManagedItem(project, item); err == nil || !strings.Contains(err.Error(), "local modifications") {
		t.Fatalf("error = %v", err)
	}
}

func TestInspectLegacyHardDirectoryRejectsChecksumBlindSpots(t *testing.T) {
	project := t.TempDir()
	path := ".claude/skills/review"
	absolute := filepath.Join(project, path)
	os.MkdirAll(filepath.Join(absolute, "empty"), 0755)
	os.WriteFile(filepath.Join(absolute, "SKILL.md"), []byte("review"), 0644)
	legacyChecksum, _ := computeLegacyDirChecksum(absolute)
	os.WriteFile(filepath.Join(absolute, ".lore-checksum"), []byte(legacyChecksum), 0644)
	_, err := InspectLegacyItem(project, manifest.Item{Path: path, Provider: "claude", Resource: "skills", Legacy: true})
	if err == nil || !strings.Contains(err.Error(), "cannot be verified") {
		t.Fatalf("error = %v", err)
	}
}

func TestFetchSourcesDeduplicatesAcrossResources(t *testing.T) {
	source := "git@github.com:user/resources.git"
	fetcher := &mockFetcher{}
	_, errs := FetchSources(fetcher, []config.SkillSource{{Source: source}, {Source: source}})
	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if fetcher.cloneCount != 1 {
		t.Fatalf("clone count = %d, want 1", fetcher.cloneCount)
	}
}
