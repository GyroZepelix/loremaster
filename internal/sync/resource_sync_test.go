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

func TestSyncRejectsSourceSymlinkEscapesAndContinues(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		include string
		setup   func(t *testing.T, source string) string
	}{
		{
			name:    "hard final file symlink",
			mode:    "hard",
			include: "escape.md",
			setup: func(t *testing.T, source string) string {
				outside := filepath.Join(t.TempDir(), "outside.md")
				os.WriteFile(outside, []byte("outside"), 0644)
				os.Symlink(outside, filepath.Join(source, "escape.md"))
				return outside
			},
		},
		{
			name:    "soft final file symlink",
			mode:    "soft",
			include: "escape.md",
			setup: func(t *testing.T, source string) string {
				outside := filepath.Join(t.TempDir(), "outside.md")
				os.WriteFile(outside, []byte("outside"), 0644)
				os.Symlink(outside, filepath.Join(source, "escape.md"))
				return outside
			},
		},
		{
			name:    "soft final directory symlink",
			mode:    "soft",
			include: "escape-dir",
			setup: func(t *testing.T, source string) string {
				outside := t.TempDir()
				os.WriteFile(filepath.Join(outside, "outside.md"), []byte("outside"), 0644)
				os.Symlink(outside, filepath.Join(source, "escape-dir"))
				return outside
			},
		},
		{
			name:    "hard intermediate directory symlink",
			mode:    "hard",
			include: "escape-dir/nested.md",
			setup: func(t *testing.T, source string) string {
				outside := t.TempDir()
				os.WriteFile(filepath.Join(outside, "nested.md"), []byte("outside"), 0644)
				os.Symlink(outside, filepath.Join(source, "escape-dir"))
				return filepath.Join(outside, "nested.md")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			source := t.TempDir()
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
			os.WriteFile(filepath.Join(source, "valid.md"), []byte("valid"), 0644)
			outside := tt.setup(t, source)
			cfg := &config.Config{
				Providers: config.ProviderList{"claude"},
				Resources: []config.Resource{{Name: "prompts", Sources: []config.SkillSource{{
					Source: source,
					Type:   tt.mode,
					ParsedIncludes: []config.IncludeEntry{
						{Src: tt.include, Dst: tt.include},
						{Src: "valid.md", Dst: "valid.md"},
					},
				}}}},
			}
			prov, _ := provider.Get("claude")
			result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(cfg, map[string]string{source: source})
			if err == nil || result.Synced != 1 || !strings.Contains(strings.Join(result.Errors, "\n"), "resolves outside source root") {
				t.Fatalf("result = %#v, error = %v", result, err)
			}
			if _, err := os.Lstat(filepath.Join(project, ".claude", "prompts", filepath.FromSlash(tt.include))); !os.IsNotExist(err) {
				t.Fatalf("escaping destination exists: %v", err)
			}
			valid := filepath.Join(project, ".claude", "prompts", "valid.md")
			content, readErr := os.ReadFile(valid)
			if readErr != nil || string(content) != "valid" {
				t.Fatalf("valid include was not synced: content=%q err=%v", content, readErr)
			}
			if content, readErr := os.ReadFile(outside); readErr == nil && string(content) != "outside" {
				t.Fatalf("external content changed: %q", content)
			}
		})
	}
}

