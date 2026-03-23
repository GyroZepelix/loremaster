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
	if len(all) != 2 {
		t.Fatalf("All() returned %d providers, want 2", len(all))
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
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name   string
		dirs   []string
		expect []string
	}{
		{"claude only", []string{".claude"}, []string{"claude"}},
		{"opencode only", []string{".opencode"}, []string{"opencode"}},
		{"both", []string{".claude", ".opencode"}, []string{"claude", "opencode"}},
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

func TestClaudeMarkerDir(t *testing.T) {
	c := &Claude{}
	if got := c.MarkerDir(); got != ".claude" {
		t.Errorf("MarkerDir = %q, want .claude", got)
	}
}

func TestOpenCodeMarkerDir(t *testing.T) {
	o := &OpenCode{}
	if got := o.MarkerDir(); got != ".opencode" {
		t.Errorf("MarkerDir = %q, want .opencode", got)
	}
}
