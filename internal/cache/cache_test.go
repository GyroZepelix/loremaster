package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name string
		a, b string
	}{
		{
			name: "SSH and HTTPS same repo",
			a:    "git@github.com:user/repo.git",
			b:    "https://github.com/user/repo.git",
		},
		{
			name: "SSH and HTTPS no .git suffix",
			a:    "git@github.com:user/repo",
			b:    "https://github.com/user/repo",
		},
		{
			name: "ssh:// with user@ and HTTPS",
			a:    "ssh://git@github.com/user/repo",
			b:    "https://github.com/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			na := NormalizeURL(tt.a)
			nb := NormalizeURL(tt.b)
			if na != nb {
				t.Errorf("NormalizeURL(%q) = %q, NormalizeURL(%q) = %q — want equal", tt.a, na, tt.b, nb)
			}
		})
	}
}

func TestNormalizeURLStrips(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"git@GitHub.COM:user/repo.git", "github.com/user/repo"},
		{"https://GitHub.COM/user/repo.git", "github.com/user/repo"},
		{"https://github.com/user/repo", "github.com/user/repo"},
		{"ssh://git@github.com/user/repo", "github.com/user/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRepoDirDeterministic(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a, err := RepoDir("git@github.com:user/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	b, err := RepoDir("git@github.com:user/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("RepoDir not deterministic: %q != %q", a, b)
	}
}

func TestRepoDirDifferentURLs(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	a, _ := RepoDir("git@github.com:user/repo-a.git")
	b, _ := RepoDir("git@github.com:user/repo-b.git")
	if a == b {
		t.Errorf("different URLs produced same RepoDir: %q", a)
	}
}

func TestSkillPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	got, err := SkillPath("git@github.com:user/repo.git", "my-skill")
	if err != nil {
		t.Fatal(err)
	}
	repoDir, _ := RepoDir("git@github.com:user/repo.git")
	want := filepath.Join(repoDir, "my-skill")
	if got != want {
		t.Errorf("SkillPath = %q, want %q", got, want)
	}
}

func TestDirRespectsXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "loremaster")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDirErrorsWithoutHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")
	_, err := Dir()
	if err == nil {
		t.Error("expected error when HOME is unset, got nil")
	}
}

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	if err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir() error: %v", err)
	}
	expected := filepath.Join(tmp, "loremaster")
	info, err := os.Stat(expected)
	if err != nil {
		t.Fatalf("cache dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("cache dir is not a directory")
	}
}