func TestSyncAllowsContainedSourceSymlink(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	os.Mkdir(filepath.Join(source, "real"), 0755)
	real := filepath.Join(source, "real", "review.md")
	os.WriteFile(real, []byte("review"), 0644)
	os.Symlink(real, filepath.Join(source, "review.md"))
	cfg := &config.Config{Providers: config.ProviderList{"claude"}, Resources: []config.Resource{{Name: "prompts", Sources: []config.SkillSource{{Source: source, Type: "soft", ParsedIncludes: []config.IncludeEntry{{Src: "review.md", Dst: "review.md"}}}}}}}
	prov, _ := provider.Get("claude")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(cfg, map[string]string{source: source})
	if err != nil || result.Synced != 1 {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	target, err := os.Readlink(filepath.Join(project, ".claude", "prompts", "review.md"))
	resolvedReal, resolveErr := filepath.EvalSymlinks(real)
	if err != nil || resolveErr != nil || target != resolvedReal {
		t.Fatalf("target = %q, error = %v, want %q (resolve error %v)", target, err, resolvedReal, resolveErr)
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

func TestMappingTransitionsSucceedInOneSync(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		oldIsDirectory bool
		oldDestination string
		newIsDirectory bool
		newDestination string
		resultFile     string
	}{
		{name: "soft ancestor to child", mode: "soft", oldIsDirectory: true, oldDestination: "templates", newDestination: "templates/a.md", resultFile: "templates/a.md"},
		{name: "soft child to ancestor", mode: "soft", oldDestination: "templates/a.md", newIsDirectory: true, newDestination: "templates", resultFile: "templates/content.md"},
		{name: "hard ancestor to child", mode: "hard", oldIsDirectory: true, oldDestination: "templates", newDestination: "templates/a.md", resultFile: "templates/a.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
			prov, _ := provider.Get("claude")
			oldSource, oldInclude := createTransitionSource(t, tt.oldIsDirectory, "old")
			oldCfg := transitionConfig(oldSource, tt.mode, oldInclude, tt.oldDestination)
			first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(oldCfg, map[string]string{oldSource: oldSource})
			if err != nil {
				t.Fatalf("initial sync: %v", err)
			}
			mf := manifest.New()
			mf.SetProfileItems("default", first.Items)

			newSource, newInclude := createTransitionSource(t, tt.newIsDirectory, "new")
			newCfg := transitionConfig(newSource, tt.mode, newInclude, tt.newDestination)
			result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(newCfg, map[string]string{newSource: newSource})
			if err != nil {
				t.Fatalf("transition sync: %v, result=%#v", err, result)
			}
			if len(result.Removed) != 1 || result.Removed[0] != ".claude/prompts/"+tt.oldDestination {
				t.Fatalf("removed = %v", result.Removed)
			}
			if commitErrs := CommitChanges(result.Changes); len(commitErrs) != 0 {
				t.Fatalf("commit changes: %v", commitErrs)
			}
			applySyncResultToManifest(mf, "default", result)
			content, readErr := os.ReadFile(filepath.Join(project, ".claude", "prompts", filepath.FromSlash(tt.resultFile)))
			if readErr != nil || string(content) != "new" {
				t.Fatalf("new destination content = %q, err = %v", content, readErr)
			}
			assertNoLoreTemporaryPaths(t, project)

			second, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(newCfg, map[string]string{newSource: newSource})
			if err != nil {
				t.Fatalf("idempotent sync: %v, result=%#v", err, second)
			}
			if len(second.Removed) != 0 {
				t.Fatalf("idempotent sync removed = %v", second.Removed)
			}
			if commitErrs := CommitChanges(second.Changes); len(commitErrs) != 0 {
				t.Fatalf("commit idempotent changes: %v", commitErrs)
			}
			assertNoLoreTemporaryPaths(t, project)
		})
	}
}

func TestMappingTransitionToEmptyAncestorPersistsAfterCommit(t *testing.T) {
	for _, mode := range []string{"soft", "hard"} {
		t.Run(mode, func(t *testing.T) {
			project := t.TempDir()
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
			prov, _ := provider.Get("claude")
			oldSource, oldInclude := createTransitionSource(t, false, "old")
			first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(transitionConfig(oldSource, mode, oldInclude, "templates/a.md"), map[string]string{oldSource: oldSource})
			if err != nil {
				t.Fatal(err)
			}
			mf := manifest.New()
			mf.SetProfileItems("default", first.Items)
			newSource := t.TempDir()
			os.Mkdir(filepath.Join(newSource, "empty"), 0755)
			result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(transitionConfig(newSource, mode, "empty", "templates"), map[string]string{newSource: newSource})
			if err != nil {
				t.Fatalf("transition sync: %v", err)
			}
			if commitErrs := CommitChanges(result.Changes); len(commitErrs) != 0 {
				t.Fatalf("commit changes: %v", commitErrs)
			}
			destination := filepath.Join(project, ".claude", "prompts", "templates")
			info, err := os.Lstat(destination)
			if err != nil {
				t.Fatalf("empty ancestor was removed after commit: %v", err)
			}
			if mode == "soft" && info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("soft destination mode = %v", info.Mode())
			}
			if mode == "hard" && !info.IsDir() {
				t.Fatalf("hard destination mode = %v", info.Mode())
			}
			assertNoLoreTemporaryPaths(t, project)
		})
	}
}

