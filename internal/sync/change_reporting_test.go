package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/config"
	loregit "github.com/GyroZepelix/loremaster/internal/git"
	"github.com/GyroZepelix/loremaster/internal/manifest"
	"github.com/GyroZepelix/loremaster/internal/provider"
)

func TestSyncReportsSemanticItemChanges(t *testing.T) {
	t.Run("soft git update and no-op", func(t *testing.T) {
		project := t.TempDir()
		sourceDir := t.TempDir()
		skillDir := filepath.Join(sourceDir, "review")
		if err := os.Mkdir(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		workflow := filepath.Join(skillDir, "workflow.md")
		if err := os.WriteFile(workflow, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		source := "git@example.com:resources.git"
		cfg := skillChangeConfig(source, "soft")
		prov, _ := provider.Get("claude")
		mf := manifest.New()

		first := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, nil)
		assertItemChanges(t, first.ItemChanges, ItemChange{Status: ItemAdded, Path: ".claude/skills/review"})
		mf.SetProfileItems("default", first.Items)

		second := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, nil)
		assertItemChanges(t, second.ItemChanges)
		mf.SetProfileItems("default", second.Items)

		if err := os.WriteFile(workflow, []byte("new"), 0644); err != nil {
			t.Fatal(err)
		}
		inside := map[string]loregit.RepositoryUpdate{source: {ChangedPaths: []string{"review/workflow.md"}}}
		third := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, inside)
		assertItemChanges(t, third.ItemChanges, ItemChange{Status: ItemUpdated, Path: ".claude/skills/review"})
		mf.SetProfileItems("default", third.Items)

		outside := map[string]loregit.RepositoryUpdate{source: {ChangedPaths: []string{"unrelated.md"}}}
		fourth := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, outside)
		assertItemChanges(t, fourth.ItemChanges)
	})

	t.Run("hard content update", func(t *testing.T) {
		project := t.TempDir()
		sourceDir := t.TempDir()
		skillDir := filepath.Join(sourceDir, "review")
		if err := os.Mkdir(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		workflow := filepath.Join(skillDir, "workflow.md")
		if err := os.WriteFile(workflow, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		source := sourceDir
		cfg := skillChangeConfig(source, "hard")
		prov, _ := provider.Get("claude")
		mf := manifest.New()

		first := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, nil)
		assertItemChanges(t, first.ItemChanges, ItemChange{Status: ItemAdded, Path: ".claude/skills/review"})
		mf.SetProfileItems("default", first.Items)
		if err := os.WriteFile(workflow, []byte("new"), 0644); err != nil {
			t.Fatal(err)
		}
		second := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, nil)
		assertItemChanges(t, second.ItemChanges, ItemChange{Status: ItemUpdated, Path: ".claude/skills/review"})
	})

	t.Run("hard ignored symlink change", func(t *testing.T) {
		project := t.TempDir()
		sourceDir := t.TempDir()
		skillDir := filepath.Join(sourceDir, "review")
		if err := os.Mkdir(skillDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "target-a"), []byte("a"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "target-b"), []byte("b"), 0644); err != nil {
			t.Fatal(err)
		}
		ignored := filepath.Join(skillDir, "ignored")
		if err := os.Symlink("target-a", ignored); err != nil {
			t.Fatal(err)
		}
		source := "git@example.com:resources.git"
		cfg := skillChangeConfig(source, "hard")
		prov, _ := provider.Get("claude")
		mf := manifest.New()

		first := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, nil)
		mf.SetProfileItems("default", first.Items)
		if err := os.Remove(ignored); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target-b", ignored); err != nil {
			t.Fatal(err)
		}
		updates := map[string]loregit.RepositoryUpdate{source: {ChangedPaths: []string{"review/ignored"}}}
		second := runChangeSync(t, project, prov, mf, cfg, source, sourceDir, updates)
		assertItemChanges(t, second.ItemChanges)
		if _, err := os.Lstat(filepath.Join(project, ".claude", "skills", "review", "ignored")); !os.IsNotExist(err) {
			t.Fatalf("ignored symlink was copied: %v", err)
		}
	})
}

