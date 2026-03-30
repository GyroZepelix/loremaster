package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/gitignore"
	loresync "github.com/GyroZepelix/loremaster/internal/sync"
)

func TestCleanRemovedProviders_SymlinkRemoval(t *testing.T) {
	projectDir := t.TempDir()

	// Create symlinks under both .claude/skills/foo and .opencode/skills/foo
	for _, dir := range []string{".claude/skills", ".opencode/skills"} {
		skillsDir := filepath.Join(projectDir, dir)
		os.MkdirAll(skillsDir, 0755)
		// Create a symlink target
		targetDir := filepath.Join(t.TempDir(), "target", filepath.Base(dir))
		os.MkdirAll(targetDir, 0755)
		os.WriteFile(filepath.Join(targetDir, "workflow.md"), []byte("# test"), 0644)
		os.Symlink(targetDir, filepath.Join(skillsDir, "foo"))
	}

	// Setup gitignore with both entries
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	gitignore.EnsureEntries(gitignorePath, []string{".claude/skills/foo", ".opencode/skills/foo"})

	snapshot := []string{".claude/skills/foo", ".opencode/skills/foo"}

	// Remove opencode provider (only claude remains)
	removed, errs := cleanRemovedProviders(snapshot, config.ProviderList{"claude"}, projectDir, gitignorePath)

	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}

	// .opencode/skills/foo should be removed
	ocPath := filepath.Join(projectDir, ".opencode", "skills", "foo")
	if _, err := os.Lstat(ocPath); err == nil {
		t.Error(".opencode/skills/foo should be removed")
	}

	// .claude/skills/foo should be untouched
	claudePath := filepath.Join(projectDir, ".claude", "skills", "foo")
	if _, err := os.Lstat(claudePath); err != nil {
		t.Error(".claude/skills/foo should be untouched")
	}

	// Removed list should contain only opencode entry
	if len(removed) != 1 {
		t.Fatalf("removed = %d, want 1", len(removed))
	}
	if removed[0] != ".opencode/skills/foo" {
		t.Errorf("removed[0] = %q, want .opencode/skills/foo", removed[0])
	}

	// Gitignore should have claude entry but not opencode
	gitContent, _ := os.ReadFile(gitignorePath)
	s := string(gitContent)
	if !strings.Contains(s, ".claude/skills/foo") {
		t.Error("gitignore should still contain .claude/skills/foo")
	}
	if strings.Contains(s, ".opencode/skills/foo") {
		t.Error("gitignore should not contain .opencode/skills/foo")
	}
}

func TestCleanRemovedProviders_HardCopy_ModifiedSkipped(t *testing.T) {
	projectDir := t.TempDir()

	// Create a hard copy under .opencode/skills/foo with .lore-checksum
	skillDir := filepath.Join(projectDir, ".opencode", "skills", "foo")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# original"), 0644)

	// Compute and write checksum
	checksum, err := loresync.ComputeDirChecksum(skillDir)
	if err != nil {
		t.Fatalf("compute checksum: %v", err)
	}
	os.WriteFile(filepath.Join(skillDir, ".lore-checksum"), []byte(checksum), 0644)

	// Modify a file (so checksum mismatches)
	os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# MODIFIED"), 0644)

	snapshot := []string{".opencode/skills/foo"}
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	removed, errs := cleanRemovedProviders(snapshot, config.ProviderList{"claude"}, projectDir, gitignorePath)

	// .opencode/skills/foo should NOT be removed (local modifications)
	if _, err := os.Stat(skillDir); err != nil {
		t.Error(".opencode/skills/foo should be preserved (modified)")
	}

	if len(removed) != 0 {
		t.Errorf("removed = %d, want 0 (modified hard copy skipped)", len(removed))
	}

	// Should have a warning about local modifications
	foundWarning := false
	for _, e := range errs {
		if strings.Contains(e, "local modifications") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Error("expected warning about local modifications")
	}
}
