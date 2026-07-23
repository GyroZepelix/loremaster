package config

import (
	"strings"
	"testing"
)

func TestParseDynamicResources(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
provider: [pi, claude]
skills:
  - source: repo
    include: [example-skill]
prompts:
  - source: repo
    include: [review.md]
hooks/tools:
  - source: repo
    include: [validate.sh:check.sh]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(cfg.Resources) != 3 {
		t.Fatalf("resources = %d, want 3", len(cfg.Resources))
	}
	wantNames := []string{"skills", "prompts", "hooks/tools"}
	for i, want := range wantNames {
		if cfg.Resources[i].Name != want {
			t.Errorf("resources[%d].Name = %q, want %q", i, cfg.Resources[i].Name, want)
		}
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0].Include[0] != "example-skill" {
		t.Fatalf("legacy Skills view not populated: %#v", cfg.Skills)
	}
	if got := cfg.Resources[2].Sources[0].ParsedIncludes[0].Dst; got != "check.sh" {
		t.Errorf("mapped destination = %q, want check.sh", got)
	}
}

func TestParsePromptsOnly(t *testing.T) {
	cfg, err := Parse(strings.NewReader(`
provider: pi
prompts:
  - source: repo
    include: [review.md]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Resources) != 1 || cfg.Resources[0].Name != "prompts" {
		t.Fatalf("resources = %#v", cfg.Resources)
	}
	if len(cfg.Skills) != 0 {
		t.Fatalf("skills = %#v, want empty", cfg.Skills)
	}
}

func TestParseDynamicResourceValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "unknown source field",
			input:   "provider: pi\nprompts:\n  - source: repo\n    incldue: [review.md]\n",
			wantErr: "field incldue not found",
		},
		{
			name:    "glob include",
			input:   "provider: pi\nprompts:\n  - source: repo\n    include: ['*.md']\n",
			wantErr: "glob metacharacters",
		},
		{
			name:    "glob resource",
			input:   "provider: pi\n'prompts/*':\n  - source: repo\n    include: [review.md]\n",
			wantErr: "glob metacharacters",
		},
		{
			name:    "glob in cleaned include component",
			input:   "provider: pi\nprompts:\n  - source: repo\n    include: ['bad*/../review.md']\n",
			wantErr: "glob metacharacters",
		},
		{
			name:    "glob in cleaned resource component",
			input:   "provider: pi\n'bad*/../prompts':\n  - source: repo\n    include: [review.md]\n",
			wantErr: "glob metacharacters",
		},
		{
			name:    "empty resource",
			input:   "provider: pi\nprompts: []\n",
			wantErr: "must have at least one source",
		},
		{
			name:    "only provider",
			input:   "provider: pi\n",
			wantErr: "at least one resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseCrossResourceDestinationOverlap(t *testing.T) {
	_, err := Parse(strings.NewReader(`
provider: claude
skills:
  - source: repo
    include: [foo/bar]
skills/foo:
  - source: repo
    include: [bar]
`))
	if err == nil || !strings.Contains(err.Error(), "overlapping resource destinations") {
		t.Fatalf("error = %v, want overlapping resource destinations", err)
	}
}

func TestParseSameSourceDifferentRefsAcrossResources(t *testing.T) {
	_, err := Parse(strings.NewReader(`
provider: claude
skills:
  - source: https://github.com/user/repo.git
    ref: v1
    include: [foo]
prompts:
  - source: https://github.com/user/repo.git
    ref: v2
    include: [review.md]
`))
	if err == nil || !strings.Contains(err.Error(), "same source") {
		t.Fatalf("error = %v, want same source conflict", err)
	}
}
