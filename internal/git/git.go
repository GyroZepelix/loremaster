package git

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

type UpdateStatus string

const (
	UpdateUnchanged     UpdateStatus = "unchanged"
	UpdateCloned        UpdateStatus = "cloned"
	UpdateFastForwarded UpdateStatus = "fast-forwarded"
)

type RepositoryUpdate struct {
	Source        string
	Status        UpdateStatus
	BeforeCommit  string
	AfterCommit   string
	CurrentCommit string
	ChangedPaths  []string
}

type Fetcher interface {
	Fetch(url string, targetDir string, ref string) (RepositoryUpdate, error)
}

type GoGitFetcher struct{}

func (g *GoGitFetcher) CloneOrPull(url string, targetDir string) error {
	_, err := g.Fetch(url, targetDir, "")
	return err
}

func (g *GoGitFetcher) Checkout(repoDir string, ref string) error {
	repo, err := gogit.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("open repo at %q: %w", repoDir, err)
	}
	return checkoutGoGit(repo, ref)
}

func (g *GoGitFetcher) Fetch(url string, targetDir string, ref string) (RepositoryUpdate, error) {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return g.clone(url, targetDir, ref, false)
	}

	repo, err := gogit.PlainOpen(targetDir)
	if err != nil {
		return g.clone(url, targetDir, ref, true)
	}

	beforeRef, err := repo.Head()
	if err != nil {
		return RepositoryUpdate{}, fmt.Errorf("resolve HEAD at %q: %w", targetDir, err)
	}
	before := beforeRef.Hash().String()
	update := RepositoryUpdate{Status: UpdateUnchanged, BeforeCommit: before, AfterCommit: before, CurrentCommit: before}

	if ref == "" {
		beforeBranch := ""
		if beforeRef.Name().IsBranch() {
			beforeBranch = beforeRef.Name().Short()
		}
		if err := repo.Fetch(&gogit.FetchOptions{RemoteName: "origin"}); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return update, fmt.Errorf("pull %q: %w", url, err)
		}
		defaultBranch, err := goGitRemoteDefaultBranch(repo)
		if err != nil {
			return update, fmt.Errorf("resolve remote default branch at %q: %w", targetDir, err)
		}
		if _, err := checkoutFetchedGoGitRef(repo, defaultBranch); err != nil {
			return update, err
		}
		currentRef, err := repo.Head()
		if err != nil {
			return update, fmt.Errorf("resolve final HEAD at %q: %w", targetDir, err)
		}
		update.CurrentCommit = currentRef.Hash().String()
		if beforeBranch == defaultBranch && update.CurrentCommit != before {
			update.Status = UpdateFastForwarded
			update.AfterCommit = update.CurrentCommit
		}
	} else {
		beforeBranch := ""
		if beforeRef.Name().IsBranch() {
			beforeBranch = beforeRef.Name().Short()
		}
		if err := repo.Fetch(&gogit.FetchOptions{RemoteName: "origin"}); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return update, fmt.Errorf("fetch %q: %w", url, err)
		}
		remoteBranch, err := checkoutFetchedGoGitRef(repo, ref)
		if err != nil {
			return update, err
		}
		currentRef, err := repo.Head()
		if err != nil {
			return update, fmt.Errorf("resolve final HEAD at %q: %w", targetDir, err)
		}
		update.CurrentCommit = currentRef.Hash().String()
		if remoteBranch && beforeBranch == ref && update.CurrentCommit != before {
			update.Status = UpdateFastForwarded
			update.AfterCommit = update.CurrentCommit
		}
	}

	update.ChangedPaths, err = goGitChangedPaths(repo, before, update.CurrentCommit)
	if err != nil {
		return update, fmt.Errorf("diff revisions in %q: %w", targetDir, err)
	}
	return update, nil
}

func (g *GoGitFetcher) clone(url string, targetDir string, ref string, clear bool) (RepositoryUpdate, error) {
	if clear {
		if err := os.RemoveAll(targetDir); err != nil {
			return RepositoryUpdate{}, fmt.Errorf("clear invalid cache %q: %w", targetDir, err)
		}
	}
	repo, err := gogit.PlainClone(targetDir, false, &gogit.CloneOptions{URL: url})
	if err != nil {
		os.RemoveAll(targetDir)
		if clear {
			return RepositoryUpdate{}, fmt.Errorf("clone %q (after clearing corrupt cache): %w", url, err)
		}
		return RepositoryUpdate{}, fmt.Errorf("clone %q: %w", url, err)
	}

	update := RepositoryUpdate{Status: UpdateCloned}
	if head, headErr := repo.Head(); headErr == nil {
		update.AfterCommit = head.Hash().String()
		update.CurrentCommit = update.AfterCommit
		if head.Name().IsBranch() {
			_ = setGoGitRemoteDefaultBranch(repo, head.Name().Short())
		}
	}
	if ref != "" {
		if err := checkoutGoGit(repo, ref); err != nil {
			return update, err
		}
	}
	head, err := repo.Head()
	if err != nil {
		return update, fmt.Errorf("resolve cloned HEAD at %q: %w", targetDir, err)
	}
	update.AfterCommit = head.Hash().String()
	update.CurrentCommit = update.AfterCommit
	return update, nil
}

