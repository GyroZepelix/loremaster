package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/gitignore"
	"github.com/GyroZepelix/loremaster/internal/manifest"
	loresync "github.com/GyroZepelix/loremaster/internal/sync"
)

func TestRunSyncRetainsOwnershipWhenDesiredItemFails(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	prompt := filepath.Join(source, "review.md")
	os.WriteFile(prompt, []byte("review"), 0644)
	os.WriteFile(filepath.Join(project, "lore.yml"), []byte(fmt.Sprintf("provider: claude\nprompts:\n  - source: %q\n    include: [review.md]\n", source)), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false

	if err := runSync(nil, nil); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	os.Remove(prompt)
	if err := runSync(nil, nil); err == nil {
		t.Fatal("expected missing desired item error")
	}
	mf, err := manifest.Load(filepath.Join(project, ".lore-manifest.yml"))
	if err != nil || mf == nil {
		t.Fatalf("manifest: %v, %#v", err, mf)
	}
	items, _ := mf.GetProfileItems("default")
	if len(items) != 1 || items[0].Path != ".claude/prompts/review.md" {
		t.Fatalf("ownership was not retained: %#v", items)
	}
}

func TestPrunePreservesModifiedHardFileOwnership(t *testing.T) {
	project := t.TempDir()
	path := ".claude/prompts/review.md"
	absolute := filepath.Join(project, path)
	os.MkdirAll(filepath.Dir(absolute), 0755)
	os.WriteFile(absolute, []byte("original"), 0644)
	checksum, err := loresync.ComputeFileChecksum(absolute)
	if err != nil {
		t.Fatal(err)
	}
	item := manifest.Item{Path: path, Provider: "claude", Resource: "prompts", Mode: "hard", Kind: "file", Checksum: checksum}
	mf := manifest.New()
	mf.SetProfileItems("orphan", []manifest.Item{item})
	manifestPath := filepath.Join(project, ".lore-manifest.yml")
	gitignorePath := filepath.Join(project, ".gitignore")
	if err := manifest.Save(manifestPath, mf); err != nil {
		t.Fatal(err)
	}
	gitignore.EnsureEntries(gitignorePath, []string{path, ".lore-manifest.yml"})
	os.WriteFile(absolute, []byte("modified"), 0644)

	if err := pruneOrphaned(mf, project, manifestPath, gitignorePath); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if content, _ := os.ReadFile(absolute); string(content) != "modified" {
		t.Fatalf("modified hard file changed: %q", content)
	}
	items, ok := mf.GetProfileItems("orphan")
	if !ok || len(items) != 1 {
		t.Fatalf("modified item ownership was removed: %#v", items)
	}
	managed, _ := gitignore.ManagedEntries(gitignorePath)
	found := false
	for _, entry := range managed {
		found = found || entry == path
	}
	if !found {
		t.Fatal("modified item gitignore entry was removed")
	}
}

func TestPruneRollsBackWhenManifestSaveFails(t *testing.T) {
	project := t.TempDir()
	targetDir := t.TempDir()
	target := filepath.Join(project, ".claude", "skills", "review")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.Symlink(targetDir, target)
	item := manifest.Item{Path: ".claude/skills/review", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: targetDir}
	mf := manifest.New()
	mf.SetProfileItems("orphan", []manifest.Item{item})
	manifestPath := filepath.Join(project, ".lore-manifest.yml")
	os.Mkdir(manifestPath, 0755)
	if err := pruneOrphaned(mf, project, manifestPath, filepath.Join(project, ".gitignore")); err == nil {
		t.Fatal("expected manifest save failure")
	}
	if symlinkTarget, err := os.Readlink(target); err != nil || symlinkTarget != targetDir {
		t.Fatalf("prune did not roll back: target=%q err=%v", symlinkTarget, err)
	}
}

func TestRunSyncRemovesDeletedResource(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "review.md"), []byte("review"), 0644)
	os.WriteFile(filepath.Join(source, "deploy.md"), []byte("deploy"), 0644)
	configPath := filepath.Join(project, "lore.yml")
	first := fmt.Sprintf("provider: claude\nprompts:\n  - source: %q\n    include: [review.md]\ncommands:\n  - source: %q\n    include: [deploy.md]\n", source, source)
	os.WriteFile(configPath, []byte(first), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false
	if err := runSync(nil, nil); err != nil {
		t.Fatal(err)
	}

	second := fmt.Sprintf("provider: claude\ncommands:\n  - source: %q\n    include: [deploy.md]\n", source)
	os.WriteFile(configPath, []byte(second), 0644)
	if err := runSync(nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "prompts", "review.md")); !os.IsNotExist(err) {
		t.Fatalf("stale prompt still exists: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "commands", "deploy.md")); err != nil {
		t.Fatalf("desired command missing: %v", err)
	}
}

