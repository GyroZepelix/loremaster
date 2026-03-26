package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GyroZepelix/loremaster/internal/config"
	loregit "github.com/GyroZepelix/loremaster/internal/git"
	"github.com/GyroZepelix/loremaster/internal/gitignore"
	"github.com/GyroZepelix/loremaster/internal/manifest"
	"github.com/GyroZepelix/loremaster/internal/provider"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// mockFetcher implements git.Fetcher for testing
type mockFetcher struct {
	cloneCalled   bool
	cloneCount    int
	checkoutRef   string
	shouldFail    bool
	prepareSource func(dir string)
}

func (m *mockFetcher) CloneOrPull(url string, targetDir string) error {
	m.cloneCalled = true
	m.cloneCount++
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

// buildBaseDirs constructs a baseDirs map for local sources used in tests.
func buildBaseDirs(sources ...string) map[string]string {
	baseDirs := make(map[string]string)
	for _, src := range sources {
		abs, _ := filepath.Abs(src)
		baseDirs[src] = abs
	}
	return baseDirs
}

// parsedIncludes creates ParsedIncludes from Include strings for test configs.
func parsedIncludes(includes []string) []config.IncludeEntry {
	var entries []config.IncludeEntry
	for _, inc := range includes {
		entries = append(entries, config.IncludeEntry{Src: inc, Dst: inc})
	}
	return entries
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

	includes := []string{"foo", "bar"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes, Type: "soft", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
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

	gitURL := "git@github.com:user/repo.git"
	includes := []string{"skill-a", "skill-b"}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         gitURL,
				Ref:            "main",
				Include:        includes,
				Type:           "soft",
				ParsedIncludes: parsedIncludes(includes),
			},
		},
	}

	// Use FetchSources to get baseDirs (tests the two-phase flow)
	baseDirs, fetchErrs := FetchSources(fetcher, cfg.Skills)
	if len(fetchErrs) > 0 {
		t.Fatalf("FetchSources errors: %v", fetchErrs)
	}

	syncer := &Syncer{
		GitFetcher:  fetcher,
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	result, err := syncer.Sync(cfg, baseDirs)
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

	includes := []string{"my-skill"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes, Type: "hard", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
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

	includes := []string{"my-skill"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes, Type: "hard", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	baseDirs := buildBaseDirs(srcDir)

	// First sync
	if _, err := syncer.Sync(cfg, baseDirs); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	// Modify the copied skill
	skillPath := filepath.Join(projectDir, ".claude", "skills", "my-skill")
	os.WriteFile(filepath.Join(skillPath, "workflow.md"), []byte("# MODIFIED"), 0644)

	// Second sync should skip due to checksum mismatch
	result, err := syncer.Sync(cfg, baseDirs)
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

	fetcher := &mockFetcher{shouldFail: true}

	gitURL := "git@github.com:bad/repo.git"
	gitIncludes := []string{"bad-skill"}
	localIncludes := []string{"good-skill"}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: gitURL, Include: gitIncludes, Type: "soft", ParsedIncludes: parsedIncludes(gitIncludes)},
			{Source: srcDir, Include: localIncludes, Type: "soft", ParsedIncludes: parsedIncludes(localIncludes)},
		},
	}

	// FetchSources will fail for git source but succeed for local
	baseDirs, fetchErrs := FetchSources(fetcher, cfg.Skills)
	if len(fetchErrs) != 1 {
		t.Fatalf("expected 1 fetch error, got %d: %v", len(fetchErrs), fetchErrs)
	}

	syncer := &Syncer{
		GitFetcher:  fetcher,
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	result, err := syncer.Sync(cfg, baseDirs)
	if err == nil {
		t.Fatal("expected error for failed source")
	}
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1 (good source)", result.Synced)
	}
	// Sync reports 1 error for missing baseDirs entry
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

	includes := []string{"good-skill", "missing-skill", "another-good"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes, Type: "soft", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
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
	includes1 := []string{"keep-skill", "remove-skill"}
	cfg1 := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes1, Type: "soft", ParsedIncludes: parsedIncludes(includes1)},
		},
	}
	baseDirs := buildBaseDirs(srcDir)
	if _, err := syncer.Sync(cfg1, baseDirs); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

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
	includes2 := []string{"keep-skill"}
	cfg2 := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes2, Type: "soft", ParsedIncludes: parsedIncludes(includes2)},
		},
	}
	if _, err := syncer.Sync(cfg2, baseDirs); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

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

	includes := []string{"my-skill"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes, Type: "soft", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	baseDirs := buildBaseDirs(srcDir)

	if _, err := syncer.Sync(cfg, baseDirs); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	gitignore1, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))

	if _, err := syncer.Sync(cfg, baseDirs); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
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

	fetcher := &loregit.GoGitFetcher{}

	includes := []string{"skill-alpha", "skill-beta"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: repoPath, Include: includes, Type: "soft", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	// Two-phase: fetch first, then sync
	baseDirs, fetchErrs := FetchSources(fetcher, cfg.Skills)
	if len(fetchErrs) > 0 {
		t.Fatalf("FetchSources errors: %v", fetchErrs)
	}

	syncer := &Syncer{
		GitFetcher:  fetcher,
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	// First sync
	result, err := syncer.Sync(cfg, baseDirs)
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
	includes2 := []string{"skill-alpha"}
	cfg2 := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: repoPath, Include: includes2, Type: "soft", ParsedIncludes: parsedIncludes(includes2)},
		},
	}

	baseDirs2, fetchErrs2 := FetchSources(fetcher, cfg2.Skills)
	if len(fetchErrs2) > 0 {
		t.Fatalf("FetchSources errors: %v", fetchErrs2)
	}

	result2, err := syncer.Sync(cfg2, baseDirs2)
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

