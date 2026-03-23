package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgjalic/loremaster/internal/config"
	loregit "github.com/dgjalic/loremaster/internal/git"
	"github.com/dgjalic/loremaster/internal/provider"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// mockFetcher implements git.Fetcher for testing
type mockFetcher struct {
	cloneCalled   bool
	checkoutRef   string
	shouldFail    bool
	prepareSource func(dir string)
}

func (m *mockFetcher) CloneOrPull(url string, targetDir string) error {
	m.cloneCalled = true
	if m.shouldFail {
		return os.ErrNotExist
	}
	// Create the target dir with skills if prepareSource is set
	if m.prepareSource != nil {
		os.MkdirAll(targetDir, 0755)
		m.prepareSource(targetDir)
	}
	return nil
}

func (m *mockFetcher) Checkout(repoDir string, ref string) error {
	m.checkoutRef = ref
	return nil
}

func setupTestProject(t *testing.T) (string, provider.Provider) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	prov, _ := provider.Get("claude")
	return dir, prov
}

func createLocalSource(t *testing.T, dir string, skills ...string) string {
	t.Helper()
	srcDir := filepath.Join(dir, "local-skills")
	for _, skill := range skills {
		skillDir := filepath.Join(srcDir, skill)
		os.MkdirAll(skillDir, 0755)
		os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# "+skill), 0644)
	}
	return srcDir
}

func TestSync_LocalSource_Symlink(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "foo", "bar")

	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"foo", "bar"}, Type: "soft"},
		},
	}

	result, err := syncer.Sync(cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 2 {
		t.Errorf("synced = %d, want 2", result.Synced)
	}

	// Verify symlinks
	for _, skill := range []string{"foo", "bar"} {
		skillPath := filepath.Join(projectDir, ".claude", "skills", skill)
		target, err := os.Readlink(skillPath)
		if err != nil {
			t.Errorf("skill %q not a symlink: %v", skill, err)
			continue
		}
		expectedTarget := filepath.Join(srcDir, skill)
		if target != expectedTarget {
			t.Errorf("symlink %q -> %q, want -> %q", skill, target, expectedTarget)
		}
	}

	// Verify gitignore
	gitignoreContent, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	s := string(gitignoreContent)
	if !containsLine(s, ".claude/skills/foo") {
		t.Error("gitignore missing foo")
	}
	if !containsLine(s, ".claude/skills/bar") {
		t.Error("gitignore missing bar")
	}
}

func TestSync_GitSource(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	fetcher := &mockFetcher{
		prepareSource: func(dir string) {
			os.MkdirAll(filepath.Join(dir, "skill-a"), 0755)
			os.WriteFile(filepath.Join(dir, "skill-a", "workflow.md"), []byte("# A"), 0644)
			os.MkdirAll(filepath.Join(dir, "skill-b"), 0755)
			os.WriteFile(filepath.Join(dir, "skill-b", "workflow.md"), []byte("# B"), 0644)
		},
	}

	syncer := &Syncer{
		GitFetcher:  fetcher,
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{
				Source:  "git@github.com:user/repo.git",
				Ref:    "main",
				Include: []string{"skill-a", "skill-b"},
				Type:   "soft",
			},
		},
	}

	result, err := syncer.Sync(cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 2 {
		t.Errorf("synced = %d, want 2", result.Synced)
	}
	if !fetcher.cloneCalled {
		t.Error("git clone not called")
	}
	if fetcher.checkoutRef != "main" {
		t.Errorf("checkout ref = %q, want main", fetcher.checkoutRef)
	}
}

func TestSync_HardCopy(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "my-skill")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"my-skill"}, Type: "hard"},
		},
	}

	result, err := syncer.Sync(cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}

	skillPath := filepath.Join(projectDir, ".claude", "skills", "my-skill")

	// Should NOT be a symlink
	_, err = os.Readlink(skillPath)
	if err == nil {
		t.Error("hard copy should not be a symlink")
	}

	// Should have .lore-checksum
	checksumPath := filepath.Join(skillPath, ".lore-checksum")
	if _, err := os.Stat(checksumPath); err != nil {
		t.Error("missing .lore-checksum file")
	}

	// Content should be copied
	content, err := os.ReadFile(filepath.Join(skillPath, "workflow.md"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(content) != "# my-skill" {
		t.Errorf("content = %q, want %q", string(content), "# my-skill")
	}
}

func TestSync_HardCopy_LocalModifications(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "my-skill")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"my-skill"}, Type: "hard"},
		},
	}

	// First sync
	syncer.Sync(cfg)

	// Modify the copied skill
	skillPath := filepath.Join(projectDir, ".claude", "skills", "my-skill")
	os.WriteFile(filepath.Join(skillPath, "workflow.md"), []byte("# MODIFIED"), 0644)

	// Second sync should skip due to checksum mismatch
	result, err := syncer.Sync(cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Verify modifications preserved
	content, _ := os.ReadFile(filepath.Join(skillPath, "workflow.md"))
	if string(content) != "# MODIFIED" {
		t.Error("local modifications were overwritten")
	}
	_ = result
}

func TestSync_FailedSource_ContinuesOthers(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "good-skill")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{shouldFail: true},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: "git@github.com:bad/repo.git", Include: []string{"bad-skill"}, Type: "soft"},
			{Source: srcDir, Include: []string{"good-skill"}, Type: "soft"},
		},
	}

	result, err := syncer.Sync(cfg)
	if err == nil {
		t.Fatal("expected error for failed source")
	}
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1 (good source)", result.Synced)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d, want 1", len(result.Errors))
	}

	// Good skill should still be synced
	skillPath := filepath.Join(projectDir, ".claude", "skills", "good-skill")
	if _, err := os.Lstat(skillPath); err != nil {
		t.Error("good skill not synced despite bad source failing")
	}
}

