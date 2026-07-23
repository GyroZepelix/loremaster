package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	loregit "github.com/GyroZepelix/loremaster/internal/git"
	loresync "github.com/GyroZepelix/loremaster/internal/sync"
	"github.com/spf13/cobra"
)

func TestRunSyncReportsFirstSyncAndNoChanges(t *testing.T) {
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(source, "foo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "foo", "SKILL.md"), []byte("# Foo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	configContent := fmt.Sprintf("provider: claude\nskills:\n  - source: %q\n    include: [foo]\n", source)
	if err := os.WriteFile(filepath.Join(project, "lore.yml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() {
		profileFlag, pruneFlag = oldProfile, oldPrune
		if err := os.Chdir(oldWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	profileFlag, pruneFlag = "", false

	var first bytes.Buffer
	firstCmd := &cobra.Command{}
	firstCmd.SetOut(&first)
	if err := runSync(firstCmd, nil); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if got, want := first.String(), "Synced items:\n  added .claude/skills/foo\n"; got != want {
		t.Fatalf("first output = %q, want %q", got, want)
	}

	var second bytes.Buffer
	secondCmd := &cobra.Command{}
	secondCmd.SetOut(&second)
	if err := runSync(secondCmd, nil); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got, want := second.String(), "No repository or synced item changes.\n"; got != want {
		t.Fatalf("second output = %q, want %q", got, want)
	}
}

func TestRunSyncReportsPartialSuccessBeforeError(t *testing.T) {
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(source, "foo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "foo", "SKILL.md"), []byte("# Foo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	configContent := fmt.Sprintf("provider: claude\nskills:\n  - source: %q\n    include: [foo]\n  - source: %q\n    include: [bar]\n", source, missing)
	if err := os.WriteFile(filepath.Join(project, "lore.yml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "cache"))

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	oldProfile, oldPrune := profileFlag, pruneFlag
	t.Cleanup(func() {
		profileFlag, pruneFlag = oldProfile, oldPrune
		if err := os.Chdir(oldWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	profileFlag, pruneFlag = "", false

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runSync(command, nil); err == nil {
		t.Fatal("partial sync returned nil error")
	}
	if got, want := output.String(), "Synced items:\n  added .claude/skills/foo\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSanitizedRepositorySource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "authenticated HTTPS", source: "https://user:token@example.com/repo.git", want: "https://example.com/repo.git"},
		{name: "username only", source: "https://deploy@example.com/repo.git", want: "https://example.com/repo.git"},
		{name: "ordinary HTTPS", source: "https://example.com/repo.git", want: "https://example.com/repo.git"},
		{name: "SSH username", source: "ssh://git@example.com/repo.git", want: "ssh://example.com/repo.git"},
		{name: "scp style", source: "git@example.com:repo.git", want: "git@example.com:repo.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizedRepositorySource(tt.source); got != tt.want {
				t.Fatalf("sanitized source = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderSyncSummaryRedactsCredentials(t *testing.T) {
	var output bytes.Buffer
	renderSyncSummary(&output, map[string]loregit.RepositoryUpdate{
		"https://user:token@example.com/repo.git": {
			Source:      "https://user:token@example.com/repo.git",
			Status:      loregit.UpdateCloned,
			AfterCommit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}, nil)
	want := "Repositories:\n  cloned https://example.com/repo.git @ aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderSyncSummaryUnchanged(t *testing.T) {
	var output bytes.Buffer
	renderSyncSummary(&output, map[string]loregit.RepositoryUpdate{
		"git@example.com:resources.git": {Source: "git@example.com:resources.git", Status: loregit.UpdateUnchanged},
	}, nil)
	if got, want := output.String(), "No repository or synced item changes.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderSyncSummaryChanges(t *testing.T) {
	var output bytes.Buffer
	renderSyncSummary(&output, map[string]loregit.RepositoryUpdate{
		"z-repo": {
			Source:       "z-repo",
			Status:       loregit.UpdateFastForwarded,
			BeforeCommit: "1111111111111111111111111111111111111111",
			AfterCommit:  "2222222222222222222222222222222222222222",
		},
		"a-repo": {
			Source:      "a-repo",
			Status:      loregit.UpdateCloned,
			AfterCommit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}, []loresync.ItemChange{
		{Status: loresync.ItemUpdated, Path: ".pi/skills/zeta"},
		{Status: loresync.ItemAdded, Path: ".claude/prompts/review.md"},
		{Status: loresync.ItemDeleted, Path: ".pi/skills/alpha"},
		{Status: loresync.ItemAdded, Path: ".claude/prompts/review.md"},
	})

	want := "Repositories:\n" +
		"  cloned a-repo @ aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
		"  fast-forwarded z-repo 1111111111111111111111111111111111111111 -> 2222222222222222222222222222222222222222\n" +
		"Synced items:\n" +
		"  added .claude/prompts/review.md\n" +
		"  deleted .pi/skills/alpha\n" +
		"  updated .pi/skills/zeta\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