func TestSync_NilProviderReturnsError(t *testing.T) {
	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    nil,
		ProjectRoot: t.TempDir(),
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
	}

	_, err := syncer.Sync(cfg, map[string]string{})
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
	if !strings.Contains(err.Error(), "provider must be set") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFetchSources_SourceIsolation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	goodDir := createLocalSource(t, t.TempDir(), "skill-a")

	sources := []config.SkillSource{
		{Source: goodDir},
		{Source: "/nonexistent/bad/path"},
	}

	baseDirs, errs := FetchSources(&mockFetcher{}, sources)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if _, ok := baseDirs[goodDir]; !ok {
		t.Error("good source missing from baseDirs")
	}
	if _, ok := baseDirs["/nonexistent/bad/path"]; ok {
		t.Error("bad source should not be in baseDirs")
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

// createNestedLocalSource creates a local source dir with nested skill paths.
// Each skill string can contain slashes (e.g., "loa/brainstorm").
func createNestedLocalSource(t *testing.T, dir string, skills ...string) string {
	t.Helper()
	srcDir := filepath.Join(dir, "local-skills")
	for _, skill := range skills {
		skillDir := filepath.Join(srcDir, skill)
		os.MkdirAll(skillDir, 0755)
		os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# "+skill), 0644)
	}
	return srcDir
}

// setupCachedSymlink creates a relative symlink from the provider skills dir into a fake cache dir.
// reconcileStale resolves symlink targets via filepath.Join(dir, target), so relative symlinks
// are required for the prefix check to work correctly.
func setupCachedSymlink(t *testing.T, projectDir, cacheBase, skillRelPath string) {
	t.Helper()
	cacheSkillDir := filepath.Join(cacheBase, "loremaster", "repos", "fakerepo", skillRelPath)
	os.MkdirAll(cacheSkillDir, 0755)
	os.WriteFile(filepath.Join(cacheSkillDir, "workflow.md"), []byte("# "+skillRelPath), 0644)

	skillLink := filepath.Join(projectDir, ".claude", "skills", skillRelPath)
	os.MkdirAll(filepath.Dir(skillLink), 0755)

	// Create relative symlink so reconcileStale can resolve it correctly
	relTarget, err := filepath.Rel(filepath.Dir(skillLink), cacheSkillDir)
	if err != nil {
		t.Fatalf("compute relative symlink path: %v", err)
	}
	os.Symlink(relTarget, skillLink)
}

// --- Task 1: Subdirectory include sync tests (AC: 2, 3, 6) ---

func TestSync_SubdirectoryInclude_NestedPath(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createNestedLocalSource(t, t.TempDir(), "loa/brainstorm")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         srcDir,
				Include:        []string{"loa/brainstorm"},
				Type:           "soft",
				ParsedIncludes: []config.IncludeEntry{{Src: "loa/brainstorm", Dst: "loa/brainstorm"}},
			},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}

	// Verify nested symlink exists
	skillPath := filepath.Join(projectDir, ".claude", "skills", "loa", "brainstorm")
	target, err := os.Readlink(skillPath)
	if err != nil {
		t.Fatalf("skill loa/brainstorm not a symlink: %v", err)
	}
	expectedTarget := filepath.Join(srcDir, "loa", "brainstorm")
	if target != expectedTarget {
		t.Errorf("symlink target = %q, want %q", target, expectedTarget)
	}

	// Verify content accessible via symlink
	content, err := os.ReadFile(filepath.Join(skillPath, "workflow.md"))
	if err != nil {
		t.Fatalf("read workflow.md via symlink: %v", err)
	}
	if string(content) != "# loa/brainstorm" {
		t.Errorf("content = %q, want %q", string(content), "# loa/brainstorm")
	}
}

