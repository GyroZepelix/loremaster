package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// ExecGitFetcher implements Fetcher by shelling out to the system git binary.
// This respects the user's full SSH configuration (agent, config, keys).
type ExecGitFetcher struct{}

func (e *ExecGitFetcher) CloneOrPull(url string, targetDir string) error {
	_, err := e.Fetch(url, targetDir, "")
	return err
}

func (e *ExecGitFetcher) Checkout(repoDir string, ref string) error {
	return e.checkout(repoDir, ref)
}

func (e *ExecGitFetcher) Fetch(url string, targetDir string, ref string) (RepositoryUpdate, error) {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return e.clone(url, targetDir, ref, false)
	}
	if err := runGit("-C", targetDir, "rev-parse", "--git-dir"); err != nil {
		return e.clone(url, targetDir, ref, true)
	}

	before, err := gitRevision(targetDir)
	if err != nil {
		return RepositoryUpdate{}, err
	}
	update := RepositoryUpdate{Status: UpdateUnchanged, BeforeCommit: before, AfterCommit: before, CurrentCommit: before}

	if ref == "" {
		beforeBranch := gitBranch(targetDir)
		if err := runGit("-C", targetDir, "fetch", "origin"); err != nil {
			return update, fmt.Errorf("pull %q: %w", url, err)
		}
		defaultBranch, err := gitRemoteDefaultBranch(targetDir)
		if err != nil {
			return update, err
		}
		if err := runGit("-C", targetDir, "checkout", defaultBranch); err != nil {
			return update, fmt.Errorf("checkout default branch %q: %w", defaultBranch, err)
		}
		if err := runGit("-C", targetDir, "merge", "--ff-only", "origin/"+defaultBranch); err != nil {
			if !strings.Contains(err.Error(), "Not possible to fast-forward") {
				return update, fmt.Errorf("pull %q: %w", url, err)
			}
		}
		current, err := gitRevision(targetDir)
		if err != nil {
			return update, err
		}
		update.CurrentCommit = current
		if beforeBranch == defaultBranch && current != before {
			update.Status = UpdateFastForwarded
			update.AfterCommit = current
		}
	} else {
		beforeBranch := gitBranch(targetDir)
		if err := runGit("-C", targetDir, "fetch", "origin"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: fetch failed for %q: %s - using cached refs\n", targetDir, err)
		}
		remoteBranch := "refs/remotes/origin/" + ref
		hasRemoteBranch := runGit("-C", targetDir, "show-ref", "--verify", "--quiet", remoteBranch) == nil
		if err := runGit("-C", targetDir, "checkout", ref); err != nil {
			return update, fmt.Errorf("checkout ref %q: %w", ref, err)
		}
		if hasRemoteBranch {
			if err := runGit("-C", targetDir, "merge", "--ff-only", "origin/"+ref); err != nil {
				return update, fmt.Errorf("fast-forward ref %q: %w", ref, err)
			}
		}
		current, err := gitRevision(targetDir)
		if err != nil {
			return update, err
		}
		update.CurrentCommit = current
		if hasRemoteBranch && beforeBranch == ref && current != before {
			update.Status = UpdateFastForwarded
			update.AfterCommit = current
		}
	}

	update.ChangedPaths, err = execChangedPaths(targetDir, before, update.CurrentCommit)
	if err != nil {
		return update, fmt.Errorf("diff revisions in %q: %w", targetDir, err)
	}
	return update, nil
}

func (e *ExecGitFetcher) clone(url string, targetDir string, ref string, clear bool) (RepositoryUpdate, error) {
	if clear {
		if err := os.RemoveAll(targetDir); err != nil {
			return RepositoryUpdate{}, fmt.Errorf("clear invalid cache %q: %w", targetDir, err)
		}
	}
	if err := runGit("clone", url, targetDir); err != nil {
		os.RemoveAll(targetDir)
		if clear {
			return RepositoryUpdate{}, fmt.Errorf("clone %q (after clearing corrupt cache): %w", url, err)
		}
		return RepositoryUpdate{}, fmt.Errorf("clone %q: %w", url, err)
	}

	commit, err := gitRevision(targetDir)
	if err != nil {
		return RepositoryUpdate{Status: UpdateCloned}, err
	}
	update := RepositoryUpdate{Status: UpdateCloned, AfterCommit: commit, CurrentCommit: commit}
	if ref != "" {
		if err := e.checkout(targetDir, ref); err != nil {
			return update, err
		}
		commit, err = gitRevision(targetDir)
		if err != nil {
			return update, err
		}
		update.AfterCommit = commit
		update.CurrentCommit = commit
	}
	return update, nil
}

func (e *ExecGitFetcher) checkout(repoDir string, ref string) error {
	if err := runGit("-C", repoDir, "fetch", "origin"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: fetch failed for %q: %s - using cached refs\n", repoDir, err)
	}
	if err := runGit("-C", repoDir, "checkout", ref); err != nil {
		return fmt.Errorf("checkout ref %q: %w", ref, err)
	}
	return nil
}

func gitRemoteDefaultBranch(repoDir string) (string, error) {
	output, err := runGitOutput("-C", repoDir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		if setErr := runGit("-C", repoDir, "remote", "set-head", "origin", "--auto"); setErr != nil {
			return "", fmt.Errorf("resolve remote default branch at %q: %w", repoDir, setErr)
		}
		output, err = runGitOutput("-C", repoDir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			return "", fmt.Errorf("resolve remote default branch at %q: %w", repoDir, err)
		}
	}
	name := strings.TrimSpace(string(output))
	if !strings.HasPrefix(name, "origin/") || name == "origin/" {
		return "", fmt.Errorf("resolve remote default branch at %q: invalid ref %q", repoDir, name)
	}
	return strings.TrimPrefix(name, "origin/"), nil
}

func gitBranch(repoDir string) string {
	output, err := runGitOutput("-C", repoDir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func gitRevision(repoDir string) (string, error) {
	output, err := runGitOutput("-C", repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve HEAD at %q: %w", repoDir, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func execChangedPaths(repoDir string, before string, after string) ([]string, error) {
	if before == "" || after == "" || before == after {
		return nil, nil
	}
	output, err := runGitOutput("-C", repoDir, "diff", "--name-only", "--no-renames", "-z", before, after)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, path := range bytes.Split(output, []byte{0}) {
		if len(path) > 0 {
			set[string(path)] = true
		}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func runGit(args ...string) error {
	_, err := runGitOutput(args...)
	return err
}

func runGitOutput(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, err
	}
	return output, nil
}
