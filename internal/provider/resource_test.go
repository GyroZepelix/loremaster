package provider

import (
	"path/filepath"
	"testing"
)

func TestProviderResourceRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	tests := []struct {
		name     string
		provider Provider
		wantRoot string
	}{
		{"claude", &Claude{}, filepath.Join(root, ".claude")},
		{"opencode", &OpenCode{}, filepath.Join(root, ".opencode")},
		{"pi", &Pi{}, filepath.Join(root, ".pi")},
		{"codex", &Codex{}, filepath.Join(root, ".agents")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.ConfigRoot(root); got != tt.wantRoot {
				t.Fatalf("ConfigRoot = %q, want %q", got, tt.wantRoot)
			}
			wantItem := filepath.Join(tt.wantRoot, "hooks", "tools", "check.sh")
			if got := tt.provider.ResourceDir(root, "hooks/tools", "check.sh"); got != wantItem {
				t.Fatalf("ResourceDir = %q, want %q", got, wantItem)
			}
			if got := tt.provider.SkillRoot(root); got != filepath.Join(tt.wantRoot, "skills") {
				t.Fatalf("SkillRoot = %q", got)
			}
		})
	}
}

func TestPiGlobalResourceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	pi := &Pi{}
	want := filepath.Join(home, ".pi", "agent")
	if got := pi.ConfigRoot(home); got != want {
		t.Fatalf("ConfigRoot = %q, want %q", got, want)
	}
	if got := pi.ResourceDir(home, "prompts", "review.md"); got != filepath.Join(want, "prompts", "review.md") {
		t.Fatalf("ResourceDir = %q", got)
	}
}
