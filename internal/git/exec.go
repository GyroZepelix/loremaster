package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExecGitFetcher implements Fetcher by shelling out to the system git binary.
// This respects the user's full SSH configuration (agent, config, keys).
type ExecGitFetcher struct{}

func (e *ExecGitFetcher) CloneOrPull(url string, targetDir string) error {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		if err := runGit("clone", url, targetDir); err != nil {
			os.RemoveAll(targetDir)
			return fmt.Errorf("clone %q: %w", url, err)
		}
		return nil
	}

	// Check if it's a valid git repo
	if err := runGit("-C", targetDir, "rev-parse", "--git-dir"); err != nil {
		// Not a valid repo — remove and re-clone
		os.RemoveAll(targetDir)
		if err := runGit("clone", url, targetDir); err != nil {
			os.RemoveAll(targetDir)
			return fmt.Errorf("clone %q (after clearing corrupt cache): %w", url, err)
		}
		return nil
	}

	if err := runGit("-C", targetDir, "pull", "--ff-only"); err != nil {
		// Pull failure is non-fatal for detached HEAD or diverged states;
		// the checkout step will position to the right ref anyway.
		// Only fail if this is a branch-tracking setup that genuinely broke.
		if !strings.Contains(err.Error(), "Not possible to fast-forward") {
			return fmt.Errorf("pull %q: %w", url, err)
		}
	}

	return nil
}

func (e *ExecGitFetcher) Checkout(repoDir string, ref string) error {
	// Fetch latest refs first
	_ = runGit("-C", repoDir, "fetch", "origin")

	if err := runGit("-C", repoDir, "checkout", ref); err != nil {
		return fmt.Errorf("checkout ref %q: %w", ref, err)
	}
	return nil
}

func runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil // capture stderr in error
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}