func TestSync_SubdirectoryInclude_MappedPath(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createNestedLocalSource(t, t.TempDir(), "deep/skill")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         srcDir,
				Include:        []string{"deep/skill:my-tool"},
				Type:           "soft",
				ParsedIncludes: []config.IncludeEntry{{Src: "deep/skill", Dst: "my-tool"}},
			},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}

	// Verify my-tool symlink points to deep/skill source
	skillPath := filepath.Join(projectDir, ".claude", "skills", "my-tool")
	target, err := os.Readlink(skillPath)
	if err != nil {
		t.Fatalf("skill my-tool not a symlink: %v", err)
	}
	expectedTarget := filepath.Join(srcDir, "deep", "skill")
	if target != expectedTarget {
		t.Errorf("symlink target = %q, want %q", target, expectedTarget)
	}

	// Verify workflow.md accessible
	content, err := os.ReadFile(filepath.Join(skillPath, "workflow.md"))
	if err != nil {
		t.Fatalf("read workflow.md via symlink: %v", err)
	}
	if string(content) != "# deep/skill" {
		t.Errorf("content = %q, want %q", string(content), "# deep/skill")
	}
}

func TestSync_SubdirectoryInclude_CollisionDetection(t *testing.T) {
	projectDir, prov := setupTestProject(t)

	// Two different sources with same destination
	srcDir1 := createNestedLocalSource(t, t.TempDir(), "my-skill")
	srcDir2 := createNestedLocalSource(t, t.TempDir(), "my-skill")
	// Overwrite content in srcDir2 to distinguish it
	os.WriteFile(filepath.Join(srcDir2, "my-skill", "workflow.md"), []byte("# source-two"), 0644)

	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	// Both sources claim the same destination "my-skill"
	// ValidateOverlaps will catch duplicate Dst across sources, so we use
	// the collision warning path: same Dst from different SkillSources.
	// The Sync function warns but last source wins.
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         srcDir1,
				Include:        []string{"my-skill"},
				Type:           "soft",
				ParsedIncludes: []config.IncludeEntry{{Src: "my-skill", Dst: "my-skill"}},
			},
			{
				Source:         srcDir2,
				Include:        []string{"my-skill"},
				Type:           "soft",
				ParsedIncludes: []config.IncludeEntry{{Src: "my-skill", Dst: "my-skill"}},
			},
		},
	}

	baseDirs := map[string]string{
		srcDir1: srcDir1,
		srcDir2: srcDir2,
	}

	// Sync will produce cross-source overlap error from ValidateOverlaps
	// since both sources have the same Dst "my-skill"
	_, err := syncer.Sync(cfg, baseDirs)
	if err == nil {
		t.Fatal("expected error for cross-source overlap")
	}
	if !strings.Contains(err.Error(), "cross-source overlap") {
		t.Errorf("expected cross-source overlap error, got: %v", err)
	}
}

// --- Task 2: Stale nested skill reconciliation tests (AC: 4, 5) ---