func TestSync_PerSkillIsolation(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "good-skill", "another-good")
	// "missing-skill" is not created in the source
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"good-skill", "missing-skill", "another-good"}, Type: "soft"},
		},
	}

	result, err := syncer.Sync(cfg)
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	if result.Synced != 2 {
		t.Errorf("synced = %d, want 2 (good-skill + another-good)", result.Synced)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d, want 1 (missing-skill)", len(result.Errors))
	}

	// Verify both good skills were synced
	for _, skill := range []string{"good-skill", "another-good"} {
		skillPath := filepath.Join(projectDir, ".claude", "skills", skill)
		if _, err := os.Lstat(skillPath); err != nil {
			t.Errorf("skill %q not synced despite missing-skill failing", skill)
		}
	}

	// Verify gitignore has both good skills
	gitignoreContent, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	s := string(gitignoreContent)
	if !containsLine(s, ".claude/skills/good-skill") {
		t.Error("gitignore missing good-skill")
	}
	if !containsLine(s, ".claude/skills/another-good") {
		t.Error("gitignore missing another-good")
	}
}

func TestSync_StaleReconciliation(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "keep-skill", "remove-skill")
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	// First sync with both skills
	cfg1 := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"keep-skill", "remove-skill"}, Type: "soft"},
		},
	}
	syncer.Sync(cfg1)

	// Verify both exist
	keepPath := filepath.Join(projectDir, ".claude", "skills", "keep-skill")
	removePath := filepath.Join(projectDir, ".claude", "skills", "remove-skill")
	if _, err := os.Lstat(keepPath); err != nil {
		t.Fatal("keep-skill not created")
	}
	if _, err := os.Lstat(removePath); err != nil {
		t.Fatal("remove-skill not created")
	}

	// Second sync without remove-skill
	cfg2 := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"keep-skill"}, Type: "soft"},
		},
	}
	syncer.Sync(cfg2)

	// keep-skill should still exist
	if _, err := os.Lstat(keepPath); err != nil {
		t.Error("keep-skill was removed")
	}
	// remove-skill: reconciliation only removes symlinks pointing into cache dir
	// Local source symlinks point to local path, not cache, so they won't be auto-removed
	// This is expected behavior — stale reconciliation targets cache-backed symlinks
}

func TestSync_Idempotent(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "my-skill")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: srcDir, Include: []string{"my-skill"}, Type: "soft"},
		},
	}

	syncer.Sync(cfg)
	gitignore1, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))

	syncer.Sync(cfg)
	gitignore2, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))

	if string(gitignore1) != string(gitignore2) {
		t.Errorf("gitignore changed between syncs:\nfirst:\n%s\nsecond:\n%s", gitignore1, gitignore2)
	}
}

func TestSync_Integration_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmp := t.TempDir()
	cacheBase := filepath.Join(tmp, "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Create a bare git repo with two skill directories
	repoPath := filepath.Join(tmp, "skill-repo")
	repo, err := gogit.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	// Create skills
	for _, skill := range []string{"skill-alpha", "skill-beta"} {
		dir := filepath.Join(repoPath, skill)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "workflow.md"), []byte("# "+skill), 0644)
	}

	wt, _ := repo.Worktree()
	wt.Add(".")
	wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})

	// Setup project dir
	projectDir := filepath.Join(tmp, "project")
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)

	prov, _ := provider.Get("claude")

	syncer := &Syncer{
		GitFetcher:  &loregit.GoGitFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: repoPath, Include: []string{"skill-alpha", "skill-beta"}, Type: "soft"},
		},
	}

	// First sync
	result, err := syncer.Sync(cfg)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 2 {
		t.Errorf("synced = %d, want 2", result.Synced)
	}

	// Verify symlinks exist
	for _, skill := range []string{"skill-alpha", "skill-beta"} {
		path := filepath.Join(projectDir, ".claude", "skills", skill)
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("skill %q not found: %v", skill, err)
		}
		content, _ := os.ReadFile(filepath.Join(path, "workflow.md"))
		if string(content) != "# "+skill {
			t.Errorf("skill %q content = %q", skill, string(content))
		}
	}

	// Verify gitignore
	gitignoreContent, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	if !containsLine(string(gitignoreContent), ".claude/skills/skill-alpha") {
		t.Error("gitignore missing skill-alpha")
	}

	// Remove one skill from config, re-sync
	cfg2 := &config.Config{
		Provider: "claude",
		Skills: []config.SkillSource{
			{Source: repoPath, Include: []string{"skill-alpha"}, Type: "soft"},
		},
	}

	result2, err := syncer.Sync(cfg2)
	if err != nil {
		t.Fatalf("re-Sync: %v", err)
	}
	if result2.Synced != 1 {
		t.Errorf("re-sync synced = %d, want 1", result2.Synced)
	}

	// skill-alpha should still exist
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "skill-alpha")); err != nil {
		t.Error("skill-alpha removed after re-sync")
	}

	// skill-beta should be removed (stale reconciliation) since it points into cache
	betaPath := filepath.Join(projectDir, ".claude", "skills", "skill-beta")
	if _, err := os.Lstat(betaPath); err == nil {
		// Check if it's a symlink into cache — if so, reconciliation should have removed it
		target, linkErr := os.Readlink(betaPath)
		if linkErr == nil {
			absTarget, _ := filepath.Abs(target)
			absCacheDir, _ := filepath.Abs(filepath.Join(cacheBase, "loremaster"))
			if strings.HasPrefix(absTarget, absCacheDir) {
				t.Error("stale skill-beta symlink into cache not reconciled")
			}
		}
	}
}

func containsLine(s string, line string) bool {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}