func TestSourceIncludeChanged(t *testing.T) {
	update := loregit.RepositoryUpdate{ChangedPaths: []string{"skills/review/workflow.md", "prompts/old.md", "prompts/new.md"}}
	if !sourceIncludeChanged(update, "skills/review", true) {
		t.Fatal("directory descendant was not matched")
	}
	if sourceIncludeChanged(update, "skills/review", false) {
		t.Fatal("file include matched a descendant")
	}
	if !sourceIncludeChanged(update, "prompts/old.md", false) || !sourceIncludeChanged(update, "prompts/new.md", false) {
		t.Fatal("rename sides were not matched")
	}
	if sourceIncludeChanged(update, "skills/other", true) {
		t.Fatal("unrelated directory matched")
	}
}

func TestSyncReportsOnlyActualDeletion(t *testing.T) {
	project := t.TempDir()
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "review")
	if err := os.Mkdir(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	prov, _ := provider.Get("claude")
	cfg := skillChangeConfig(sourceDir, "soft")
	mf := manifest.New()
	first := runChangeSync(t, project, prov, mf, cfg, sourceDir, sourceDir, nil)
	mf.SetProfileItems("default", first.Items)

	empty := &config.Config{Providers: config.ProviderList{"claude"}}
	removed := runChangeSync(t, project, prov, mf, empty, sourceDir, sourceDir, nil)
	assertItemChanges(t, removed.ItemChanges, ItemChange{Status: ItemDeleted, Path: ".claude/skills/review"})

	absentProject := t.TempDir()
	absentMF := manifest.New()
	absentFirst := runChangeSync(t, absentProject, prov, absentMF, cfg, sourceDir, sourceDir, nil)
	absentMF.SetProfileItems("default", absentFirst.Items)
	if err := os.Remove(filepath.Join(absentProject, ".claude", "skills", "review")); err != nil {
		t.Fatal(err)
	}
	absent := runChangeSync(t, absentProject, prov, absentMF, empty, sourceDir, sourceDir, nil)
	assertItemChanges(t, absent.ItemChanges)

	sharedProject := t.TempDir()
	sharedMF := manifest.New()
	sharedFirst := runChangeSync(t, sharedProject, prov, sharedMF, cfg, sourceDir, sourceDir, nil)
	sharedMF.SetProfileItems("default", sharedFirst.Items)
	sharedMF.SetProfileItems("other", sharedFirst.Items)
	shared := runChangeSync(t, sharedProject, prov, sharedMF, empty, sourceDir, sourceDir, nil)
	assertItemChanges(t, shared.ItemChanges)
	if _, err := os.Lstat(filepath.Join(sharedProject, ".claude", "skills", "review")); err != nil {
		t.Fatalf("shared item was removed: %v", err)
	}
}

func skillChangeConfig(source string, mode string) *config.Config {
	return &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{{
			Source:         source,
			Type:           mode,
			ParsedIncludes: []config.IncludeEntry{{Src: "review", Dst: "review"}},
		}},
	}
}

func runChangeSync(t *testing.T, project string, prov provider.Provider, mf *manifest.Manifest, cfg *config.Config, source string, sourceDir string, updates map[string]loregit.RepositoryUpdate) *SyncResult {
	t.Helper()
	result, err := (&Syncer{
		Provider:      prov,
		ProjectRoot:   project,
		ProfileName:   "default",
		Manifest:      mf,
		SourceUpdates: updates,
		Transactional: true,
	}).Sync(cfg, map[string]string{source: sourceDir})
	if err != nil {
		t.Fatalf("sync: %v, result=%#v", err, result)
	}
	if commitErrors := CommitChanges(result.Changes); len(commitErrors) != 0 {
		t.Fatalf("commit changes: %v", commitErrors)
	}
	return result
}

func assertItemChanges(t *testing.T, got []ItemChange, want ...ItemChange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("item changes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item changes = %#v, want %#v", got, want)
		}
	}
}