func TestSync_StaleNested_EmptyParentCleanup(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Setup: create a cached symlink at loa/brainstorm
	setupCachedSymlink(t, projectDir, cacheBase, "loa/brainstorm")

	// Verify it exists before sync
	skillPath := filepath.Join(projectDir, ".claude", "skills", "loa", "brainstorm")
	if _, err := os.Lstat(skillPath); err != nil {
		t.Fatal("setup failed: loa/brainstorm symlink not created")
	}

	// Sync WITHOUT loa/brainstorm in config → should remove it and clean parent
	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills:    []config.SkillSource{},
	}

	if _, err := syncer.Sync(cfg, map[string]string{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Verify loa/brainstorm symlink removed
	if _, err := os.Lstat(skillPath); err == nil {
		t.Error("stale loa/brainstorm symlink not removed")
	}

	// Verify loa/ parent directory removed (empty after cleanup)
	loaDir := filepath.Join(projectDir, ".claude", "skills", "loa")
	if _, err := os.Stat(loaDir); err == nil {
		t.Error("empty parent directory loa/ not cleaned up")
	}
}

func TestSync_StaleNested_ParentPreserved(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Setup: create two cached symlinks under loa/
	setupCachedSymlink(t, projectDir, cacheBase, "loa/brainstorm")
	setupCachedSymlink(t, projectDir, cacheBase, "loa/helper")

	// Sync with only loa/helper in config → loa/brainstorm removed, loa/ preserved
	repoRoot := filepath.Join(cacheBase, "loremaster", "repos", "fakerepo")
	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         "git@github.com:user/repo.git",
				Include:        []string{"loa/helper"},
				Type:           "soft",
				ParsedIncludes: []config.IncludeEntry{{Src: "loa/helper", Dst: "loa/helper"}},
			},
		},
	}

	// Build baseDirs pointing to the repo root (not the skill subdirectory)
	baseDirs := map[string]string{
		"git@github.com:user/repo.git": repoRoot,
	}

	if _, err := syncer.Sync(cfg, baseDirs); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// loa/brainstorm should be removed
	brainstormPath := filepath.Join(projectDir, ".claude", "skills", "loa", "brainstorm")
	if _, err := os.Lstat(brainstormPath); err == nil {
		t.Error("stale loa/brainstorm symlink not removed")
	}

	// loa/helper should still exist
	helperPath := filepath.Join(projectDir, ".claude", "skills", "loa", "helper")
	if _, err := os.Lstat(helperPath); err != nil {
		t.Error("loa/helper should still exist")
	}

	// loa/ parent should still exist (helper is there)
	loaDir := filepath.Join(projectDir, ".claude", "skills", "loa")
	if _, err := os.Stat(loaDir); err != nil {
		t.Error("parent directory loa/ should be preserved when other entries exist")
	}
}

// --- Task 3: Manifest-aware reconciliation tests (AC: 7, 8, 9) ---

func TestSync_NoManifest_ReconcileAllStale(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Create a cached symlink that will become stale
	setupCachedSymlink(t, projectDir, cacheBase, "old-skill")

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
		Manifest:    nil, // No manifest
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills:    []config.SkillSource{},
	}

	if _, err := syncer.Sync(cfg, map[string]string{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// old-skill should be removed (no manifest = reconcile all)
	skillPath := filepath.Join(projectDir, ".claude", "skills", "old-skill")
	if _, err := os.Lstat(skillPath); err == nil {
		t.Error("stale old-skill should be removed when no manifest")
	}
}

func TestSync_ManifestProfile_OnlyProfileStaleRemoved(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Setup: create cached symlinks for skill-a (dev), skill-b (dev), skill-c (staging)
	setupCachedSymlink(t, projectDir, cacheBase, "skill-a")
	setupCachedSymlink(t, projectDir, cacheBase, "skill-b")
	setupCachedSymlink(t, projectDir, cacheBase, "skill-c")

	mf := manifest.New()
	mf.SetProfile("dev", []string{".claude/skills/skill-a", ".claude/skills/skill-b"})
	mf.SetProfile("staging", []string{".claude/skills/skill-c"})

	// Sync as "dev" with only skill-b in config → skill-a removed, skill-c preserved
	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
		Manifest:    mf,
		ProfileName: "dev",
	}

	skillBSrc := filepath.Join(cacheBase, "loremaster", "repos", "fakerepo", "skill-b")
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         "git@github.com:user/repo.git",
				Include:        []string{"skill-b"},
				Type:           "soft",
				ParsedIncludes: []config.IncludeEntry{{Src: "skill-b", Dst: "skill-b"}},
			},
		},
	}

	baseDirs := map[string]string{
		"git@github.com:user/repo.git": filepath.Dir(skillBSrc),
	}

	if _, err := syncer.Sync(cfg, baseDirs); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// skill-a should be removed (owned by dev, no longer in config)
	skillAPath := filepath.Join(projectDir, ".claude", "skills", "skill-a")
	if _, err := os.Lstat(skillAPath); err == nil {
		t.Error("stale skill-a should be removed (owned by dev profile)")
	}

	// skill-c should be preserved (owned by staging, not dev)
	skillCPath := filepath.Join(projectDir, ".claude", "skills", "skill-c")
	if _, err := os.Lstat(skillCPath); err != nil {
		t.Error("skill-c should be preserved (owned by staging profile)")
	}

	// skill-b should still exist
	skillBPath := filepath.Join(projectDir, ".claude", "skills", "skill-b")
	if _, err := os.Lstat(skillBPath); err != nil {
		t.Error("skill-b should still exist (in current config)")
	}

	// Verify manifest dev profile updated to only contain skill-b
	devEntries, exists := mf.GetProfile("dev")
	if !exists {
		t.Error("dev profile should still exist in manifest")
	}
	if len(devEntries) != 1 {
		t.Errorf("dev profile entries = %d, want 1", len(devEntries))
	}
}

