package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileName(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{"empty profile", "", "lore.yml"},
		{"default profile", "default", "lore.yml"},
		{"dev profile", "dev", "lore-dev.yml"},
		{"complex profile", "my-staging_2", "lore-my-staging_2.yml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConfigFileName(tt.profile)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ConfigFileName(%q) = %q, want %q", tt.profile, got, tt.want)
			}
		})
	}

	// ConfigFileName rejects invalid profiles directly
	invalidTests := []struct {
		name    string
		profile string
	}{
		{"uppercase", "UPPER"},
		{"path traversal", "../../etc/passwd"},
		{"leading underscore", "_leading"},
	}
	for _, tt := range invalidTests {
		t.Run("invalid "+tt.name, func(t *testing.T) {
			_, err := ConfigFileName(tt.profile)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid profile name") {
				t.Errorf("error %q should contain %q", err.Error(), "invalid profile name")
			}
		})
	}
}

func TestLocateProfile(t *testing.T) {
	t.Run("empty profile delegates to Locate", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "lore.yml"), []byte("test"), 0644)
		got, err := LocateProfile(dir, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(dir, "lore.yml") {
			t.Errorf("got %q, want %q", got, filepath.Join(dir, "lore.yml"))
		}
	})

	t.Run("default profile delegates to Locate", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "lore.yml"), []byte("test"), 0644)
		got, err := LocateProfile(dir, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(dir, "lore.yml") {
			t.Errorf("got %q, want %q", got, filepath.Join(dir, "lore.yml"))
		}
	})

	t.Run("dev profile found in root", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "lore-dev.yml"), []byte("test"), 0644)
		got, err := LocateProfile(dir, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(dir, "lore-dev.yml") {
			t.Errorf("got %q, want %q", got, filepath.Join(dir, "lore-dev.yml"))
		}
	})

	t.Run("dev profile found in .claude", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
		os.WriteFile(filepath.Join(dir, ".claude", "lore-dev.yml"), []byte("test"), 0644)
		got, err := LocateProfile(dir, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(dir, ".claude", "lore-dev.yml") {
			t.Errorf("got %q, want %q", got, filepath.Join(dir, ".claude", "lore-dev.yml"))
		}
	})

	t.Run("dev profile found in .opencode", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".opencode"), 0755)
		os.WriteFile(filepath.Join(dir, ".opencode", "lore-dev.yml"), []byte("test"), 0644)
		got, err := LocateProfile(dir, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(dir, ".opencode", "lore-dev.yml") {
			t.Errorf("got %q, want %q", got, filepath.Join(dir, ".opencode", "lore-dev.yml"))
		}
	})

	t.Run("dev profile not found", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LocateProfile(dir, "dev")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no lore-dev.yml found") {
			t.Errorf("error %q should contain %q", err.Error(), "no lore-dev.yml found")
		}
	})

	t.Run("priority order root wins over .claude", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "lore-dev.yml"), []byte("root"), 0644)
		os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
		os.WriteFile(filepath.Join(dir, ".claude", "lore-dev.yml"), []byte("claude"), 0644)
		got, err := LocateProfile(dir, "dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(dir, "lore-dev.yml") {
			t.Errorf("got %q, want root path %q", got, filepath.Join(dir, "lore-dev.yml"))
		}
	})

	// Invalid profile names
	invalidTests := []struct {
		name    string
		profile string
	}{
		{"uppercase", "UPPER"},
		{"has spaces", "has spaces"},
		{"too long", strings.Repeat("a", 65)},
		{"leading underscore", "_leading"},
		{"leading hyphen", "-leading"},
	}
	for _, tt := range invalidTests {
		t.Run("invalid "+tt.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := LocateProfile(dir, tt.profile)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "invalid profile name") {
				t.Errorf("error %q should contain %q", err.Error(), "invalid profile name")
			}
		})
	}
}
