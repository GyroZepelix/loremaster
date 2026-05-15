package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"claude", "claude", false},
		{"opencode", "opencode", false},
		{"pi", "pi", false},
		{"codex", "codex", false},
		{"unknown", "unknown", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Get(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Name() != tt.input {
				t.Errorf("Name() = %q, want %q", p.Name(), tt.input)
			}
		})
	}
}

func TestAll(t *testing.T) {
	all := All()
	if len(all) != 4 {
		t.Fatalf("All() returned %d providers, want 4", len(all))
	}
	names := map[string]bool{}
	for _, p := range all {
		names[p.Name()] = true
	}
	if !names["claude"] {
		t.Error("All() missing claude")
	}
	if !names["opencode"] {
		t.Error("All() missing opencode")
	}
	if !names["pi"] {
		t.Error("All() missing pi")
	}
	if !names["codex"] {
		t.Error("All() missing codex")
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name   string
		dirs   []string
		expect []string
	}{
		{"claude only", []string{".claude"}, []string{"claude"}},
		{"opencode only", []string{".opencode"}, []string{"opencode"}},
		{"pi only", []string{".pi"}, []string{"pi"}},
		{"pi nested marker", []string{filepath.Join(".pi", "agent")}, []string{"pi"}},
		{"codex agents", []string{".agents"}, []string{"codex"}},
		{"codex legacy config", []string{".codex"}, []string{"codex"}},
		{"codex duplicate markers once", []string{".agents", ".codex"}, []string{"codex"}},
		{"all", []string{".claude", ".opencode", ".pi", ".agents"}, []string{"claude", "opencode", "pi", "codex"}},
		{"neither", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, d := range tt.dirs {
				os.MkdirAll(filepath.Join(dir, d), 0755)
			}

			detected, err := Detect(dir)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(detected) != len(tt.expect) {
				t.Fatalf("Detect returned %d providers, want %d", len(detected), len(tt.expect))
			}
			for i, p := range detected {
				if p.Name() != tt.expect[i] {
					t.Errorf("detected[%d].Name() = %q, want %q", i, p.Name(), tt.expect[i])
				}
			}
		})
	}
}

func TestClaudeSkillDir(t *testing.T) {
	c := &Claude{}
	got := c.SkillDir("/root", "foo")
	want := filepath.Join("/root", ".claude", "skills", "foo")
	if got != want {
		t.Errorf("SkillDir = %q, want %q", got, want)
	}
}

func TestOpenCodeSkillDir(t *testing.T) {
	o := &OpenCode{}
	got := o.SkillDir("/root", "foo")
	want := filepath.Join("/root", ".opencode", "skills", "foo")
	if got != want {
		t.Errorf("SkillDir = %q, want %q", got, want)
	}
}

func TestPiSkillDir(t *testing.T) {
	p := &Pi{}
	got := p.SkillDir("/root", "foo")
	want := filepath.Join("/root", ".pi", "skills", "foo")
	if got != want {
		t.Errorf("SkillDir = %q, want %q", got, want)
	}
}

func TestPiGlobalSkillDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	p := &Pi{}
	got := p.SkillDir(home, "foo")
	want := filepath.Join(home, ".pi", "agent", "skills", "foo")
	if got != want {
		t.Errorf("SkillDir = %q, want %q", got, want)
	}
}

func TestCodexSkillDir(t *testing.T) {
	c := &Codex{}
	got := c.SkillDir("/root", "foo")
	want := filepath.Join("/root", ".agents", "skills", "foo")
	if got != want {
		t.Errorf("SkillDir = %q, want %q", got, want)
	}
}

func TestProviderDirs(t *testing.T) {
	tests := []struct {
		name             string
		provider         Provider
		wantConfigDirs   []string
		wantMarkerDirs   []string
		wantDefaultDir   string
		wantSkillRootSub string
	}{
		{"claude", &Claude{}, []string{".claude"}, []string{".claude"}, ".claude", filepath.Join(".claude", "skills")},
		{"opencode", &OpenCode{}, []string{".opencode"}, []string{".opencode"}, ".opencode", filepath.Join(".opencode", "skills")},
		{"pi", &Pi{}, []string{".pi", filepath.Join(".pi", "agent")}, []string{".pi", filepath.Join(".pi", "agent")}, ".pi", filepath.Join(".pi", "skills")},
		{"codex", &Codex{}, []string{".agents", ".codex"}, []string{".agents", ".codex"}, ".agents", filepath.Join(".agents", "skills")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.ConfigDirs(); !sameStrings(got, tt.wantConfigDirs) {
				t.Errorf("ConfigDirs = %v, want %v", got, tt.wantConfigDirs)
			}
			if got := tt.provider.MarkerDirs(); !sameStrings(got, tt.wantMarkerDirs) {
				t.Errorf("MarkerDirs = %v, want %v", got, tt.wantMarkerDirs)
			}
			if got := tt.provider.DefaultConfigDir(); got != tt.wantDefaultDir {
				t.Errorf("DefaultConfigDir = %q, want %q", got, tt.wantDefaultDir)
			}
			wantRoot := filepath.Join("/root", tt.wantSkillRootSub)
			if got := tt.provider.SkillRoot("/root"); got != wantRoot {
				t.Errorf("SkillRoot = %q, want %q", got, wantRoot)
			}
		})
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