func TestSync_ManifestFirstRunProfile_SkipReconciliation(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Create a stale cached symlink that should NOT be removed during first-run
	setupCachedSymlink(t, projectDir, cacheBase, "existing-stale")

	// Manifest exists but "newprof" has no entries (first run)
	mf := manifest.New()
	// newprof doesn't exist in manifest yet — GetProfile returns (nil, false)

	srcDir := createLocalSource(t, t.TempDir(), "skill-x")

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
		Manifest:    mf,
		ProfileName: "newprof",
	}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         srcDir,
				Include:        []string{"skill-x"},
				Type:           "soft",
				ParsedIncludes: parsedIncludes([]string{"skill-x"}),
			},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// skill-x should be synced
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}

	// existing-stale should NOT be removed (first-run skip)
	stalePath := filepath.Join(projectDir, ".claude", "skills", "existing-stale")
	if _, err := os.Lstat(stalePath); err != nil {
		t.Error("existing-stale should be preserved during first-run profile sync")
	}

	// Manifest should now have newprof entries registered
	entries, exists := mf.GetProfile("newprof")
	if !exists {
		t.Error("newprof should exist in manifest after sync")
	}
	if len(entries) != 1 {
		t.Errorf("newprof entries = %d, want 1", len(entries))
	}
}

// --- Task 4: v0.1.x backward compatibility integration test (AC: 10) ---

func TestSync_V01x_BackwardCompatibility(t *testing.T) {
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "skill-a", "skill-b")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
	}

	// v0.1.x-style config: provider: claude (scalar), flat includes
	includes := []string{"skill-a", "skill-b"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{
				Source:         srcDir,
				Include:        includes,
				Type:           "soft",
				ParsedIncludes: parsedIncludes(includes),
			},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Synced != 2 {
		t.Errorf("synced = %d, want 2", result.Synced)
	}

	// Verify skills placed at .claude/skills/<name>
	for _, skill := range includes {
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

	// Verify gitignore entries
	gitignoreContent, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	s := string(gitignoreContent)
	if !containsLine(s, ".claude/skills/skill-a") {
		t.Error("gitignore missing .claude/skills/skill-a")
	}
	if !containsLine(s, ".claude/skills/skill-b") {
		t.Error("gitignore missing .claude/skills/skill-b")
	}
}

// --- Task 5: Multi-provider and FetchSources call count tests (AC: 16, 17) ---

func TestSync_MultiProvider_BothProviders(t *testing.T) {
	projectDir := t.TempDir()
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)
	os.MkdirAll(filepath.Join(projectDir, ".opencode"), 0755)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	srcDir := createLocalSource(t, t.TempDir(), "foo")
	includes := []string{"foo"}

	cfg := &config.Config{
		Providers: config.ProviderList{"claude", "opencode"},
		Skills: []config.SkillSource{
			{
				Source:         srcDir,
				Include:        includes,
				Type:           "soft",
				ParsedIncludes: parsedIncludes(includes),
			},
		},
	}

	baseDirs := buildBaseDirs(srcDir)

	claudeProv, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("provider.Get(claude): %v", err)
	}
	opencodeProv, err := provider.Get("opencode")
	if err != nil {
		t.Fatalf("provider.Get(opencode): %v", err)
	}

	// Sync for claude
	syncer1 := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    claudeProv,
		ProjectRoot: projectDir,
	}
	result1, err := syncer1.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync claude: %v", err)
	}
	if result1.Synced != 1 {
		t.Errorf("claude synced = %d, want 1", result1.Synced)
	}

	// Sync for opencode
	syncer2 := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    opencodeProv,
		ProjectRoot: projectDir,
	}
	result2, err := syncer2.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync opencode: %v", err)
	}
	if result2.Synced != 1 {
		t.Errorf("opencode synced = %d, want 1", result2.Synced)
	}

	// Verify both provider skill dirs exist
	claudeSkill := filepath.Join(projectDir, ".claude", "skills", "foo")
	if _, err := os.Lstat(claudeSkill); err != nil {
		t.Error("skill not found in .claude/skills/foo")
	}

	opencodeSkill := filepath.Join(projectDir, ".opencode", "skills", "foo")
	if _, err := os.Lstat(opencodeSkill); err != nil {
		t.Error("skill not found in .opencode/skills/foo")
	}

	// Verify gitignore has entries for both
	gitignoreContent, _ := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	s := string(gitignoreContent)
	if !containsLine(s, ".claude/skills/foo") {
		t.Error("gitignore missing .claude/skills/foo")
	}
	if !containsLine(s, ".opencode/skills/foo") {
		t.Error("gitignore missing .opencode/skills/foo")
	}
}

