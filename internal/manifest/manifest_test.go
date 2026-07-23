package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	m := New()
	if m.Version != CurrentVersion {
		t.Fatalf("Version = %d, want %d", m.Version, CurrentVersion)
	}
	if m.Profiles == nil {
		t.Fatal("Profiles is nil, want initialized map")
	}
	if len(m.Profiles) != 0 {
		t.Fatalf("len(Profiles) = %d, want 0", len(m.Profiles))
	}
}

func TestLoadManifest(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string) string
		wantNil bool
		check   func(t *testing.T, m *Manifest)
	}{
		{
			name: "valid YAML",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "manifest.yml")
				data := "version: 1\nprofiles:\n  default:\n    - .claude/skills/brainstorm\n  dev:\n    - .claude/skills/debug-tool\n"
				if err := os.WriteFile(p, []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			check: func(t *testing.T, m *Manifest) {
				if m.Version != CurrentVersion {
					t.Fatalf("Version = %d, want %d", m.Version, CurrentVersion)
				}
				if len(m.Profiles) != 2 {
					t.Fatalf("len(Profiles) = %d, want 2", len(m.Profiles))
				}
				entries, ok := m.Profiles["default"]
				if !ok {
					t.Fatal("missing 'default' profile")
				}
				if len(entries) != 1 || entries[0] != ".claude/skills/brainstorm" {
					t.Fatalf("default entries = %v, want [.claude/skills/brainstorm]", entries)
				}
			},
		},
		{
			name: "missing file returns nil nil",
			setup: func(t *testing.T, dir string) string {
				return filepath.Join(dir, "nonexistent.yml")
			},
			wantNil: true,
		},
		{
			name: "corrupted YAML returns nil nil",
			setup: func(t *testing.T, dir string) string {
				p := filepath.Join(dir, "corrupt.yml")
				if err := os.WriteFile(p, []byte("{{{{not yaml at all::::"), 0644); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := tt.setup(t, dir)

			m, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if tt.wantNil {
				if m != nil {
					t.Fatalf("Load() = %+v, want nil", m)
				}
				return
			}
			if m == nil {
				t.Fatal("Load() = nil, want non-nil manifest")
			}
			if tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}

func TestSaveManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yml")

	m := New()
	m.SetProfileItems("default", []Item{
		{Path: ".claude/skills/brainstorm", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/cache/brainstorm"},
		{Path: ".claude/skills/commit", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/cache/commit"},
	})
	m.SetProfileItems("dev", []Item{{Path: ".claude/skills/debug-tool", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/cache/debug-tool"}})

	if err := Save(path, m); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file does not exist: %v", err)
	}

	// Verify no temp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".lore-manifest-") && strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file not cleaned up: %s", e.Name())
		}
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yml")

	original := New()
	original.SetProfileItems("default", []Item{
		{Path: ".claude/skills/brainstorm", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/cache/brainstorm"},
		{Path: ".claude/skills/commit", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/cache/commit"},
	})
	original.SetProfileItems("dev", []Item{{Path: ".claude/skills/debug-tool", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/cache/debug-tool"}})

	if err := Save(path, original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("Load() = nil after Save()")
	}

	if loaded.Version != original.Version {
		t.Fatalf("Version = %d, want %d", loaded.Version, original.Version)
	}
	if len(loaded.Profiles) != len(original.Profiles) {
		t.Fatalf("len(Profiles) = %d, want %d", len(loaded.Profiles), len(original.Profiles))
	}
	for name, wantEntries := range original.Profiles {
		gotEntries, ok := loaded.Profiles[name]
		if !ok {
			t.Fatalf("missing profile %q after roundtrip", name)
		}
		if len(gotEntries) != len(wantEntries) {
			t.Fatalf("profile %q: len = %d, want %d", name, len(gotEntries), len(wantEntries))
		}
		for i := range wantEntries {
			if gotEntries[i] != wantEntries[i] {
				t.Fatalf("profile %q[%d] = %q, want %q", name, i, gotEntries[i], wantEntries[i])
			}
		}
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	// File does not exist
	if Exists(filepath.Join(dir, "nope.yml")) {
		t.Fatal("Exists() = true for missing file")
	}

	// File exists
	path := filepath.Join(dir, "manifest.yml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !Exists(path) {
		t.Fatal("Exists() = false for existing file")
	}
}

func TestSetProfile(t *testing.T) {
	m := New()
	entries := []string{".claude/skills/brainstorm"}
	m.SetProfile("dev", entries)

	got, ok := m.Profiles["dev"]
	if !ok {
		t.Fatal("SetProfile did not store profile")
	}
	if len(got) != 1 || got[0] != ".claude/skills/brainstorm" {
		t.Fatalf("got %v, want %v", got, entries)
	}

	// Calling again replaces
	m.SetProfile("dev", []string{".claude/skills/other"})
	got, _ = m.Profiles["dev"]
	if len(got) != 1 || got[0] != ".claude/skills/other" {
		t.Fatalf("SetProfile did not replace: got %v", got)
	}
}

func TestGetProfile(t *testing.T) {
	m := New()
	m.Profiles["dev"] = []string{".claude/skills/debug-tool"}

	entries, ok := m.GetProfile("dev")
	if !ok {
		t.Fatal("GetProfile() ok = false for existing profile")
	}
	if len(entries) != 1 || entries[0] != ".claude/skills/debug-tool" {
		t.Fatalf("entries = %v, want [.claude/skills/debug-tool]", entries)
	}

	_, ok = m.GetProfile("nonexistent")
	if ok {
		t.Fatal("GetProfile() ok = true for nonexistent profile")
	}
}

func TestRemoveProfile(t *testing.T) {
	m := New()
	m.Profiles["dev"] = []string{".claude/skills/debug-tool"}

	m.RemoveProfile("dev")
	_, ok := m.GetProfile("dev")
	if ok {
		t.Fatal("profile still exists after RemoveProfile")
	}
}

func TestProfileNames(t *testing.T) {
	m := New()
	m.Profiles["dev"] = []string{}
	m.Profiles["default"] = []string{}
	m.Profiles["staging"] = []string{}

	names := m.ProfileNames()
	want := []string{"default", "dev", "staging"}
	if len(names) != len(want) {
		t.Fatalf("len = %d, want %d", len(names), len(want))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestScanManagedEntries(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	skillsDir := filepath.Join(dir, "skills")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a cached skill target
	cachedSkill := filepath.Join(cacheDir, "brainstorm")
	if err := os.MkdirAll(cachedSkill, 0755); err != nil {
		t.Fatal(err)
	}

	// Symlink pointing into cache
	symSkill := filepath.Join(skillsDir, "brainstorm")
	if err := os.Symlink(cachedSkill, symSkill); err != nil {
		t.Fatal(err)
	}

	// Hard copy with .lore-checksum
	hardSkill := filepath.Join(skillsDir, "commit")
	if err := os.MkdirAll(hardSkill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hardSkill, ".lore-checksum"), []byte("abc123"), 0644); err != nil {
		t.Fatal(err)
	}

	// User-created dir (no symlink, no .lore-checksum)
	userSkill := filepath.Join(skillsDir, "my-custom-skill")
	if err := os.MkdirAll(userSkill, 0755); err != nil {
		t.Fatal(err)
	}

	entries, err := ScanManagedEntries(skillsDir, cacheDir)
	if err != nil {
		t.Fatalf("ScanManagedEntries() error = %v", err)
	}

	// Should find brainstorm (symlink) and commit (checksum), but not my-custom-skill
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2; got %v", len(entries), entries)
	}

	// Entries should be sorted
	if entries[0] != "brainstorm" || entries[1] != "commit" {
		t.Fatalf("entries = %v, want [brainstorm commit]", entries)
	}
}

func TestScanManagedEntriesNested(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	skillsDir := filepath.Join(dir, "skills")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Nested symlink: skills/loa/brainstorm -> cache/brainstorm
	cachedSkill := filepath.Join(cacheDir, "brainstorm")
	if err := os.MkdirAll(cachedSkill, 0755); err != nil {
		t.Fatal(err)
	}
	nestedDir := filepath.Join(skillsDir, "loa")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cachedSkill, filepath.Join(nestedDir, "brainstorm")); err != nil {
		t.Fatal(err)
	}

	entries, err := ScanManagedEntries(skillsDir, cacheDir)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(entries) != 1 || entries[0] != "loa/brainstorm" {
		t.Fatalf("entries = %v, want [loa/brainstorm]", entries)
	}
}

func TestScanManagedEntriesRelativeSymlink(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	skillsDir := filepath.Join(dir, "skills")

	if err := os.MkdirAll(filepath.Join(cacheDir, "brainstorm"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a relative symlink: skills/brainstorm -> ../cache/brainstorm
	relTarget := filepath.Join("..", "cache", "brainstorm")
	if err := os.Symlink(relTarget, filepath.Join(skillsDir, "brainstorm")); err != nil {
		t.Fatal(err)
	}

	entries, err := ScanManagedEntries(skillsDir, cacheDir)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(entries) != 1 || entries[0] != "brainstorm" {
		t.Fatalf("entries = %v, want [brainstorm]", entries)
	}
}

func TestScanManagedEntriesEmpty(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache")
	skillsDir := filepath.Join(dir, "skills")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries, err := ScanManagedEntries(skillsDir, cacheDir)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len = %d, want 0", len(entries))
	}
}

func TestFindOrphaned(t *testing.T) {
	m := New()
	m.Profiles["existing"] = []string{".claude/skills/brainstorm"}
	m.Profiles["missing"] = []string{".claude/skills/debug-tool"}
	m.Profiles["also-missing"] = []string{".claude/skills/other"}

	locateFn := func(dir, profile string) (string, error) {
		if profile == "existing" {
			return "/some/path/lore.yml", nil
		}
		return "", os.ErrNotExist
	}

	orphaned := m.FindOrphaned("/some/dir", locateFn)

	// Should be sorted
	if len(orphaned) != 2 {
		t.Fatalf("len = %d, want 2; got %v", len(orphaned), orphaned)
	}
	if orphaned[0] != "also-missing" || orphaned[1] != "missing" {
		t.Fatalf("orphaned = %v, want [also-missing missing]", orphaned)
	}
}

func TestFindOrphanedNoneOrphaned(t *testing.T) {
	m := New()
	m.Profiles["a"] = []string{}
	m.Profiles["b"] = []string{}

	locateFn := func(dir, profile string) (string, error) {
		return "/some/path", nil
	}

	orphaned := m.FindOrphaned("/some/dir", locateFn)
	if len(orphaned) != 0 {
		t.Fatalf("len = %d, want 0; got %v", len(orphaned), orphaned)
	}
}

func TestFindOrphanedDefault(t *testing.T) {
	m := New()
	m.Profiles["default"] = []string{".claude/skills/brainstorm"}

	called := false
	locateFn := func(dir, profile string) (string, error) {
		called = true
		if profile != "default" {
			t.Fatalf("locateFn called with profile %q, want 'default'", profile)
		}
		return "", os.ErrNotExist
	}

	orphaned := m.FindOrphaned("/some/dir", locateFn)
	if !called {
		t.Fatal("locateFn was not called for 'default' profile")
	}
	if len(orphaned) != 1 || orphaned[0] != "default" {
		t.Fatalf("orphaned = %v, want [default]", orphaned)
	}
}