func TestMappingTransitionMissingSourcePreservesStaleItem(t *testing.T) {
	project := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	prov, _ := provider.Get("claude")
	oldSource, oldInclude := createTransitionSource(t, true, "old")
	oldCfg := transitionConfig(oldSource, "soft", oldInclude, "templates")
	first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(oldCfg, map[string]string{oldSource: oldSource})
	if err != nil {
		t.Fatal(err)
	}
	mf := manifest.New()
	mf.SetProfileItems("default", first.Items)
	missingSource := t.TempDir()
	newCfg := transitionConfig(missingSource, "soft", "missing.md", "templates/a.md")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(newCfg, map[string]string{missingSource: missingSource})
	if err == nil || len(result.Removed) != 0 || len(result.Changes) != 0 {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude", "prompts", "templates")); err != nil {
		t.Fatalf("stale ancestor was not preserved: %v", err)
	}
	assertNoLoreTemporaryPaths(t, project)
}

func TestMappingTransitionInstallFailureRollsBack(t *testing.T) {
	project := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	prov, _ := provider.Get("claude")
	oldSource, oldInclude := createTransitionSource(t, false, "old")
	oldCfg := transitionConfig(oldSource, "soft", oldInclude, "templates/a.md")
	first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(oldCfg, map[string]string{oldSource: oldSource})
	if err != nil {
		t.Fatal(err)
	}
	mf := manifest.New()
	mf.SetProfileItems("default", first.Items)
	unmanaged := filepath.Join(project, ".claude", "prompts", "templates", "keep.md")
	os.WriteFile(unmanaged, []byte("keep"), 0644)
	newSource, newInclude := createTransitionSource(t, true, "new")
	newCfg := transitionConfig(newSource, "soft", newInclude, "templates")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(newCfg, map[string]string{newSource: newSource})
	if err == nil || len(result.Removed) != 0 || len(result.Changes) != 0 {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	oldPath := filepath.Join(project, ".claude", "prompts", "templates", "a.md")
	if _, err := os.Readlink(oldPath); err != nil {
		t.Fatalf("old mapping was not restored: %v", err)
	}
	if content, err := os.ReadFile(unmanaged); err != nil || string(content) != "keep" {
		t.Fatalf("unmanaged sibling changed: content=%q err=%v", content, err)
	}
	assertNoLoreTemporaryPaths(t, project)
}

func TestMappingTransitionPreservesModifiedHardAndSharedItems(t *testing.T) {
	t.Run("modified hard", func(t *testing.T) {
		project := t.TempDir()
		t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
		prov, _ := provider.Get("claude")
		oldSource, oldInclude := createTransitionSource(t, true, "old")
		first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(transitionConfig(oldSource, "hard", oldInclude, "templates"), map[string]string{oldSource: oldSource})
		if err != nil {
			t.Fatal(err)
		}
		mf := manifest.New()
		mf.SetProfileItems("default", first.Items)
		oldContent := filepath.Join(project, ".claude", "prompts", "templates", "content.md")
		os.WriteFile(oldContent, []byte("modified"), 0644)
		newSource, newInclude := createTransitionSource(t, false, "new")
		result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(transitionConfig(newSource, "hard", newInclude, "templates/a.md"), map[string]string{newSource: newSource})
		if err == nil || len(result.Removed) != 0 {
			t.Fatalf("result = %#v, error = %v", result, err)
		}
		if content, _ := os.ReadFile(oldContent); string(content) != "modified" {
			t.Fatalf("modified content changed: %q", content)
		}
		assertNoLoreTemporaryPaths(t, project)
	})

	t.Run("shared ownership", func(t *testing.T) {
		project := t.TempDir()
		t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
		prov, _ := provider.Get("claude")
		oldSource, oldInclude := createTransitionSource(t, true, "old")
		first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(transitionConfig(oldSource, "soft", oldInclude, "templates"), map[string]string{oldSource: oldSource})
		if err != nil {
			t.Fatal(err)
		}
		mf := manifest.New()
		mf.SetProfileItems("default", first.Items)
		mf.SetProfileItems("staging", first.Items)
		newSource, newInclude := createTransitionSource(t, false, "new")
		result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(transitionConfig(newSource, "soft", newInclude, "templates/a.md"), map[string]string{newSource: newSource})
		if err == nil || len(result.Removed) != 0 || !strings.Contains(strings.Join(result.Errors, "\n"), "owned by profile") {
			t.Fatalf("result = %#v, error = %v", result, err)
		}
		if _, err := os.Lstat(filepath.Join(project, ".claude", "prompts", "templates")); err != nil {
			t.Fatalf("shared item was removed: %v", err)
		}
		assertNoLoreTemporaryPaths(t, project)
	})
}

func TestMappingTransitionGlobalRollbackRestoresOriginal(t *testing.T) {
	project := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	prov, _ := provider.Get("claude")
	oldSource, oldInclude := createTransitionSource(t, false, "old")
	first, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: manifest.New()}).Sync(transitionConfig(oldSource, "soft", oldInclude, "templates/a.md"), map[string]string{oldSource: oldSource})
	if err != nil {
		t.Fatal(err)
	}
	mf := manifest.New()
	mf.SetProfileItems("default", first.Items)
	newSource, newInclude := createTransitionSource(t, true, "new")
	result, err := (&Syncer{Provider: prov, ProjectRoot: project, ProfileName: "default", Manifest: mf, Transactional: true}).Sync(transitionConfig(newSource, "soft", newInclude, "templates"), map[string]string{newSource: newSource})
	if err != nil {
		t.Fatalf("transition sync: %v", err)
	}
	if rollbackErrs := RollbackChanges(result.Changes); len(rollbackErrs) != 0 {
		t.Fatalf("rollback: %v", rollbackErrs)
	}
	oldPath := filepath.Join(project, ".claude", "prompts", "templates", "a.md")
	if _, err := os.Readlink(oldPath); err != nil {
		t.Fatalf("old mapping was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "prompts", "templates", "content.md")); !os.IsNotExist(err) {
		t.Fatalf("new mapping remains after rollback: %v", err)
	}
	assertNoLoreTemporaryPaths(t, project)
}

