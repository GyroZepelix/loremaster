package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func createBareRepo(t *testing.T, dir string) string {
	t.Helper()
	repoPath := filepath.Join(dir, "bare-repo")
	repo, err := gogit.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("init bare repo: %v", err)
	}

	// Create a skill directory with a file
	skillDir := filepath.Join(repoPath, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "workflow.md"), []byte("# Skill"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	wt, _ := repo.Worktree()
	wt.Add("my-skill/workflow.md")
	_, err = wt.Commit("initial", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	return repoPath
}

func TestCloneOrPull_Clone(t *testing.T) {
	tmp := t.TempDir()
	repoPath := createBareRepo(t, tmp)

	fetcher := &GoGitFetcher{}
	cloneDir := filepath.Join(tmp, "clone")

	if err := fetcher.CloneOrPull(repoPath, cloneDir); err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}

	// Verify skill file exists in clone
	content, err := os.ReadFile(filepath.Join(cloneDir, "my-skill", "workflow.md"))
	if err != nil {
		t.Fatalf("read cloned file: %v", err)
	}
	if string(content) != "# Skill" {
		t.Errorf("content = %q, want %q", string(content), "# Skill")
	}
}

func TestCloneOrPull_Pull(t *testing.T) {
	tmp := t.TempDir()
	repoPath := createBareRepo(t, tmp)

	fetcher := &GoGitFetcher{}
	cloneDir := filepath.Join(tmp, "clone")

	// Initial clone
	if err := fetcher.CloneOrPull(repoPath, cloneDir); err != nil {
		t.Fatalf("initial clone: %v", err)
	}

	// Add another commit to source repo
	repo, _ := gogit.PlainOpen(repoPath)
	wt, _ := repo.Worktree()
	os.WriteFile(filepath.Join(repoPath, "my-skill", "new-file.md"), []byte("new"), 0644)
	wt.Add("my-skill/new-file.md")
	wt.Commit("add new file", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})

	// Pull
	if err := fetcher.CloneOrPull(repoPath, cloneDir); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// Verify new file exists
	if _, err := os.Stat(filepath.Join(cloneDir, "my-skill", "new-file.md")); err != nil {
		t.Fatalf("new file not found after pull: %v", err)
	}
}

func TestCheckout_Tag(t *testing.T) {
	tmp := t.TempDir()
	repoPath := createBareRepo(t, tmp)

	// Create a tag on the initial commit
	repo, _ := gogit.PlainOpen(repoPath)
	head, _ := repo.Head()
	repo.CreateTag("v1.0.0", head.Hash(), &gogit.CreateTagOptions{
		Message: "v1.0.0",
		Tagger: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})

	// Add another commit
	wt, _ := repo.Worktree()
	os.WriteFile(filepath.Join(repoPath, "extra.txt"), []byte("extra"), 0644)
	wt.Add("extra.txt")
	wt.Commit("post-tag", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})

	// Clone and checkout tag
	fetcher := &GoGitFetcher{}
	cloneDir := filepath.Join(tmp, "clone")
	fetcher.CloneOrPull(repoPath, cloneDir)
	if err := fetcher.Checkout(cloneDir, "v1.0.0"); err != nil {
		t.Fatalf("Checkout tag: %v", err)
	}

	// At the tag, extra.txt should not exist
	if _, err := os.Stat(filepath.Join(cloneDir, "extra.txt")); !os.IsNotExist(err) {
		t.Error("extra.txt should not exist at v1.0.0 tag")
	}
}

func TestCloneOrPull_InvalidURL(t *testing.T) {
	tmp := t.TempDir()
	fetcher := &GoGitFetcher{}
	err := fetcher.CloneOrPull("/nonexistent/repo/path", filepath.Join(tmp, "clone"))
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestCheckout_Branch(t *testing.T) {
	tmp := t.TempDir()
	repoPath := createBareRepo(t, tmp)

	repo, _ := gogit.PlainOpen(repoPath)
	head, _ := repo.Head()
	wt, _ := repo.Worktree()

	// Create a branch
	ref := plumbing.NewBranchReferenceName("feature")
	repo.Storer.SetReference(plumbing.NewHashReference(ref, head.Hash()))
	wt.Checkout(&gogit.CheckoutOptions{Branch: ref})

	// Add file on branch
	os.WriteFile(filepath.Join(repoPath, "branch-file.txt"), []byte("branch"), 0644)
	wt.Add("branch-file.txt")
	wt.Commit("branch commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()},
	})

	// Switch back to master
	wt.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master")})

	// Clone and checkout branch
	fetcher := &GoGitFetcher{}
	cloneDir := filepath.Join(tmp, "clone")
	fetcher.CloneOrPull(repoPath, cloneDir)
	if err := fetcher.Checkout(cloneDir, "feature"); err != nil {
		t.Fatalf("Checkout branch: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cloneDir, "branch-file.txt")); err != nil {
		t.Error("branch-file.txt should exist after checking out feature branch")
	}
}
