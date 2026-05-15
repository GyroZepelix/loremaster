package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GyroZepelix/loremaster/internal/cache"
	"github.com/GyroZepelix/loremaster/internal/config"
	"github.com/GyroZepelix/loremaster/internal/git"
	"github.com/GyroZepelix/loremaster/internal/gitignore"
	"github.com/GyroZepelix/loremaster/internal/provider"
)

type Syncer struct {
	GitFetcher  git.Fetcher
	Provider    provider.Provider
	ProjectRoot string
	ProfileName string
	// ManifestSnapshot holds the profile entries from the manifest BEFORE the
	// current sync. Used by reconcileStale for ownership detection. Must be a
	// copy, not an alias, of the manifest slice.
	ManifestSnapshot []string
}

type SyncResult struct {
	Synced  int
	Sources int
	Errors  []string
	// Entries captures the project-root-relative paths of all synced skills
	// for this provider run.
	Entries []string
}

// FetchSources resolves base directories for all skill sources. For git sources
// it clones/pulls and optionally checks out a ref. For local sources it resolves
// the absolute path. Each source is isolated: one failure does not block others.
func FetchSources(fetcher git.Fetcher, sources []config.SkillSource) (map[string]string, []string) {
	baseDirs := make(map[string]string)
	var errs []string
	for _, src := range sources {
		if config.IsGitSource(src.Source) {
			repoDir, err := cache.RepoDir(src.Source)
			if err != nil {
				errs = append(errs, fmt.Sprintf("error: resolve cache for %q: %s", src.Source, err))
				continue
			}
			if err := fetcher.CloneOrPull(src.Source, repoDir); err != nil {
				errs = append(errs, fmt.Sprintf("error: fetch %q: %s (check URL and authentication)", src.Source, err))
				continue
			}
			if src.Ref != "" {
				if err := fetcher.Checkout(repoDir, src.Ref); err != nil {
					errs = append(errs, fmt.Sprintf("error: checkout ref %q for %q: %s", src.Ref, src.Source, err))
					continue
				}
			}
			baseDirs[src.Source] = repoDir
		} else {
			absSource, err := filepath.Abs(src.Source)
			if err != nil {
				errs = append(errs, fmt.Sprintf("error: resolve local path %q: %s", src.Source, err))
				continue
			}
			info, err := os.Stat(absSource)
			if err != nil || !info.IsDir() {
				errs = append(errs, fmt.Sprintf("error: local path %q does not exist or is not a directory", src.Source))
				continue
			}
			baseDirs[src.Source] = absSource
		}
	}
	return baseDirs, errs
}

func (s *Syncer) Sync(cfg *config.Config, baseDirs map[string]string) (*SyncResult, error) {
	if s.Provider == nil {
		return nil, fmt.Errorf("provider must be set before calling Sync()")
	}

	if err := cache.EnsureDir(); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	// Collect desired skill set and detect collisions using ParsedIncludes
	type skillOrigin struct {
		src    string // entry.Src
		source string // SkillSource.Source
	}
	desiredSkills := make(map[string]bool)
	skillSource := make(map[string]skillOrigin) // entry.Dst -> first origin
	for _, src := range cfg.Skills {
		for _, entry := range src.ParsedIncludes {
			if prev, exists := skillSource[entry.Dst]; exists {
				fmt.Fprintf(os.Stderr, "warning: destination %q (from %q in %q) conflicts with %q in %q — last source wins\n",
					entry.Dst, entry.Src, src.Source, prev.src, prev.source)
			}
			skillSource[entry.Dst] = skillOrigin{src: entry.Src, source: src.Source}
			desiredSkills[entry.Dst] = true
		}
	}

	// Cross-source overlap detection
	var allEntries []config.IncludeEntry
	for _, src := range cfg.Skills {
		allEntries = append(allEntries, src.ParsedIncludes...)
	}
	if err := config.ValidateOverlaps(allEntries); err != nil {
		return nil, fmt.Errorf("cross-source overlap: %w", err)
	}

	result := &SyncResult{Sources: len(cfg.Skills)}
	var syncedEntries []string

	for _, src := range cfg.Skills {
		srcBaseDirs, ok := baseDirs[src.Source]
		if !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("error: no base directory for source %q (fetch may have failed)", src.Source))
			continue
		}
		skillErrors, err := s.syncSource(src, srcBaseDirs, &syncedEntries)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("error: sync failed for source %q: %s", src.Source, err))
			continue
		}
		for _, skillErr := range skillErrors {
			result.Errors = append(result.Errors, skillErr)
		}
	}

	result.Synced = len(syncedEntries)
	result.Entries = syncedEntries

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

	if len(result.Errors) > 0 {
		return result, fmt.Errorf("sync completed with %d error(s)", len(result.Errors))
	}

	return result, nil
}