func goGitRemoteDefaultBranch(repo *gogit.Repository) (string, error) {
	remoteHead, err := repo.Reference(plumbing.NewRemoteHEADReferenceName("origin"), false)
	if err == nil {
		if remoteHead.Type() != plumbing.SymbolicReference {
			return "", fmt.Errorf("remote HEAD is not symbolic")
		}
		name := remoteHead.Target().Short()
		if !strings.HasPrefix(name, "origin/") || name == "origin/" {
			return "", fmt.Errorf("invalid remote HEAD target %q", name)
		}
		return strings.TrimPrefix(name, "origin/"), nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return "", err
	}

	remote, err := repo.Remote("origin")
	if err != nil {
		return "", err
	}
	refs, err := remote.List(&gogit.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if ref.Name() != plumbing.HEAD || ref.Type() != plumbing.SymbolicReference || !ref.Target().IsBranch() {
			continue
		}
		branch := ref.Target().Short()
		if err := setGoGitRemoteDefaultBranch(repo, branch); err != nil {
			return "", err
		}
		return branch, nil
	}
	return "", fmt.Errorf("remote HEAD is unavailable")
}

func setGoGitRemoteDefaultBranch(repo *gogit.Repository, branch string) error {
	return repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.NewRemoteHEADReferenceName("origin"),
		plumbing.NewRemoteReferenceName("origin", branch),
	))
}

func checkoutFetchedGoGitRef(repo *gogit.Repository, ref string) (bool, error) {
	branchRef := plumbing.NewBranchReferenceName(ref)
	remoteRef := plumbing.NewRemoteReferenceName("origin", ref)
	remote, err := repo.Reference(remoteRef, true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return false, checkoutGoGit(repo, ref)
		}
		return false, fmt.Errorf("resolve remote ref %q: %w", ref, err)
	}

	targetHash := remote.Hash()
	var previousLocal *plumbing.Reference
	if local, err := repo.Reference(branchRef, true); err == nil {
		previousLocal = local
		localCommit, err := repo.CommitObject(local.Hash())
		if err != nil {
			return true, fmt.Errorf("resolve local ref %q: %w", ref, err)
		}
		remoteCommit, err := repo.CommitObject(remote.Hash())
		if err != nil {
			return true, fmt.Errorf("resolve remote ref %q: %w", ref, err)
		}
		localBehind, err := localCommit.IsAncestor(remoteCommit)
		if err != nil {
			return true, fmt.Errorf("compare ref %q: %w", ref, err)
		}
		if !localBehind {
			localAhead, err := remoteCommit.IsAncestor(localCommit)
			if err != nil {
				return true, fmt.Errorf("compare ref %q: %w", ref, err)
			}
			if !localAhead {
				return true, fmt.Errorf("fast-forward ref %q: local and remote histories have diverged", ref)
			}
			targetHash = local.Hash()
		}
		if targetHash == local.Hash() {
			head, err := repo.Head()
			if err != nil {
				return true, fmt.Errorf("resolve HEAD before checkout ref %q: %w", ref, err)
			}
			if head.Name() == branchRef {
				return true, nil
			}
		}
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return true, fmt.Errorf("resolve local ref %q: %w", ref, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return true, fmt.Errorf("get worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return true, fmt.Errorf("inspect worktree before checkout ref %q: %w", ref, err)
	}
	if !status.IsClean() {
		return true, fmt.Errorf("checkout ref %q: cached worktree has local changes", ref)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branchRef, targetHash)); err != nil {
		return true, fmt.Errorf("update local ref %q: %w", ref, err)
	}
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: branchRef}); err != nil {
		var rollbackErr error
		if previousLocal != nil {
			rollbackErr = repo.Storer.SetReference(previousLocal)
		} else {
			rollbackErr = repo.Storer.RemoveReference(branchRef)
		}
		if rollbackErr != nil {
			return true, fmt.Errorf("checkout ref %q: %v; restore local ref: %w", ref, err, rollbackErr)
		}
		return true, fmt.Errorf("checkout ref %q without discarding local changes: %w", ref, err)
	}
	return true, nil
}

func checkoutGoGit(repo *gogit.Repository, ref string) error {
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(ref)
	if _, err := repo.Reference(branchRef, true); err == nil {
		return wt.Checkout(&gogit.CheckoutOptions{Branch: branchRef})
	}
	remoteRef := plumbing.NewRemoteReferenceName("origin", ref)
	if r, err := repo.Reference(remoteRef, true); err == nil {
		return wt.Checkout(&gogit.CheckoutOptions{Branch: branchRef, Hash: r.Hash(), Create: true})
	}
	tagRef := plumbing.NewTagReferenceName(ref)
	if _, err := repo.Reference(tagRef, true); err == nil {
		return wt.Checkout(&gogit.CheckoutOptions{Branch: tagRef})
	}
	if !isHexString(ref) {
		return fmt.Errorf("ref %q: not a valid branch, tag, or commit hash", ref)
	}
	hash := plumbing.NewHash(ref)
	if hash.IsZero() {
		return fmt.Errorf("ref %q: not a valid branch, tag, or commit hash", ref)
	}
	return wt.Checkout(&gogit.CheckoutOptions{Hash: hash})
}

func goGitChangedPaths(repo *gogit.Repository, before string, after string) ([]string, error) {
	if before == "" || after == "" || before == after {
		return nil, nil
	}
	beforeCommit, err := repo.CommitObject(plumbing.NewHash(before))
	if err != nil {
		return nil, err
	}
	afterCommit, err := repo.CommitObject(plumbing.NewHash(after))
	if err != nil {
		return nil, err
	}
	beforeTree, err := beforeCommit.Tree()
	if err != nil {
		return nil, err
	}
	afterTree, err := afterCommit.Tree()
	if err != nil {
		return nil, err
	}
	changes, err := beforeTree.Diff(afterTree)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]bool)
	for _, change := range changes {
		if change.From.Name != "" {
			paths[change.From.Name] = true
		}
		if change.To.Name != "" {
			paths[change.To.Name] = true
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
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