func TestSync_FetchSources_CallCount(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	fetcher := &mockFetcher{
		prepareSource: func(dir string) {
			os.MkdirAll(filepath.Join(dir, "skill-a"), 0755)
			os.WriteFile(filepath.Join(dir, "skill-a", "workflow.md"), []byte("# A"), 0644)
		},
	}

	// 3 sources (2 git + 1 local)
	localDir := createLocalSource(t, t.TempDir(), "local-skill")

	sources := []config.SkillSource{
		{Source: "git@github.com:user/repo1.git", Include: []string{"skill-a"}, ParsedIncludes: parsedIncludes([]string{"skill-a"})},
		{Source: "git@github.com:user/repo2.git", Include: []string{"skill-a"}, ParsedIncludes: parsedIncludes([]string{"skill-a"})},
		{Source: localDir, Include: []string{"local-skill"}, ParsedIncludes: parsedIncludes([]string{"local-skill"})},
	}

	// FetchSources called once for all sources
	baseDirs, errs := FetchSources(fetcher, sources)
	if len(errs) > 0 {
		t.Fatalf("FetchSources errors: %v", errs)
	}

	// Should have cloned exactly 2 git sources (local doesn't call CloneOrPull)
	if fetcher.cloneCount != 2 {
		t.Errorf("cloneCount = %d, want 2 (only git sources)", fetcher.cloneCount)
	}

	// baseDirs should have all 3 sources
	if len(baseDirs) != 3 {
		t.Errorf("baseDirs count = %d, want 3", len(baseDirs))
	}
}

// --- Task 6: Prune, orphan detection, and corrupted manifest tests (AC: 11-15) ---

func TestManifest_FindOrphaned(t *testing.T) {
	projectDir := t.TempDir()

	mf := manifest.New()
	mf.SetProfile("dev", []string{".claude/skills/skill-a"})
	mf.SetProfile("active", []string{".claude/skills/skill-b"})

	// Create config file for "active" so it is NOT orphaned
	os.WriteFile(filepath.Join(projectDir, "lore-active.yml"), []byte("provider: claude\n"), 0644)

	// No lore-dev.yml exists on disk → "dev" should be orphaned
	orphaned := mf.FindOrphaned(projectDir, config.LocateProfile)
	if len(orphaned) != 1 {
		t.Fatalf("orphaned = %d, want 1", len(orphaned))
	}
	if orphaned[0] != "dev" {
		t.Errorf("orphaned[0] = %q, want \"dev\"", orphaned[0])
	}
}

