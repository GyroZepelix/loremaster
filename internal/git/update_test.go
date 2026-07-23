package git

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchReportsRepositoryUpdates(t *testing.T) {
	requireGit(t)
	for _, tt := range []struct {
		name    string
		fetcher Fetcher
	}{
		{name: "exec", fetcher: &ExecGitFetcher{}},
		{name: "go-git", fetcher: &GoGitFetcher{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := createTestRepoExec(t)
			cloneDir := filepath.Join(t.TempDir(), "clone")

			cloned, err := tt.fetcher.Fetch(repoPath, cloneDir, "")
			if err != nil {
				t.Fatalf("clone: %v", err)
			}
			if cloned.Status != UpdateCloned || cloned.AfterCommit == "" || cloned.CurrentCommit != cloned.AfterCommit {
				t.Fatalf("clone update = %#v", cloned)
			}

			unchanged, err := tt.fetcher.Fetch(repoPath, cloneDir, "")
			if err != nil {
				t.Fatalf("unchanged fetch: %v", err)
			}
			if unchanged.Status != UpdateUnchanged || unchanged.BeforeCommit != cloned.AfterCommit || len(unchanged.ChangedPaths) != 0 {
				t.Fatalf("unchanged update = %#v", unchanged)
			}

			changedPath := filepath.Join(repoPath, "my-skill", "new-file.md")
			if err := os.WriteFile(changedPath, []byte("new"), 0644); err != nil {
				t.Fatal(err)
			}
			mustRunGit(t, "-C", repoPath, "add", ".")
			mustRunGit(t, "-C", repoPath, "commit", "-m", "add new file")

			updated, err := tt.fetcher.Fetch(repoPath, cloneDir, "")
			if err != nil {
				t.Fatalf("fast-forward: %v", err)
			}
			if updated.Status != UpdateFastForwarded || updated.BeforeCommit != cloned.AfterCommit || updated.AfterCommit == cloned.AfterCommit {
				t.Fatalf("fast-forward update = %#v", updated)
			}
			if len(updated.ChangedPaths) != 1 || updated.ChangedPaths[0] != "my-skill/new-file.md" {
				t.Fatalf("changed paths = %v", updated.ChangedPaths)
			}
		})
	}
}

