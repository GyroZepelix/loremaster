package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgjalic/loremaster/internal/cache"
	"github.com/dgjalic/loremaster/internal/config"
	"github.com/dgjalic/loremaster/internal/git"
	"github.com/dgjalic/loremaster/internal/gitignore"
	"github.com/dgjalic/loremaster/internal/provider"
)

type Syncer struct {
	GitFetcher  git.Fetcher
	Provider    provider.Provider
	ProjectRoot string
}

type SyncResult struct {
	Synced  int
	Sources int
	Errors  []string
}

func (s *Syncer) Sync(cfg *config.Config) (*SyncResult, error) {
	if s.Provider == nil {
		prov, err := provider.Get(cfg.Provider)
		if err != nil {
			return nil, err
		}
		s.Provider = prov
	}

	if err := cache.EnsureDir(); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	// Collect desired skill set and detect collisions
	desiredSkills := make(map[string]bool)
	skillSource := make(map[string]string) // skill name -> first source
	for _, src := range cfg.Skills {
		for _, skill := range src.Include {
			if prev, exists := skillSource[skill]; exists {
				fmt.Fprintf(os.Stderr, "warning: skill %q declared in both %q and %q — last source wins\n", skill, prev, src.Source)
			}
			skillSource[skill] = src.Source
			desiredSkills[skill] = true
		}
	}

	result := &SyncResult{Sources: len(cfg.Skills)}
	var syncedEntries []string
	var syncErrors []error

	for _, src := range cfg.Skills {
		if err := s.syncSource(src, &syncedEntries); err != nil {
			errMsg := fmt.Sprintf("error: sync failed for source %q: %s", src.Source, err)
			result.Errors = append(result.Errors, errMsg)
			syncErrors = append(syncErrors, fmt.Errorf("%s", errMsg))
			continue
		}
	}

	result.Synced = len(syncedEntries)

	// Reconcile stale skills
	staleEntries, err := s.reconcileStale(desiredSkills)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("reconcile stale: %s", err))
	}

	// Update gitignore
	gitignorePath := filepath.Join(s.ProjectRoot, ".gitignore")
	if len(syncedEntries) > 0 {
		if err := gitignore.EnsureEntries(gitignorePath, syncedEntries); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("update gitignore: %s", err))
		}
	}
	if len(staleEntries) > 0 {
		if err := gitignore.RemoveEntries(gitignorePath, staleEntries); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("cleanup gitignore: %s", err))
		}
	}

	if len(syncErrors) > 0 {
		return result, fmt.Errorf("sync completed with %d error(s)", len(syncErrors))
	}

	return result, nil
}

func (s *Syncer) syncSource(src config.SkillSource, syncedEntries *[]string) error {
	isGit := config.IsGitSource(src.Source)

	var baseDir string

	if isGit {
		repoDir, err := cache.RepoDir(src.Source)
		if err != nil {
			return fmt.Errorf("resolve cache dir: %w (check $HOME or $XDG_DATA_HOME)", err)
		}
		if err := s.GitFetcher.CloneOrPull(src.Source, repoDir); err != nil {
			return fmt.Errorf("%w (check URL and authentication)", err)
		}
		if src.Ref != "" {
			if err := s.GitFetcher.Checkout(repoDir, src.Ref); err != nil {
				return fmt.Errorf("checkout ref %q: %w (verify ref exists in remote)", src.Ref, err)
			}
		}
		baseDir = repoDir
	} else {
		// Local source — resolve to absolute path
		absSource, err := filepath.Abs(src.Source)
		if err != nil {
			return fmt.Errorf("resolve local path %q: %w (check path syntax)", src.Source, err)
		}
		info, err := os.Stat(absSource)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("local path %q does not exist or is not a directory (check path in lore.yml)", src.Source)
		}
		baseDir = absSource
	}

	linkType := src.Type
	if linkType == "" {
		linkType = "soft"
	}

	for _, skill := range src.Include {
		srcPath := filepath.Join(baseDir, skill)
		if info, err := os.Stat(srcPath); err != nil || !info.IsDir() {
			return fmt.Errorf("skill %q not found in source (expected directory at %q, verify include list)", skill, srcPath)
		}

		dstPath := s.Provider.SkillDir(s.ProjectRoot, skill)

		if err := LinkSkill(srcPath, dstPath, linkType); err != nil {
			return fmt.Errorf("link skill %q: %w (check filesystem permissions)", skill, err)
		}

		// Compute gitignore entry relative to project root
		relPath, _ := filepath.Rel(s.ProjectRoot, dstPath)
		*syncedEntries = append(*syncedEntries, relPath)
	}

	return nil
}

func (s *Syncer) reconcileStale(desiredSkills map[string]bool) ([]string, error) {
	// Scan provider skill directory for managed entries
	skillsParent := filepath.Dir(s.Provider.SkillDir(s.ProjectRoot, "dummy"))
	entries, err := os.ReadDir(skillsParent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	cacheDir, err := cache.Dir()
	if err != nil {
		return nil, fmt.Errorf("resolve cache dir: %w", err)
	}
	absCacheDir, _ := filepath.Abs(cacheDir)

	var staleEntries []string

	for _, entry := range entries {
		name := entry.Name()
		if desiredSkills[name] {
			continue
		}

		fullPath := filepath.Join(skillsParent, name)

		// Check if it's a symlink pointing into our cache
		target, linkErr := os.Readlink(fullPath)
		if linkErr == nil {
			// It's a symlink — resolve to absolute for comparison
			absTarget, _ := filepath.Abs(filepath.Join(filepath.Dir(fullPath), target))
			if !strings.HasPrefix(absTarget, absCacheDir) {
				continue // Symlink but not managed by us
			}
			if err := os.Remove(fullPath); err != nil {
				return staleEntries, fmt.Errorf("remove stale symlink %q: %w", fullPath, err)
			}
			relPath, _ := filepath.Rel(s.ProjectRoot, fullPath)
			staleEntries = append(staleEntries, relPath)
			continue
		}

		// Check if it's a hard-copied skill managed by us (.lore-checksum marker)
		checksumFile := filepath.Join(fullPath, ".lore-checksum")
		if _, err := os.Stat(checksumFile); err == nil {
			if err := os.RemoveAll(fullPath); err != nil {
				return staleEntries, fmt.Errorf("remove stale hard copy %q: %w", fullPath, err)
			}
			relPath, _ := filepath.Rel(s.ProjectRoot, fullPath)
			staleEntries = append(staleEntries, relPath)
		}
	}

	return staleEntries, nil
}
