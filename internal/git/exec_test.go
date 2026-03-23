package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
}

// mustRunGit runs a git command and fails the test if it errors.
func mustRunGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s (%v)", args, out, err)
	}
}

// createTestRepoExec creates a local git repo using system git for ExecGitFetcher tests.
func createTestRepoExec(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(dir, 0755)

	mustRunGit(t, "init", "-b", "master", dir)
	mustRunGit(t, "-C", dir, "config", "user.email", "test@test.com")
	mustRunGit(t, "-C", dir, "config", "user.name", "test")

	// Create skill directory
	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# Skill"), 0644)

	mustRunGit(t, "-C", dir, "add", ".")
	mustRunGit(t, "-C", dir, "commit", "-m", "initial")

	return dir
}

func TestExecCloneOrPull_Clone(t *testing.T) {
	requireGit(t)
	repoPath := createTestRepoExec(t)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	fetcher := &ExecGitFetcher{}
	if err := fetcher.CloneOrPull(repoPath, cloneDir); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(cloneDir, "my-skill", "workflow.md"))
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}
	if string(content) != "# Skill" {
		t.Errorf("content = %q, want %q", string(content), "# Skill")
	}
}

func TestExecCloneOrPull_Pull(t *testing.T) {
	requireGit(t)
	repoPath := createTestRepoExec(t)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	fetcher := &ExecGitFetcher{}

	// Initial clone
	if err := fetcher.CloneOrPull(repoPath, cloneDir); err != nil {
		t.Fatalf("initial clone: %v", err)
	}

	// Add a new file to source repo
	os.WriteFile(filepath.Join(repoPath, "my-skill", "new-file.md"), []byte("new"), 0644)
	mustRunGit(t, "-C", repoPath, "add", ".")
	mustRunGit(t, "-C", repoPath, "commit", "-m", "add new file")

	// Pull
	if err := fetcher.CloneOrPull(repoPath, cloneDir); err != nil {
		t.Fatalf("pull: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cloneDir, "my-skill", "new-file.md")); err != nil {
		t.Fatal("new file not found after pull")
	}
}

func TestExecCheckout_Tag(t *testing.T) {
	requireGit(t)
	repoPath := createTestRepoExec(t)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	mustRunGit(t, "-C", repoPath, "tag", "v1.0.0")

	// Add another commit after tag
	os.WriteFile(filepath.Join(repoPath, "extra.txt"), []byte("extra"), 0644)
	mustRunGit(t, "-C", repoPath, "add", ".")
	mustRunGit(t, "-C", repoPath, "commit", "-m", "post-tag")

	fetcher := &ExecGitFetcher{}
	fetcher.CloneOrPull(repoPath, cloneDir)

	if err := fetcher.Checkout(cloneDir, "v1.0.0"); err != nil {
		t.Fatalf("Checkout tag: %v", err)
	}

	// extra.txt should not exist at v1.0.0
	if _, err := os.Stat(filepath.Join(cloneDir, "extra.txt")); !os.IsNotExist(err) {
		t.Error("extra.txt should not exist at v1.0.0 tag")
	}
}

func TestExecCheckout_Branch(t *testing.T) {
	requireGit(t)
	repoPath := createTestRepoExec(t)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	// Create a branch with a file
	mustRunGit(t, "-C", repoPath, "checkout", "-b", "feature")
	os.WriteFile(filepath.Join(repoPath, "branch-file.txt"), []byte("branch"), 0644)
	mustRunGit(t, "-C", repoPath, "add", ".")
	mustRunGit(t, "-C", repoPath, "commit", "-m", "branch commit")
	mustRunGit(t, "-C", repoPath, "checkout", "master")

	fetcher := &ExecGitFetcher{}
	fetcher.CloneOrPull(repoPath, cloneDir)

	if err := fetcher.Checkout(cloneDir, "feature"); err != nil {
		t.Fatalf("Checkout branch: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cloneDir, "branch-file.txt")); err != nil {
		t.Error("branch-file.txt should exist after checking out feature branch")
	}
}

func TestExecCheckout_FetchFailureDoesNotBlockCheckout(t *testing.T) {
	requireGit(t)
	repoPath := createTestRepoExec(t)
	cloneDir := filepath.Join(t.TempDir(), "clone")

	fetcher := &ExecGitFetcher{}
	fetcher.CloneOrPull(repoPath, cloneDir)

	// Get current HEAD ref
	cmd := exec.Command("git", "-C", cloneDir, "rev-parse", "HEAD")
	headBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %s (%v)", headBytes, err)
	}
	head := string(headBytes[:len(headBytes)-1]) // trim newline

	// Remove origin to make fetch fail
	mustRunGit(t, "-C", cloneDir, "remote", "remove", "origin")

	// Checkout should still succeed with local ref (fetch will warn but not fail)
	if err := fetcher.Checkout(cloneDir, head); err != nil {
		t.Fatalf("Checkout with broken remote should succeed for local ref: %v", err)
	}
}

func TestExecCloneOrPull_InvalidURL(t *testing.T) {
	requireGit(t)
	fetcher := &ExecGitFetcher{}
	err := fetcher.CloneOrPull("/nonexistent/repo/path", filepath.Join(t.TempDir(), "clone"))
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