func TestModifiedHardFilesSurviveResourceAndProviderRemoval(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "review.md"), []byte("review"), 0644)
	os.WriteFile(filepath.Join(source, "deploy.md"), []byte("deploy"), 0644)
	configPath := filepath.Join(project, "lore.yml")
	first := fmt.Sprintf("provider: [claude, pi]\nprompts:\n  - source: %q\n    include: [review.md]\n    type: hard\n", source)
	os.WriteFile(configPath, []byte(first), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false
	if err := runSync(nil, nil); err != nil {
		t.Fatal(err)
	}

	claudePath := filepath.Join(project, ".claude", "prompts", "review.md")
	piPath := filepath.Join(project, ".pi", "prompts", "review.md")
	os.WriteFile(claudePath, []byte("claude modified"), 0644)
	os.WriteFile(piPath, []byte("pi modified"), 0644)
	second := fmt.Sprintf("provider: claude\ncommands:\n  - source: %q\n    include: [deploy.md]\n", source)
	os.WriteFile(configPath, []byte(second), 0644)
	if err := runSync(nil, nil); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	for path, want := range map[string]string{claudePath: "claude modified", piPath: "pi modified"} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != want {
			t.Errorf("preserved file %s = %q, err = %v", path, content, err)
		}
	}
	mf, _ := manifest.Load(filepath.Join(project, ".lore-manifest.yml"))
	items, _ := mf.GetProfileItems("default")
	if len(items) != 3 {
		t.Fatalf("manifest items = %#v, want two retained prompts plus command", items)
	}
	managed, _ := gitignore.ManagedEntries(filepath.Join(project, ".gitignore"))
	set := make(map[string]bool)
	for _, entry := range managed {
		set[entry] = true
	}
	if !set[".claude/prompts/review.md"] || !set[".pi/prompts/review.md"] {
		t.Fatalf("retained paths missing from gitignore: %v", managed)
	}
}

func TestReconcileGitignoreKeepsSharedOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	mf := manifest.New()
	item := manifest.Item{Path: ".claude/skills/review", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory"}
	mf.SetProfileItems("dev", []manifest.Item{item})
	mf.SetProfileItems("staging", []manifest.Item{item})
	if err := reconcileGitignore(mf, path); err != nil {
		t.Fatal(err)
	}
	mf.RemoveProfile("dev")
	if err := reconcileGitignore(mf, path); err != nil {
		t.Fatal(err)
	}
	managed, _ := gitignore.ManagedEntries(path)
	found := false
	for _, entry := range managed {
		found = found || entry == item.Path
	}
	if !found {
		t.Fatal("shared path was unignored while staging still owned it")
	}
	mf.RemoveProfile("staging")
	if err := reconcileGitignore(mf, path); err != nil {
		t.Fatal(err)
	}
	managed, _ = gitignore.ManagedEntries(path)
	for _, entry := range managed {
		if entry == item.Path {
			t.Fatal("unowned path remains in gitignore")
		}
	}
}

func TestRemovedProviderReleasesSharedOwnershipWithoutDeleting(t *testing.T) {
	project := t.TempDir()
	targetDir := t.TempDir()
	target := filepath.Join(project, ".pi", "prompts", "review.md")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.Symlink(filepath.Join(targetDir, "review.md"), target)
	item := manifest.Item{Path: ".pi/prompts/review.md", Provider: "pi", Resource: "prompts", Mode: "soft", Kind: "file"}
	mf := manifest.New()
	mf.SetProfileItems("default", []manifest.Item{item})
	mf.SetProfileItems("staging", []manifest.Item{item})
	items := map[string]manifest.Item{item.Path: item}
	warnings, _ := reconcileRemovedProviderItems(items, config.ProviderList{"claude"}, project, mf, "default", false)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, ok := items[item.Path]; ok {
		t.Fatal("active profile ownership was not released")
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("shared removed-provider target was deleted: %v", err)
	}
}

func TestCorruptManifestDoesNotRetroactivelyClaimDestinations(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	skill := filepath.Join(source, "review")
	os.MkdirAll(skill, 0755)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("new"), 0644)
	xdg := t.TempDir()
	legacy := filepath.Join(xdg, "loremaster", "legacy-review")
	os.MkdirAll(legacy, 0755)
	os.WriteFile(filepath.Join(legacy, "SKILL.md"), []byte("legacy"), 0644)
	target := filepath.Join(project, ".claude", "skills", "review")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.Symlink(legacy, target)
	os.WriteFile(filepath.Join(project, ".lore-manifest.yml"), []byte("{{corrupt"), 0644)
	os.WriteFile(filepath.Join(project, "lore.yml"), []byte(fmt.Sprintf("provider: claude\nskills:\n  - source: %q\n    include: [review]\n", source)), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", xdg)
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false
	if err := runSync(nil, nil); err == nil {
		t.Fatal("expected unmanaged destination error")
	}
	if symlinkTarget, err := os.Readlink(target); err != nil || symlinkTarget != legacy {
		t.Fatalf("corrupt manifest destination was claimed: target=%q err=%v", symlinkTarget, err)
	}
}

func TestRunSyncMigratesVersionOneManifest(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	skill := filepath.Join(source, "review")
	os.MkdirAll(skill, 0755)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("review"), 0644)
	target := filepath.Join(project, ".claude", "skills", "review")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.Symlink(skill, target)
	os.WriteFile(filepath.Join(project, ".lore-manifest.yml"), []byte("version: 1\nprofiles:\n  default:\n    - .claude/skills/review\n"), 0644)
	os.WriteFile(filepath.Join(project, "lore.yml"), []byte(fmt.Sprintf("provider: claude\nskills:\n  - source: %q\n    include: [review]\n", source)), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false
	if err := runSync(nil, nil); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	mf, err := manifest.Load(filepath.Join(project, ".lore-manifest.yml"))
	if err != nil || mf.Version != manifest.CurrentVersion {
		t.Fatalf("manifest version = %v, err = %v", mf, err)
	}
	items, _ := mf.GetProfileItems("default")
	if len(items) != 1 || items[0].Mode != "soft" || items[0].Kind != "directory" || items[0].Legacy {
		t.Fatalf("migrated item = %#v", items)
	}
}