func createTransitionSource(t *testing.T, directory bool, content string) (string, string) {
	t.Helper()
	source := t.TempDir()
	if directory {
		path := filepath.Join(source, "item")
		os.Mkdir(path, 0755)
		os.WriteFile(filepath.Join(path, "content.md"), []byte(content), 0644)
		return source, "item"
	}
	os.WriteFile(filepath.Join(source, "item.md"), []byte(content), 0644)
	return source, "item.md"
}

func transitionConfig(source string, mode string, include string, destination string) *config.Config {
	return &config.Config{Providers: config.ProviderList{"claude"}, Resources: []config.Resource{{Name: "prompts", Sources: []config.SkillSource{{Source: source, Type: mode, ParsedIncludes: []config.IncludeEntry{{Src: include, Dst: destination}}}}}}}
}

func applySyncResultToManifest(mf *manifest.Manifest, profile string, result *SyncResult) {
	items, _ := mf.GetProfileItems(profile)
	byPath := make(map[string]manifest.Item, len(items)+len(result.Items))
	for _, item := range items {
		byPath[item.Path] = item
	}
	for _, path := range result.Removed {
		delete(byPath, path)
	}
	for _, item := range result.Items {
		byPath[item.Path] = item
	}
	items = items[:0]
	for _, item := range byPath {
		items = append(items, item)
	}
	mf.SetProfileItems(profile, items)
}

func assertNoLoreTemporaryPaths(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".lore-remove-") || strings.HasPrefix(name, ".lore-stage-") || strings.HasPrefix(name, ".lore-backup-") {
			t.Errorf("temporary path remains: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk temporary paths: %v", err)
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
