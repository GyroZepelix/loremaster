package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestV2RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yml")
	m := New()
	m.SetProfileItems("default", []Item{
		{
			Path:     ".pi/prompts/review.md",
			Provider: "pi",
			Resource: "prompts",
			Mode:     "hard",
			Kind:     "file",
			Checksum: "abc123",
		},
	})
	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "path: .pi/prompts/review.md") || !strings.Contains(string(data), "checksum: abc123") {
		t.Fatalf("structured manifest not written:\n%s", data)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != 2 {
		t.Fatalf("Version = %d, want 2", loaded.Version)
	}
	items, ok := loaded.GetProfileItems("default")
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, ok = %v", items, ok)
	}
	if items[0].Kind != "file" || items[0].Checksum != "abc123" {
		t.Fatalf("item = %#v", items[0])
	}
}

func TestLoadManifestV1AsLegacyItems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yml")
	if err := os.WriteFile(path, []byte("version: 1\nprofiles:\n  default:\n    - .claude/skills/review\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Version != 2 {
		t.Fatalf("Version = %d, want normalized version 2", m.Version)
	}
	items, _ := m.GetProfileItems("default")
	if len(items) != 1 || !items[0].Legacy || items[0].Path != ".claude/skills/review" {
		t.Fatalf("legacy items = %#v", items)
	}
}

func TestLoadUnsupportedManifestVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yml")
	if err := os.WriteFile(path, []byte("version: 99\nprofiles: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unsupported manifest version") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsMalformedVersionTwoItems(t *testing.T) {
	tests := []struct {
		name string
		item string
	}{
		{"unknown field", "path: .claude/prompts/review.md\n      provider: claude\n      resource: prompts\n      mode: soft\n      kind: file\n      target: /source/review.md\n      unexpected: true"},
		{"soft missing target", "path: .claude/prompts/review.md\n      provider: claude\n      resource: prompts\n      mode: soft\n      kind: file"},
		{"escaping path", "path: ../review.md\n      provider: claude\n      resource: prompts\n      mode: hard\n      kind: file\n      checksum: abc"},
		{"forged legacy state", "path: .claude/skills/review\n      provider: claude\n      resource: skills\n      legacy: true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.yml")
			data := "version: 2\nprofiles:\n  default:\n    - " + tt.item + "\n"
			if err := os.WriteFile(path, []byte(data), 0644); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if err != nil || loaded != nil {
				t.Fatalf("Load = %#v, error = %v, want corrupt manifest result", loaded, err)
			}
		})
	}
}

func TestSaveRejectsIncompleteVersionTwoItem(t *testing.T) {
	m := New()
	m.SetProfileItems("default", []Item{{Path: ".claude/prompts/review.md", Provider: "claude", Resource: "prompts", Mode: "soft", Kind: "file"}})
	if err := Save(filepath.Join(t.TempDir(), "manifest.yml"), m); err == nil || !strings.Contains(err.Error(), "no recorded target") {
		t.Fatalf("error = %v", err)
	}
}

func TestManifestPreservesPerProfileMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yml")
	m := New()
	m.SetProfileItems("dev", []Item{{Path: ".claude/skills/review", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/dev/review"}})
	m.SetProfileItems("staging", []Item{{Path: ".claude/skills/review", Provider: "claude", Resource: "skills", Mode: "soft", Kind: "directory", Target: "/staging/review"}})
	if err := Save(path, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded == nil {
		t.Fatalf("Load: %#v, %v", loaded, err)
	}
	dev, _ := loaded.GetProfileItems("dev")
	staging, _ := loaded.GetProfileItems("staging")
	if dev[0].Target != "/dev/review" || staging[0].Target != "/staging/review" {
		t.Fatalf("metadata collapsed: dev=%#v staging=%#v", dev, staging)
	}
}

func TestManifestOwner(t *testing.T) {
	m := New()
	m.SetProfileItems("dev", []Item{{Path: ".claude/prompts/review.md", Provider: "claude", Resource: "prompts", Mode: "soft", Kind: "file"}})

	owner, item, ok := m.Owner(".claude/prompts/review.md")
	if !ok || owner != "dev" || item.Resource != "prompts" {
		t.Fatalf("owner = %q, item = %#v, ok = %v", owner, item, ok)
	}
	if _, _, ok := m.Owner("missing"); ok {
		t.Fatal("missing path unexpectedly has owner")
	}
}
