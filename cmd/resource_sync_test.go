package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/manifest"
)

func TestRunSyncResourcesPiAndClaude(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	os.MkdirAll(filepath.Join(source, "example-skill"), 0755)
	os.WriteFile(filepath.Join(source, "example-skill", "SKILL.md"), []byte("skill"), 0644)
	os.WriteFile(filepath.Join(source, "review.md"), []byte("review"), 0644)
	os.WriteFile(filepath.Join(source, "tool.json"), []byte("tool"), 0644)

	configContent := fmt.Sprintf(`provider: [pi, claude]
skills:
  - source: %q
    include: [example-skill]
prompts:
  - source: %q
    include: [review.md]
hooks/tools:
  - source: %q
    include: [tool.json]
`, source, source, source)
	os.WriteFile(filepath.Join(home, "lore.yml"), []byte(configContent), 0644)

	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, home)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false

	if err := runSync(nil, nil); err != nil {
		t.Fatalf("first runSync: %v", err)
	}
	if err := runSync(nil, nil); err != nil {
		t.Fatalf("idempotent runSync: %v", err)
	}

	paths := []string{
		filepath.Join(home, ".pi", "agent", "skills", "example-skill"),
		filepath.Join(home, ".claude", "skills", "example-skill"),
		filepath.Join(home, ".pi", "agent", "prompts", "review.md"),
		filepath.Join(home, ".claude", "prompts", "review.md"),
		filepath.Join(home, ".pi", "agent", "hooks", "tools", "tool.json"),
		filepath.Join(home, ".claude", "hooks", "tools", "tool.json"),
	}
	for _, path := range paths {
		if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected symlink at %s: info=%v err=%v", path, info, err)
		}
	}

	mf, err := manifest.Load(filepath.Join(home, ".lore-manifest.yml"))
	if err != nil || mf == nil {
		t.Fatalf("manifest load: %v, %#v", err, mf)
	}
	items, _ := mf.GetProfileItems("default")
	if len(items) != 6 {
		t.Fatalf("manifest items = %d, want 6: %#v", len(items), items)
	}
}

func TestRunSyncResourceFanoutAllProviders(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "review.md"), []byte("review"), 0644)
	os.WriteFile(filepath.Join(project, "lore.yml"), []byte(fmt.Sprintf("provider: [claude, opencode, pi, codex]\nprompts:\n  - source: %q\n    include: [review.md]\n", source)), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false
	if err := runSync(nil, nil); err != nil {
		t.Fatalf("runSync: %v", err)
	}

	for _, relative := range []string{
		".claude/prompts/review.md",
		".opencode/prompts/review.md",
		".pi/prompts/review.md",
		".agents/prompts/review.md",
	} {
		if info, err := os.Lstat(filepath.Join(project, relative)); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected symlink at %s: info=%v err=%v", relative, info, err)
		}
	}
}

func TestRunSyncRefusesUnmanagedResource(t *testing.T) {
	project := t.TempDir()
	source := t.TempDir()
	os.WriteFile(filepath.Join(source, "review.md"), []byte("managed"), 0644)
	target := filepath.Join(project, ".claude", "prompts", "review.md")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.WriteFile(target, []byte("user"), 0644)
	os.WriteFile(filepath.Join(project, "lore.yml"), []byte(fmt.Sprintf("provider: claude\nprompts:\n  - source: %q\n    include: [review.md]\n", source)), 0644)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))
	withWorkingDirectory(t, project)
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() { profileFlag, pruneFlag = oldProfile, oldPrune })
	profileFlag, pruneFlag = "", false

	if err := runSync(nil, nil); err == nil {
		t.Fatal("expected unmanaged destination error")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "user" {
		t.Fatalf("unmanaged target changed: %q", content)
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