func (s *Syncer) syncSource(src config.SkillSource, baseDir string, syncedEntries *[]string) ([]string, error) {
	linkType := src.Type
	if linkType == "" {
		linkType = "soft"
	}

	var skillErrors []string
	for _, entry := range src.ParsedIncludes {
		srcPath := filepath.Join(baseDir, entry.Src)
		if info, err := os.Stat(srcPath); err != nil || !info.IsDir() {
			skillErrors = append(skillErrors, fmt.Sprintf("error: skill %q from source %q: not found (expected directory at %q, verify include list)", entry.Src, src.Source, srcPath))
			continue
		}

		dstPath := s.Provider.SkillDir(s.ProjectRoot, entry.Dst)

		if err := LinkSkill(srcPath, dstPath, linkType); err != nil {
			skillErrors = append(skillErrors, fmt.Sprintf("error: skill %q from source %q: %s (check filesystem permissions)", entry.Dst, src.Source, err))
			continue
		}

		// Compute gitignore entry relative to project root
		relPath, _ := filepath.Rel(s.ProjectRoot, dstPath)
		*syncedEntries = append(*syncedEntries, relPath)
	}

	return skillErrors, nil
}

// reconcileStale removes skills that are in the manifest snapshot but not in
// the desired set. Detection is manifest-driven: if an entry is in
// ManifestSnapshot and not in desiredSkills, it is stale. Filesystem type
// checks (symlink, .lore-checksum) serve as safety verification.
func (s *Syncer) reconcileStale(desiredSkills map[string]bool) ([]string, error) {
	// First-run guard: empty snapshot means nothing to reconcile.
	// This collapses "no manifest" and "empty profile" into one path.
	if len(s.ManifestSnapshot) == 0 {
		return nil, nil
	}

	skillsParent := s.Provider.SkillRoot(s.ProjectRoot)

	if _, err := os.Stat(skillsParent); os.IsNotExist(err) {
		return nil, nil
	}

	// Build set of profile-owned entries from the snapshot.
	// Cross-provider entries produce ../.. relative paths via filepath.Rel,
	// which never match WalkDir results. This implicitly filters ownedEntries
	// to only the current provider's entries.
	ownedEntries := make(map[string]bool, len(s.ManifestSnapshot))
	for _, e := range s.ManifestSnapshot {
		// Manifest entries are project-root-relative (e.g. ".claude/skills/brainstorm")
		// Convert to skillsParent-relative for comparison with WalkDir results
		rel, err := filepath.Rel(skillsParent, filepath.Join(s.ProjectRoot, e))
		if err == nil {
			ownedEntries[rel] = true
		}
	}

	var staleEntries []string
	var stalePaths []string // absolute paths of removed leaves, for parent cleanup

	err := filepath.WalkDir(skillsParent, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Broken symlinks cause WalkDir errors — attempt cleanup if owned and stale
			if info, lstatErr := os.Lstat(path); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
				rel, _ := filepath.Rel(skillsParent, path)
				if ownedEntries[rel] && !desiredSkills[rel] {
					if rmErr := os.Remove(path); rmErr == nil {
						relFromRoot, _ := filepath.Rel(s.ProjectRoot, path)
						staleEntries = append(staleEntries, relFromRoot)
						stalePaths = append(stalePaths, path)
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "warning: reconcile walk: %s\n", err)
			}
			return nil
		}
		if path == skillsParent {
			return nil
		}

		rel, _ := filepath.Rel(skillsParent, path)

		// Skip desired skills entirely
		if desiredSkills[rel] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Check for symlinks (Lstat needed since WalkDir follows symlinks)
		info, lstatErr := os.Lstat(path)
		if lstatErr != nil {
			return nil
		}

		if info.Mode()&os.ModeSymlink != 0 {
			if !ownedEntries[rel] {
				return nil // Not in manifest — not managed by loremaster
			}
			if desiredSkills[rel] {
				return nil // Still desired — defense-in-depth
			}
			// Stale managed symlink — remove
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove stale symlink %q: %w", path, err)
			}
			relFromRoot, _ := filepath.Rel(s.ProjectRoot, path)
			staleEntries = append(staleEntries, relFromRoot)
			stalePaths = append(stalePaths, path)
			return nil
		}

		if d.IsDir() {
			checksumFile := filepath.Join(path, ".lore-checksum")
			storedChecksum, readErr := os.ReadFile(checksumFile)
			if readErr != nil {
				return nil // No checksum — not managed, continue walking
			}
			if !ownedEntries[rel] {
				return fs.SkipDir // Managed but not owned by this profile
			}
			if desiredSkills[rel] {
				return fs.SkipDir // Still desired
			}
			// Verify checksum before removing
			currentChecksum, err := ComputeDirChecksum(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping %q: could not verify checksum: %s\n", rel, err)
				return fs.SkipDir
			}
			if strings.TrimSpace(string(storedChecksum)) != currentChecksum {
				fmt.Fprintf(os.Stderr, "warning: skipping %q: local modifications detected\n", rel)
				return fs.SkipDir
			}
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("remove stale hard copy %q: %w", path, err)
			}
			relFromRoot, _ := filepath.Rel(s.ProjectRoot, path)
			staleEntries = append(staleEntries, relFromRoot)
			stalePaths = append(stalePaths, path)
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return staleEntries, err
	}

	// Clean up empty parent directories bottom-up
	cleanedDirs := make(map[string]bool)
	for _, stalePath := range stalePaths {
		dir := filepath.Dir(stalePath)
		for dir != skillsParent && !cleanedDirs[dir] {
			cleanedDirs[dir] = true
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) > 0 {
				break
			}
			if err := os.Remove(dir); err != nil {
				break
			}
			dir = filepath.Dir(dir)
		}
	}

	return staleEntries, nil
}