func TestFetchConfiguredRefs(t *testing.T) {
	requireGit(t)
	for _, tt := range []struct {
		name    string
		fetcher Fetcher
	}{
		{name: "exec", fetcher: &ExecGitFetcher{}},
		{name: "go-git", fetcher: &GoGitFetcher{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := createTestRepoExec(t)
			mustRunGit(t, "-C", repoPath, "tag", "-a", "v1.0.0", "-m", "v1.0.0")
			if err := os.WriteFile(filepath.Join(repoPath, "post-tag.md"), []byte("post"), 0644); err != nil {
				t.Fatal(err)
			}
			mustRunGit(t, "-C", repoPath, "add", ".")
			mustRunGit(t, "-C", repoPath, "commit", "-m", "post tag")
			cloneDir := filepath.Join(t.TempDir(), "clone")

			cloned, err := tt.fetcher.Fetch(repoPath, cloneDir, "v1.0.0")
			if err != nil || cloned.Status != UpdateCloned {
				t.Fatalf("tag clone = %#v, error = %v", cloned, err)
			}
			unchanged, err := tt.fetcher.Fetch(repoPath, cloneDir, "v1.0.0")
			if err != nil || unchanged.Status != UpdateUnchanged || len(unchanged.ChangedPaths) != 0 {
				t.Fatalf("repeated tag fetch = %#v, error = %v", unchanged, err)
			}

			switched, err := tt.fetcher.Fetch(repoPath, cloneDir, "")
			if err != nil || switched.Status != UpdateUnchanged || !hasPath(switched.ChangedPaths, "post-tag.md") {
				t.Fatalf("restore default branch = %#v, error = %v", switched, err)
			}
			pinnedAgain, err := tt.fetcher.Fetch(repoPath, cloneDir, "v1.0.0")
			if err != nil || pinnedAgain.Status != UpdateUnchanged || !hasPath(pinnedAgain.ChangedPaths, "post-tag.md") {
				t.Fatalf("switch back to tag = %#v, error = %v", pinnedAgain, err)
			}
			if _, err := tt.fetcher.Fetch(repoPath, cloneDir, ""); err != nil {
				t.Fatalf("restore default after tag: %v", err)
			}

			if err := os.WriteFile(filepath.Join(repoPath, "branch-update.md"), []byte("update"), 0644); err != nil {
				t.Fatal(err)
			}
			mustRunGit(t, "-C", repoPath, "add", ".")
			mustRunGit(t, "-C", repoPath, "commit", "-m", "branch update")
			fastForwarded, err := tt.fetcher.Fetch(repoPath, cloneDir, "master")
			if err != nil || fastForwarded.Status != UpdateFastForwarded || !hasPath(fastForwarded.ChangedPaths, "branch-update.md") {
				t.Fatalf("configured branch fast-forward = %#v, error = %v", fastForwarded, err)
			}

			mustRunGit(t, "-C", repoPath, "mv", "my-skill/workflow.md", "my-skill/renamed.md")
			mustRunGit(t, "-C", repoPath, "commit", "-m", "rename workflow")
			renamed, err := tt.fetcher.Fetch(repoPath, cloneDir, "master")
			if err != nil || !hasPath(renamed.ChangedPaths, "my-skill/workflow.md") || !hasPath(renamed.ChangedPaths, "my-skill/renamed.md") {
				t.Fatalf("rename update = %#v, error = %v", renamed, err)
			}

			mustRunGit(t, "-C", repoPath, "checkout", "-b", "feature")
			if err := os.WriteFile(filepath.Join(repoPath, "feature.md"), []byte("feature"), 0644); err != nil {
				t.Fatal(err)
			}
			mustRunGit(t, "-C", repoPath, "add", ".")
			mustRunGit(t, "-C", repoPath, "commit", "-m", "feature")
			mustRunGit(t, "-C", repoPath, "checkout", "master")
			feature, err := tt.fetcher.Fetch(repoPath, cloneDir, "feature")
			if err != nil || feature.Status != UpdateUnchanged || !hasPath(feature.ChangedPaths, "feature.md") {
				t.Fatalf("feature switch = %#v, error = %v", feature, err)
			}
			defaultAgain, err := tt.fetcher.Fetch(repoPath, cloneDir, "")
			if err != nil || defaultAgain.Status != UpdateUnchanged || !hasPath(defaultAgain.ChangedPaths, "feature.md") {
				t.Fatalf("feature to default switch = %#v, error = %v", defaultAgain, err)
			}
		})
	}
}

func TestFetchConfiguredBranchAllowsLocalAhead(t *testing.T) {
	requireGit(t)
	for _, tt := range []struct {
		name    string
		fetcher Fetcher
	}{
		{name: "exec", fetcher: &ExecGitFetcher{}},
		{name: "go-git", fetcher: &GoGitFetcher{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := createTestRepoExec(t)
			cloneDir := filepath.Join(t.TempDir(), "clone")
			if _, err := tt.fetcher.Fetch(repoPath, cloneDir, "master"); err != nil {
				t.Fatal(err)
			}
			mustRunGit(t, "-C", cloneDir, "config", "user.email", "test@test.com")
			mustRunGit(t, "-C", cloneDir, "config", "user.name", "test")
			if err := os.WriteFile(filepath.Join(cloneDir, "local.md"), []byte("local"), 0644); err != nil {
				t.Fatal(err)
			}
			mustRunGit(t, "-C", cloneDir, "add", ".")
			mustRunGit(t, "-C", cloneDir, "commit", "-m", "local ahead")
			beforeOutput, err := runGitOutput("-C", cloneDir, "rev-parse", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			before := string(bytes.TrimSpace(beforeOutput))

			update, err := tt.fetcher.Fetch(repoPath, cloneDir, "master")
			if err != nil || update.Status != UpdateUnchanged || update.CurrentCommit != before {
				t.Fatalf("local-ahead update = %#v, error = %v", update, err)
			}
		})
	}
}

func hasPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
