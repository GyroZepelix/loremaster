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
				if len(cfg.Providers) != 1 || cfg.Providers[0] != "claude" {
					t.Errorf("providers = %v, want [claude]", cfg.Providers)
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
				if len(cfg.Providers) != 1 || cfg.Providers[0] != "opencode" {
					t.Errorf("providers = %v, want [opencode]", cfg.Providers)
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
		// --- Provider parsing (AC 1-6, 15) ---
		{
			name: "scalar provider backward compat",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: [foo]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Providers) != 1 || cfg.Providers[0] != "claude" {
					t.Errorf("providers = %v, want [claude]", cfg.Providers)
				}
			},
		},
		{
			name: "list provider multi-provider",
			input: `
provider: [claude, opencode]
skills:
  - source: git@github.com:user/repo.git
    include: [foo]
`,
			check: func(t *testing.T, cfg *Config) {
				want := []string{"claude", "opencode"}
				if len(cfg.Providers) != len(want) {
					t.Fatalf("providers = %v, want %v", cfg.Providers, want)
				}
				for i, w := range want {
					if cfg.Providers[i] != w {
						t.Errorf("providers[%d] = %q, want %q", i, cfg.Providers[i], w)
					}
				}
			},
		},
		{
			name:    "duplicate provider",
			input:   "provider: [claude, claude]\nskills:\n  - source: x\n    include: [a]\n",
			wantErr: "duplicate provider",
		},
		{
			name:    "empty provider list",
			input:   "provider: []\nskills:\n  - source: x\n    include: [a]\n",
			wantErr: "missing required field: provider",
		},
		{
			name:    "null provider",
			input:   "provider:\nskills:\n  - source: x\n    include: [a]\n",
			wantErr: "missing required field: provider",
		},
		{
			name:    "mapping provider rejected",
			input:   "provider: {foo: bar}\nskills:\n  - source: x\n    include: [a]\n",
			wantErr: "invalid provider type",
		},
		{
			name:    "invalid provider in list",
			input:   "provider: [unknown]\nskills:\n  - source: x\n    include: [a]\n",
			wantErr: "invalid provider",
		},
		{
			name:    "missing provider",
			input:   "skills:\n  - source: x\n    include: [a]\n",
			wantErr: "missing required field: provider",
		},
		{
			name:    "invalid provider scalar",
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
		// --- Include path parsing (AC 7-10, 13) ---
		{
			name: "include path with slash is valid",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: [loa/brainstorm]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Skills[0].ParsedIncludes) != 1 {
					t.Fatalf("ParsedIncludes len = %d, want 1", len(cfg.Skills[0].ParsedIncludes))
				}
				e := cfg.Skills[0].ParsedIncludes[0]
				if e.Src != "loa/brainstorm" || e.Dst != "loa/brainstorm" {
					t.Errorf("ParsedIncludes[0] = {%q, %q}, want {loa/brainstorm, loa/brainstorm}", e.Src, e.Dst)
				}
			},
		},
		{
			name: "include path with colon mapping",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: ["deep/skill:my-tool"]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Skills[0].ParsedIncludes) != 1 {
					t.Fatalf("ParsedIncludes len = %d, want 1", len(cfg.Skills[0].ParsedIncludes))
				}
				e := cfg.Skills[0].ParsedIncludes[0]
				if e.Src != "deep/skill" || e.Dst != "my-tool" {
					t.Errorf("ParsedIncludes[0] = {%q, %q}, want {deep/skill, my-tool}", e.Src, e.Dst)
				}
			},
		},
		{
			name: "ParsedIncludes populated for multiple includes",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: [alpha, beta, gamma]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Skills[0].ParsedIncludes) != 3 {
					t.Fatalf("ParsedIncludes len = %d, want 3", len(cfg.Skills[0].ParsedIncludes))
				}
				want := []string{"alpha", "beta", "gamma"}
				for i, w := range want {
					if cfg.Skills[0].ParsedIncludes[i].Src != w {
						t.Errorf("ParsedIncludes[%d].Src = %q, want %q", i, cfg.Skills[0].ParsedIncludes[i].Src, w)
					}
				}
			},
		},
		{
			name: "intra-source overlap detected",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: ["a/b:target", "c/d:target"]
`,
			wantErr: "duplicate include destination",
		},
		{
			name:    "dot-dot include rejected",
			input:   "provider: claude\nskills:\n  - source: x\n    include: [..]\n",
			wantErr: "must not escape root",
		},
		// --- Same-URL-different-ref (AC 11-12) ---
		{
			name: "same git URL different refs error",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    ref: v1.0
    include: [foo]
  - source: git@github.com:user/repo.git
    ref: v2.0
    include: [bar]
`,
			wantErr: "same source",
		},
		{
			name: "same git URL same ref is valid",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    ref: v1.0
    include: [foo]
  - source: git@github.com:user/repo.git
    ref: v1.0
    include: [bar]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Skills) != 2 {
					t.Fatalf("skills len = %d, want 2", len(cfg.Skills))
				}
			},
		},
		{
			name: "same git URL both empty ref is valid",
			input: `
provider: claude
skills:
  - source: git@github.com:user/repo.git
    include: [foo]
  - source: git@github.com:user/repo.git
    include: [bar]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Skills) != 2 {
					t.Fatalf("skills len = %d, want 2", len(cfg.Skills))
				}
			},
		},
		{
			name: "local sources with same path different content is valid",
			input: `
provider: claude
skills:
  - source: /home/user/skills
    include: [foo]
  - source: /home/user/skills
    include: [bar]
`,
			check: func(t *testing.T, cfg *Config) {
				if len(cfg.Skills) != 2 {
					t.Fatalf("skills len = %d, want 2", len(cfg.Skills))
				}
			},
		},
		{
			name:    "SCP-style URL with port",
			input:   "provider: claude\nskills:\n  - source: git@github.com:2222/user/repo.git\n    include: [a]\n",
			wantErr: "SCP-style URLs",
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
