package gitignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureEntries_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	err := EnsureEntries(path, []string{".claude/skills/foo", ".claude/skills/bar"})
	if err != nil {
		t.Fatalf("EnsureEntries: %v", err)
	}

	content, _ := os.ReadFile(path)
	s := string(content)

	if !strings.Contains(s, header) {
		t.Error("missing header")
	}
	if !strings.Contains(s, ".claude/skills/foo") {
		t.Error("missing foo entry")
	}
	if !strings.Contains(s, ".claude/skills/bar") {
		t.Error("missing bar entry")
	}
}

func TestEnsureEntries_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	os.WriteFile(path, []byte("node_modules/\n.env\n"), 0644)

	err := EnsureEntries(path, []string{".claude/skills/foo"})
	if err != nil {
		t.Fatalf("EnsureEntries: %v", err)
	}

	content, _ := os.ReadFile(path)
	s := string(content)

	if !strings.Contains(s, "node_modules/") {
		t.Error("existing entries removed")
	}
	if !strings.Contains(s, ".claude/skills/foo") {
		t.Error("new entry not added")
	}
}

func TestEnsureEntries_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	entries := []string{".claude/skills/foo", ".claude/skills/bar"}
	EnsureEntries(path, entries)
	first, _ := os.ReadFile(path)

	EnsureEntries(path, entries)
	second, _ := os.ReadFile(path)

	if string(first) != string(second) {
		t.Errorf("not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEnsureEntries_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	EnsureEntries(path, []string{".claude/skills/foo"})
	EnsureEntries(path, []string{".claude/skills/foo", ".claude/skills/bar"})

	content, _ := os.ReadFile(path)
	count := strings.Count(string(content), ".claude/skills/foo")
	if count != 1 {
		t.Errorf("foo appears %d times, want 1", count)
	}
}

func TestRemoveEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	EnsureEntries(path, []string{".claude/skills/foo", ".claude/skills/bar"})
	err := RemoveEntries(path, []string{".claude/skills/foo"})
	if err != nil {
		t.Fatalf("RemoveEntries: %v", err)
	}

	content, _ := os.ReadFile(path)
	s := string(content)

	if strings.Contains(s, ".claude/skills/foo") {
		t.Error("foo not removed")
	}
	if !strings.Contains(s, ".claude/skills/bar") {
		t.Error("bar was removed")
	}
	if !strings.Contains(s, header) {
		t.Error("header removed while entries remain")
	}
}

func TestRemoveEntries_CleansHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	EnsureEntries(path, []string{".claude/skills/foo"})
	RemoveEntries(path, []string{".claude/skills/foo"})

	content, _ := os.ReadFile(path)
	s := string(content)

	if strings.Contains(s, header) {
		t.Error("header should be removed when section is empty")
	}
}

func TestManagedEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	os.WriteFile(path, []byte("node_modules/\n\n"+header+"\n.claude/skills/foo\n.claude/skills/bar\n"), 0644)

	entries, err := ManagedEntries(path)
	if err != nil {
		t.Fatalf("ManagedEntries: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0] != ".claude/skills/foo" || entries[1] != ".claude/skills/bar" {
		t.Errorf("entries = %v", entries)
	}
}

func TestRemoveEntries_NonexistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	err := RemoveEntries(path, []string{"foo"})
	if err != nil {
		t.Fatalf("RemoveEntries on nonexistent file: %v", err)
	}
}
