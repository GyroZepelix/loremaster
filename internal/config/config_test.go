package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name: "valid minimal config",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: [foo]
`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Provider != "claude" {
					t.Errorf("provider = %q, want claude", cfg.Provider)
				}
				if len(cfg.Skills) != 1 {
					t.Fatalf("skills len = %d, want 1", len(cfg.Skills))
				}
				if cfg.Skills[0].Type != "soft" {
					t.Errorf("type = %q, want soft (default)", cfg.Skills[0].Type)
				}
			},
		},
		{
			name: "valid full config",
			input: `
provider: opencode
skills:
  - source: https://github.com/user/repo.git
    ref: v1.0.0
    include: [foo, bar]
    type: hard
  - source: /home/user/local
    include: [baz]
    type: soft
`,
			check: func(t *testing.T, cfg *Config) {
				if cfg.Provider != "opencode" {
					t.Errorf("provider = %q, want opencode", cfg.Provider)
				}
				if len(cfg.Skills) != 2 {
					t.Fatalf("skills len = %d, want 2", len(cfg.Skills))
				}
				if cfg.Skills[0].Ref != "v1.0.0" {
					t.Errorf("ref = %q, want v1.0.0", cfg.Skills[0].Ref)
				}
				if cfg.Skills[0].Type != "hard" {
					t.Errorf("type = %q, want hard", cfg.Skills[0].Type)
				}
			},
		},
		{
			name:    "missing provider",
			input:   "skills:\n  - source: x\n    include: [a]\n",
			wantErr: "missing required field: provider",
		},
		{
			name:    "invalid provider",
			input:   "provider: cursor\nskills:\n  - source: x\n    include: [a]\n",
			wantErr: "invalid provider",
		},
		{
			name:    "empty skills",
			input:   "provider: claude\nskills: []\n",
			wantErr: "at least one entry",
		},
		{
			name:    "missing include",
			input:   "provider: claude\nskills:\n  - source: x\n",
			wantErr: "missing required field: include",
		},
		{
			name:    "missing source",
			input:   "provider: claude\nskills:\n  - include: [a]\n",
			wantErr: "missing required field: source",
		},
		{
			name:    "invalid type",
			input:   "provider: claude\nskills:\n  - source: x\n    include: [a]\n    type: link\n",
			wantErr: "invalid type",
		},
		{
			name:    "skill name with path separator",
			input:   "provider: claude\nskills:\n  - source: x\n    include: [../../.ssh]\n",
			wantErr: "must not contain path separators",
		},
		{
			name:    "skill name with dot-dot",
			input:   "provider: claude\nskills:\n  - source: x\n    include: [..]\n",
			wantErr: "must be a simple directory name",
		},
		{
			name:    "skill name is dot",
			input:   "provider: claude\nskills:\n  - source: x\n    include: [.]\n",
			wantErr: "must be a simple directory name",
		},
		{
			name:  "type defaults to soft",
			input: "provider: claude\nskills:\n  - source: x\n    include: [a]\n",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Skills[0].Type != "soft" {
					t.Errorf("type = %q, want soft", cfg.Skills[0].Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(strings.NewReader(tt.input))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestIsGitSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"git@github.com:user/repo.git", true},
		{"https://github.com/user/repo.git", true},
		{"ssh://git@github.com/user/repo", true},
		{"/home/user/local-skills", false},
		{"./relative/path", false},
		{"../other/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			if got := IsGitSource(tt.source); got != tt.want {
				t.Errorf("IsGitSource(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestLocate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(dir string)
		wantSub string
		wantErr bool
	}{
		{
			name: "lore.yml in CWD",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "lore.yml"), []byte("provider: claude\n"), 0644)
			},
			wantSub: "lore.yml",
		},
		{
			name: "lore.yml in .claude",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
				os.WriteFile(filepath.Join(dir, ".claude", "lore.yml"), []byte("provider: claude\n"), 0644)
			},
			wantSub: ".claude/lore.yml",
		},
		{
			name: "lore.yml in .opencode",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".opencode"), 0755)
				os.WriteFile(filepath.Join(dir, ".opencode", "lore.yml"), []byte("provider: opencode\n"), 0644)
			},
			wantSub: ".opencode/lore.yml",
		},
		{
			name:    "not found",
			setup:   func(dir string) {},
			wantErr: true,
		},
		{
			name: "CWD takes priority over .claude",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "lore.yml"), []byte("provider: claude\n"), 0644)
				os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
				os.WriteFile(filepath.Join(dir, ".claude", "lore.yml"), []byte("provider: claude\n"), 0644)
			},
			wantSub: "lore.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)

			got, err := Locate(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			expected := filepath.Join(dir, tt.wantSub)
			if got != expected {
				t.Errorf("Locate() = %q, want %q", got, expected)
			}
		})
	}
}