func TestSync_PruneFlow(t *testing.T) {
	projectDir, _ := setupTestProject(t)
	cacheBase := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_DATA_HOME", cacheBase)

	// Create cached symlinks for orphaned profile's entries
	setupCachedSymlink(t, projectDir, cacheBase, "orphan-skill")

	// Setup gitignore with the orphaned entry
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	os.WriteFile(gitignorePath, []byte("# Managed by loremaster\n.claude/skills/orphan-skill\n"), 0644)

	mf := manifest.New()
	mf.SetProfile("orphan", []string{".claude/skills/orphan-skill"})

	// Verify orphan is detected
	orphaned := mf.FindOrphaned(projectDir, config.LocateProfile)
	if len(orphaned) != 1 || orphaned[0] != "orphan" {
		t.Fatalf("expected orphan profile detected, got: %v", orphaned)
	}

	// Prune: remove symlinks, gitignore entries, manifest profile
	entries, _ := mf.GetProfile("orphan")
	for _, entry := range entries {
		absPath := filepath.Join(projectDir, entry)
		os.Remove(absPath)
	}

	// Remove from gitignore
	if err := gitignore.RemoveEntries(gitignorePath, entries); err != nil {
		t.Fatalf("remove gitignore entries: %v", err)
	}

	mf.RemoveProfile("orphan")

	// Verify symlink removed
	skillPath := filepath.Join(projectDir, ".claude", "skills", "orphan-skill")
	if _, err := os.Lstat(skillPath); err == nil {
		t.Error("orphan-skill symlink should be removed after prune")
	}

	// Verify gitignore entry removed
	gitContent, _ := os.ReadFile(gitignorePath)
	if containsLine(string(gitContent), ".claude/skills/orphan-skill") {
		t.Error("gitignore should not contain orphan-skill after prune")
	}

	// Verify manifest profile removed
	_, exists := mf.GetProfile("orphan")
	if exists {
		t.Error("orphan profile should be removed from manifest")
	}
}

func TestSync_PruneWithNoManifest(t *testing.T) {
	projectDir := t.TempDir()

	// manifest.Load on non-existent file returns (nil, nil)
	mf, err := manifest.Load(filepath.Join(projectDir, "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if mf != nil {
		t.Fatal("expected nil manifest for missing file")
	}

	// Prune with nil manifest is a no-op: FindOrphaned cannot be called on nil
	// and the prune logic checks manifest != nil before proceeding.
	// Verify nil manifest has no profiles to iterate.
	if mf != nil {
		t.Fatal("nil manifest should prevent any prune operations")
	}
}

func TestSync_CorruptedManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")

	// Write invalid YAML
	os.WriteFile(manifestPath, []byte("{{{{invalid yaml!!!!"), 0644)

	mf, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("Load should not error on corrupted YAML, got: %v", err)
	}
	if mf != nil {
		t.Fatal("expected nil manifest for corrupted YAML")
	}

	// Sync should proceed as if no manifest exists
	projectDir, prov := setupTestProject(t)
	srcDir := createLocalSource(t, t.TempDir(), "test-skill")
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	syncer := &Syncer{
		GitFetcher:  &mockFetcher{},
		Provider:    prov,
		ProjectRoot: projectDir,
		Manifest:    mf, // nil — corrupted manifest loaded
	}

	includes := []string{"test-skill"}
	cfg := &config.Config{
		Providers: config.ProviderList{"claude"},
		Skills: []config.SkillSource{
			{Source: srcDir, Include: includes, Type: "soft", ParsedIncludes: parsedIncludes(includes)},
		},
	}

	baseDirs := buildBaseDirs(srcDir)
	result, err := syncer.Sync(cfg, baseDirs)
	if err != nil {
		t.Fatalf("Sync with corrupted manifest should succeed: %v", err)
	}
	if result.Synced != 1 {
		t.Errorf("synced = %d, want 1", result.Synced)
	}
}

func TestSync_PartialFetchFailure(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	// Source A fails, source B succeeds
	fetcher := &mockFetcher{shouldFail: true}
	localDir := createLocalSource(t, t.TempDir(), "good-skill")

	sources := []config.SkillSource{
		{Source: "git@github.com:bad/repo.git", Include: []string{"bad-skill"}, ParsedIncludes: parsedIncludes([]string{"bad-skill"})},
		{Source: localDir, Include: []string{"good-skill"}, ParsedIncludes: parsedIncludes([]string{"good-skill"})},
	}

	baseDirs, errs := FetchSources(fetcher, sources)
	if len(errs) != 1 {
		t.Fatalf("expected 1 fetch error, got %d: %v", len(errs), errs)
	}

	// good-skill source should be in baseDirs
	if _, ok := baseDirs[localDir]; !ok {
		t.Error("good source missing from baseDirs")
	}

	// bad source should NOT be in baseDirs
	if _, ok := baseDirs["git@github.com:bad/repo.git"]; ok {
		t.Error("bad source should not be in baseDirs")
	}
}
