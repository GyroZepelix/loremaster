package git

import (
	"errors"
	"fmt"
	"os"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type Fetcher interface {
	CloneOrPull(url string, targetDir string) error
	Checkout(repoDir string, ref string) error
}

type GoGitFetcher struct{}

func (g *GoGitFetcher) CloneOrPull(url string, targetDir string) error {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		_, cloneErr := gogit.PlainClone(targetDir, false, &gogit.CloneOptions{
			URL: url,
		})
		if cloneErr != nil {
			// Clean up partial clone directory on failure
			os.RemoveAll(targetDir)
			return fmt.Errorf("clone %q: %w", url, cloneErr)
		}
		return nil
	}

	repo, err := gogit.PlainOpen(targetDir)
	if err != nil {
		// Cache directory exists but is not a valid git repo (partial clone?)
		// Remove and re-clone
		os.RemoveAll(targetDir)
		_, cloneErr := gogit.PlainClone(targetDir, false, &gogit.CloneOptions{
			URL: url,
		})
		if cloneErr != nil {
			os.RemoveAll(targetDir)
			return fmt.Errorf("clone %q (after clearing corrupt cache): %w", url, cloneErr)
		}
		return nil
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree at %q: %w", targetDir, err)
	}

	err = wt.Pull(&gogit.PullOptions{})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("pull %q: %w", url, err)
	}

	return nil
}

func (g *GoGitFetcher) Checkout(repoDir string, ref string) error {
	repo, err := gogit.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("open repo at %q: %w", repoDir, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree at %q: %w", repoDir, err)
	}

	// Try as local branch
	branchRef := plumbing.NewBranchReferenceName(ref)
	if _, err := repo.Reference(branchRef, true); err == nil {
		return wt.Checkout(&gogit.CheckoutOptions{Branch: branchRef})
	}

	// Try as remote tracking branch
	remoteRef := plumbing.NewRemoteReferenceName("origin", ref)
	if r, err := repo.Reference(remoteRef, true); err == nil {
		// Create a local branch tracking the remote
		return wt.Checkout(&gogit.CheckoutOptions{
			Branch: branchRef,
			Hash:   r.Hash(),
			Create: true,
		})
	}

	// Try as tag
	tagRef := plumbing.NewTagReferenceName(ref)
	if _, err := repo.Reference(tagRef, true); err == nil {
		return wt.Checkout(&gogit.CheckoutOptions{Branch: tagRef})
	}

	// Try as commit hash — validate it looks like hex before attempting
	if !isHexString(ref) {
		return fmt.Errorf("ref %q: not a valid branch, tag, or commit hash", ref)
	}
	hash := plumbing.NewHash(ref)
	if hash.IsZero() {
		return fmt.Errorf("ref %q: not a valid branch, tag, or commit hash", ref)
	}

	return wt.Checkout(&gogit.CheckoutOptions{Hash: hash})
}

func isHexString(s string) bool {
	if len(s) < 4 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
